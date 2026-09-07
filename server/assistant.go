package main

import (
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type assistantRequest struct {
	Task                string `json:"task"`
	Question            string `json:"question"`
	AllowExternal       bool   `json:"allowExternal"`
	Initiative          string `json:"initiative"`
	Team                string `json:"team"`
	ReviewDate          string `json:"reviewDate"`
	Timezone            string `json:"timezone"`
	SnapshotID          string `json:"snapshotId"`
	ExpectedFingerprint string `json:"expectedFingerprint"`
}
type assistantSource struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}
type assistantFact struct {
	Kind    string          `json:"kind"`
	Title   string          `json:"title"`
	Details []string        `json:"details"`
	Source  assistantSource `json:"source"`
}
type assistantContext struct {
	EvidenceFingerprint string                   `json:"evidenceFingerprint,omitempty"`
	PlanID              string                   `json:"planId"`
	PlanName            string                   `json:"planName"`
	PlanFingerprint     string                   `json:"planFingerprint"`
	PeriodStart         string                   `json:"periodStart"`
	Initiative          string                   `json:"initiative"`
	Team                string                   `json:"team"`
	PreparedAt          int64                    `json:"preparedAt"`
	Agreement           *planning.ReviewBaseline `json:"agreement,omitempty"`
	Review              *planning.ReviewContext  `json:"review,omitempty"`
}
type assistantAnswer struct {
	Sources     []assistantSource `json:"sources"`
	Task        string            `json:"task"`
	Summary     string            `json:"summary"`
	Context     assistantContext  `json:"context"`
	Fingerprint string            `json:"fingerprint"`
	Facts       []assistantFact   `json:"facts"`
	Gaps        []string          `json:"gaps"`
}

// specs/027-evidence-linked-planning-assistant.md:173: every source is constructed
// from authorized context, never supplied by a model or source record.
func assistantLink(planID, view, initiative, team, snapshot string) assistantSource {
	q := url.Values{"view": {"plan"}, "plan": {planID}, "planView": {view}}
	if initiative != "" {
		q.Set("selected", initiative)
		q.Set("initiative", initiative)
	}
	if team != "" {
		q.Set("team", team)
	}
	if snapshot != "" {
		q.Set("executionSnapshot", snapshot)
	}
	labels := map[string]string{"timeline": "Open timeline", "order": "Open agreement and plan", "execution": "Open execution review"}
	return assistantSource{Label: labels[view], URL: "?" + q.Encode()}
}
func assistantScope(inputs planning.BaselineInputs, req assistantRequest) bool {
	teamOK := req.Team == ""
	initiativeOK := req.Initiative == ""
	for _, t := range inputs.Teams {
		if t.Name == req.Team {
			teamOK = true
		}
	}
	for _, i := range inputs.Initiatives {
		if i.Name == req.Initiative {
			initiativeOK = true
			if req.Team != "" {
				work, exists := i.Work[req.Team]
				teamOK = teamOK && exists && work.InPath
			}
		}
	}
	return teamOK && initiativeOK
}
func assistantMatches(inputs planning.BaselineInputs, name string, req assistantRequest) bool {
	if req.Initiative != "" && name != req.Initiative {
		return false
	}
	if req.Team == "" {
		return true
	}
	for _, i := range inputs.Initiatives {
		if i.Name == name {
			work, ok := i.Work[req.Team]
			return ok && work.InPath
		}
	}
	return false
}
func assistantWeek(origin string, week int) string {
	if d, e := time.Parse("2006-01-02", origin); e == nil {
		return fmt.Sprintf("%s (week %d)", d.AddDate(0, 0, 7*week).Format("2006-01-02"), week)
	}
	return fmt.Sprintf("relative week %d", week)
}
func assistantPlaced(i planning.ScheduledInitiative) bool {
	return len(i.Slices) > 0 && i.Verdict != "beyond-horizon" && i.Verdict != "unschedulable"
}
func assistantScheduleFacts(inputs planning.BaselineInputs, sched *planning.Schedule, req assistantRequest, planID string) []assistantFact {
	facts := []assistantFact{}
	for _, i := range sched.Initiatives {
		if !assistantMatches(inputs, i.Name, req) {
			continue
		}
		details := []string{"Verdict: " + i.Verdict}
		if assistantPlaced(i) {
			details = append(details, "Modeled start: "+assistantWeek(sched.PeriodStart, i.StartWeek), "Modeled finish before buffer: "+assistantWeek(sched.PeriodStart, i.RawFinishWeek), "Modeled commitment with buffer: "+assistantWeek(sched.PeriodStart, i.CommitWeek))
		} else {
			details = append(details, "No complete placement to report; start and finish remain unknown.")
		}
		if i.BindingConstraint != "" {
			details = append(details, "Scheduling constraint: "+i.BindingConstraint)
		} else {
			details = append(details, "No specific binding constraint was reported by the scheduler.")
		}
		details = append(details, fmt.Sprintf("Ordering: %s; proposed rank %d, stated rank %d.", sched.Rule, i.ProposedRank, i.StatedRank))
		if i.Provisional {
			details = append(details, "Provisional: this result depends on incomplete estimates or assumptions.")
		}
		if len(i.UnestimatedPods) > 0 {
			details = append(details, "Missing estimates: "+strings.Join(i.UnestimatedPods, ", "))
		}
		details = append(details, i.Assumptions...)
		for _, slice := range i.Slices {
			if req.Team == "" || slice.Pod == req.Team {
				details = append(details, fmt.Sprintf("%s: modeled weeks %d–%d.", slice.Pod, slice.StartWeek, slice.FinishWeek))
			}
		}
		facts = append(facts, assistantFact{Kind: "Modeled", Title: i.Name, Details: details, Source: assistantLink(planID, "timeline", i.Name, req.Team, "")})
	}
	return facts
}
func assistantChangeFacts(before, now *planning.Schedule, inputs, oldInputs planning.BaselineInputs, req assistantRequest, planID string) []assistantFact {
	facts := []assistantFact{}
	comparison := planning.CompareToBaseline(before, now)
	old := map[string]planning.ScheduledInitiative{}
	current := map[string]planning.ScheduledInitiative{}
	for _, i := range before.Initiatives {
		old[i.Name] = i
	}
	for _, i := range now.Initiatives {
		current[i.Name] = i
	}
	for _, d := range comparison.Initiatives {
		if !assistantMatches(inputs, d.Name, req) && !assistantMatches(oldInputs, d.Name, req) {
			continue
		}
		details := []string{fmt.Sprintf("Rank: %d → %d. Verdict: %s → %s.", d.BaselineRank, d.ProposedRank, d.BaselineVerdict, d.Verdict)}
		switch {
		case !assistantPlaced(old[d.Name]) || !assistantPlaced(current[d.Name]):
			details = append(details, "Date movement unknown: one or both schedules lack a complete placement.")
		case before.PeriodStart == "" || now.PeriodStart == "" || before.PeriodStart != now.PeriodStart:
			details = append(details, "Date movement withheld: period origins are absent or different. Compare the original schedules.")
		default:
			details = append(details, fmt.Sprintf("Start movement: %+d weeks. Commitment movement: %+d weeks.", d.StartDeltaWeeks, d.CommitDeltaWeeks))
		}
		facts = append(facts, assistantFact{Kind: "Modeled comparison", Title: d.Name, Details: details, Source: assistantLink(planID, "order", d.Name, req.Team, "")})
	}
	for _, group := range []struct {
		names []string
		label string
		in    planning.BaselineInputs
	}{{comparison.Added, "Added since agreement", inputs}, {comparison.Removed, "Removed since agreement", oldInputs}} {
		for _, name := range group.names {
			if assistantMatches(group.in, name, req) {
				facts = append(facts, assistantFact{Kind: "Recorded scope change", Title: name, Details: []string{group.label}, Source: assistantLink(planID, "order", "", req.Team, "")})
			}
		}
	}
	return facts
}

// specs/027-evidence-linked-planning-assistant.md:76: independent role checking
// precedes every evidence read; the route intentionally exposes no mutations.
func (s *server) handleAssistant(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	w.Header().Set("Cache-Control", "no-store")
	if c.GameID != "" || (!c.Has("manager") && !c.Has("admin")) || (p.Owner != c.Sub && !c.Has("admin")) {
		http.Error(w, "Manager access to this plan is required.", http.StatusForbidden)
		return
	}
	roles, actorErr := s.db.EvidenceActor(r.Context(), c.Sub, time.Now().Unix())
	if actorErr != nil {
		http.Error(w, "Current manager access is required. Sign in again or contact an administrator.", http.StatusForbidden)
		return
	}
	c.Roles = roles
	if p.Owner != c.Sub && !c.Has("admin") {
		http.Error(w, "Manager access to this plan is required.", http.StatusForbidden)
		return
	}
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		http.Error(w, "Saved planning inputs could not be read. Review plan setup.", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		teams := []string{}
		initiatives := []string{}
		for _, t := range inputs.Teams {
			teams = append(teams, t.Name)
		}
		for _, i := range inputs.Initiatives {
			initiatives = append(initiatives, i.Name)
		}
		writeJSON(w, map[string]any{"planId": p.ID, "planName": p.Name, "fingerprint": inputs.Fingerprint(), "teams": teams, "initiatives": initiatives, "externalAvailable": s.assistantModel != nil, "externalDisclosure": "OpenAI receives your question and this plan’s initiative/team names to select a supported question. Schedule results, snapshots and review records stay within Conway. Guided questions use no external model."})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	var req assistantRequest
	if !decodeReviewBody(w, r, &req) {
		return
	}
	if !slices.Contains([]string{"schedule", "changes", "review", "question"}, req.Task) || len(req.Question) > 2000 || len(req.Team) > 200 || len(req.Initiative) > 1000 || !assistantScope(inputs, req) {
		http.Error(w, "Choose a supported question and a team/initiative in this saved plan.", http.StatusBadRequest)
		return
	}
	if req.ExpectedFingerprint != "" && req.ExpectedFingerprint != inputs.Fingerprint() {
		http.Error(w, "Saved plan changed. Refresh assistant context and ask again; your question is retained.", http.StatusConflict)
		return
	}
	if req.Task == "question" {
		if !req.AllowExternal {
			http.Error(w, "Consent is required before sending your question to the configured provider.", http.StatusBadRequest)
			return
		}
		if s.assistantModel == nil {
			http.Error(w, "Free-text interpretation is not configured. Choose a guided question.", http.StatusServiceUnavailable)
			return
		}
		names, teams := []string{}, []string{}
		for _, i := range inputs.Initiatives {
			names = append(names, i.Name)
		}
		for _, t := range inputs.Teams {
			teams = append(teams, t.Name)
		}
		// Before model processing, the existing request context has already passed plan access checks.
		choice, e := s.assistantModel.interpret(r.Context(), req.Question, names, teams)
		if e != nil {
			http.Error(w, e.Error(), http.StatusServiceUnavailable)
			return
		}
		if choice.Task == "unsupported" {
			http.Error(w, "This assistant explains schedules, compares agreements and prepares reviews. Choose one of those questions; no changes were made.", http.StatusUnprocessableEntity)
			return
		}
		if (req.Team != "" && choice.Team != "" && req.Team != choice.Team) || (req.Initiative != "" && choice.Initiative != "" && req.Initiative != choice.Initiative) {
			http.Error(w, "The question names a different scope. Change the selectors and ask again.", http.StatusBadRequest)
			return
		}
		req.Task = choice.Task
		if choice.Team != "" {
			req.Team = choice.Team
		}
		if choice.Initiative != "" {
			req.Initiative = choice.Initiative
		}
		if !assistantScope(inputs, req) {
			http.Error(w, "The requested initiative is not assigned to the selected team.", http.StatusBadRequest)
			return
		}
		fresh, e := s.db.GetPlan(p.ID)
		if e != nil {
			http.Error(w, "Could not refresh saved context. Retry.", http.StatusInternalServerError)
			return
		}
		if fresh == nil || (fresh.Owner != c.Sub && !c.Has("admin")) {
			http.Error(w, "Plan access changed. Reopen the plan.", http.StatusForbidden)
			return
		}
		currentRoles, actorErr := s.db.EvidenceActor(r.Context(), c.Sub, time.Now().Unix())
		if actorErr != nil {
			http.Error(w, "Account access changed. Sign in again.", http.StatusForbidden)
			return
		}
		c.Roles = currentRoles
		if fresh.Owner != c.Sub && !c.Has("admin") {
			http.Error(w, "Plan access changed. Reopen the plan.", http.StatusForbidden)
			return
		}
		if c.Exp != 0 && time.Now().Unix() >= c.Exp {
			http.Error(w, "Session expired. Sign in again.", http.StatusUnauthorized)
			return
		}
		freshInputs, e := s.planScheduleFor(fresh, scheduleRequest{})
		if e != nil || freshInputs.Fingerprint() != inputs.Fingerprint() {
			http.Error(w, "Saved plan changed while interpreting the question. Refresh context and ask again.", http.StatusConflict)
			return
		}
		p = fresh
	}
	out := assistantAnswer{Task: req.Task, Context: assistantContext{PlanID: p.ID, PlanName: p.Name, PlanFingerprint: inputs.Fingerprint(), PeriodStart: inputs.Scheduling.PeriodStart, Initiative: req.Initiative, Team: req.Team}, Facts: []assistantFact{}, Gaps: []string{"Answers use saved inputs; unsaved edits are not included. Planned placement does not establish an actual start or completion."}}
	switch req.Task {
	case "schedule":
		sched := inputs.RecomputeWith(planning.ScheduleOptions{})
		out.Facts = assistantScheduleFacts(inputs, sched, req, p.ID)
		out.Summary = fmt.Sprintf("%d initiative(s) in the selected saved schedule scope.", len(out.Facts))
		out.Gaps = append(out.Gaps, sched.Assumptions...)
	case "changes":
		b, e := s.db.ActiveBaseline(p.ID)
		if e != nil {
			http.Error(w, "Could not load the active agreement. Retry.", http.StatusInternalServerError)
			return
		}
		if b == nil {
			out.Summary = "No active agreement is available for comparison."
			out.Gaps = append(out.Gaps, "Save an agreed baseline from Plan commitments before comparing changes.")
			break
		}
		var before planning.Schedule
		var oldInputs planning.BaselineInputs
		if json.Unmarshal(b.Schedule, &before) != nil || json.Unmarshal(b.Inputs, &oldInputs) != nil {
			http.Error(w, "The active agreement is unreadable. Open the agreement to inspect it.", http.StatusInternalServerError)
			return
		}
		now := inputs.RecomputeWith(planning.ScheduleOptions{})
		out.Summary = "Comparison with active agreement: " + b.Name
		out.Context.Agreement = &planning.ReviewBaseline{ID: b.ID, Name: b.Name, CreatedAt: b.CreatedAt, PeriodStart: before.PeriodStart, Fingerprint: b.Fingerprint}
		if inputs.Fingerprint() != b.Fingerprint {
			out.Gaps = append(out.Gaps, "Saved planning inputs differ from this agreement; unchanged dates do not establish unchanged scope or assumptions.")
		}
		out.Gaps = append(out.Gaps, fmt.Sprintf("Agreement period origin: %s. Current period origin: %s.", before.PeriodStart, now.PeriodStart))
		out.Facts = assistantChangeFacts(&before, now, inputs, oldInputs, req, p.ID)
	case "review":
		if req.SnapshotID == "" {
			http.Error(w, "Choose a snapshot or explicitly choose Manual review.", http.StatusBadRequest)
			return
		}
		snapshot := req.SnapshotID
		if snapshot == "manual" {
			snapshot = ""
		}
		review, _, code, msg := s.prepareWeeklyReview(r.Context(), p, c, weeklyReviewRequest{ReviewDate: req.ReviewDate, Timezone: req.Timezone, SnapshotID: snapshot, Filters: planning.ReviewFilters{Team: req.Team, Initiative: req.Initiative}})
		if code != 200 {
			http.Error(w, msg, code)
			return
		}
		out.Context.Review = &review.Context
		out.Context.EvidenceFingerprint = review.Fingerprint
		out.Summary = fmt.Sprintf("Review agenda: %d delivery exceptions, %d data gaps, %d open actions (%d overdue).", review.Counts.Delivery, review.Counts.Gaps, review.Counts.OpenActions, review.Counts.OverdueActions)
		if snapshot == "" {
			out.Gaps = append(out.Gaps, "Manual review: measured delivery progress remains unknown.")
		}
		out.Gaps = append(out.Gaps, review.Context.ComparisonLabel)
		source := assistantLink(p.ID, "execution", req.Initiative, req.Team, req.SnapshotID)
		source.URL += "&reviewDate=" + url.QueryEscape(req.ReviewDate) + "&reviewTimezone=" + url.QueryEscape(req.Timezone) + "&reviewInitiative=" + url.QueryEscape(req.Initiative)
		out.Sources = []assistantSource{source}
		for _, group := range []struct {
			kind    string
			entries []planning.ReviewEntry
		}{{"Observed evidence", review.Agenda.Delivery}, {"Unknown / evidence gap", review.Agenda.Gaps}} {
			for _, entry := range group.entries {
				title := entry.Initiative
				if title == "" {
					title = "Plan-wide context"
				}
				details := []string{entry.Reason}
				if entry.Team != "" {
					details = append(details, "Team: "+entry.Team)
				}
				out.Facts = append(out.Facts, assistantFact{Kind: group.kind, Title: title, Details: details, Source: source})
			}
		}
		for _, a := range review.Actions {
			scope := a.Initiative
			if scope == "" {
				scope = "Plan-wide action"
			}
			out.Facts = append(out.Facts, assistantFact{Kind: "Recorded action", Title: a.Action, Details: []string{scope, "Owner: " + a.Owner, "Status: " + a.Status, "Due for review: " + a.ReviewDate, a.Rationale}, Source: source})
		}
	}
	raw, _ := json.Marshal(out)
	hash := sha256.Sum256(raw)
	out.Fingerprint = hex.EncodeToString(hash[:])
	out.Context.PreparedAt = time.Now().Unix()
	writeJSON(w, out)
}
