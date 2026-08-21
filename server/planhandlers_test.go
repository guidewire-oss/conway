package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
