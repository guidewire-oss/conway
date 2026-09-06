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
	snap, err := s.db.GetSnapshot(id)
	if err != nil {
		http.Error(w, "Could not load snapshot.", 500)
		return
	}
	if !canReadExecutionSnapshot(snap, c) {
		http.Error(w, "Snapshot not found or inaccessible.", 404)
		return
	}
	issues, err := s.db.ExecutionIssues(id)
	if err != nil {
		http.Error(w, "Could not read snapshot issue evidence.", 500)
		return
	}
	var current []planning.Initiative
	if err = json.Unmarshal(p.Initiatives, &current); err != nil {
		http.Error(w, "Plan initiatives are unreadable.", 500)
		return
	}
	rows, err := s.db.ListBaselines(p.ID)
	if err != nil {
		http.Error(w, "Could not load agreed baselines.", 500)
		return
	}
	var base *planning.BaselineInputs
	var schedule *planning.Schedule
	var baseline any
	for _, b := range rows {
		if !b.Active {
			continue
		}
		stored, err := s.db.GetBaseline(p.ID, b.ID)
		if err != nil || stored == nil {
			http.Error(w, "Could not read active baseline.", 500)
			return
		}
		base = &planning.BaselineInputs{}
		schedule = &planning.Schedule{}
		if json.Unmarshal(stored.Inputs, base) != nil || json.Unmarshal(stored.Schedule, schedule) != nil {
			http.Error(w, "Agreed baseline is unreadable.", 500)
			return
		}
		baseline = map[string]any{"id": b.ID, "name": b.Name, "createdAt": b.CreatedAt, "periodStart": schedule.PeriodStart, "horizonWeeks": schedule.HorizonWeeks}
		break
	}
	evidence := make([]planning.ExecutionIssue, 0, len(issues))
	for _, i := range issues {
		evidence = append(evidence, planning.ExecutionIssue{Key: i.Key, ParentKey: i.ParentKey, Pod: i.Pod, Type: i.IssueType, Summary: i.Summary, StatusCategory: i.StatusCat, Created: i.Created, Updated: i.Updated, Resolved: i.Resolved})
	}
	actuals := planning.DeriveActuals(current, base, schedule, evidence, time.Unix(snap.CreatedAt, 0))
	writeJSON(w, map[string]any{"snapshot": map[string]any{"id": snap.ID, "name": snap.Name, "source": snap.Source, "createdAt": snap.CreatedAt, "ageDays": math.Max(0, time.Since(time.Unix(snap.CreatedAt, 0)).Hours()/24)}, "baseline": baseline, "coverage": actuals.Coverage, "initiatives": actuals.Initiatives, "calibration": actuals.Calibration, "adherence": actuals.Adherence, "gaps": actuals.Gaps})
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
	if err := s.db.AppendExecutionDecision(v); err != nil {
		http.Error(w, "Could not save review decision.", 500)
		return
	}
	writeJSON(w, v)
}
