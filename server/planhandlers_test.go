package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"conway/server/auth"
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
		Expect(sched.RulesTried).To(HaveLen(6), "Decision 6 runs every rule and keeps the best")

		By("giving every initiative a rank, a span and a commit week (FR-003)")
		ranks := map[int]bool{}
		for _, si := range sched.Initiatives {
			Expect(si.Name).NotTo(BeEmpty())
			Expect(si.ProposedRank).To(BeNumerically(">", 0))
			Expect(ranks).NotTo(HaveKey(si.ProposedRank), "ranks must be unique")
			ranks[si.ProposedRank] = true
			Expect(si.Verdict).NotTo(BeEmpty())
			// Decision 28: an initiative that could not begin inside the period has
			// no span, because none was computed. It still carries a rank and a
			// verdict -- FR-003's guarantee is that every initiative is accounted
			// for, not that every one is placed.
			if si.Verdict == "beyond-horizon" {
				Expect(si.StartWeek).To(BeZero(), si.Name)
				Expect(si.CommitWeek).To(BeZero(), si.Name)
				Expect(si.Slices).To(BeEmpty(), si.Name)
				continue
			}
			Expect(si.CommitWeek).To(BeNumerically(">=", si.RawFinishWeek))
			Expect(si.RawFinishWeek).To(BeNumerically(">", si.StartWeek))
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
		// The explicit number sets the limit; naming no model leaves the rule unchosen,
		// which the response reports rather than silently defaulting (§11 D22 amended).
		Expect(sched.WipLimit).To(Equal(planning.WipLimit{Value: 3, Model: planning.WipUnchosen}))
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

	It("rejects two concatenated objects with no separator (dec.More misses these)", func() {
		// Regression for the review finding: More() only inspects the current
		// composite, so "{...}{...}" sailed through it. The second-Decode-to-EOF
		// contract is what actually enforces one object.
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule",
			strings.NewReader(`{"params":{"periodStart":"2026-01-05"}}{"params":{"maxConcurrentInitiatives":99}}`))
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

// POST /api/plan/{id}/schedule/remedies — spec 001 §8, Story 5. Stateless like
// /schedule: remedies are proposals, and FR-022 forbids the plan moving before
// an explicit acceptance that does not exist yet.
var _ = Describe("remediesPlan", func() {
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
		schedB, err := json.Marshal(planning.DemoScheduling())
		Expect(err).NotTo(HaveOccurred())
		srv = &server{}
		plan = &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1,
			Teams: teamsB, Initiatives: initsB, Scheduling: schedB}
	})

	post := func(body string) map[string]any {
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule/remedies", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.remediesPlan(rec, req, plan)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var out map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
		return out
	}

	It("prices remedies for every missed date on the demo plan", func() {
		out := post(`{}`)
		remedies, ok := out["remedies"].([]any)
		Expect(ok).To(BeTrue())
		Expect(remedies).NotTo(BeEmpty(), "the demo plan misses dates, so it must have remedies")
		for _, r := range remedies {
			m := r.(map[string]any)
			Expect(m["kind"]).NotTo(BeEmpty())
			Expect(m["target"]).NotTo(BeEmpty())
			Expect(m["resultingVerdict"]).NotTo(BeEmpty())
		}
	})

	It("narrows to a named target and never touches the plan", func() {
		before := plan.Initiatives
		out := post(`{"targets":["Managed database MVP"]}`)
		remedies := out["remedies"].([]any)
		Expect(remedies).NotTo(BeEmpty())
		for _, r := range remedies {
			Expect(r.(map[string]any)["target"]).To(Equal("Managed database MVP"))
		}
		Expect(plan.Initiatives).To(MatchJSON(before), "FR-022: a remedy is a proposal, not a change")
	})

	It("says why transfer-capacity is absent rather than being silent about it", func() {
		out := post(`{}`)
		warnings, _ := out["warnings"].([]any)
		Expect(warnings).NotTo(BeEmpty())
		joined := ""
		for _, w := range warnings {
			joined += fmt.Sprint(w)
		}
		Expect(joined).To(ContainSubstring("transfer"))
	})

	It("refuses two concatenated objects, sharing /schedule's body contract", func() {
		req := httptest.NewRequest("POST", "/api/plan/plan1/schedule/remedies",
			strings.NewReader(`{"targets":["A"]}{"targets":["B"]}`))
		rec := httptest.NewRecorder()
		srv.remediesPlan(rec, req, plan)
		Expect(rec.Code).To(Equal(400))
		Expect(rec.Body.String()).To(ContainSubstring("single JSON object"))
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

// The whole upload chain, end to end through HTTP: workbook bytes -> ReadGrid ->
// ParseMatrix -> JSON -> ComputeSchedule. Until this held, spec 001's attributes
// existed in the model but no uploaded sheet could carry them.
var _ = Describe("an uploaded sheet carrying sequencing attributes", func() {
	It("reaches the schedule as dates and priorities, not as no-date defaults", func() {
		teams := []planning.Team{{Name: "Delta", Tracks: 1}, {Name: "Atlas", Tracks: 3}}
		inits := []planning.Initiative{
			{
				Name: "Regulatory reporting",
				Work: map[string]planning.TeamWork{
					"Delta": {Weeks: 6, Estimated: true, InPath: true},
				},
				StatedPriority: 2, Tier: 1, CostOfDelayPerWeek: 10,
				TargetDate: "2026-03-16", DateLocked: true,
			},
			{
				Name: "Internal tooling",
				Work: map[string]planning.TeamWork{
					"Delta": {Weeks: 6, Estimated: true, InPath: true},
				},
				StatedPriority: 1, PriorityLocked: true, Tier: 4, CostOfDelayPerWeek: 1,
			},
		}
		body, ct := multipartFileG("file", "initiatives.xlsx", planning.WriteInitiativesXLSX(teams, inits))

		teamsB, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		srv := &server{}
		plan := &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0, Teams: teamsB}

		By("uploading the workbook for preview, which parses but must not persist")
		req := httptest.NewRequest("POST", "/api/plan/plan1/initiatives/preview", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		srv.previewPlanInitiatives(rec, req, plan)
		Expect(rec.Code).To(Equal(200), rec.Body.String())

		var preview struct {
			Initiatives []planning.Initiative `json:"initiatives"`
		}
		Expect(json.Unmarshal(rec.Body.Bytes(), &preview)).To(Succeed())
		Expect(preview.Initiatives).To(HaveLen(2))
		Expect(plan.Initiatives).To(BeNil(), "preview must not persist")

		parsed := map[string]planning.Initiative{}
		for _, it := range preview.Initiatives {
			parsed[it.Name] = it
		}
		Expect(parsed["Regulatory reporting"].TargetDate).To(Equal("2026-03-16"))
		Expect(parsed["Regulatory reporting"].DateLocked).To(BeTrue())
		Expect(parsed["Internal tooling"].PriorityLocked).To(BeTrue())

		By("scheduling exactly those parsed initiatives")
		schedReq, err := json.Marshal(map[string]any{
			"params":      map[string]any{"periodStart": "2026-01-05", "maxConcurrentInitiatives": 2},
			"initiatives": preview.Initiatives,
		})
		Expect(err).NotTo(HaveOccurred())
		req = httptest.NewRequest("POST", "/api/plan/plan1/schedule", bytes.NewReader(schedReq))
		rec = httptest.NewRecorder()
		srv.schedulePlan(rec, req, plan)
		Expect(rec.Code).To(Equal(200), rec.Body.String())

		var sched planning.Schedule
		Expect(json.Unmarshal(rec.Body.Bytes(), &sched)).To(Succeed())

		byName := map[string]planning.ScheduledInitiative{}
		for _, si := range sched.Initiatives {
			byName[si.Name] = si
		}
		reg, tool := byName["Regulatory reporting"], byName["Internal tooling"]

		By("honouring the lock the sheet declared, at a measurable cost")
		Expect(tool.ProposedRank).To(Equal(1), "the sheet locked it to priority 1")
		Expect(reg.TargetWeek).NotTo(BeNil(), "its target date survived the round trip")
		Expect(*reg.TargetWeek).To(Equal(10))
		// Alone it would commit in week 8 and make the date; behind the lock it commits
		// in 14. So this is contention, which is the verdict that has a remedy.
		Expect(reg.Verdict).To(Equal("late"), "the lock pushes the dated commitment out")
		Expect(reg.WeeksLate).To(Equal(4))
		Expect(sched.ObjectiveScore).To(BeNumerically(">", 0),
			"a date-locked miss must show up in the objective")
	})
})

// multipartFileG is the Gomega-side twin of multipartFile: same body, but failing
// through Expect rather than needing a *testing.T.
func multipartFileG(field, filename string, data []byte) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile(field, filename)
	Expect(err).NotTo(HaveOccurred())
	_, err = fw.Write(data)
	Expect(err).NotTo(HaveOccurred())
	Expect(w.Close()).To(Succeed())
	return body, w.FormDataContentType()
}

// §8's two save endpoints, and /schedule reading the policy they store. The DB
// write itself needs a live Postgres, so the specs here cover what a handler can
// be held to on its own: what it refuses, and what it reads.
var _ = Describe("the plan's sequencing inputs", func() {
	var srv *server
	var plan *db.PlanRow

	BeforeEach(func() {
		teams, inits := planning.Demo()
		teamsB, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		initsB, err := json.Marshal(inits)
		Expect(err).NotTo(HaveOccurred())
		schedB, err := json.Marshal(planning.DemoScheduling())
		Expect(err).NotTo(HaveOccurred())
		srv = &server{}
		plan = &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1,
			Teams: teamsB, Initiatives: initsB, Scheduling: schedB}
	})

	patch := func(path, body string, h func(http.ResponseWriter, *http.Request, *db.PlanRow)) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PATCH", path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, req, plan)
		return rec
	}

	Describe("PATCH /api/plan/{id}/initiatives", func() {
		// AC 2.4: rejected at entry, naming the period, and nothing recomputed. The
		// handler must refuse before it reaches the database, or "no schedule is
		// recomputed" would depend on the write failing too.
		It("refuses a target date outside the period, naming the bounds", func() {
			rec := patch("/api/plan/plan1/initiatives",
				`{"initiatives":[{"name":"Telemetry GA","targetDate":"2027-06-01"}]}`,
				srv.editPlanInitiatives)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("2026-01-05"))
			Expect(rec.Body.String()).To(ContainSubstring("2026-07-06"))
		})

		It("refuses an edit naming an initiative the plan does not have", func() {
			rec := patch("/api/plan/plan1/initiatives",
				`{"initiatives":[{"name":"Ghost initiative","tier":1}]}`,
				srv.editPlanInitiatives)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("Ghost initiative"))
		})

		It("refuses a request with no edits in it", func() {
			rec := patch("/api/plan/plan1/initiatives", `{"initiatives":[]}`, srv.editPlanInitiatives)
			Expect(rec.Code).To(Equal(400))
		})

		It("refuses a malformed body", func() {
			rec := patch("/api/plan/plan1/initiatives", `{"initiatives":`, srv.editPlanInitiatives)
			Expect(rec.Code).To(Equal(400))
		})
	})

	// json.Unmarshal can leave fields populated before it errors, so a corrupt blob
	// must not be able to apply half a policy.
	Describe("reading a stored policy", func() {
		It("falls back to no policy when the stored blob is corrupt", func() {
			plan.Scheduling = []byte(`{"periodStart":"2026-01-05","maxConcurrentInitiatives":`)
			Expect(planScheduling(plan)).To(Equal(planning.SchedulingParams{}),
				"a partly-decoded policy is not a policy the planner chose")
		})

		It("reads a good blob", func() {
			Expect(planScheduling(plan).PeriodStart).To(Equal("2026-01-05"))
		})

		It("treats an absent blob as no policy", func() {
			plan.Scheduling = nil
			Expect(planScheduling(plan)).To(Equal(planning.SchedulingParams{}))
		})
	})

	Describe("PATCH /api/plan/{id}/scheduling", func() {
		It("refuses a period start that is not a date", func() {
			rec := patch("/api/plan/plan1/scheduling", `{"periodStart":"next quarter"}`, srv.savePlanScheduling)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("YYYY-MM-DD"))
		})

		It("refuses a malformed body", func() {
			rec := patch("/api/plan/plan1/scheduling", `{"periodStart":`, srv.savePlanScheduling)
			Expect(rec.Code).To(Equal(400))
		})

		// FR-018: a window with unusable dates or an unknown effect would
		// either move the schedule on a constraint nobody set, or sit in the
		// blob looking applied while the engine ignores it. Both are refused
		// at the door.
		It("refuses a calendar window with unparseable dates", func() {
			rec := patch("/api/plan/plan1/scheduling",
				`{"periodStart":"2026-01-05","calendars":[{"kind":"change-freeze","scope":"org","fromDate":"soon","toDate":"2026-02-02","effect":"block-start"}]}`,
				srv.savePlanScheduling)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("calendar"))
		})

		It("refuses a window whose end precedes its start", func() {
			rec := patch("/api/plan/plan1/scheduling",
				`{"periodStart":"2026-01-05","calendars":[{"kind":"change-freeze","scope":"org","fromDate":"2026-03-02","toDate":"2026-02-02","effect":"block-start"}]}`,
				srv.savePlanScheduling)
			Expect(rec.Code).To(Equal(400))
		})

		It("refuses an unknown effect rather than storing an inert one", func() {
			rec := patch("/api/plan/plan1/scheduling",
				`{"periodStart":"2026-01-05","calendars":[{"kind":"event","scope":"org","fromDate":"2026-02-02","toDate":"2026-02-09","effect":"teleport"}]}`,
				srv.savePlanScheduling)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("effect"))
		})

		It("refuses a bad window on the stateless schedule endpoint too", func() {
			// The windows ride in the request params; the save path is not
			// involved, so the refusal has to live in the shared resolution.
			req := httptest.NewRequest("POST", "/api/plan/plan1/schedule",
				strings.NewReader(`{"params":{"periodStart":"2026-01-05","calendars":[`+
					`{"kind":"event","scope":"org","fromDate":"soon","toDate":"2026-02-02","effect":"reduce-capacity"}]}}`))
			rec := httptest.NewRecorder()
			srv.schedulePlan(rec, req, plan)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("YYYY-MM-DD"))
		})

		It("round-trips a well-formed window through the stored blob", func() {
			// The write path needs a live Postgres (see the baseline specs); the
			// persistence contract this guards is the decode side — a window
			// the planner saved must come back off the blob, not vanish.
			plan.Scheduling = []byte(`{"periodStart":"2026-01-05","calendars":[` +
				`{"kind":"change-freeze","scope":"org","fromDate":"2026-02-02","toDate":"2026-02-09","effect":"block-start"}]}`)
			sp := planScheduling(plan)
			Expect(sp.Calendars).To(HaveLen(1))
			Expect(sp.Calendars[0].Effect).To(Equal("block-start"))
			Expect(sp.Calendars[0].From).To(Equal("2026-02-02"))
		})
	})

	Describe("POST /api/plan/{id}/schedule", func() {
		schedule := func(body string) *planning.Schedule {
			req := httptest.NewRequest("POST", "/api/plan/plan1/schedule", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.schedulePlan(rec, req, plan)
			Expect(rec.Code).To(Equal(200), rec.Body.String())
			var out planning.Schedule
			Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
			return &out
		}

		It("uses the saved policy when the request carries none", func() {
			sched := schedule(`{}`)
			Expect(sched.PeriodStart).To(Equal("2026-01-05"))
			dated := 0
			for _, si := range sched.Initiatives {
				if si.TargetWeek != nil {
					dated++
				}
			}
			Expect(dated).To(BeNumerically(">", 0), "the saved period start is what turns dates into weeks")
		})

		// A supplied policy is a complete what-if, not a patch over the saved one:
		// merging would make "the same plan with no period start" impossible to ask for.
		It("replaces the saved policy rather than merging when params are supplied", func() {
			sched := schedule(`{"params":{"maxConcurrentInitiatives":3}}`)
			Expect(sched.WipLimit).To(Equal(planning.WipLimit{Value: 3, Model: planning.WipUnchosen}))
			Expect(sched.PeriodStart).To(BeEmpty(), "the supplied policy named no period start")
			for _, si := range sched.Initiatives {
				Expect(si.TargetWeek).To(BeNil(), "with no period start there is nothing to date against")
			}
		})
	})

	// The demo plan is the first thing anyone opens, so it has to demonstrate what
	// the feature is for. Asserted by shape, not by exact weeks, so the demo data can
	// still be tuned without breaking this.
	Describe("the demo plan's execution order", func() {
		It("shows a deviation, a date that holds, one that misses and one that cannot fit", func() {
			teams, inits := planning.Demo()
			sched := planning.ComputeSchedule(teams, inits,
				planning.Params{HorizonWeeks: 26, CapacityLoss: 0.1}, planning.DemoScheduling())

			verdicts := map[string]int{}
			for _, si := range sched.Initiatives {
				verdicts[si.Verdict]++
			}
			Expect(verdicts).To(HaveKey("on-time"))
			Expect(verdicts).To(HaveKey("structurally-infeasible"))
			Expect(verdicts["late"] + verdicts["structurally-infeasible"]).To(BeNumerically(">", 0))

			Expect(sched.Reconciliation).NotTo(BeEmpty(),
				"the reconciliation report is the feature's primary artefact (Decision 3)")
			Expect(sched.StatedOrderObjectiveScore).To(BeNumerically(">", 0),
				"the planner's own order must have a price to compare against")
			Expect(sched.WipLimit.Derived).To(BeTrue(), "Decision 22, visible out of the box")

			By("starting the carryover initiative at week 0")
			for _, si := range sched.Initiatives {
				if si.Name == "Bring-your-own-auth (early access)" {
					Expect(si.StartWeek).To(BeZero(), "it is already in flight (AC X.4)")
				}
			}
		})
	})
})

// CONWAY_ADMIN_PASSWORD is read only when no admin exists. Setting it against a
// deployment that already has one is a no-op whose only symptom is "wrong
// credentials", so the boot log has to say what happened.
var _ = Describe("adminAction", func() {
	var st *auth.Store

	BeforeEach(func() { st = auth.NewStore(nil) })

	It("mints and prints a password on a first boot with nothing set", func() {
		Expect(adminAction(st, "")).To(Equal(adminGenerate))
	})

	It("takes the variable when there is no admin yet", func() {
		Expect(adminAction(st, "letmein")).To(Equal(adminSetFromEnv))
	})

	// The behaviour this PR changed: it used to be inert here, which made the
	// variable silently a no-op on every boot after the first.
	It("replaces an existing password that differs", func() {
		st.SetAdmin("old")
		Expect(adminAction(st, "letmein")).To(Equal(adminReplaceFromEnv))
	})

	// So a deployment that leaves the variable set does not rewrite its own hash on
	// every restart, and the log line means something when it does appear.
	It("does nothing when the variable already matches the stored password", func() {
		st.SetAdmin("letmein")
		Expect(adminAction(st, "letmein")).To(Equal(adminNothing))
	})

	It("leaves an existing admin alone when the variable is unset", func() {
		st.SetAdmin("whatever")
		Expect(adminAction(st, "")).To(Equal(adminNothing))
	})

	It("compares against the stored salt, not a fresh one", func() {
		st.SetAdmin("letmein")
		first := st.Users["admin"].Salt
		Expect(adminAction(st, "letmein")).To(Equal(adminNothing))
		Expect(st.Users["admin"].Salt).To(Equal(first), "an unchanged password must not re-salt")
	})
})

var _ = Describe("retireLegacyStore", func() {
	It("renames the file aside so it cannot be imported again", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "store.json")
		Expect(os.WriteFile(path, []byte(`{"users":{}}`), 0o600)).To(Succeed())

		retireLegacyStore(path)

		_, err := os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue(), "the original must be gone, or it re-imports")
		// Renamed, not deleted: it holds credentials and a signing secret.
		body, err := os.ReadFile(path + ".migrated")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal(`{"users":{}}`), "the contents must be recoverable")
	})

	// The file holds credentials and a signing secret, so an older backup is not
	// something to trade away for a convenience rename.
	It("refuses to overwrite an existing backup", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "store.json")
		retired := path + ".migrated"
		Expect(os.WriteFile(path, []byte("new"), 0o600)).To(Succeed())
		Expect(os.WriteFile(retired, []byte("older backup"), 0o600)).To(Succeed())

		retireLegacyStore(path)

		kept, err := os.ReadFile(retired)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(kept)).To(Equal("older backup"), "the earlier backup must survive")
		_, err = os.Stat(path)
		Expect(err).NotTo(HaveOccurred(), "and the original stays, so nothing is lost")
	})

	It("treats a dangling symlink as present rather than a free slot", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "store.json")
		Expect(os.WriteFile(path, []byte("new"), 0o600)).To(Succeed())
		Expect(os.Symlink(filepath.Join(dir, "gone"), path+".migrated")).To(Succeed())

		retireLegacyStore(path)

		_, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred(), "Lstat sees the link; Stat would not")
	})

	It("does not stop the server when the file cannot be renamed", func() {
		// A path that was never there: the accounts are already saved either way, so
		// this is a warning, not a failure.
		Expect(func() { retireLegacyStore(filepath.Join(GinkgoT().TempDir(), "absent.json")) }).
			NotTo(Panic())
	})
})

// The baseline endpoints (§8, Story 7). The database paths need a live Postgres,
// so these cover what a handler can be held to alone: what it refuses, and the
// input resolution it shares with /schedule.
var _ = Describe("the baseline endpoints", func() {
	var srv *server
	var plan *db.PlanRow

	BeforeEach(func() {
		teams, inits := planning.Demo()
		teamsB, err := json.Marshal(teams)
		Expect(err).NotTo(HaveOccurred())
		initsB, err := json.Marshal(inits)
		Expect(err).NotTo(HaveOccurred())
		schedB, err := json.Marshal(planning.DemoScheduling())
		Expect(err).NotTo(HaveOccurred())
		srv = &server{}
		plan = &db.PlanRow{ID: "plan1", HorizonWeeks: 26, CapacityLoss: 0.1,
			Teams: teamsB, Initiatives: initsB, Scheduling: schedB}
	})

	post := func(path, body string, h func(http.ResponseWriter, *http.Request, *db.PlanRow, auth.Claims)) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, req, plan, auth.Claims{Sub: "ann@example.com"})
		return rec
	}

	Describe("POST /baseline", func() {
		// FR-033: a baseline is how a period's agreed order is referred to later, so
		// an unnamed one is not much use to anybody.
		It("refuses a baseline with no name", func() {
			rec := post("/api/plan/plan1/baseline", `{}`, srv.saveBaseline)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("name"))
		})

		It("refuses a name that is only whitespace", func() {
			Expect(post("/api/plan/plan1/baseline", `{"name":"   "}`, srv.saveBaseline).Code).To(Equal(400))
		})

		It("refuses a malformed body", func() {
			Expect(post("/api/plan/plan1/baseline", `{"name":`, srv.saveBaseline).Code).To(Equal(400))
		})

		It("refuses two concatenated objects (the shared-body contract with /schedule)", func() {
			rec := post("/api/plan/plan1/baseline", `{"name":"v1"}{"name":"v2"}`, srv.saveBaseline)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("single JSON object"))
		})

		It("refuses to baseline a plan with no initiatives", func() {
			plan.Initiatives = nil
			rec := post("/api/plan/plan1/baseline", `{"name":"v1"}`, srv.saveBaseline)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("no initiatives"))
		})
	})

	Describe("POST /baseline/{bid}/compare-to/{other}", func() {
		call := func(bid, other string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/api/plan/plan1/baseline/"+bid+"/compare-to/"+other, nil)
			rec := httptest.NewRecorder()
			srv.compareBaselines(rec, req, plan, bid, other)
			return rec
		}

		It("refuses comparing a baseline to itself", func() {
			rec := call("b1", "b1")
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("different baseline"))
		})

		It("refuses an empty other id", func() {
			Expect(call("b1", "").Code).To(Equal(400))
		})
	})

	Describe("PATCH /baseline/{bid}", func() {
		patch := func(body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("PATCH", "/api/plan/plan1/baseline/b1", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.patchBaseline(rec, req, plan, "b1")
			return rec
		}

		It("refuses a request that changes nothing", func() {
			rec := patch(`{}`)
			Expect(rec.Code).To(Equal(400))
			Expect(rec.Body.String()).To(ContainSubstring("active or name"))
		})

		It("refuses an empty rename", func() {
			Expect(patch(`{"name":"  "}`).Code).To(Equal(400))
		})

		It("refuses a malformed body", func() {
			Expect(patch(`{"active":`).Code).To(Equal(400))
		})
	})

	Describe("routing /baseline/{bid}", func() {
		route := func(method, rest string) int {
			req := httptest.NewRequest(method, "/api/plan/plan1/baseline/"+rest, strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			srv.baselineItem(rec, req, plan, rest)
			return rec.Code
		}

		It("refuses a missing baseline id", func() {
			Expect(route("GET", "")).To(Equal(400))
		})

		It("refuses a method the endpoint does not have", func() {
			Expect(route("DELETE", "b1")).To(Equal(405),
				"baselines are immutable, so there is no delete (FR-030)")
			Expect(route("PUT", "b1/compare")).To(Equal(405))
		})
	})

	// The resolution shared with /schedule. If these two ever disagree, a baseline
	// records an order the planner never saw.
	Describe("planScheduleFor", func() {
		It("reads the plan's saved roster, initiatives and policy", func() {
			in, err := srv.planScheduleFor(plan, scheduleRequest{})
			Expect(err).NotTo(HaveOccurred())
			_, demoInits := planning.Demo()
			Expect(in.Initiatives).To(HaveLen(len(demoInits)))
			Expect(in.Teams).NotTo(BeEmpty())
			Expect(in.Scheduling.PeriodStart).To(Equal(planning.DemoPeriodStart))
			Expect(in.Params.HorizonWeeks).To(Equal(26.0))
		})

		It("prefers a draft override to the saved sheet", func() {
			in, err := srv.planScheduleFor(plan, scheduleRequest{
				Initiatives: []planning.Initiative{{Name: "Draft only"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(in.Initiatives).To(HaveLen(1))
			Expect(in.Initiatives[0].Name).To(Equal("Draft only"))
		})

		It("applies levers, so a baseline records the plan that was on screen", func() {
			plain, err := srv.planScheduleFor(plan, scheduleRequest{})
			Expect(err).NotTo(HaveOccurred())
			levered, err := srv.planScheduleFor(plan, scheduleRequest{
				Levers: []planning.Lever{{Type: "addCapacity", Pod: "Delta", N: 4}},
			})
			Expect(err).NotTo(HaveOccurred())

			tracks := func(in planning.BaselineInputs) int {
				for _, t := range in.Teams {
					if t.Name == "Delta" {
						return t.EffectiveTracks()
					}
				}
				return 0
			}
			Expect(tracks(levered)).To(BeNumerically(">", tracks(plain)))
		})

		It("replaces the saved policy when params are supplied", func() {
			in, err := srv.planScheduleFor(plan, scheduleRequest{
				Params: &planning.SchedulingParams{MaxConcurrentInitiatives: 3},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(in.Scheduling.PeriodStart).To(BeEmpty(), "a what-if is complete, not a patch")
			Expect(in.Scheduling.MaxConcurrentInitiatives).To(Equal(3))
		})

		It("reports unreadable stored initiatives rather than scheduling nothing", func() {
			plan.Initiatives = []byte(`{"not":"an array"`)
			_, err := srv.planScheduleFor(plan, scheduleRequest{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("initiatives"))
		})
	})
})

var _ = Describe("methodNotAllowed", func() {
	// A 405 whose body is the word "method" is unreadable in a network tab and
	// worse in a UI that surfaces it: the baselines panel once rendered exactly
	// that beside its Save button, looking like a control labelled "Method". The
	// cause was a server binary older than the page, which the body should say,
	// because app/ is served from disk while routes are compiled in.
	It("names the method and path it refused", func() {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/plan/p1/baseline", nil)

		methodNotAllowed(w, r)

		Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
		body := w.Body.String()
		Expect(body).To(ContainSubstring("POST"))
		Expect(body).To(ContainSubstring("/api/plan/p1/baseline"))
		Expect(strings.TrimSpace(body)).NotTo(Equal("method"))
	})

	It("points at the likeliest cause, since a stale binary is not guessable", func() {
		w := httptest.NewRecorder()
		methodNotAllowed(w, httptest.NewRequest(http.MethodPost, "/api/plan/p1/baseline", nil))
		Expect(w.Body.String()).To(MatchRegexp(`(?i)older|restart|rebuil`))
	})

	// The sweep: the fifteen older "method" refusals across gamesadmin.go,
	// jirahandlers.go, main.go, snapshots.go and rosterhandlers.go were converted
	// to this helper. One of them, exercised through its handler (not the helper
	// directly), stands for the batch — the others differ only in setup cost.
	It("refuses through the converted admin-users handler, not just the helper", func() {
		srv := &server{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/admin/users", nil)

		srv.handleUsers(w, r, auth.Claims{})

		Expect(w.Code).To(Equal(http.StatusMethodNotAllowed))
		Expect(w.Body.String()).To(ContainSubstring("DELETE /api/admin/users"))
		Expect(strings.TrimSpace(w.Body.String())).NotTo(Equal("method"))
	})
})
