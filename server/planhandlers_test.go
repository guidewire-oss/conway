package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"conway/db"
	"conway/planning"
)

// A plan's team structure can be sourced from a saved roster (netPodsToTeams)
// instead of a manual CSV/XLSX upload — see attachPlanRoster.
func TestNetPodsToTeams(t *testing.T) {
	pods := []NetPod{
		{Name: "Alpha", Location: "Bengaluru", Pairing: true, DevCount: 6, Streams: 3},
		{Name: "Beta", Location: "Remote", Pairing: false, DevCount: 4},
	}
	teams := netPodsToTeams(pods)
	if len(teams) != 2 {
		t.Fatalf("len(teams) = %d, want 2", len(teams))
	}
	a := teams[0]
	if a.Name != "Alpha" || a.Site != "Bengaluru" || !a.Pairs || a.Devs != 6 || a.Tracks != 3 {
		t.Fatalf("Alpha team = %+v, want {Alpha Bengaluru pairs=true devs=6 tracks=3}", a)
	}
	b := teams[1]
	if b.Name != "Beta" || b.Site != "Remote" || b.Pairs || b.Devs != 4 || b.Tracks != 0 {
		t.Fatalf("Beta team = %+v, want {Beta Remote pairs=false devs=4 tracks=0}", b)
	}
}

func TestAttachPlanRosterRequiresRosterID(t *testing.T) {
	s := &server{db: &db.DB{}}
	p := &db.PlanRow{ID: "plan1"}
	req := httptest.NewRequest("POST", "/api/plan/plan1/roster", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.attachPlanRoster(rec, req, p)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "roster") {
		t.Fatalf("error message = %q, want it to mention the missing roster", rec.Body.String())
	}
}

func multipartFile(t *testing.T, field, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return body, w.FormDataContentType()
}

// Uploading an initiatives sheet for preview must return the network/sim a
// save would produce, but leave the plan row's Initiatives untouched — a
// manager should be able to see the result of a still-in-progress sheet
// before deciding to keep it.
func TestPreviewPlanInitiativesDoesNotSave(t *testing.T) {
	teams, inits := planning.Demo()
	xlsx := planning.WriteInitiativesXLSX(teams, inits)
	body, ct := multipartFile(t, "file", "initiatives.xlsx", xlsx)

	s := &server{}
	p := &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1} // no saved teams/initiatives
	req := httptest.NewRequest("POST", "/api/plan/plan1/initiatives/preview", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	s.previewPlanInitiatives(rec, req, p)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Initiatives []planning.Initiative `json:"initiatives"`
		Network     *planning.Network     `json:"network"`
		Sim         struct {
			Before planning.SimResult `json:"before"`
		} `json:"sim"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response JSON: %v", err)
	}
	if len(resp.Initiatives) != len(inits) {
		t.Fatalf("initiatives = %d, want %d", len(resp.Initiatives), len(inits))
	}
	if resp.Network == nil || len(resp.Network.Nodes) == 0 {
		t.Fatal("expected a non-empty network in the preview response")
	}
	if resp.Sim.Before.Total == 0 {
		t.Fatal("expected a non-empty sim result in the preview response")
	}
	// the plan row passed in must not have been mutated/saved
	if p.Initiatives != nil {
		t.Fatal("previewPlanInitiatives must not persist — PlanRow.Initiatives should stay nil")
	}
}

// simulatePlan must use a client-supplied initiatives override when present
// (draft preview mode) instead of the plan's saved initiatives — without
// this, editing levers on an unsaved draft would silently simulate the OLD
// saved sheet instead of the one on screen.
func TestSimulatePlanUsesInitiativesOverride(t *testing.T) {
	savedInits := []planning.Initiative{{Name: "Saved one", Work: map[string]planning.TeamWork{
		"Alpha": {Weeks: 4, Estimated: true, InPath: true},
	}}}
	savedB, _ := json.Marshal(savedInits)
	teams := []planning.Team{{Name: "Alpha", Devs: 4}}
	teamsB, _ := json.Marshal(teams)
	p := &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1, Teams: teamsB, Initiatives: savedB}

	draftInits := []planning.Initiative{{Name: "Draft one", Work: map[string]planning.TeamWork{
		"Alpha": {Weeks: 9, Estimated: true, InPath: true},
	}}}
	reqBody, _ := json.Marshal(map[string]any{"levers": []planning.Lever{}, "initiatives": draftInits})

	s := &server{}
	req := httptest.NewRequest("POST", "/api/plan/plan1/simulate", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	s.simulatePlan(rec, req, p)

	var resp struct {
		Before planning.SimResult `json:"before"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response JSON: %v", err)
	}
	if len(resp.Before.Initiatives) != 1 || resp.Before.Initiatives[0].Name != "Draft one" {
		t.Fatalf("before.initiatives = %+v, want the draft override ([Draft one]), not the saved sheet", resp.Before.Initiatives)
	}
}
