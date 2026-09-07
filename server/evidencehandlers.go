package main

import (
	"context"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/evidence"
	"conway/server/jira"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type evidenceCredential struct {
	Email string `json:"email"`
	Token string `json:"token"`
}
type evidenceInput struct {
	Config        evidence.Config `json:"config"`
	Version       int64           `json:"version"`
	Email         string          `json:"email"`
	Token         string          `json:"token"`
	Consent       bool            `json:"consent"`
	RefreshRoster bool            `json:"refreshRoster"`
}

func (s *server) evidenceTime() int64 {
	if s.evidenceNow != nil {
		return s.evidenceNow().Unix()
	}
	return time.Now().Unix()
}
func (s *server) evidenceFailure(w http.ResponseWriter, err error) {
	s.logger().Error().Err(err).Msg("evidence persistence failed")
	http.Error(w, "Could not access capture data. Refresh and retry.", http.StatusInternalServerError)
}
func (s *server) evidenceAuthorized(w http.ResponseWriter, r *http.Request, c *auth.Claims) bool {
	if c.GameID != "" || c.Sub == "" || (!c.Has("manager") && !c.Has("admin")) {
		http.Error(w, "Manager access required.", http.StatusForbidden)
		return false
	}
	if s.db == nil {
		http.Error(w, "Evidence sources require the database.", http.StatusServiceUnavailable)
		return false
	}
	roles, err := s.db.EvidenceActor(r.Context(), c.Sub, s.evidenceTime())
	if errors.Is(err, db.ErrEvidenceOwner) {
		http.Error(w, "Current manager access required.", http.StatusForbidden)
		return false
	}
	if err != nil {
		s.evidenceFailure(w, err)
		return false
	}
	c.Roles = roles
	return true
}
func (s *server) handleEvidenceSources(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.evidenceAuthorized(w, r, &c) {
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.db.ListEvidenceSources(r.Context(), c.Sub, c.Has("admin"))
		if err != nil {
			s.evidenceFailure(w, err)
			return
		}
		writeJSON(w, rows)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	row := db.EvidenceSource{ID: newID(), Owner: c.Sub, Version: 1}
	if !s.readEvidence(w, r, c, &row, true) {
		return
	}
	if err := s.db.CreateEvidenceSource(r.Context(), row); err != nil {
		s.evidenceFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, row)
}
func (s *server) readEvidence(w http.ResponseWriter, r *http.Request, c auth.Claims, row *db.EvidenceSource, create bool) bool {
	var b evidenceInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		http.Error(w, "Enter valid source settings.", http.StatusBadRequest)
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "Enter one source configuration.", http.StatusBadRequest)
		return false
	}
	cfg := b.Config
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Site = strings.TrimRight(strings.TrimSpace(cfg.Site), "/")
	if !create && (b.Version != row.Version || cfg.Site != row.Config.Site) {
		http.Error(w, "Source changed or site differs. Reload; use a new source for a different site.", http.StatusConflict)
		return false
	}
	if create || cfg.RosterID != row.Config.RosterID || b.RefreshRoster {
		roster, err := s.db.GetRoster(cfg.RosterID)
		if err != nil {
			s.evidenceFailure(w, err)
			return false
		}
		if roster == nil || (!roster.Public && roster.Owner != c.Sub && !c.Has("admin")) {
			http.Error(w, "Roster not found or inaccessible.", http.StatusNotFound)
			return false
		}
		var pods []NetPod
		if json.Unmarshal(roster.Pods, &pods) != nil || len(pods) == 0 {
			http.Error(w, "Choose a nonempty roster.", http.StatusBadRequest)
			return false
		}
		cfg.Roster = append(json.RawMessage{}, roster.Pods...)
		if create {
			cfg.Teams = []evidence.Team{}
			for _, p := range pods {
				cfg.Teams = append(cfg.Teams, evidence.Team{ID: newID(), Name: p.Name, Aliases: []string{p.Name}})
			}
		}
	} else {
		cfg.Roster = row.Config.Roster
	}
	if !create {
		known := map[string]bool{}
		for _, t := range row.Config.Teams {
			known[t.ID] = true
		}
		for i := range cfg.Teams {
			if cfg.Teams[i].ID == "" {
				cfg.Teams[i].ID = newID()
			} else if !known[cfg.Teams[i].ID] {
				http.Error(w, "Unknown team identity. Add new teams without an ID.", http.StatusBadRequest)
				return false
			}
		}
		for _, t := range cfg.Teams {
			delete(known, t.ID)
		}
		if len(known) > 0 {
			http.Error(w, "Retain existing team identities; edit their names and aliases instead.", http.StatusBadRequest)
			return false
		}
	}
	if err := evidence.Validate(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	if create || b.Token != "" || b.Email != "" {
		if !b.Consent || strings.TrimSpace(b.Email) == "" || b.Token == "" || len(b.Token) > 16384 || len(b.Email) > 320 {
			http.Error(w, "Provide email, API token and consent to encrypted storage for scheduled reads.", http.StatusBadRequest)
			return false
		}
		plain, _ := json.Marshal(evidenceCredential{Email: strings.TrimSpace(b.Email), Token: b.Token})
		sealed, err := evidence.Seal(s.store.Secret, row.ID, plain)
		if err != nil {
			http.Error(w, "Credential storage unavailable.", http.StatusServiceUnavailable)
			return false
		}
		row.Credential = sealed
	}
	if create || cfg.IntervalHours != row.Config.IntervalHours || cfg.Enabled != row.Config.Enabled {
		row.NextAt = s.evidenceTime() + int64(max(cfg.IntervalHours, 1))*3600
	}
	row.Config = cfg
	return true
}
func (s *server) handleEvidenceSource(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.evidenceAuthorized(w, r, &c) {
		return
	}
	id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/evidence-sources/"), "/")
	row, err := s.db.GetEvidenceSource(r.Context(), id)
	if err != nil {
		s.evidenceFailure(w, err)
		return
	}
	if row == nil || (row.Owner != c.Sub && !c.Has("admin")) {
		http.Error(w, "Source not found or inaccessible.", http.StatusNotFound)
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		runs, err := s.db.EvidenceRuns(r.Context(), id)
		if err != nil {
			s.evidenceFailure(w, err)
			return
		}
		writeJSON(w, map[string]any{"source": row, "runs": runs, "freshness": evidence.Freshness(row.LastSuccess, row.Config.FreshnessHours, s.evidenceTime())})
	case r.Method == http.MethodPut && action == "":
		if !s.readEvidence(w, r, c, row, false) {
			return
		}
		err := s.db.UpdateEvidenceSource(r.Context(), *row, s.evidenceTime())
		if errors.Is(err, db.ErrEvidenceConflict) {
			http.Error(w, "Capture running or settings changed. Refresh before saving.", http.StatusConflict)
			return
		}
		if err != nil {
			s.evidenceFailure(w, err)
			return
		}
		row, err = s.db.GetEvidenceSource(r.Context(), row.ID)
		if err != nil {
			s.evidenceFailure(w, err)
			return
		}
		writeJSON(w, row)
	case r.Method == http.MethodPost && action == "capture":
		run, err := s.launchEvidence(r.Context(), id, true)
		if errors.Is(err, db.ErrEvidenceConflict) {
			http.Error(w, "A capture is already running or workers are busy. Refresh and retry.", http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrEvidenceOwner) {
			http.Error(w, "Source owner no longer has manager access.", http.StatusForbidden)
			return
		}
		if err != nil {
			s.evidenceFailure(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, map[string]string{"runId": run})
	default:
		methodNotAllowed(w, r)
	}
}
func (s *server) evidenceSlotsChannel() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.evidenceSlots == nil {
		s.evidenceSlots = make(chan struct{}, 4)
	}
	return s.evidenceSlots
}
func (s *server) launchEvidence(ctx context.Context, id string, manual bool) (string, error) {
	slots := s.evidenceSlotsChannel()
	select {
	case slots <- struct{}{}:
	default:
		return "", db.ErrEvidenceConflict
	}
	run := newID()
	source, err := s.db.ClaimEvidence(ctx, id, run, s.evidenceTime(), manual)
	if err != nil {
		<-slots
		return "", err
	}
	go func() { defer func() { <-slots }(); s.runEvidence(source) }()
	return run, nil
}

// RunEvidenceSources resumes durable due work after restart; one minute scan.
// per specs/026-reliable-evidence-foundation.md:128
func (s *server) RunEvidenceSources(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		s.captureDueEvidence(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *server) captureDueEvidence(ctx context.Context) {
	if s.db == nil {
		return
	}
	sources, err := s.db.DueEvidenceSources(ctx, s.evidenceTime())
	if err != nil {
		s.logger().Error().Err(err).Msg("could not load capture schedules")
		return
	}
	for _, id := range sources {
		if _, err = s.launchEvidence(ctx, id, false); err != nil && !errors.Is(err, db.ErrEvidenceConflict) && !errors.Is(err, db.ErrEvidenceOwner) {
			s.logger().Error().Err(err).Msg("could not claim capture")
		}
	}
}
func (s *server) runEvidence(source *db.EvidenceSource) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	finish := func(snapshot db.SnapshotRow, data db.SnapshotData, lineage []byte, message string) {
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer saveCancel()
		if err := s.db.FinishEvidence(saveCtx, source.ID, source.ActiveRun, s.evidenceTime(), snapshot, data, lineage, message); err != nil && !errors.Is(err, db.ErrEvidenceConflict) {
			s.logger().Error().Err(err).Msg("could not finish capture; lease recovery will retry")
		}
	}
	fail := func(message string) { finish(db.SnapshotRow{}, db.SnapshotData{}, nil, message) }
	plain, err := evidence.Open(s.store.Secret, source.ID, source.Credential)
	if err != nil {
		fail("Saved credentials unavailable. Replace credentials and retry.")
		return
	}
	var cr evidenceCredential
	if json.Unmarshal(plain, &cr) != nil {
		fail("Saved credentials unavailable. Replace credentials and retry.")
		return
	}
	var detailed []jira.DetailedIssue
	if s.evidenceFetch != nil {
		detailed, err = s.evidenceFetch(ctx, source.Config, cr)
	} else {
		cl := jira.New(source.Config.Site, cr.Email, cr.Token, source.Config.PodField)
		cl.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		detailed, err = cl.SearchDetailed(ctx, projectJQL(source.Config.Projects), "", nil)
	}
	if err != nil {
		fail("Capture failed. Check Jira access, project keys and connection; replace credentials if needed, then retry.")
		return
	}
	if ctx.Err() != nil {
		fail("Capture timed out. Check project scope and retry.")
		return
	}
	var roster []NetPod
	if json.Unmarshal(source.Config.Roster, &roster) != nil {
		fail("Pinned roster is unavailable. Reselect the roster and retry.")
		return
	}
	pods, err := s.importStructure(roster)
	if err != nil {
		fail("Pinned roster is empty. Reselect the roster and retry.")
		return
	}
	identities := []map[string]string{}
	unmapped := map[string]bool{}
	basic := make([]jira.Issue, len(detailed))
	seen := map[string]bool{}
	for i, it := range detailed {
		if it.ID == "" || it.Key == "" || seen[it.ID] {
			fail("Capture lacks unique Jira issue IDs. Check the source response before retrying.")
			return
		}
		seen[it.ID] = true
		team := evidence.TeamID(source.Config.Teams, it.Pod)
		if it.Pod != "" && team == "" {
			unmapped[it.Pod] = true
		}
		identities = append(identities, map[string]string{"id": evidence.Identity(source.ID, "issue", it.ID), "providerId": it.ID, "key": it.Key, "teamId": team, "teamName": it.Pod})
		basic[i] = it.Basic()
	}
	now := s.evidenceTime()
	lineage, _ := json.Marshal(map[string]any{"sourceId": source.ID, "sourceName": source.Config.Name, "version": source.Version, "capturedAt": now, "teams": source.Config.Teams, "issues": identities, "unmappedTeams": unmapped, "rosterId": source.Config.RosterID, "roster": source.Config.Roster})
	stats, edges := jira.Aggregate(basic, source.Config.WipMode)
	data := snapshotDataFromJira(pods, detailed, stats, edges)
	scope, _ := json.Marshal(source.Config.Projects)
	finish(db.SnapshotRow{ID: newID(), Name: fmt.Sprintf("%s · %s", source.Config.Name, time.Unix(now, 0).UTC().Format("2006-01-02 15:04 UTC")), Scope: scope, CreatedAt: now}, data, lineage, "")
}
