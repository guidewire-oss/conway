package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
)

type weeklyReviewRequest struct {
	ReviewDate string                 `json:"reviewDate"`
	Timezone   string                 `json:"timezone"`
	SnapshotID string                 `json:"snapshotId"`
	Filters    planning.ReviewFilters `json:"filters"`
}

func decodeReviewBody(w http.ResponseWriter, r *http.Request, body any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		http.Error(w, "provide a valid review request", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "provide one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *server) handleWeeklyReview(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, sub string) {
	if (!c.Has("manager") && !c.Has("admin")) || c.GameID != "" || (p.Owner != c.Sub && !c.Has("admin")) {
		http.Error(w, "manager access to this plan is required", http.StatusForbidden)
		return
	}
	parts := strings.Split(sub, "/")
	if parts[0] == "decisions" {
		s.actionTransition(w, r, p, c, parts)
		return
	}
	if r.Method == http.MethodGet {
		if len(parts) == 1 {
			reviews, err := s.db.ExecutionReviews(r.Context(), p.ID)
			if err != nil {
				s.reviewFailure(w, err)
				return
			}
			writeJSON(w, map[string]any{"reviews": reviews})
			return
		}
		if len(parts) == 2 {
			review, err := s.db.GetExecutionReview(r.Context(), p.ID, parts[1])
			if err != nil {
				s.reviewFailure(w, err)
				return
			}
			if review == nil {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, review)
			return
		}
	}
	if r.Method != http.MethodPost || len(parts) > 2 || (len(parts) == 2 && parts[1] != "preview") {
		methodNotAllowed(w, r)
		return
	}
	if len(parts) == 2 {
		var body weeklyReviewRequest
		if !decodeReviewBody(w, r, &body) {
			return
		}
		preview, _, code, msg := s.prepareWeeklyReview(r.Context(), p, c, body)
		if code != http.StatusOK {
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, preview)
		return
	}
	var body struct {
		weeklyReviewRequest
		ExpectedFingerprint string `json:"expectedFingerprint"`
		OutcomeNote         string `json:"outcomeNote"`
		NextCheckpoint      string `json:"nextCheckpoint"`
	}
	if !decodeReviewBody(w, r, &body) {
		return
	}
	body.OutcomeNote = strings.TrimSpace(body.OutcomeNote)
	body.NextCheckpoint = strings.TrimSpace(body.NextCheckpoint)
	if body.ExpectedFingerprint == "" || body.OutcomeNote == "" || len(body.OutcomeNote) > 10000 {
		http.Error(w, "a current preview fingerprint and an outcome note within 10000 characters are required", http.StatusBadRequest)
		return
	}
	if body.NextCheckpoint != "" {
		if _, err := time.Parse("2006-01-02", body.NextCheckpoint); err != nil {
			http.Error(w, "next checkpoint must be YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}
	preview, guard, code, msg := s.prepareWeeklyReview(r.Context(), p, c, body.weeklyReviewRequest)
	if code != http.StatusOK {
		http.Error(w, msg, code)
		return
	}
	if preview.Fingerprint != body.ExpectedFingerprint {
		http.Error(w, "the reviewed context changed; refresh the preview before completing this review", http.StatusConflict)
		return
	}
	baselineID := ""
	if preview.Context.Baseline != nil {
		baselineID = preview.Context.Baseline.ID
	}
	snapshotID := ""
	if preview.Context.Snapshot != nil {
		snapshotID = preview.Context.Snapshot.ID
	}
	review := db.ExecutionReview{ID: newID(), PlanID: p.ID, ReviewDate: preview.Context.ReviewDate, Timezone: preview.Context.Timezone, SnapshotID: snapshotID, BaselineID: baselineID, CreatedBy: c.Sub, CreatedAt: time.Now().Unix(), OutcomeNote: body.OutcomeNote, NextCheckpoint: body.NextCheckpoint, Preview: preview}
	saved, err := s.db.CompleteExecutionReview(r.Context(), review, guard)
	if err != nil {
		s.reviewFailure(w, err)
		return
	}
	if !saved {
		http.Error(w, "the reviewed context changed during completion; refresh the preview", http.StatusConflict)
		return
	}
	writeJSON(w, review)
}

func (s *server) reviewFailure(w http.ResponseWriter, err error) {
	s.logger().Error().Err(err).Msg("weekly review operation failed")
	http.Error(w, "could not save or load this review; retry", http.StatusInternalServerError)
}

func (s *server) actionTransition(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, parts []string) {
	if len(parts) != 3 || parts[2] != "transitions" {
		http.NotFound(w, r)
		return
	}
	action, err := s.db.GetExecutionDecision(r.Context(), p.ID, parts[1])
	if err != nil {
		s.reviewFailure(w, err)
		return
	}
	if action == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		events, err := s.db.ActionTransitions(r.Context(), p.ID, action.ID)
		if err != nil {
			s.reviewFailure(w, err)
			return
		}
		writeJSON(w, map[string]any{"transitions": events})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	var body struct {
		ExpectedVersion int    `json:"expectedVersion"`
		Status          string `json:"status"`
		Evidence        string `json:"evidence"`
	}
	if !decodeReviewBody(w, r, &body) {
		return
	}
	body.Evidence = strings.TrimSpace(body.Evidence)
	if body.ExpectedVersion <= 0 {
		http.Error(w, "expectedVersion must be a positive action version", http.StatusBadRequest)
		return
	}
	if body.ExpectedVersion != action.Version {
		http.Error(w, "this action changed; reload its current version", http.StatusConflict)
		return
	}
	if err := planning.ValidateActionTransition(action.Status, body.Status, body.Evidence); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event := db.ActionTransition{ID: newID(), DecisionID: action.ID, PlanID: p.ID, FromStatus: action.Status, Status: body.Status, Version: action.Version + 1, Evidence: body.Evidence, CreatedBy: c.Sub, CreatedAt: time.Now().Unix()}
	saved, err := s.db.TransitionAction(r.Context(), event, body.ExpectedVersion)
	if err != nil {
		s.reviewFailure(w, err)
		return
	}
	if !saved {
		http.Error(w, "this action changed; reload its current version", http.StatusConflict)
		return
	}
	action.Status = event.Status
	action.Version = event.Version
	writeJSON(w, action)
}

// prepareWeeklyReview fingerprints the complete observed evidence and context,
// including action versions. specs/024-weekly-execution-review.md:305
func (s *server) prepareWeeklyReview(ctx context.Context, p *db.PlanRow, c auth.Claims, request weeklyReviewRequest) (db.ReviewPreview, db.ReviewGuard, int, string) {
	preview := db.ReviewPreview{}
	guard := db.ReviewGuard{Plan: p}
	request.ReviewDate = strings.TrimSpace(request.ReviewDate)
	request.Timezone = strings.TrimSpace(request.Timezone)
	request.SnapshotID = strings.TrimSpace(request.SnapshotID)
	if err := planning.ValidateReviewDateZone(request.ReviewDate, request.Timezone); err != nil {
		return preview, guard, http.StatusBadRequest, err.Error()
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		return preview, guard, http.StatusBadRequest, err.Error()
	}
	if len(request.Filters.Team) > 200 || len(request.Filters.Initiative) > 1000 {
		return preview, guard, http.StatusBadRequest, "review filters are too long"
	}
	loaded, code, msg := s.loadExecutionEvidence(p, c, request.SnapshotID)
	if code != http.StatusOK {
		return preview, guard, code, msg
	}
	actions, err := s.db.ExecutionDecisions(p.ID)
	if err != nil {
		return preview, guard, http.StatusInternalServerError, "could not load review actions"
	}
	previous, err := s.db.LatestExecutionReview(ctx, p.ID)
	if err != nil {
		return preview, guard, http.StatusInternalServerError, "could not load the preceding review"
	}
	in := planning.ReviewInput{ReviewDate: request.ReviewDate, Timezone: request.Timezone, PlanID: p.ID, PlanFingerprint: inputs.Fingerprint(), Actuals: loaded.Actuals, Filters: request.Filters, Actions: []planning.ReviewAction{}, Initiatives: inputs.Initiatives}
	if loaded.Snapshot != nil {
		snap := loaded.Snapshot
		in.Snapshot = &planning.ReviewSnapshot{ID: snap.ID, Name: snap.Name, Source: snap.Source, CreatedAt: snap.CreatedAt}
	}
	if loaded.Baseline != nil {
		b := loaded.Baseline
		in.Baseline = &planning.ReviewBaseline{ID: b.ID, Name: b.Name, CreatedAt: b.CreatedAt, PeriodStart: loaded.Schedule.PeriodStart, Fingerprint: b.Fingerprint}
	}
	if previous != nil {
		in.Previous = &planning.ReviewPrevious{ID: previous.ID, ReviewDate: previous.ReviewDate, Snapshot: previous.Preview.Context.Snapshot, Baseline: previous.Preview.Context.Baseline, PlanFingerprint: previous.Preview.Context.PlanFingerprint}
		in.PreviousEvidence = previous.Preview.Evidence
		guard.PreviousID = previous.ID
	}
	for _, a := range actions {
		in.Actions = append(in.Actions, planning.ReviewAction{ID: a.ID, PlanID: a.PlanID, Action: a.Action, Owner: a.Owner, ReviewDate: a.ReviewDate, Rationale: a.Rationale, Initiative: a.Initiative, SnapshotID: a.SnapshotID, BaselineID: a.BaselineID, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, Status: a.Status, Version: a.Version})
	}
	summary, err := planning.BuildWeeklyReview(in)
	if err != nil {
		return preview, guard, http.StatusBadRequest, err.Error()
	}
	preview.ReviewSummary = summary
	guard.Actions = actions
	guard.Baseline = loaded.Baseline
	guard.Snapshot = loaded.Snapshot
	guard.Issues = loaded.Issues
	raw, err := json.Marshal(struct {
		Summary  planning.ReviewSummary
		Issues   []db.IssueRow
		Baseline *db.BaselineRow
		Sites    json.RawMessage
		Actions  []db.ExecutionDecision
	}{summary, loaded.Issues, loaded.Baseline, p.Sites, actions})
	if err != nil {
		return preview, guard, http.StatusInternalServerError, "could not encode review context"
	}
	hash := sha256.Sum256(raw)
	preview.Fingerprint = hex.EncodeToString(hash[:])
	return preview, guard, http.StatusOK, ""
}
