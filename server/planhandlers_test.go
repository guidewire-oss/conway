package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"conway/server/db"
	"conway/server/planning"
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

// POST /api/plan/{id}/schedule — spec 001 §8. Stateless like simulate, so an
// unsaved draft can be sequenced before anything is kept (FR-022).
var _ = Describe("schedulePlan", func() {
	var (
		srv  *server
		plan *db.PlanRow
	)

	BeforeEach(func() {
		teams, inits := planning.Demo()
		teamsB, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		initsB, err := json.Marshal(inits)
		Expect(err).NotTo(HaveOccurred())
		srv = &server{}
		plan = &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1, Teams: teamsB, Initiatives: initsB}
	})

	post := func(body string) *planning.Schedule {
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.schedulePlan(rec, req, plan)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var sched planning.Schedule
		Expect(json.Unmarshal(rec.Body.Bytes(), &sched)).To(Succeed())
		return &sched
	}

	It("returns the §7 Schedule shape for the demo plan", func() {
		sched := post(`{"params":{"periodStart":"2026-01-05"}}`)

		_, demoInits := planning.Demo()
		Expect(sched.Initiatives).To(HaveLen(len(demoInits)))
		Expect(sched.PodWeeks).NotTo(BeEmpty())
		Expect(sched.DrumPods).To(ContainElement("Delta"))
		Expect(sched.HorizonWeeks).To(Equal(26))
		Expect(sched.Rule).NotTo(BeEmpty())
		Expect(sched.RulesTried).To(HaveLen(5), "Decision 6 runs every rule and keeps the best")

		By("giving every initiative a rank, a span and a commit week (FR-003)")
		ranks := map[int]bool{}
		for _, si := range sched.Initiatives {
			Expect(si.Name).NotTo(BeEmpty())
			Expect(si.ProposedRank).To(BeNumerically(">", 0))
			Expect(ranks).NotTo(HaveKey(si.ProposedRank), "ranks must be unique")
			ranks[si.ProposedRank] = true
			Expect(si.CommitWeek).To(BeNumerically(">=", si.RawFinishWeek))
			Expect(si.RawFinishWeek).To(BeNumerically(">", si.StartWeek))
			Expect(si.Verdict).NotTo(BeEmpty())
			Expect(si.Slices).NotTo(BeEmpty())
		}

		By("labelling the WIP limit as derived, with the pod it came from (Decision 22)")
		Expect(sched.WipLimit.Derived).To(BeTrue())
		Expect(sched.WipLimit.FromPod).To(Equal("Delta"))

		By("reporting per-pod weekly load for the heatmap (FR-004)")
		for _, ps := range sched.PodWeeks {
			Expect(ps.Weeks).NotTo(BeEmpty())
			for _, wk := range ps.Weeks {
				Expect(wk.Busy).To(BeNumerically("<=", ps.Tracks),
					"pod %s week %d ran %d slices on %d tracks", ps.Pod, wk.Week, wk.Busy, ps.Tracks)
			}
		}
	})

	It("honours an explicit WIP limit from the request", func() {
		sched := post(`{"params":{"periodStart":"2026-01-05","maxConcurrentInitiatives":3}}`)
		Expect(sched.WipLimit).To(Equal(planning.WipLimit{Value: 3}))
	})

	It("sequences a draft override instead of the saved sheet", func() {
		sched := post(`{"params":{"periodStart":"2026-01-05"},
			"initiatives":[{"name":"Draft only","work":{"Atlas":{"weeks":4,"estimated":true,"inPath":true}}}]}`)
		Expect(sched.Initiatives).To(HaveLen(1))
		Expect(sched.Initiatives[0].Name).To(Equal("Draft only"))
	})

	It("applies levers before ordering, and still writes nothing to the plan", func() {
		before := post(`{"params":{"periodStart":"2026-01-05","maxConcurrentInitiatives":4}}`)
		after := post(`{"params":{"periodStart":"2026-01-05","maxConcurrentInitiatives":4},
			"levers":[{"type":"addCapacity","pod":"Delta","n":4}]}`)

		Expect(deltaTracks(after)).To(BeNumerically(">", deltaTracks(before)),
			"the lever should widen the drum for this computation only")
		_, demoInits := planning.Demo()
		savedB, err := json.Marshal(demoInits)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan.Initiatives).To(MatchJSON(savedB), "schedulePlan must never persist")
	})

	It("rejects a body carrying more than the one object it takes", func() {
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule",
			strings.NewReader(`{"params":{"periodStart":"2026-01-05"}} {"params":{"maxConcurrentInitiatives":99}}`))
		rec := httptest.NewRecorder()
		srv.schedulePlan(rec, req, plan)
		Expect(rec.Code).To(Equal(400))
		Expect(rec.Body.String()).To(ContainSubstring("single JSON object"))
	})

	It("rejects a malformed body instead of silently scheduling the saved plan", func() {
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule", strings.NewReader(`{"params":`))
		rec := httptest.NewRecorder()
		srv.schedulePlan(rec, req, plan)
		Expect(rec.Code).To(Equal(400))
	})

	It("returns an empty schedule rather than failing on a plan with nothing in it", func() {
		plan = &db.PlanRow{ID: "empty", HorizonWeeks: 26, CapacityLoss: 0.1}
		sched := post(`{"params":{"periodStart":"2026-01-05"}}`)
		Expect(sched.Initiatives).To(BeEmpty())
	})
})

func deltaTracks(s *planning.Schedule) int {
	for _, ps := range s.PodWeeks {
		if ps.Pod == "Delta" {
			return ps.Tracks
		}
	}
	return 0
}
