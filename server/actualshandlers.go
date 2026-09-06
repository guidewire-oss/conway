package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
)

func canReadExecutionSnapshot(snap *db.SnapshotRow, c auth.Claims) bool {
	return snap != nil && (snap.Owner == c.Sub || snap.Public || snap.Source == "baseline" || c.Has("admin"))
}

func (s *server) planActuals(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	id := strings.TrimSpace(r.URL.Query().Get("snapshot"))
	if id == "" {
		http.Error(w, "Choose an imported snapshot to review execution.", 400)
		return
	}
	loaded, status, message := s.loadExecutionEvidence(p, c, id)
	if status != http.StatusOK {
		http.Error(w, message, status)
		return
	}
	snap := loaded.Snapshot
	var baseline any
	if loaded.Baseline != nil {
		baseline = map[string]any{"id": loaded.Baseline.ID, "name": loaded.Baseline.Name, "createdAt": loaded.Baseline.CreatedAt, "periodStart": loaded.Schedule.PeriodStart, "horizonWeeks": loaded.Schedule.HorizonWeeks}
	}
	actuals := loaded.Actuals
	writeJSON(w, map[string]any{"snapshot": map[string]any{"id": snap.ID, "name": snap.Name, "source": snap.Source, "createdAt": snap.CreatedAt, "ageDays": math.Max(0, time.Since(time.Unix(snap.CreatedAt, 0)).Hours()/24)}, "baseline": baseline, "coverage": actuals.Coverage, "initiatives": actuals.Initiatives, "calibration": actuals.Calibration, "adherence": actuals.Adherence, "gaps": actuals.Gaps})
}

type executionEvidence struct {
	Snapshot *db.SnapshotRow
	Baseline *db.BaselineRow
	Schedule *planning.Schedule
	Issues   []db.IssueRow
	Actuals  *planning.ExecutionActuals
}

// Both live inspection and completed reviews use this evidence path. A manual
// review has no derived measurements. specs/024-weekly-execution-review.md:265
func (s *server) loadExecutionEvidence(p *db.PlanRow, c auth.Claims, id string) (executionEvidence, int, string) {
	loaded := executionEvidence{}
	var err error
	if id != "" {
		loaded.Snapshot, err = s.db.GetSnapshot(id)
		if err != nil {
			return loaded, http.StatusInternalServerError, "Could not load snapshot."
		}
		if !canReadExecutionSnapshot(loaded.Snapshot, c) {
			return loaded, http.StatusNotFound, "Snapshot not found or inaccessible."
		}
		loaded.Issues, err = s.db.ExecutionIssues(id)
		if err != nil {
			return loaded, http.StatusInternalServerError, "Could not read snapshot issue evidence."
		}
	}
	var current []planning.Initiative
	if len(p.Initiatives) > 0 && json.Unmarshal(p.Initiatives, &current) != nil {
		return loaded, http.StatusInternalServerError, "Plan initiatives are unreadable."
	}
	loaded.Baseline, err = s.db.ActiveBaseline(p.ID)
	if err != nil {
		return loaded, http.StatusInternalServerError, "Could not load agreed baselines."
	}
	var base *planning.BaselineInputs
	if loaded.Baseline != nil {
		base = &planning.BaselineInputs{}
		loaded.Schedule = &planning.Schedule{}
		if json.Unmarshal(loaded.Baseline.Inputs, base) != nil || json.Unmarshal(loaded.Baseline.Schedule, loaded.Schedule) != nil {
			return loaded, http.StatusInternalServerError, "Agreed baseline is unreadable."
		}
	}
	if loaded.Snapshot == nil {
		return loaded, http.StatusOK, ""
	}
	evidence := make([]planning.ExecutionIssue, 0, len(loaded.Issues))
	for _, i := range loaded.Issues {
		evidence = append(evidence, planning.ExecutionIssue{Key: i.Key, ParentKey: i.ParentKey, Pod: i.Pod, Type: i.IssueType, Summary: i.Summary, StatusCategory: i.StatusCat, Created: i.Created, Updated: i.Updated, Resolved: i.Resolved})
	}
	actuals := planning.DeriveActuals(current, base, loaded.Schedule, evidence, time.Unix(loaded.Snapshot.CreatedAt, 0))
	loaded.Actuals = &actuals
	return loaded, http.StatusOK, ""
}

func validateExecutionDecision(v *db.ExecutionDecision, inits []planning.Initiative) error {
	v.Action = strings.TrimSpace(v.Action)
	v.Owner = strings.TrimSpace(v.Owner)
	v.Rationale = strings.TrimSpace(v.Rationale)
	v.ReviewDate = strings.TrimSpace(v.ReviewDate)
	if v.Action == "" || v.Owner == "" || v.Rationale == "" {
		return fmt.Errorf("action, owner and rationale are required")
	}
	if len(v.Action) > 1000 || len(v.Owner) > 200 || len(v.Rationale) > 10000 {
		return fmt.Errorf("keep action within 1000 characters, owner within 200 and rationale within 10000")
	}
	if _, err := time.Parse("2006-01-02", v.ReviewDate); err != nil {
		return fmt.Errorf("review date must be YYYY-MM-DD")
	}
	if v.Initiative != "" {
		found := false
		for _, it := range inits {
			if it.Name == v.Initiative {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("choose an initiative in this plan")
		}
	}
	return nil
}

// Review history appends; no request can rewrite a previous review or agreement.
// specs/017-planning-and-execution-usability.md:90
func (s *server) planDecisions(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	if (!c.Has("manager") && !c.Has("admin")) || c.GameID != "" {
		http.Error(w, "manager access is required", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.db.ExecutionDecisions(p.ID)
		if err != nil {
			http.Error(w, "Could not load review decisions.", 500)
			return
		}
		writeJSON(w, map[string]any{"decisions": rows})
		return
	}
	var v db.ExecutionDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&v); err != nil {
		http.Error(w, "Could not read decision.", 400)
		return
	}
	var inits []planning.Initiative
	if err := json.Unmarshal(p.Initiatives, &inits); err != nil {
		http.Error(w, "Plan initiatives are unreadable.", 500)
		return
	}
	if err := validateExecutionDecision(&v, inits); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if v.SnapshotID != "" {
		snap, err := s.db.GetSnapshot(v.SnapshotID)
		if err != nil {
			http.Error(w, "Could not load snapshot.", 500)
			return
		}
		if !canReadExecutionSnapshot(snap, c) {
			http.Error(w, "Snapshot not found or inaccessible.", 404)
			return
		}
	}
	if v.BaselineID != "" {
		base, err := s.db.GetBaseline(p.ID, v.BaselineID)
		if err != nil {
			http.Error(w, "Could not load baseline.", 500)
			return
		}
		if base == nil {
			http.Error(w, "Baseline not found in this plan.", 404)
			return
		}
	}
	v.ID = newID()
	v.PlanID = p.ID
	v.CreatedBy = c.Sub
	v.CreatedAt = time.Now().Unix()
	v.Status = "open"
	v.Version = 1
	if err := s.db.AppendExecutionDecision(v); err != nil {
		http.Error(w, "Could not save review decision.", 500)
		return
	}
	writeJSON(w, v)
}
