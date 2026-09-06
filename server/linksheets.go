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
	"conway/server/sheets"
)

func (s *server) sheetNow() time.Time {
	if s.sheetsNow != nil {
		return s.sheetsNow()
	}
	return time.Now()
}

func decodeSheetBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		http.Error(w, "provide a valid JSON request", http.StatusBadRequest)
		return false
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "provide one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

// Every nested ID inherits both plan access and manager authorization, including
// reads of raw captures (specs/023-linked-google-sheets.md:111).
func (s *server) handlePlanSources(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, sub string) {
	if (!c.Has("manager") && !c.Has("admin")) || (p.Owner != c.Sub && !c.Has("admin")) {
		http.Error(w, "manager access to this plan is required", http.StatusForbidden)
		return
	}
	if s.db == nil {
		http.Error(w, "linked sheets require the database", http.StatusServiceUnavailable)
		return
	}
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			sources, err := s.db.ListPlanSources(r.Context(), p.ID)
			if err != nil {
				s.sheetFailure(w, err)
				return
			}
			writeJSON(w, map[string]any{"sources": sources, "configured": s.sheetsProvider != nil, "serviceAccountEmail": s.sheetsAccountEmail})
		case http.MethodPost:
			s.createLinkedSheet(w, r, p, c)
		default:
			methodNotAllowed(w, r)
		}
		return
	}
	source, err := s.db.GetPlanSource(r.Context(), p.ID, parts[1])
	if err != nil {
		s.sheetFailure(w, err)
		return
	}
	if source == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 3 && parts[2] == "versions" && r.Method == http.MethodGet {
		versions, err := s.db.ListSourceVersions(r.Context(), source.ID)
		if err != nil {
			s.sheetFailure(w, err)
			return
		}
		writeJSON(w, map[string]any{"versions": versions})
		return
	}
	if len(parts) == 4 && parts[2] == "versions" && r.Method == http.MethodGet {
		v, err := s.db.GetSourceVersion(r.Context(), source.ID, parts[3])
		if err != nil {
			s.sheetFailure(w, err)
			return
		}
		if v == nil {
			http.NotFound(w, r)
			return
		}
		in, err := s.planScheduleFor(p, scheduleRequest{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		candidate := sheets.Parse(source.Kind, v.Rows, in)
		writeJSON(w, map[string]any{"version": v, "planFingerprint": in.Fingerprint(), "preview": candidate, "applyable": candidate.Valid()})
		return
	}
	if len(parts) > 3 {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 3 && parts[2] == "check" && r.Method == http.MethodPost {
		if s.sheetsProvider == nil {
			http.Error(w, "Google Sheets is not configured; ask an administrator to configure its service account", http.StatusServiceUnavailable)
			return
		}
		if source.Status == "disconnected" {
			http.Error(w, "this source is disconnected; its captures remain available", http.StatusConflict)
			return
		}
		result, err := s.checkLinkedSheet(r.Context(), p, *source, c.Sub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, result)
		return
	}
	if len(parts) == 3 && parts[2] == "apply" && r.Method == http.MethodPost {
		s.applyLinkedSheet(w, r, p, c, *source)
		return
	}
	if len(parts) != 2 {
		methodNotAllowed(w, r)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		methodNotAllowed(w, r)
		return
	}
	var body struct {
		Mode        *string `json:"mode"`
		Status      *string `json:"status"`
		PollMinutes *int    `json:"pollMinutes"`
	}
	if r.Method == http.MethodPatch && !decodeSheetBody(w, r, &body) {
		return
	}
	if body.Mode != nil && *body.Mode != "review" && *body.Mode != "auto_apply" {
		http.Error(w, "mode must be review or auto_apply", http.StatusBadRequest)
		return
	}
	if body.Status != nil && *body.Status != "active" && *body.Status != "paused" {
		http.Error(w, "status must be active or paused", http.StatusBadRequest)
		return
	}
	if body.PollMinutes != nil && (*body.PollMinutes < 5 || *body.PollMinutes > 1440) {
		http.Error(w, "pollMinutes must be between 5 and 1440", http.StatusBadRequest)
		return
	}
	token, ok := s.leaseSheet(w, r, *source)
	if !ok {
		return
	}
	defer s.releaseSheet(source.ID, token)
	source, err = s.db.GetPlanSource(r.Context(), p.ID, source.ID)
	if err != nil {
		s.sheetFailure(w, err)
		return
	}
	if source.Status == "disconnected" && r.Method != http.MethodDelete {
		http.Error(w, "disconnected sources retain history; create a new link to resume capture", http.StatusConflict)
		return
	}
	if r.Method == http.MethodDelete {
		source.Status = "disconnected"
	} else {
		if body.Mode != nil {
			source.Mode = *body.Mode
		}
		if body.Status != nil {
			source.Status = *body.Status
		}
		if body.PollMinutes != nil {
			source.PollMinutes = *body.PollMinutes
		}
		source.NextCheckAt = s.sheetNow().Unix() + int64(source.PollMinutes)*60
	}
	if saved, err := s.db.SavePlanSource(r.Context(), *source, token); err != nil || !saved {
		s.sheetFailure(w, err)
		return
	}
	writeJSON(w, map[string]any{"source": source})
}

func (s *server) sheetFailure(w http.ResponseWriter, err error) {
	s.logger().Error().Err(err).Msg("linked sheet persistence failed")
	http.Error(w, "could not save or load the linked source; retry", http.StatusInternalServerError)
}

func (s *server) createLinkedSheet(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	if s.sheetsProvider == nil {
		http.Error(w, "Google Sheets is not configured; ask an administrator to configure its service account", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Kind           string `json:"kind"`
		SpreadsheetURL string `json:"spreadsheetUrl"`
		Range          string `json:"range"`
		Mode           string `json:"mode"`
		PollMinutes    int    `json:"pollMinutes"`
	}
	if !decodeSheetBody(w, r, &body) {
		return
	}
	id, err := sheets.ParseLink(body.SpreadsheetURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Kind != "teams" && body.Kind != "initiatives" {
		http.Error(w, "kind must be teams or initiatives", http.StatusBadRequest)
		return
	}
	body.Range = strings.TrimSpace(body.Range)
	if body.Range == "" || len(body.Range) > 300 || strings.ContainsAny(body.Range, "\r\n\x00") {
		http.Error(w, "provide a tab name or A1 range up to 300 characters", http.StatusBadRequest)
		return
	}
	if body.Mode == "" {
		body.Mode = "review"
	}
	if body.Mode != "review" && body.Mode != "auto_apply" {
		http.Error(w, "mode must be review or auto_apply", http.StatusBadRequest)
		return
	}
	if body.PollMinutes == 0 {
		body.PollMinutes = 15
	}
	if body.PollMinutes < 5 || body.PollMinutes > 1440 {
		http.Error(w, "pollMinutes must be between 5 and 1440", http.StatusBadRequest)
		return
	}
	in, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	now := s.sheetNow().Unix()
	source := db.PlanSource{ID: newID(), PlanID: p.ID, Kind: body.Kind, Provider: "google_sheets", SpreadsheetID: id, SpreadsheetURL: "https://docs.google.com/spreadsheets/d/" + id + "/edit", Range: body.Range, Mode: body.Mode, Status: "active", PollMinutes: body.PollMinutes, CreatedAt: now, NextCheckAt: now, CheckpointFingerprint: in.Fingerprint()}
	if err := s.db.CreatePlanSource(r.Context(), source); err != nil {
		http.Error(w, "could not link this source; disconnect the existing source for this input kind before linking another", http.StatusConflict)
		return
	}
	result, err := s.checkLinkedSheet(r.Context(), p, source, c.Sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, result)
}

func (s *server) leaseSheet(w http.ResponseWriter, r *http.Request, source db.PlanSource) (string, bool) {
	token := newID()
	ok, err := s.db.LeasePlanSource(r.Context(), source.ID, token, s.sheetNow().Unix())
	if err != nil {
		s.sheetFailure(w, err)
		return "", false
	}
	if !ok {
		http.Error(w, "a source check or application is already running; retry when it completes", http.StatusConflict)
	}
	return token, ok
}
func (s *server) releaseSheet(id, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.db.ReleasePlanSource(ctx, id, token); err != nil {
		s.logger().Error().Err(err).Msg("could not release linked source lease")
	}
}

type sheetCheckResult struct {
	Source    db.PlanSource     `json:"source"`
	Version   *db.SourceVersion `json:"version,omitempty"`
	Unchanged bool              `json:"unchanged"`
	Applied   bool              `json:"applied"`
	Conflict  string            `json:"conflict,omitempty"`
}

func (s *server) checkLinkedSheet(ctx context.Context, p *db.PlanRow, source db.PlanSource, actor string) (sheetCheckResult, error) {
	result := sheetCheckResult{Source: source}
	token := newID()
	now := s.sheetNow().Unix()
	ok, err := s.db.LeasePlanSource(ctx, source.ID, token, now)
	if err != nil {
		return result, errors.New("could not start this source check")
	}
	if !ok {
		return result, errors.New("a source check or application is already running")
	}
	defer s.releaseSheet(source.ID, token)
	fresh, err := s.db.GetPlanSource(ctx, p.ID, source.ID)
	if err != nil || fresh == nil {
		return result, errors.New("could not load this source")
	}
	source = *fresh
	if source.Status == "disconnected" {
		return result, errors.New("this source is disconnected")
	}
	if actor == "system:linked-sheets" && (source.Status != "active" || source.NextCheckAt > now) {
		result.Source = source
		return result, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, fetchErr := s.sheetsProvider.Fetch(bounded, source.SpreadsheetID, source.Range)
	source.LastCheckedAt = now
	source.NextCheckAt = now + int64(source.PollMinutes)*60
	source.LastError = ""
	if fetchErr != nil {
		source.LastError = fetchErr.Error()
		if len(source.LastError) > 500 {
			source.LastError = "the source check failed"
		}
		if _, err := s.db.SavePlanSource(ctx, source, token); err != nil {
			return result, errors.New("could not record source check failure")
		}
		result.Source = source
		return result, nil
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return result, errors.New("could not encode captured cells")
	}
	if len(raw) > sheets.MaxBytes {
		return result, errors.New("the capture exceeds the 8 MB limit")
	}
	hash := sha256.Sum256(raw)
	contentHash := hex.EncodeToString(hash[:])
	var version *db.SourceVersion
	if source.LatestVersionID != "" {
		version, err = s.db.GetSourceVersion(ctx, source.ID, source.LatestVersionID)
		if err != nil {
			return result, errors.New("could not load the previous capture")
		}
	}
	if version != nil && version.ContentHash == contentHash {
		result.Unchanged = true
		if _, err := s.db.SavePlanSource(ctx, source, token); err != nil {
			return result, errors.New("could not record source check")
		}
	} else {
		p, err = s.db.GetPlan(p.ID)
		if err != nil || p == nil {
			return result, errors.New("could not load the working plan")
		}
		in, err := s.planScheduleFor(p, scheduleRequest{})
		if err != nil {
			return result, errors.New("the working plan cannot currently be validated")
		}
		candidate := sheets.Parse(source.Kind, rows, in)
		parsed, err := json.Marshal(candidate)
		if err != nil {
			return result, errors.New("could not encode validated capture")
		}
		version = &db.SourceVersion{ID: newID(), SourceID: source.ID, CapturedAt: now, ContentHash: contentHash, Rows: rows, Parsed: parsed, Valid: candidate.Valid(), Errors: candidate.Errors, Warnings: candidate.Warnings, Removals: candidate.Removals, Count: candidate.Count}
		source.LatestVersionID = version.ID
		if saved, err := s.db.CaptureSourceVersion(ctx, source, *version, token); err != nil || !saved {
			return result, errors.New("could not store captured content")
		}
	}
	if source.Mode == "auto_apply" && source.Status == "active" && version.Valid && source.AppliedVersionID != version.ID {
		p, err = s.db.GetPlan(p.ID)
		if err != nil || p == nil {
			return result, errors.New("could not load the working plan")
		}
		updated, _, status, msg := s.applySheetVersion(ctx, p, source, *version, source.CheckpointFingerprint, false, true, actor, token)
		if status == http.StatusOK {
			source = updated
			result.Applied = true
		} else {
			result.Conflict = msg
			source.LastError = msg
			if _, err := s.db.SavePlanSource(ctx, source, token); err != nil {
				return result, errors.New("could not record pending source changes")
			}
		}
	}
	result.Source = source
	result.Version = version
	return result, nil
}

func (s *server) applyLinkedSheet(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims, source db.PlanSource) {
	var body struct {
		VersionID           string `json:"versionId"`
		ExpectedFingerprint string `json:"expectedFingerprint"`
		AllowRemovals       bool   `json:"allowRemovals"`
	}
	if !decodeSheetBody(w, r, &body) {
		return
	}
	if body.VersionID == "" || body.ExpectedFingerprint == "" {
		http.Error(w, "versionId and expectedFingerprint are required; open a fresh capture preview", http.StatusBadRequest)
		return
	}
	token, ok := s.leaseSheet(w, r, source)
	if !ok {
		return
	}
	defer s.releaseSheet(source.ID, token)
	v, err := s.db.GetSourceVersion(r.Context(), source.ID, body.VersionID)
	if err != nil {
		s.sheetFailure(w, err)
		return
	}
	if v == nil {
		http.NotFound(w, r)
		return
	}
	fresh, err := s.db.GetPlanSource(r.Context(), p.ID, source.ID)
	if err != nil || fresh == nil {
		s.sheetFailure(w, err)
		return
	}
	p, err = s.db.GetPlan(p.ID)
	if err != nil || p == nil {
		s.sheetFailure(w, err)
		return
	}
	updated, fingerprint, status, msg := s.applySheetVersion(r.Context(), p, *fresh, *v, body.ExpectedFingerprint, body.AllowRemovals, false, c.Sub, token)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	writeJSON(w, map[string]any{"source": updated, "versionId": v.ID, "planFingerprint": fingerprint})
}

func (s *server) applySheetVersion(ctx context.Context, p *db.PlanRow, source db.PlanSource, v db.SourceVersion, expected string, allowRemovals, automatic bool, actor, token string) (db.PlanSource, string, int, string) {
	refuse := func(code int, msg string) (db.PlanSource, string, int, string) { return source, "", code, msg }
	// Explicit review can recover a contextual failure without rewriting the
	// original evidence (specs/023-linked-google-sheets.md:298).
	if automatic && !v.Valid {
		return refuse(http.StatusUnprocessableEntity, "this capture was invalid when captured and requires explicit review before it can apply")
	}
	in, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		return refuse(http.StatusUnprocessableEntity, "the working plan cannot currently be validated")
	}
	if in.Fingerprint() != expected {
		return refuse(http.StatusConflict, "the working plan changed after the source checkpoint or preview; review this capture against the current plan")
	}
	candidate := sheets.Parse(source.Kind, v.Rows, in)
	if !candidate.Valid() {
		return refuse(http.StatusUnprocessableEntity, strings.Join(candidate.Errors, "; "))
	}
	if len(candidate.Removals) > 0 && (!allowRemovals || automatic) {
		return refuse(http.StatusConflict, "this capture removes scope; open its preview and explicitly acknowledge removals")
	}
	teams, inits := p.Teams, p.Initiatives
	if source.Kind == "teams" {
		teams, err = json.Marshal(candidate.Teams)
	} else {
		inits, err = json.Marshal(candidate.Initiatives)
	}
	if err != nil {
		return refuse(http.StatusUnprocessableEntity, "could not encode the proposed inputs")
	}
	after := planning.NewBaselineInputs(candidate.Teams, candidate.Initiatives, in.Params, in.Scheduling).Fingerprint()
	application := db.SourceApplication{ID: newID(), SourceID: source.ID, VersionID: v.ID, Actor: actor, Automatic: automatic, Restore: source.LatestVersionID != v.ID, AllowRemovals: allowRemovals, AppliedAt: s.sheetNow().Unix(), PreviousFingerprint: expected, ResultingFingerprint: after}
	source.CheckpointFingerprint = after
	source.AppliedVersionID = v.ID
	source.LastError = ""
	saved, err := s.db.ApplySourceVersion(ctx, p, source, teams, inits, application, token)
	if err != nil {
		return refuse(http.StatusInternalServerError, "could not apply the capture; no plan changes were saved")
	}
	if !saved {
		return refuse(http.StatusConflict, "the working plan or source changed during apply; review again")
	}
	return source, after, http.StatusOK, ""
}

// RunLinkedSheets is a server-owned polling loop; the database lease protects
// checks across processes (specs/023-linked-google-sheets.md:258).
func (s *server) RunLinkedSheets(ctx context.Context) {
	if s.sheetsProvider == nil || s.db == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		s.pollLinkedSheets(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *server) pollLinkedSheets(ctx context.Context) {
	if s.sheetsProvider == nil || s.db == nil {
		return
	}
	sources, err := s.db.DuePlanSources(ctx, s.sheetNow().Unix())
	if err != nil {
		s.logger().Error().Err(err).Msg("could not list due linked sources")
		return
	}
	for _, source := range sources {
		if ctx.Err() != nil {
			return
		}
		p, err := s.db.GetPlan(source.PlanID)
		if err != nil || p == nil {
			continue
		}
		if _, err := s.checkLinkedSheet(ctx, p, source, "system:linked-sheets"); err != nil {
			s.logger().Warn().Err(err).Msg("linked source check deferred")
		}
	}
}
