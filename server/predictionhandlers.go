package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type predictionRequest struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Fingerprint string                    `json:"fingerprint"`
	Settings    planning.ForecastSettings `json:"settings"`
	SnapshotID  string                    `json:"snapshotId"`
}

func (s *server) predictionError(w http.ResponseWriter, err error) {
	s.logger().Error().Err(err).Msg("prediction history request failed")
	http.Error(w, "Prediction history could not be read or saved. Retry when the service is available.", 500)
}
func (s *server) predictionCapture(w http.ResponseWriter, r *http.Request, c auth.Claims, id string) (planning.PredictionEvidence, []planning.ExecutionIssue, bool) {
	ev := planning.PredictionEvidence{}
	snap, err := s.db.GetSnapshot(id)
	if err != nil {
		s.predictionError(w, err)
		return ev, nil, false
	}
	if !canReadExecutionSnapshot(snap, c) {
		http.Error(w, "Snapshot not found or inaccessible.", 404)
		return ev, nil, false
	}
	source, err := s.db.PredictionSnapshotSource(r.Context(), id)
	if err != nil {
		s.predictionError(w, err)
		return ev, nil, false
	}
	if snap.Source != "jira" || source == nil || source.StartedAt <= 0 || source.FinishedAt < source.StartedAt || source.FinishedAt > time.Now().Unix() {
		http.Error(w, "Choose a successful capture from a managed Jira source. Legacy imports remain available for diagnostics.", 400)
		return ev, nil, false
	}
	rows, err := s.db.ExecutionIssues(id)
	if err != nil {
		s.predictionError(w, err)
		return ev, nil, false
	}
	ev = planning.PredictionEvidence{SnapshotID: id, SourceID: source.SourceID, ConfigFingerprint: source.Fingerprint, StartedAt: source.StartedAt, CapturedAt: source.FinishedAt}
	issues := make([]planning.ExecutionIssue, 0, len(rows))
	for _, v := range rows {
		issues = append(issues, planning.ExecutionIssue{Key: v.Key, ParentKey: v.ParentKey, Pod: v.Pod, Type: v.IssueType, StatusCategory: v.StatusCat, Created: v.Created, Updated: v.Updated, Resolved: v.Resolved})
	}
	return ev, issues, true
}
func (s *server) readPrediction(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, id string) (*db.PredictionRow, *planning.ForecastPrediction) {
	row, err := s.db.Prediction(r.Context(), p.ID, id)
	if err != nil {
		s.predictionError(w, err)
		return nil, nil
	}
	if row == nil {
		http.Error(w, "Prediction not found in this plan.", 404)
		return nil, nil
	}
	snap, err := s.db.GetSnapshot(row.SnapshotID)
	if err != nil {
		s.predictionError(w, err)
		return nil, nil
	}
	if !canReadExecutionSnapshot(snap, c) {
		http.Error(w, "The prediction's original capture is missing or inaccessible.", 404)
		return nil, nil
	}
	var pred planning.ForecastPrediction
	if err = json.Unmarshal(row.Data, &pred); err != nil {
		s.predictionError(w, err)
		return nil, nil
	}
	return row, &pred
}

// specs/028-portfolio-forecasts.md:196: explicit recording and read-only outcomes
// share the existing plan and evidence access boundaries.
func (s *server) handlePredictions(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, sub string) {
	w.Header().Set("Cache-Control", "no-store")
	var ok bool
	c, ok = s.forecastAccess(w, r, p, c)
	if !ok {
		return
	}
	if sub == "predictions" {
		switch r.Method {
		case http.MethodGet:
			var before int64
			if raw := r.URL.Query().Get("before"); raw != "" {
				v, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || v <= 0 {
					http.Error(w, "Invalid history cursor. Refresh prediction history.", 400)
					return
				}
				before = v
			}
			rows, err := s.db.Predictions(r.Context(), p.ID, before)
			if err != nil {
				s.predictionError(w, err)
				return
			}
			next := int64(0)
			if len(rows) > 50 {
				rows = rows[:50]
				next = rows[49].Order
			}
			writeJSON(w, map[string]any{"predictions": rows, "next": next})
		case http.MethodPost:
			s.recordPrediction(w, r, p, c)
		default:
			methodNotAllowed(w, r)
		}
		return
	}
	rest := strings.TrimPrefix(sub, "predictions/")
	id, action, _ := strings.Cut(rest, "/")
	if (action == "" && r.Method != http.MethodGet) || ((action == "assessment" || action == "validation") && r.Method != http.MethodPost) || (action != "" && action != "assessment" && action != "validation") {
		methodNotAllowed(w, r)
		return
	}
	_, pred := s.readPrediction(w, r, p, c, id)
	if pred == nil {
		return
	}
	if action == "" {
		writeJSON(w, pred)
		return
	}
	var req struct {
		SnapshotID string `json:"snapshotId"`
	}
	if !decodeReviewBody(w, r, &req) {
		return
	}
	ev, issues, ok := s.predictionCapture(w, r, c, req.SnapshotID)
	if !ok {
		return
	}
	var current []planning.Initiative
	if err := json.Unmarshal(p.Initiatives, &current); err != nil {
		s.predictionError(w, err)
		return
	}
	if action == "validation" {
		s.validatePredictionHistory(w, r, p, c, *pred, current, ev, issues)
		return
	}
	writeJSON(w, planning.AssessPrediction(*pred, current, ev, issues))
}

// specs/029-forecast-history-validation.md:93: original evidence access is required
// for all candidate records; an incomplete private cohort must not become a report.
func (s *server) validatePredictionHistory(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, reference planning.ForecastPrediction, current []planning.Initiative, ev planning.PredictionEvidence, issues []planning.ExecutionIssue) {
	rows, err := s.db.PredictionValidationHistory(r.Context(), p.ID)
	if err != nil {
		s.predictionError(w, err)
		return
	}
	if len(rows) > 200 {
		http.Error(w, "History validation supports at most 200 recorded predictions. No partial report was produced; retain history and contact an administrator.", http.StatusUnprocessableEntity)
		return
	}
	history := make([]planning.ForecastPrediction, 0, len(rows))
	access := map[string]bool{}
	for _, row := range rows {
		var pred planning.ForecastPrediction
		if err := json.Unmarshal(row.Data, &pred); err != nil {
			s.predictionError(w, err)
			return
		}
		if planning.ValidationMatches(reference, pred) && pred.IssuedAt < ev.StartedAt && !access[row.SnapshotID] {
			snap, err := s.db.GetSnapshot(row.SnapshotID)
			if err != nil {
				s.predictionError(w, err)
				return
			}
			if !canReadExecutionSnapshot(snap, c) {
				http.Error(w, "An original capture in this history is missing or inaccessible. Restore capture access before validating; no partial report was produced.", 404)
				return
			}
			access[row.SnapshotID] = true
		}
		history = append(history, pred)
	}
	report, err := planning.ValidatePredictionHistory(reference, history, current, ev, issues)
	if err != nil {
		status := 400
		if errors.Is(err, planning.ErrValidationLimit) {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, report)
}
func (s *server) recordPrediction(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	var req predictionRequest
	if !decodeReviewBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`).MatchString(req.ID) || req.Name == "" || len(req.Name) > 120 || len(req.Fingerprint) != 64 || req.SnapshotID == "" {
		http.Error(w, "Name the prediction (up to 120 characters), choose a capture and compare saved inputs first.", 400)
		return
	}
	raw, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "Prediction settings could not be encoded. Compare again with valid settings.", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	existing, err := s.db.Prediction(r.Context(), p.ID, req.ID)
	if err != nil {
		s.predictionError(w, err)
		return
	}
	if existing != nil {
		if existing.RequestHash != hash {
			http.Error(w, "This save key belongs to a different prediction. Compare again before saving.", http.StatusConflict)
			return
		}
		_, pred := s.readPrediction(w, r, p, c, req.ID)
		if pred != nil {
			writeJSON(w, pred)
		}
		return
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		if msg, invalid := windowError(err); invalid {
			http.Error(w, msg, 400)
		} else {
			s.predictionError(w, err)
		}
		return
	}
	if inputs.Fingerprint() != req.Fingerprint {
		http.Error(w, "Saved inputs changed. Compare scenarios again before recording a prediction.", http.StatusConflict)
		return
	}
	if _, err = time.Parse("2006-01-02", inputs.Scheduling.PeriodStart); err != nil {
		http.Error(w, "Set a dated planning period before recording a prediction.", 400)
		return
	}
	forecast, err := planning.ComputeForecast(inputs, req.Settings)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ev, issues, ok := s.predictionCapture(w, r, c, req.SnapshotID)
	if !ok {
		return
	}
	pred := planning.ForecastPrediction{ID: req.ID, Name: req.Name, IssuedAt: time.Now().Unix(), CreatedBy: c.Sub, Inputs: inputs, Forecast: forecast, Evidence: ev, Issues: planning.PredictionIssues(inputs, issues)}
	data, err := json.Marshal(pred)
	if err != nil {
		s.predictionError(w, err)
		return
	}
	inserted, err := s.db.SavePrediction(r.Context(), p, db.PredictionRow{ID: pred.ID, Name: pred.Name, IssuedAt: pred.IssuedAt, SnapshotID: req.SnapshotID, RequestHash: hash, Data: data})
	if err != nil {
		s.predictionError(w, err)
		return
	}
	if !inserted {
		existing, err = s.db.Prediction(r.Context(), p.ID, req.ID)
		if err != nil {
			s.predictionError(w, err)
			return
		}
		if existing != nil && existing.RequestHash == hash {
			_, saved := s.readPrediction(w, r, p, c, req.ID)
			if saved != nil {
				writeJSON(w, saved)
			}
			return
		}
		http.Error(w, "The plan changed or this save key was used. Refresh and compare again.", http.StatusConflict)
		return
	}
	writeJSON(w, pred)
}
