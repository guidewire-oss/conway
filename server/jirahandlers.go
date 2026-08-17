package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/jira"
	"conway/server/planning"
)

// jiraCreds is the use-and-discard credential set for an import. Never persisted.
type jiraCreds struct {
	BaseURL  string `json:"baseUrl"`
	Email    string `json:"email"`
	Token    string `json:"token"`
	PodField string `json:"podField"`
}

func (cr jiraCreds) valid() bool {
	return strings.TrimSpace(cr.BaseURL) != "" && strings.TrimSpace(cr.Email) != "" && strings.TrimSpace(cr.Token) != ""
}

// handleJiraProjects (POST) lists the projects the caller can see, for the
// import picker — via their OAuth session if connected, else pasted credentials.
func (s *server) handleJiraProjects(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var cr jiraCreds
	json.NewDecoder(r.Body).Decode(&cr)
	cl, err := s.jiraClientFor(c.Sub, cr)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	projects, err := cl.ListProjects(ctx)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, projects)
}

// importJob tracks an in-flight Jira import (the fetch can take minutes — far
// longer than the gateway's request timeout — so it runs in the background and
// the client polls import-status).
type importJob struct {
	Status   string `json:"status"` // running | done | error
	Phase    string `json:"phase"`  // human-readable current step
	Issues   int    `json:"issues"`
	Snapshot string `json:"snapshot"` // set when done
	Name     string `json:"name"`
	Err      string `json:"error"`
}

func (s *server) updateJob(id string, f func(*importJob)) {
	s.importMu.Lock()
	if j := s.importJobs[id]; j != nil {
		f(j)
	}
	s.importMu.Unlock()
}

// handleSnapshotImport (POST /api/snapshots/import) kicks off a background fetch
// of the selected projects and returns a job id immediately; the client polls
// /api/snapshots/import-status/{id}. The token is used here and never stored.
func (s *server) handleSnapshotImport(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if s.db == nil {
		http.Error(w, "import requires the database", 503)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var b struct {
		jiraCreds
		Name     string   `json:"name"`
		Projects []string `json:"projects"`
		RosterID string   `json:"rosterId"` // mandatory: the dated roster this snapshot is pinned to
		WipMode  string   `json:"wipMode"`  // leaf | epic_or_parentless, default leaf
	}
	json.NewDecoder(r.Body).Decode(&b)
	name := strings.TrimSpace(b.Name)
	if name == "" {
		http.Error(w, "name the snapshot", 400)
		return
	}
	if len(b.Projects) == 0 {
		http.Error(w, "pick at least one project", 400)
		return
	}
	if strings.TrimSpace(b.RosterID) == "" {
		http.Error(w, "pick a roster — team composition changes over time, so a JIRA snapshot needs a dated roster to plan against", 400)
		return
	}
	wipMode := jira.WipModeLeaf
	if b.WipMode == jira.WipModeEpicOrParentless {
		wipMode = jira.WipModeEpicOrParentless
	}
	cl, err := s.jiraClientFor(c.Sub, b.jiraCreds) // validate creds up front
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	roster, rerr := s.rosterPods(b.RosterID)
	if rerr != nil || roster == nil {
		http.Error(w, "roster not found", 400)
		return
	}

	jobID := newID()
	s.importMu.Lock()
	s.importJobs[jobID] = &importJob{Status: "running", Phase: "Contacting Jira…", Name: name}
	s.importMu.Unlock()

	owner, projects, rosterID := c.Sub, b.Projects, b.RosterID
	go s.runImport(jobID, cl, owner, name, projects, rosterID, wipMode, roster)
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"jobId": jobID})
}

// runImport performs the fetch + aggregation + persistence off the request path.
func (s *server) runImport(jobID string, cl *jira.Client, owner, name string, projects []string, rosterID, wipMode string, roster []NetPod) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	fail := func(msg string) { s.updateJob(jobID, func(j *importJob) { j.Status, j.Err = "error", msg }) }

	s.updateJob(jobID, func(j *importJob) { j.Phase = "Fetching issues from Jira…" })
	detailed, err := cl.SearchDetailed(ctx, projectJQL(projects), "", func(n int) {
		s.updateJob(jobID, func(j *importJob) { j.Issues = n })
	})
	if err != nil {
		fail("jira fetch: " + err.Error())
		return
	}
	s.updateJob(jobID, func(j *importJob) { j.Phase = "Building the network…" })
	pods, err := s.importStructure(roster)
	if err != nil {
		fail(err.Error())
		return
	}
	s.updateJob(jobID, func(j *importJob) { j.Phase = "Storing issues…" })
	basic := make([]jira.Issue, len(detailed))
	for i, d := range detailed {
		basic[i] = d.Basic()
	}
	stats, edges := jira.Aggregate(basic, wipMode)
	data := snapshotDataFromJira(pods, detailed, stats, edges)
	id := newID()
	scope, _ := json.Marshal(projects)
	// one transaction: snapshot row + all issue/pod/stat/edge/hygiene rows. A
	// crash mid-import commits nothing — no half-written snapshot.
	if err := s.db.CreateSnapshotWithData(db.SnapshotRow{
		ID: id, Owner: owner, Name: name, Scope: scope, Source: "jira", RosterID: rosterID, WipMode: wipMode, CreatedAt: time.Now().Unix(),
	}, data); err != nil {
		fail(err.Error())
		return
	}
	s.updateJob(jobID, func(j *importJob) { j.Status, j.Snapshot = "done", id })
}

// snapshotDataFromJira builds the relational rows for a snapshot: issues + links
// from the detailed fetch, pod structure from the roster, and the materialized
// aggregates (stats/edges/hygiene).
func snapshotDataFromJira(pods []NetPod, detailed []jira.DetailedIssue, stats map[string]jira.PodStat, edges []jira.Edge) db.SnapshotData {
	tptr := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	var data db.SnapshotData
	for _, it := range detailed {
		data.Issues = append(data.Issues, db.IssueRow{
			Key: it.Key, Pod: it.Pod, IssueType: it.IssueType, Status: it.StatusName, StatusCat: it.StatusCat,
			Assignee: it.Assignee, Points: it.Points, Summary: it.Summary, DescLen: it.DescLen, ParentKey: it.ParentKey,
			Created: tptr(it.Created), Updated: tptr(it.Updated), Resolved: it.Resolved, DueDate: it.DueDate,
		})
		for _, k := range it.Blocks {
			data.Links = append(data.Links, [2]string{it.Key, k})
		}
		for _, k := range it.BlockedBy {
			data.Links = append(data.Links, [2]string{k, it.Key})
		}
	}
	for _, p := range pods {
		data.Pods = append(data.Pods, db.PodRow{Name: p.Name, Location: p.Location, Pairing: p.Pairing, DevCount: p.DevCount, Streams: p.Streams})
	}
	for pod, st := range stats {
		data.Stats = append(data.Stats, db.PodStatRow{
			Pod: pod, Resolved180d: st.ResolvedCount180d, P50: st.CycleTimeDays.P50, P85: st.CycleTimeDays.P85,
			Mean: st.CycleTimeDays.Mean, Mu: st.Lognormal.Mu, Sigma: st.Lognormal.Sigma,
			WipCount: st.WipCount, ThroughputPerWk: st.ThroughputPerWeek,
		})
	}
	for _, e := range edges {
		data.Edges = append(data.Edges, db.EdgeRow{From: e.From, To: e.To, Count: e.Count})
	}
	for _, h := range jira.HygieneStats(detailed, time.Now()) {
		data.Hygiene = append(data.Hygiene, db.PodHygieneRow{
			Pod: h.Pod, SizedPct: h.SizedPct, MedianPoints: h.MedianPoints, StaleWipPct: h.StaleWipPct,
			UnassignedWipPct: h.UnassignedWipPct, LinkDensity: h.LinkDensity, Score: h.Score,
			SampleSized: h.SampleSized, WipCount: h.WipCount,
		})
	}
	return data
}

// handleImportStatus (GET /api/snapshots/import-status/{id}) reports job progress.
func (s *server) handleImportStatus(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	id := strings.TrimPrefix(r.URL.Path, "/api/snapshots/import-status/")
	s.importMu.Lock()
	j := s.importJobs[id]
	s.importMu.Unlock()
	if j == nil {
		http.Error(w, "no such import", 404)
		return
	}
	writeJSON(w, j)
}

// projectJQL scopes a search to the selected project keys plus the same window
// the offline pipeline uses: resolved in the last 180 days, or still open.
func projectJQL(projects []string) string {
	quoted := make([]string, 0, len(projects))
	for _, p := range projects {
		p = strings.TrimSpace(p)
		if p != "" {
			quoted = append(quoted, `"`+strings.ReplaceAll(p, `"`, "")+`"`)
		}
	}
	return fmt.Sprintf("project in (%s) AND (resolutiondate >= -180d OR resolution IS EMPTY)",
		strings.Join(quoted, ","))
}

// podsAndOverlapDoc builds the pods.json document (pods + a site-derived overlap
// matrix) — shared by import and roster re-association.
func podsAndOverlapDoc(pods []NetPod) []byte {
	overlap := map[string]map[string]float64{}
	for _, a := range pods {
		overlap[a.Name] = map[string]float64{}
		for _, b := range pods {
			switch {
			case a.Name == b.Name:
				overlap[a.Name][b.Name] = 8
			case a.Location != "" && a.Location == b.Location:
				overlap[a.Name][b.Name] = 6
			default:
				overlap[a.Name][b.Name] = 2
			}
		}
	}
	doc, _ := json.Marshal(map[string]any{"pods": pods, "overlap": overlap})
	return doc
}

// handleParseRoster (POST /api/parse-roster) parses an uploaded pod-directory
// CSV/XLSX into pods (name/location/pairing/dev-count) the import can join with
// Jira by pod name. Reuses the robust planning parser (handles quoted lists).
func (s *server) handleParseRoster(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	data, err := readUpload(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rows, err := planning.ReadGrid(data, "")
	if err != nil {
		http.Error(w, "could not read file: "+err.Error(), 400)
		return
	}
	pods := make([]NetPod, 0)
	for _, t := range planning.ParseTeamsRows(rows) {
		pods = append(pods, NetPod{Name: t.Name, Location: t.Site, Pairing: t.Pairs, DevCount: t.Devs, Streams: t.Tracks})
	}
	if len(pods) == 0 {
		http.Error(w, "no teams found — needs a header row with Pod Name, Developers, Location, Pairs", 400)
		return
	}
	writeJSON(w, map[string]any{"teams": pods, "count": len(pods)})
}

// importStructure validates the roster an import is pinned to. A roster is
// mandatory (not a Plan, not a Jira-derived guess): team composition drifts
// over time, so a JIRA snapshot must carry the dated roster it was imported
// against for planning to stay accurate.
func (s *server) importStructure(roster []NetPod) ([]NetPod, error) {
	if len(roster) == 0 {
		return nil, fmt.Errorf("the selected roster has no pods")
	}
	for i := range roster {
		if roster[i].DevCount < 0 {
			roster[i].DevCount = 0
		}
	}
	return roster, nil
}
