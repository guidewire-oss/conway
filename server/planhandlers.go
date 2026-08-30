package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/game"
	"conway/server/planning"
)

const maxUpload = 20 << 20 // 20 MB

// handlePlans: GET lists the caller's plans (admins: all); POST creates one.
func (s *server) handlePlans(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "planning requires the database", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.ListPlans(c.Sub, c.Has("admin"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	case http.MethodPost:
		var body struct {
			Name         string  `json:"name"`
			HorizonWeeks float64 `json:"horizonWeeks"`
			CapacityLoss float64 `json:"capacityLoss"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.HorizonWeeks <= 0 {
			body.HorizonWeeks = 26 // half-year default
		}
		if body.CapacityLoss <= 0 {
			body.CapacityLoss = 0.10 // 10% default (PATCH can set exactly 0 later)
		}
		if strings.TrimSpace(body.Name) == "" {
			body.Name = "Untitled plan"
		}
		now := time.Now().Unix()
		p := db.PlanRow{ID: newID(), Owner: c.Sub, Name: body.Name,
			HorizonWeeks: body.HorizonWeeks, CapacityLoss: body.CapacityLoss, CreatedAt: now, UpdatedAt: now}
		if err := s.db.CreatePlan(p); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"id": p.ID, "name": p.Name})
	default:
		methodNotAllowed(w, r)
	}
}

// handleSampleInitiatives / handleSampleTeams serve downloadable filled sample
// files (same data as "Load demo plan") so managers can exercise the upload path.
// Public — sample templates carry no sensitive data.
func (s *server) handleSampleInitiatives(w http.ResponseWriter, r *http.Request) {
	teams, inits := planning.Demo()
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="conway-sample-initiatives.xlsx"`)
	w.Write(planning.WriteInitiativesXLSX(teams, inits))
}

func (s *server) handleSampleTeams(w http.ResponseWriter, r *http.Request) {
	teams, _ := planning.Demo()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="conway-sample-teams.csv"`)
	w.Write(planning.WriteTeamsCSV(teams))
}

// handlePlanDemo creates a fully-populated sample plan owned by the caller, so
// the network/constraints are visible immediately without an upload.
func (s *server) handlePlanDemo(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "planning requires the database", 503)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r)
		return
	}
	teams, inits := planning.Demo()
	now := time.Now().Unix()
	p := db.PlanRow{ID: newID(), Owner: c.Sub, Name: "Demo — Platform plan",
		HorizonWeeks: 26, CapacityLoss: 0.10, CreatedAt: now, UpdatedAt: now}
	if err := s.db.CreatePlan(p); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tb, _ := json.Marshal(teams)
	ib, _ := json.Marshal(inits)
	if err := s.db.SavePlanTeams(p.ID, tb, now); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.db.SavePlanInitiatives(p.ID, ib, now); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// The demo's dates are meaningless without a period to measure them against, so
	// the sample plan ships with the scheduling policy that makes its execution
	// order readable the first time anyone opens it.
	sb, _ := json.Marshal(planning.DemoScheduling())
	if err := s.db.SavePlanScheduling(p.ID, sb, now); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"id": p.ID})
}

// methodNotAllowed refuses a request in a way the reader can act on.
//
// The body matters more than it looks. app/ is served from disk with no-cache
// while routes are compiled into the binary, so checking out a branch that adds an
// endpoint updates the page immediately and leaves a running server that 405s on
// the new route. The old body for this was the single word "method"; the baselines
// panel surfaced it beside its Save button and it read as a control labelled
// "Method", which cost an afternoon to trace back to "restart the server".
func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, r.Method+" "+r.URL.Path+" is not a route this server has. "+
		"If the page offers it, the server binary is older than the page — rebuild and restart.",
		http.StatusMethodNotAllowed)
}

// handlePlanItem routes /api/plan/{id}[/teams|/initiatives].
func (s *server) handlePlanItem(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "planning requires the database", 503)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/plan/")
	id, sub, _ := strings.Cut(rest, "/")
	if id == "" {
		http.Error(w, "no plan id", 400)
		return
	}
	p, err := s.db.GetPlan(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if p == nil {
		http.Error(w, "plan not found", 404)
		return
	}
	if p.Owner != c.Sub && !c.Has("admin") {
		http.Error(w, "forbidden", 403)
		return
	}
	switch {
	case sub == "teams" && r.Method == http.MethodPost:
		s.uploadPlanTeams(w, r, p)
	case sub == "teams" && r.Method == http.MethodPatch:
		s.editPlanTeam(w, r, p)
	case sub == "roster" && r.Method == http.MethodPost:
		s.attachPlanRoster(w, r, p)
	case sub == "initiatives" && r.Method == http.MethodPost:
		s.uploadPlanInitiatives(w, r, p)
	case sub == "initiatives" && r.Method == http.MethodPatch:
		s.editPlanInitiatives(w, r, p)
	case sub == "scheduling" && r.Method == http.MethodPatch:
		s.savePlanScheduling(w, r, p)
	case sub == "initiatives/preview" && r.Method == http.MethodPost:
		s.previewPlanInitiatives(w, r, p)
	case sub == "simulate" && r.Method == http.MethodPost:
		s.simulatePlan(w, r, p)
	case sub == "schedule" && r.Method == http.MethodPost:
		s.schedulePlan(w, r, p)
	case sub == "schedule/remedies" && r.Method == http.MethodPost:
		s.remediesPlan(w, r, p)
	case sub == "baseline" && r.Method == http.MethodPost:
		s.saveBaseline(w, r, p, c)
	case sub == "baseline" && r.Method == http.MethodGet:
		s.listBaselines(w, p)
	case strings.HasPrefix(sub, "baseline/"):
		s.baselineItem(w, r, p, strings.TrimPrefix(sub, "baseline/"))
	case sub == "" && r.Method == http.MethodGet:
		writeJSON(w, s.assemblePlan(p))
	case sub == "" && r.Method == http.MethodPatch:
		s.patchPlan(w, r, p)
	case sub == "" && r.Method == http.MethodDelete:
		if err := s.db.DeletePlan(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		methodNotAllowed(w, r)
	}
}

func (s *server) uploadPlanTeams(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	data, err := readUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rows, err := planning.ReadGrid(data, "") // teams: first/only sheet
	if err != nil {
		http.Error(w, "could not read file: "+err.Error(), 400)
		return
	}
	teams, err := planning.ParseTeamsRows(rows)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(teams) == 0 {
		http.Error(w, "no teams found — expected a pod roster (Pod Name, Developers, …)", 400)
		return
	}
	b, _ := json.Marshal(teams)
	if err := s.db.SavePlanTeams(p.ID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"teams": len(teams)})
}

// netPodsToTeams converts a roster's pods to planning.Team rows — the fields
// capacity math needs (dev count, pairing, site, track override).
func netPodsToTeams(pods []NetPod) []planning.Team {
	teams := make([]planning.Team, len(pods))
	for i, p := range pods {
		teams[i] = planning.Team{Name: p.Name, Devs: p.DevCount, Pairs: p.Pairing, Tracks: p.Streams, Site: p.Location, CapacityLoss: p.CapacityLoss}
	}
	return teams
}

// attachPlanRoster (POST /api/plan/{id}/roster) sources the plan's team
// structure from a saved roster instead of a manual upload. Team composition
// drifts over time, so this freezes a copy at attach time — roster_id is kept
// only as a reference label, never live-joined, so later roster edits don't
// silently change an already-built plan.
func (s *server) attachPlanRoster(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		RosterID string `json:"rosterId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	rosterID := strings.TrimSpace(body.RosterID)
	if rosterID == "" {
		http.Error(w, "pick a roster", 400)
		return
	}
	pods, err := s.rosterPods(rosterID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(pods) == 0 {
		http.Error(w, "roster not found (or has no pods)", 400)
		return
	}
	teams := netPodsToTeams(pods)
	b, _ := json.Marshal(teams)
	if err := s.db.SetPlanRosterTeams(p.ID, rosterID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"teams": len(teams)})
}

// editPlanTeam updates one pod's capacity inputs (tracks override / pairing /
// site) in the roster, so a manager can correct headcount→reality in-app.
func (s *server) editPlanTeam(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		Name   string  `json:"name"`
		Tracks *int    `json:"tracks"` // 0 = clear override (auto from devs/pairs)
		Pairs  *bool   `json:"pairs"`
		Site   *string `json:"site"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "bad request", 400)
		return
	}
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	found := false
	for i := range teams {
		if teams[i].Name == body.Name {
			found = true
			if body.Tracks != nil {
				teams[i].Tracks = *body.Tracks
			}
			if body.Pairs != nil {
				teams[i].Pairs = *body.Pairs
			}
			if body.Site != nil {
				teams[i].Site = *body.Site
			}
		}
	}
	if !found {
		http.Error(w, "pod not in roster", 404)
		return
	}
	b, _ := json.Marshal(teams)
	if err := s.db.SavePlanTeams(p.ID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// teamNames extracts pod names for roster-membership checks (ParseMatrix's
// strict dependency filtering).
func teamNames(teams []planning.Team) []string {
	names := make([]string, len(teams))
	for i, t := range teams {
		names[i] = t.Name
	}
	return names
}

func (s *server) uploadPlanInitiatives(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	data, err := readUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	strict := r.FormValue("strict") == "1"
	rows, err := planning.ReadGrid(data, "FullKit exercise")
	if err != nil {
		http.Error(w, "could not read file: "+err.Error(), 400)
		return
	}
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	plan := planning.ParseMatrix(rows, teamNames(teams), strict)
	if len(plan.Initiatives) == 0 {
		http.Error(w, "no initiatives found — expected the FullKit matrix", 400)
		return
	}
	b, _ := json.Marshal(plan.Initiatives)
	if err := s.db.SavePlanInitiatives(p.ID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"initiatives": len(plan.Initiatives)})
}

// previewPlanInitiatives (POST /api/plan/{id}/initiatives/preview) parses an
// uploaded sheet and returns the network + simulation it would produce
// against the plan's already-saved roster — WITHOUT writing anything. A
// manager can see a still-in-progress sheet before deciding to keep it; the
// existing (saved) initiatives are untouched until an explicit save.
func (s *server) previewPlanInitiatives(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	data, err := readUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	strict := r.FormValue("strict") == "1"
	rows, err := planning.ReadGrid(data, "FullKit exercise")
	if err != nil {
		http.Error(w, "could not read file: "+err.Error(), 400)
		return
	}
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	parsed := planning.ParseMatrix(rows, teamNames(teams), strict)
	if len(parsed.Initiatives) == 0 {
		http.Error(w, "no initiatives found — expected the FullKit matrix", 400)
		return
	}
	net, unknowns := planNetwork(teams, parsed.Initiatives)
	before, after := planning.Simulate(teams, parsed.Initiatives,
		planning.Params{HorizonWeeks: p.HorizonWeeks, CapacityLoss: p.CapacityLoss}, nil)
	writeJSON(w, map[string]any{
		"initiatives": parsed.Initiatives, "network": net, "unknownTeams": unknowns,
		"sim": map[string]any{"before": before, "after": after, "levers": []planning.Lever{}},
	})
}

// planNetwork builds the directed dependency network for a team/initiative
// set and flags initiative teams missing from the roster — shared by
// assemblePlan (saved) and previewPlanInitiatives (unsaved draft).
func planNetwork(teams []planning.Team, inits []planning.Initiative) (*planning.Network, []string) {
	roster := map[string]bool{}
	names := make([]string, 0, len(teams))
	for _, t := range teams {
		roster[t.Name] = true
		names = append(names, t.Name)
	}
	net := planning.BuildNetwork(&planning.Plan{Teams: names, Initiatives: inits})
	unknown := map[string]bool{}
	for _, init := range inits {
		for team, wk := range init.Work {
			if wk.InPath && !roster[team] {
				unknown[team] = true
			}
		}
	}
	unknowns := make([]string, 0, len(unknown))
	for u := range unknown {
		unknowns = append(unknowns, u)
	}
	sort.Strings(unknowns)
	return net, unknowns
}

// simulatePlan runs a stateless what-if: returns the baseline and the levered
// result side by side. ρ is the primary signal; lead time is directional.
// Initiatives may be overridden by the caller (draft-preview mode, before an
// upload is saved) — when omitted, the plan's saved initiatives are used.
func (s *server) simulatePlan(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		Levers      []planning.Lever      `json:"levers"`
		Initiatives []planning.Initiative `json:"initiatives"` // optional draft override, never persisted
	}
	json.NewDecoder(r.Body).Decode(&body)
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	inits := body.Initiatives
	if inits == nil && len(p.Initiatives) > 0 {
		json.Unmarshal(p.Initiatives, &inits)
	}
	before, after := planning.Simulate(teams, inits,
		planning.Params{HorizonWeeks: p.HorizonWeeks, CapacityLoss: p.CapacityLoss}, body.Levers)
	writeJSON(w, map[string]any{"before": before, "after": after, "levers": body.Levers})
}

// planScheduling returns the plan's saved scheduling policy. An absent or
// unreadable blob yields the zero value, which schedules exactly as a plan with no
// policy at all does (FR-002).
func planScheduling(p *db.PlanRow) planning.SchedulingParams {
	var sp planning.SchedulingParams
	if len(p.Scheduling) == 0 {
		return sp
	}
	// The error has to be checked, not ignored: json.Unmarshal populates as it goes,
	// so a blob that fails half way through would otherwise leave a partial policy
	// in place — knobs the planner never set, applied as though they had.
	if err := json.Unmarshal(p.Scheduling, &sp); err != nil {
		return planning.SchedulingParams{}
	}
	return sp
}

// savePlanScheduling stores the plan-level scheduling policy (§8). Unlike
// /schedule, this one writes: it is the explicit save the planner asks for.
func (s *server) savePlanScheduling(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var sp planning.SchedulingParams
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the scheduling params: "+err.Error(), 400)
		return
	}
	if start := strings.TrimSpace(sp.PeriodStart); start != "" {
		if _, err := time.Parse("2006-01-02", start); err != nil {
			http.Error(w, "periodStart must be a date in YYYY-MM-DD form", 400)
			return
		}
	}
	// FR-018: a window with unusable dates or an unknown effect is refused
	// rather than stored — the engine silently ignores what it cannot honour,
	// and a saved-but-inert window is a constraint the planner believes in and
	// the schedule has never met. The stateless endpoints validate the same
	// way inside planScheduleFor.
	for _, win := range sp.Calendars {
		if msg := validateCalendarWindow(win); msg != "" {
			http.Error(w, msg, 400)
			return
		}
	}
	b, err := json.Marshal(sp)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.db.SavePlanScheduling(p.ID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "scheduling": sp})
}

// editPlanInitiatives edits the sequencing attributes of one or more initiatives
// in place (§8). It is the in-app half of §10 Q9's two entry points; the other is
// the uploaded sheet, which wins for the initiatives it names.
//
// A rejected value saves nothing at all, per AC 2.4.
func (s *server) editPlanInitiatives(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		Initiatives []planning.InitiativeEdit `json:"initiatives"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the initiative edits: "+err.Error(), 400)
		return
	}
	if len(body.Initiatives) == 0 {
		http.Error(w, "no initiative edits in the request", 400)
		return
	}
	var inits []planning.Initiative
	if len(p.Initiatives) > 0 {
		if err := json.Unmarshal(p.Initiatives, &inits); err != nil {
			http.Error(w, "the plan's stored initiatives are unreadable: "+err.Error(), 500)
			return
		}
	}
	var teams []planning.Team
	if len(p.Teams) > 0 {
		if err := json.Unmarshal(p.Teams, &teams); err != nil {
			http.Error(w, "the plan's stored teams are unreadable: "+err.Error(), 500)
			return
		}
	}
	edited, err := planning.ApplyInitiativeEdits(inits, body.Initiatives, planScheduling(p), p.HorizonWeeks)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Spec 008 Decision 3: lane pins are validated against the CURRENT
	// schedule's lane stacks — a drop onto lanes another slice holds in any
	// overlapping week is refused loudly, never silently re-packed.
	if msg := planning.ValidateLanePins(edited, body.Initiatives, planScheduling(p), p.HorizonWeeks, teams); msg != "" {
		http.Error(w, msg, http.StatusConflict) // the drop overlaps existing work
		return
	}
	b, err := json.Marshal(edited)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.db.SavePlanInitiatives(p.ID, b, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"initiatives": edited})
}

// schedulePlan computes the execution order for a plan (spec 001 §8). Stateless
// by design, exactly like simulatePlan: it never writes to the plan, so a planner
// can try scheduling params and levers against an unsaved draft and see the order
// before deciding to keep anything (FR-022).
func (s *server) schedulePlan(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body scheduleRequest
	// An empty body is legal — it means "schedule the saved plan with defaults" —
	// but a malformed one must not quietly become that same request.
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the schedule request: "+err.Error(), 400)
		return
	}
	// Anything after the first JSON value means the caller sent something other
	// than the one object this endpoint takes, and scheduling the part we happened
	// to parse would answer a question nobody asked. A second Decode requiring
	// EOF, not More(): More only looks inside the current composite, so
	// "{...}{...}" passes it silently.
	var extra map[string]any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "the schedule request must be a single JSON object", 400)
		return
	}
	// Shared with the baseline endpoints, so a baseline is always saved from exactly
	// the order the planner was looking at. Supplied params are a complete what-if
	// rather than a patch over the saved policy: merging would make "the same plan
	// with no WIP limit" impossible to ask for.
	inputs, err := s.planScheduleFor(p, body)
	if err != nil {
		if msg, bad := windowError(err); bad {
			http.Error(w, msg, 400)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, inputs.RecomputeWith(planning.ScheduleOptions{CompareWipModels: body.WipModels}))
}

// scheduleRequest is the body /schedule and the baseline endpoints share: the
// same inputs produce the same order, and a baseline has to be saved from exactly
// what the planner was looking at.
type scheduleRequest struct {
	Params      *planning.SchedulingParams `json:"params"`
	Initiatives []planning.Initiative      `json:"initiatives"`
	Levers      []planning.Lever           `json:"levers"`
	// WipModels asks for the per-model comparison (D22 as amended). Off by default:
	// it costs one extra full schedule per model and only the scheduling-assumptions
	// form reads it, so every other request was paying for three schedules nobody
	// looked at. The baseline endpoints ignore this and always store the comparison.
	WipModels bool `json:"wipModels"`
}

// remediesPlan prices the rescue options for missed dates (§8, Story 5). It
// shares /schedule's body so a remedy is priced against exactly the order the
// planner was looking at — a remedy computed from different inputs would answer
// a question nobody asked. Like /schedule it writes nothing: remedies are
// proposals until a planner accepts one (FR-022).
func (s *server) remediesPlan(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		scheduleRequest
		Targets []string `json:"targets"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the remedies request: "+err.Error(), 400)
		return
	}
	// Same single-object contract as /schedule: "{...}{...}" must not price the
	// first object and ignore the second.
	var extra map[string]any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "the remedies request must be a single JSON object", 400)
		return
	}
	inputs, err := s.planScheduleFor(p, body.scheduleRequest)
	if err != nil {
		if msg, bad := windowError(err); bad {
			http.Error(w, msg, 400)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	remedies := planning.ComputeRemedies(inputs.Teams, inputs.Initiatives, inputs.Params, inputs.Scheduling, body.Targets)
	// FR-015 names transfer-capacity among the minimum remedies, but Decision 7
	// defers it until §10 Q1's site-overlap factor is decided. Saying so in the
	// response keeps the gap visible instead of silently absent.
	writeJSON(w, map[string]any{
		"remedies": remedies,
		"warnings": []string{planning.TransferDeferredWarning},
	})
}

// planScheduleFor resolves a schedule request into the inputs ComputeSchedule
// takes, so /schedule and /baseline cannot drift in how they read the same body.
func (s *server) planScheduleFor(p *db.PlanRow, body scheduleRequest) (planning.BaselineInputs, error) {
	var teams []planning.Team
	if len(p.Teams) > 0 {
		if err := json.Unmarshal(p.Teams, &teams); err != nil {
			return planning.BaselineInputs{}, fmt.Errorf("the plan's stored roster is unreadable: %w", err)
		}
	}
	inits := body.Initiatives
	if inits == nil && len(p.Initiatives) > 0 {
		if err := json.Unmarshal(p.Initiatives, &inits); err != nil {
			return planning.BaselineInputs{}, fmt.Errorf("the plan's stored initiatives are unreadable: %w", err)
		}
	}
	if len(body.Levers) > 0 {
		teams, inits = planning.ApplyLevers(teams, inits, body.Levers)
	}
	sp := planScheduling(p)
	if body.Params != nil {
		sp = *body.Params
	}
	// FR-018: the stateless endpoints carry windows in their params, and those
	// deserve the same refusal the save path gives — a constraint the engine
	// would silently ignore is worse than a 400, because it looks applied.
	for _, win := range sp.Calendars {
		if msg := validateCalendarWindow(win); msg != "" {
			return planning.BaselineInputs{}, errBadWindow{msg}
		}
	}
	return planning.NewBaselineInputs(teams, inits,
		planning.Params{HorizonWeeks: p.HorizonWeeks, CapacityLoss: p.CapacityLoss}, sp), nil
}

// saveBaseline freezes the current order under a name (§8, Story 7). The inputs
// are stored with it, so the schedule can be reproduced rather than merely trusted
// (FR-029).
func (s *server) saveBaseline(w http.ResponseWriter, r *http.Request, p *db.PlanRow, c auth.Claims) {
	var body struct {
		scheduleRequest
		Name   string `json:"name"`
		Active *bool  `json:"active"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the baseline request: "+err.Error(), 400)
		return
	}
	// Same single-object contract as /schedule (shared body): "{...}{...}" must
	// not freeze the first object and ignore the second.
	var extra map[string]any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "the baseline request must be a single JSON object", 400)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, "a baseline needs a name — it is how a period's agreed order is referred to later", 400)
		return
	}
	inputs, err := s.planScheduleFor(p, body.scheduleRequest)
	if err != nil {
		if msg, bad := windowError(err); bad {
			http.Error(w, msg, 400)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	if len(inputs.Initiatives) == 0 {
		http.Error(w, "nothing to baseline — this plan has no initiatives", 400)
		return
	}

	sched := inputs.Recompute()
	inputsBlob, err := json.Marshal(inputs)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	schedBlob, err := json.Marshal(sched)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	row := db.BaselineRow{
		ID: newID(), PlanID: p.ID, Name: name,
		CreatedAt: time.Now().Unix(), CreatedBy: c.Sub,
		Fingerprint: inputs.Fingerprint(),
		Inputs:      inputsBlob, Schedule: schedBlob,
	}
	makeActive := body.Active == nil || *body.Active // default: the newest agreed order is the one that counts
	// The effective state, not the requested one: the first baseline a plan saves is
	// forced active whatever was asked, and a response that said otherwise would have
	// the client showing a chip that disagrees with the database.
	active, err := s.db.SaveBaseline(row, makeActive)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"id": row.ID, "name": row.Name, "active": active})
}

// listBaselines returns the plan's baselines with their active flag and whether
// the plan's inputs have moved since each was saved (FR-030).
func (s *server) listBaselines(w http.ResponseWriter, p *db.PlanRow) {
	rows, err := s.db.ListBaselines(p.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// A plan whose inputs cannot be read has no fingerprint to compare, and saying
	// "matches" would be a reassurance nothing supports. Fail instead.
	inputs, err := s.planScheduleFor(p, scheduleRequest{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	current := inputs.Fingerprint()
	out := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		out = append(out, map[string]any{
			"id": b.ID, "name": b.Name, "active": b.Active,
			"createdAt": b.CreatedAt, "createdBy": b.CreatedBy,
			"comparedTo": b.ComparedTo,
			// Diverged when the plan's inputs no longer hash to what was saved.
			"diverged": current != b.Fingerprint,
		})
	}
	writeJSON(w, map[string]any{"baselines": out})
}

// baselineItem routes /baseline/{bid}, /baseline/{bid}/compare and
// /baseline/{bid}/compare-to/{other}.
func (s *server) baselineItem(w http.ResponseWriter, r *http.Request, p *db.PlanRow, rest string) {
	bid, action, _ := strings.Cut(rest, "/")
	if bid == "" {
		http.Error(w, "no baseline id", 400)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		s.getBaseline(w, p, bid)
	case action == "" && r.Method == http.MethodPatch:
		s.patchBaseline(w, r, p, bid)
	case action == "compare" && r.Method == http.MethodPost:
		s.compareBaseline(w, r, p, bid)
	case strings.HasPrefix(action, "compare-to/") && r.Method == http.MethodPost:
		s.compareBaselines(w, r, p, bid, strings.TrimPrefix(action, "compare-to/"))
	default:
		methodNotAllowed(w, r)
	}
}

// compareBaselines compares two STORED baselines (spec 005): the delta between
// the agreed order and a later proposed one, without touching the live plan.
// Both schedules come from the baseline rows; CompareToBaseline is the same
// engine the live compare uses, so both flavours report identical shapes.
func (s *server) compareBaselines(w http.ResponseWriter, _ *http.Request, p *db.PlanRow, bid, other string) {
	if other == "" || other == bid {
		http.Error(w, "compare-to needs a different baseline id", 400)
		return
	}
	from, err := s.db.GetBaseline(p.ID, bid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	to, err := s.db.GetBaseline(p.ID, other)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if from == nil || to == nil {
		http.Error(w, "baseline not found", 404)
		return
	}
	var was, now planning.Schedule
	if len(from.Schedule) > 0 {
		if err := json.Unmarshal(from.Schedule, &was); err != nil {
			http.Error(w, "a stored schedule is unreadable: "+err.Error(), 500)
			return
		}
	}
	if len(to.Schedule) > 0 {
		if err := json.Unmarshal(to.Schedule, &now); err != nil {
			http.Error(w, "a stored schedule is unreadable: "+err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]any{
		"from":       map[string]any{"id": from.ID, "name": from.Name, "createdAt": from.CreatedAt},
		"to":         map[string]any{"id": to.ID, "name": to.Name, "createdAt": to.CreatedAt},
		"comparison": planning.CompareToBaseline(&was, &now),
	})
}

func (s *server) getBaseline(w http.ResponseWriter, p *db.PlanRow, bid string) {
	b, err := s.db.GetBaseline(p.ID, bid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if b == nil {
		http.Error(w, "baseline not found", 404)
		return
	}
	var sched planning.Schedule
	if len(b.Schedule) > 0 {
		if err := json.Unmarshal(b.Schedule, &sched); err != nil {
			http.Error(w, "the stored schedule is unreadable: "+err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]any{
		"id": b.ID, "name": b.Name, "active": b.Active,
		"createdAt": b.CreatedAt, "createdBy": b.CreatedBy,
		"comparedTo": b.ComparedTo, "schedule": sched,
	})
}

// patchBaseline marks a baseline active or renames it. Nothing else about a saved
// baseline can change (FR-030).
func (s *server) patchBaseline(w http.ResponseWriter, r *http.Request, p *db.PlanRow, bid string) {
	var body struct {
		Active *bool   `json:"active"`
		Name   *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the request: "+err.Error(), 400)
		return
	}
	if body.Active == nil && body.Name == nil {
		http.Error(w, "nothing to change — send active or name", 400)
		return
	}
	// "Exactly one active" (FR-031) means deactivation is not an operation: a plan
	// with baselines and none active has no answer for what actuals measure against.
	// Reporting that as success would be the API agreeing to something it did not do.
	if body.Active != nil && !*body.Active {
		http.Error(w, "a baseline cannot be deactivated — make a different one active instead", 400)
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			http.Error(w, "a baseline needs a name", 400)
			return
		}
		ok, err := s.db.RenameBaseline(p.ID, bid, name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if !ok {
			http.Error(w, "baseline not found", 404)
			return
		}
	}
	if body.Active != nil {
		ok, err := s.db.SetBaselineActive(p.ID, bid)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if !ok {
			http.Error(w, "baseline not found", 404)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

// compareBaseline measures a freshly computed order against a saved one (FR-032).
func (s *server) compareBaseline(w http.ResponseWriter, r *http.Request, p *db.PlanRow, bid string) {
	var body scheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "could not read the compare request: "+err.Error(), 400)
		return
	}
	b, err := s.db.GetBaseline(p.ID, bid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if b == nil {
		http.Error(w, "baseline not found", 404)
		return
	}
	var was planning.Schedule
	if len(b.Schedule) > 0 {
		if err := json.Unmarshal(b.Schedule, &was); err != nil {
			http.Error(w, "the stored schedule is unreadable: "+err.Error(), 500)
			return
		}
	}
	inputs, err := s.planScheduleFor(p, body)
	if err != nil {
		if msg, bad := windowError(err); bad {
			http.Error(w, msg, 400)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	now := inputs.Recompute()
	writeJSON(w, map[string]any{
		"baseline":   map[string]any{"id": b.ID, "name": b.Name, "createdAt": b.CreatedAt},
		"diverged":   inputs.Fingerprint() != b.Fingerprint,
		"comparison": planning.CompareToBaseline(&was, now),
	})
}

func (s *server) patchPlan(w http.ResponseWriter, r *http.Request, p *db.PlanRow) {
	var body struct {
		Name         *string  `json:"name"`
		HorizonWeeks *float64 `json:"horizonWeeks"`
		CapacityLoss *float64 `json:"capacityLoss"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	name, hz, loss := p.Name, p.HorizonWeeks, p.CapacityLoss
	if body.Name != nil {
		name = *body.Name
	}
	if body.HorizonWeeks != nil && *body.HorizonWeeks > 0 {
		hz = *body.HorizonWeeks
	}
	if body.CapacityLoss != nil && *body.CapacityLoss >= 0 && *body.CapacityLoss < 1 {
		loss = *body.CapacityLoss // pointer lets a manager set exactly 0% loss
	}
	if err := s.db.UpdatePlanMeta(p.ID, name, hz, loss, time.Now().Unix()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// assemblePlan returns the full plan plus the derived directed network and
// per-pod utilization, and flags initiative teams missing from the roster.
func (s *server) assemblePlan(p *db.PlanRow) map[string]any {
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	var inits []planning.Initiative
	if len(p.Initiatives) > 0 {
		json.Unmarshal(p.Initiatives, &inits)
	}
	net, unknowns := planNetwork(teams, inits)
	loads := planning.Utilization(&planning.Plan{Initiatives: inits}, teams,
		planning.Params{HorizonWeeks: p.HorizonWeeks, CapacityLoss: p.CapacityLoss})
	return map[string]any{
		"id": p.ID, "owner": p.Owner, "name": p.Name,
		"horizonWeeks": p.HorizonWeeks, "capacityLoss": p.CapacityLoss,
		"scheduling": planScheduling(p),
		"teams":      teams, "initiatives": inits, "rosterId": p.RosterID,
		"network": net, "loads": loads, "unknownTeams": unknowns,
		"createdAt": p.CreatedAt, "updatedAt": p.UpdatedAt,
	}
}

// planWorld builds a game world from a saved plan so a game can be seeded on
// real plan data: teams → pods (tracks → streams), per-pod demand → starting
// load, directed dependencies → edges. Closes the Observe → Plan → Train loop.
func (s *server) planWorld(planID string) (*World, error) {
	if s.db == nil {
		return nil, fmt.Errorf("no database")
	}
	p, err := s.db.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("plan not found")
	}
	var teams []planning.Team
	if len(p.Teams) > 0 {
		json.Unmarshal(p.Teams, &teams)
	}
	var inits []planning.Initiative
	if len(p.Initiatives) > 0 {
		json.Unmarshal(p.Initiatives, &inits)
	}
	if len(teams) == 0 {
		return nil, fmt.Errorf("plan has no teams")
	}
	demand := map[string]float64{}
	for _, it := range inits {
		for tm, wk := range it.Work {
			if wk.InPath {
				demand[tm] += wk.Weeks
			}
		}
	}
	w := &World{Stats: map[string]game.PodStat{}, Overlap: map[string]map[string]float64{}, SrePods: map[string]bool{}}
	for _, t := range teams {
		tracks := float64(t.EffectiveTracks())
		if tracks < 1 {
			tracks = 1
		}
		w.Pods = append(w.Pods, game.PodInfo{Name: t.Name, Location: t.Site, Pairing: t.Pairs, Streams: tracks, DevCount: float64(t.Devs)})
		w.Stats[t.Name] = game.PodStat{Wip: demand[t.Name], ThroughputWk: tracks, Mu: 2, Sigma: 0.6, HygieneScore: 0.5}
	}
	// overlap: same-site pods coordinate cheaply; cross-site is the slow seam
	for _, a := range teams {
		w.Overlap[a.Name] = map[string]float64{}
		for _, b := range teams {
			switch {
			case a.Name == b.Name:
				w.Overlap[a.Name][b.Name] = 8
			case a.Site != "" && a.Site == b.Site:
				w.Overlap[a.Name][b.Name] = 6
			default:
				w.Overlap[a.Name][b.Name] = 2
			}
		}
	}
	for _, e := range planning.BuildNetwork(&planning.Plan{Initiatives: inits}).Edges {
		w.Edges = append(w.Edges, &game.Edge{From: e.From, To: e.To, Count: e.Count})
	}
	return w, nil
}

// readUpload reads an uploaded file, bounded at maxUpload. The body is wrapped
// in a MaxBytesReader first: ParseMultipartForm's argument caps only what is
// buffered in memory, spilling the rest to disk, so on its own it does not stop
// an oversized body. Passing w lets the reader close the connection on overrun.
func readUpload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		// G120 no longer applies: r.Body is wrapped in a MaxBytesReader above, so
		// reads past maxUpload fail and parsing cannot consume an unbounded body.
		// gosec matches the call pattern and cannot see the wrapper.
		if err := r.ParseMultipartForm(maxUpload); err != nil { //nolint:gosec // G120: body bounded by MaxBytesReader
			return nil, err
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return readAll(f, maxUpload)
	}
	return readAll(r.Body, maxUpload)
}

// validateCalendarWindow checks one FR-018 window: parseable dates, a sane
// span, and an effect and kind the engine actually honours. The message names
// the field that was wrong, because "invalid window" is not actionable.
func validateCalendarWindow(win planning.CalendarWindow) string {
	const iso = "2006-01-02"
	from, errF := time.Parse(iso, strings.TrimSpace(win.From))
	to, errT := time.Parse(iso, strings.TrimSpace(win.To))
	if errF != nil || errT != nil {
		return "calendar window fromDate and toDate must be dates in YYYY-MM-DD form"
	}
	if to.Before(from) {
		return "calendar window toDate must not precede fromDate"
	}
	switch win.Effect {
	case planning.EffectReduceCapacity, planning.EffectBlockStart, planning.EffectBlockFinish:
	default:
		return "calendar window effect must be one of reduce-capacity, block-start, block-finish"
	}
	switch win.Kind {
	case planning.CalSiteNonWorking, planning.CalChangeFreeze, planning.CalEvent:
	default:
		return "calendar window kind must be one of site-nonworking, change-freeze, event"
	}
	return ""
}

// errBadWindow marks a calendar-window validation failure so handlers can
// answer 400 rather than 500 — the caller sent a constraint the engine will
// not honour, and that is their error, not the server's.
type errBadWindow struct{ msg string }

func (e errBadWindow) Error() string { return e.msg }

// windowError unwraps a validation failure, reporting whether it is the
// caller's (400) or the server's (500).
func windowError(err error) (string, bool) {
	var bad errBadWindow
	if errors.As(err, &bad) {
		return bad.msg, true
	}
	return "", false
}
