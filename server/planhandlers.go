package main

import (
	"encoding/json"
	"fmt"
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
		http.Error(w, "method", 405)
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
		http.Error(w, "method", 405)
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
	writeJSON(w, map[string]any{"id": p.ID})
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
	case sub == "initiatives/preview" && r.Method == http.MethodPost:
		s.previewPlanInitiatives(w, r, p)
	case sub == "simulate" && r.Method == http.MethodPost:
		s.simulatePlan(w, r, p)
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
		http.Error(w, "method", 405)
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
	teams := planning.ParseTeamsRows(rows)
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
		teams[i] = planning.Team{Name: p.Name, Devs: p.DevCount, Pairs: p.Pairing, Tracks: p.Streams, Site: p.Location}
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
		"teams": teams, "initiatives": inits, "rosterId": p.RosterID,
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
