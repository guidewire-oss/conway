package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// specs/025-team-ready-work-queue.md:111: operational records append without
// modifying planning inputs or agreements, and stale reviewed scope is refused.
var _ = Describe("team ready-work queue persistence", Label("database"), func() {
	var database *db.DB
	var srv *server
	var plan db.PlanRow
	var claims auth.Claims
	encode := func(v any) []byte { raw, err := json.Marshal(v); Expect(err).NotTo(HaveOccurred()); return raw }
	call := func(method, path string, body any, viewer auth.Claims) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var raw []byte
		if body != nil {
			raw = encode(body)
		}
		srv.handlePlanItem(rec, httptest.NewRequest(method, path, bytes.NewReader(raw)), viewer)
		return rec
	}
	object := func(rec *httptest.ResponseRecorder) map[string]any {
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var v map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &v)).To(Succeed())
		return v
	}
	base := func() string { return "/api/plan/" + plan.ID + "/ready-queue" }
	queue := func() map[string]any { return object(call("GET", base()+"?team=Team%20A&asOfWeek=0", nil, claims)) }
	row := func(q map[string]any) map[string]any {
		for _, raw := range q["items"].([]any) {
			item := raw.(map[string]any)
			if item["initiative"] == "Atlas" {
				return item
			}
		}
		Fail("Atlas assignment absent")
		return nil
	}
	checks := func() []map[string]any {
		return []map[string]any{{"key": "scope_ready", "checked": true, "owner": "Delivery lead", "evidence": "Scope accepted."}, {"key": "dependencies_accepted", "checked": true, "owner": "Dependency owner", "evidence": "Acceptance confirmed."}, {"key": "team_available", "checked": true, "owner": "Team lead", "evidence": "Availability confirmed for week zero."}}
	}
	confirmBody := func(fp any) map[string]any {
		return map[string]any{"team": "Team A", "initiative": "Atlas", "asOfWeek": 0, "expectedFingerprint": fp, "checks": checks()}
	}
	decideBody := func(fp any, kind string) map[string]any {
		return map[string]any{"team": "Team A", "initiative": "Atlas", "asOfWeek": 0, "expectedFingerprint": fp, "decision": kind, "owner": "Team lead", "evidence": "Review this explicit operational decision."}
	}
	history := func() map[string]any {
		return object(call("GET", base()+"/history?team=Team%20A&initiative=Atlas", nil, claims))
	}
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set CONWAY_TEST_DATABASE_URL for isolated PostgreSQL integration.")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		claims = auth.Claims{Sub: "ready-owner-" + newID(), Roles: []string{"manager"}}
		srv = &server{db: database}
		now := time.Now().Unix()
		plan = db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas ready-work plan", HorizonWeeks: 8, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(plan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(plan.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 1}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Atlas", Work: map[string]planning.TeamWork{"Team A": {Weeks: 2, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		zero := 0.0
		Expect(database.SavePlanScheduling(plan.ID, encode(planning.SchedulingParams{PeriodStart: "2026-09-07", WipModel: planning.WipOff, AcceptedOrdering: "stated", BufferPct: &zero}), now)).To(Succeed())
	})
	// specs/025-team-ready-work-queue.md:296: complete planning scope includes
	// site working hours even when team capacity and initiative inputs stay equal.
	It("invalidates checklist and release freshness after a site-only planning edit", func() {
		Expect(database.SavePlanSites(plan.ID, []byte(`[{"name":"Site A","workStart":9,"workEnd":17}]`), time.Now().Unix())).To(Succeed())
		object(call("POST", base()+"/confirmations", confirmBody(queue()["fingerprint"]), claims))
		Expect(row(queue())["confirmationCurrent"]).To(BeTrue())
		object(call("POST", base()+"/decisions", decideBody(queue()["fingerprint"], "release"), claims))
		Expect(row(queue())["releaseCurrent"]).To(BeTrue())
		before, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(database.SavePlanSites(plan.ID, []byte(`[{"name":"Site A","workStart":10,"workEnd":18}]`), time.Now().Unix())).To(Succeed())
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Teams).To(Equal(before.Teams))
		Expect(after.Initiatives).To(Equal(before.Initiatives))
		current := row(queue())
		Expect(current["confirmationCurrent"]).To(BeFalse())
		Expect(current["releaseCurrent"]).To(BeFalse())
		Expect(current["confirmation"]).NotTo(BeNil())
		Expect(current["lastDecision"]).NotTo(BeNil())
	})
	// specs/025-team-ready-work-queue.md:125: a reviewed token is scoped to
	// its plan even when the same manager owns an identical second plan.
	It("refuses another owned plan's token even when all inputs are identical", func() {
		first := queue()
		originalID := plan.ID
		source, err := database.GetPlan(originalID)
		Expect(err).NotTo(HaveOccurred())
		other := *source
		other.ID = newID()
		Expect(database.CreatePlan(other)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(other.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(other.ID, source.Teams, source.UpdatedAt)).To(Succeed())
		Expect(database.SavePlanInitiatives(other.ID, source.Initiatives, source.UpdatedAt)).To(Succeed())
		Expect(database.SavePlanScheduling(other.ID, source.Scheduling, source.UpdatedAt)).To(Succeed())
		plan.ID = other.ID
		defer func() { plan.ID = originalID }()
		Expect(queue()["fingerprint"]).NotTo(Equal(first["fingerprint"]))
		refused := call("POST", base()+"/confirmations", confirmBody(first["fingerprint"]), claims)
		Expect(refused.Code).To(Equal(409), refused.Body.String())
		Expect(history()["confirmations"]).To(BeEmpty())
	})
	It("appends current checklist and release evidence without changing source inputs or agreement", func() {
		before, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		inputs, err := srv.planScheduleFor(before, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		bid := newID()
		_, err = database.SaveBaseline(db.BaselineRow{ID: bid, PlanID: plan.ID, Name: "Agreed", Inputs: encode(inputs), Schedule: encode(inputs.Recompute()), Fingerprint: inputs.Fingerprint(), CreatedAt: time.Now().Unix()}, true)
		Expect(err).NotTo(HaveOccurred())
		agreementBefore, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		first := queue()
		Expect(row(first)["canRelease"]).To(BeFalse())
		confirmation := object(call("POST", base()+"/confirmations", confirmBody(first["fingerprint"]), claims))
		Expect(confirmation["createdBy"]).To(Equal(claims.Sub))
		Expect(confirmation["createdAt"]).To(BeNumerically(">", 0))
		ready := queue()
		Expect(row(ready)["canRelease"]).To(BeTrue())
		decision := object(call("POST", base()+"/decisions", decideBody(ready["fingerprint"], "release"), claims))
		Expect(decision["decision"]).To(Equal("release"))
		Expect(decision["createdBy"]).To(Equal(claims.Sub))
		released := row(queue())
		Expect(released["state"]).To(Equal("waiting"))
		Expect(released["canRelease"]).To(BeFalse())
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		agreementAfter, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		Expect(agreementAfter).To(Equal(agreementBefore))
		Expect(history()["confirmations"]).To(HaveLen(1))
		Expect(history()["decisions"]).To(HaveLen(1))
	})
	It("allows partial checklist progress but requires evidence for checked prerequisites", func() {
		initial := queue()
		body := confirmBody(initial["fingerprint"])
		partial := checks()
		partial[2]["checked"], partial[2]["owner"], partial[2]["evidence"] = false, "", ""
		body["checks"] = partial
		object(call("POST", base()+"/confirmations", body, claims))
		Expect(row(queue())["canRelease"]).To(BeFalse())
		bad := confirmBody(queue()["fingerprint"])
		invalid := checks()
		invalid[0]["evidence"] = " "
		bad["checks"] = invalid
		rec := call("POST", base()+"/confirmations", bad, claims)
		Expect(rec.Code).To(Equal(400), rec.Body.String())
		Expect(history()["confirmations"]).To(HaveLen(1))
	})
	It("refuses release before checklist gates and rejects malformed checklist keys", func() {
		initial := queue()
		Expect(call("POST", base()+"/decisions", decideBody(initial["fingerprint"], "release"), claims).Code).To(Equal(400))
		for _, key := range []string{"unknown_key", "scope_ready"} {
			body := confirmBody(initial["fingerprint"])
			invalid := checks()
			invalid[2]["key"] = key
			body["checks"] = invalid
			Expect(call("POST", base()+"/confirmations", body, claims).Code).To(Equal(400))
		}
		Expect(history()["confirmations"]).To(BeEmpty())
		Expect(history()["decisions"]).To(BeEmpty())
	})
	It("requires a fresh queue after input edits or another readiness write", func() {
		first := queue()
		object(call("POST", base()+"/confirmations", confirmBody(first["fingerprint"]), claims))
		Expect(call("POST", base()+"/confirmations", confirmBody(first["fingerprint"]), claims).Code).To(Equal(409))
		ready := queue()
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 2}}), time.Now().Unix())).To(Succeed())
		Expect(call("POST", base()+"/decisions", decideBody(ready["fingerprint"], "release"), claims).Code).To(Equal(409))
		Expect(row(queue())["confirmationCurrent"]).To(BeFalse())
		Expect(history()["decisions"]).To(BeEmpty())
	})
	It("preserves deferral through input edits and explicitly reconsiders it without auto-release", func() {
		object(call("POST", base()+"/decisions", decideBody(queue()["fingerprint"], "defer"), claims))
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 2}}), time.Now().Unix())).To(Succeed())
		deferred := queue()
		Expect(row(deferred)["state"]).To(Equal("deferred"))
		object(call("POST", base()+"/decisions", decideBody(deferred["fingerprint"], "reconsider"), claims))
		Expect(row(queue())["state"]).To(Equal("waiting"))
		Expect(row(queue())["canRelease"]).To(BeFalse())
		Expect(history()["decisions"]).To(HaveLen(2))
	})
	It("allows only one concurrent write from a reviewed queue fingerprint", func() {
		first := queue()
		payload := encode(confirmBody(first["fingerprint"]))
		path := base() + "/confirmations"
		start := make(chan struct{})
		results := make(chan int, 2)
		var workers sync.WaitGroup
		for i := 0; i < 2; i++ {
			workers.Add(1)
			go func() {
				defer GinkgoRecover()
				defer workers.Done()
				<-start
				rec := httptest.NewRecorder()
				srv.handlePlanItem(rec, httptest.NewRequest("POST", path, bytes.NewReader(payload)), claims)
				results <- rec.Code
			}()
		}
		close(start)
		workers.Wait()
		close(results)
		codes := []int{}
		for code := range results {
			codes = append(codes, code)
		}
		Expect(codes).To(ConsistOf(200, 409))
		Expect(history()["confirmations"]).To(HaveLen(1))
	})
	It("retains historical records when assignments are removed without allowing a new release", func() {
		object(call("POST", base()+"/decisions", decideBody(queue()["fingerprint"], "defer"), claims))
		Expect(database.SavePlanInitiatives(plan.ID, []byte("[]"), time.Now().Unix())).To(Succeed())
		Expect(history()["decisions"]).To(HaveLen(1))
		Expect(queue()["items"]).To(BeEmpty())
		Expect(call("POST", base()+"/decisions", decideBody(queue()["fingerprint"], "release"), claims).Code).To(Equal(404))
	})
	It("rejects foreign plan access and invalid operational coordinates", func() {
		outsider := auth.Claims{Sub: "another-manager", Roles: []string{"manager"}}
		Expect(call("GET", base()+"?team=Team%20A&asOfWeek=0", nil, outsider).Code).To(Equal(403))
		Expect(call("POST", base()+"/confirmations", confirmBody("stale"), outsider).Code).To(Equal(403))
		for _, query := range []string{"?team=Team%20A&asOfWeek=-1", "?team=Team%20A&asOfWeek=0.5", "?team=Team%20A&asOfWeek=8"} {
			Expect(call("GET", base()+query, nil, claims).Code).To(Equal(400))
		}
	})
})
