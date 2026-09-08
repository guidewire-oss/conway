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
	"strings"
	"time"
)

func registrationSummary(m planning.ForecastRegistration) map[string]any {
	return map[string]any{"id": m.ID, "name": m.Name, "registeredAt": m.RegisteredAt, "createdBy": m.CreatedBy, "model": m.Model, "probability": m.Probability, "training": m.Training.ValidationCounts, "trainingEvidence": m.TrainingEvidence, "settings": m.Reference.Forecast.Settings}
}

func (s *server) registrationAccess(w http.ResponseWriter, c auth.Claims, m planning.ForecastRegistration) bool {
	ids := map[string]bool{m.TrainingEvidence.SnapshotID: true, m.Reference.Evidence.SnapshotID: true}
	for _, p := range m.History {
		ids[p.Evidence.SnapshotID] = true
	}
	for id := range ids {
		snap, err := s.db.GetSnapshot(id)
		if err != nil {
			s.predictionError(w, err)
			return false
		}
		if !canReadExecutionSnapshot(snap, c) {
			http.Error(w, "A registered model capture is missing or inaccessible. Restore access before continuing.", 404)
			return false
		}
	}
	return true
}

func (s *server) readRegistration(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, reference, id string) (*db.RegistrationRow, *planning.ForecastRegistration) {
	row, err := s.db.ForecastRegistration(r.Context(), p.ID, id)
	if err != nil {
		s.predictionError(w, err)
		return nil, nil
	}
	if row == nil || row.ReferenceID != reference {
		http.Error(w, "Registered model not found in this prediction.", 404)
		return nil, nil
	}
	var m planning.ForecastRegistration
	if err = json.Unmarshal(row.Data, &m); err != nil {
		s.predictionError(w, err)
		return nil, nil
	}
	if !s.registrationAccess(w, c, m) {
		return nil, nil
	}
	return row, &m
}

// specs/031-prospective-forecast-registration.md:84: reuse the prediction access
// gate and exact action matching; never turn a partial private cohort into a report.
func (s *server) handleRegistrations(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, ref planning.ForecastPrediction, action string) {
	if action == "registrations" {
		switch r.Method {
		case http.MethodGet:
			rows, err := s.db.ForecastRegistrations(r.Context(), p.ID, ref.ID)
			if err != nil {
				s.predictionError(w, err)
				return
			}
			if len(rows) > 100 {
				http.Error(w, "More than 100 registered models; no partial list was produced. Contact an administrator.", http.StatusUnprocessableEntity)
				return
			}
			out := []map[string]any{}
			for _, row := range rows {
				var m planning.ForecastRegistration
				if err = json.Unmarshal(row.Data, &m); err != nil {
					s.predictionError(w, err)
					return
				}
				if !s.registrationAccess(w, c, m) {
					return
				}
				out = append(out, registrationSummary(m))
			}
			writeJSON(w, map[string]any{"registrations": out})
		case http.MethodPost:
			s.registerForecastModel(w, r, p, c, ref)
		default:
			methodNotAllowed(w, r)
		}
		return
	}
	id, suffix, _ := strings.Cut(strings.TrimPrefix(action, "registrations/"), "/")
	if id == "" || suffix != "assessment" || r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	_, m := s.readRegistration(w, r, p, c, ref.ID, id)
	if m == nil {
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
	history, ok := s.accessiblePredictionHistory(w, r, p, c, m.Reference)
	if !ok {
		return
	}
	report, err := planning.AssessRegisteredModel(*m, history, ev, issues)
	if err != nil {
		status := 400
		if errors.Is(err, planning.ErrValidationLimit) {
			status = 422
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, map[string]any{"registration": registrationSummary(*m), "evaluation": report})
}

func (s *server) registerForecastModel(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, ref planning.ForecastPrediction) {
	var req struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		TrainingSnapshotID string `json:"trainingSnapshotId"`
	}
	if !decodeReviewBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`).MatchString(req.ID) || req.Name == "" || len(req.Name) > 120 || req.TrainingSnapshotID == "" {
		http.Error(w, "Name the model (up to 120 characters) and choose a completed training capture.", 400)
		return
	}
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(append([]byte(ref.ID+"\n"), raw...))
	hash := hex.EncodeToString(sum[:])
	existing, err := s.db.ForecastRegistration(r.Context(), p.ID, req.ID)
	if err != nil {
		s.predictionError(w, err)
		return
	}
	if existing != nil {
		if existing.RequestHash != hash {
			http.Error(w, "This save key belongs to a different registered model. Change the form and retry.", http.StatusConflict)
			return
		}
		_, m := s.readRegistration(w, r, p, c, ref.ID, req.ID)
		if m != nil {
			writeJSON(w, registrationSummary(*m))
		}
		return
	}
	ev, issues, ok := s.predictionCapture(w, r, c, req.TrainingSnapshotID)
	if !ok {
		return
	}
	history, ok := s.accessiblePredictionHistory(w, r, p, c, ref)
	if !ok {
		return
	}
	m, err := planning.RegisterForecastModel(ref, history, ev, issues, time.Now().Unix())
	if err != nil {
		status := 400
		if errors.Is(err, planning.ErrValidationLimit) {
			status = 422
		}
		http.Error(w, err.Error(), status)
		return
	}
	m.ID, m.Name, m.CreatedBy = req.ID, req.Name, c.Sub
	data, err := json.Marshal(m)
	if err != nil {
		s.predictionError(w, err)
		return
	}
	_, err = s.db.SaveForecastRegistration(r.Context(), p, db.RegistrationRow{ID: m.ID, ReferenceID: ref.ID, RequestHash: hash, Data: data})
	if errors.Is(err, db.ErrRegistrationLimit) {
		http.Error(w, "This prediction already has 100 registered models. Assess an existing model; no additional model was saved.", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		s.predictionError(w, err)
		return
	}
	row, saved := s.readRegistration(w, r, p, c, ref.ID, req.ID)
	if saved == nil {
		return
	}
	if row.RequestHash != hash {
		http.Error(w, "This save key belongs to a different registered model. Change the form and retry.", http.StatusConflict)
		return
	}
	writeJSON(w, registrationSummary(*saved))
}
