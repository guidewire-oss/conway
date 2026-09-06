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

// specs/024-weekly-execution-review.md:95: action history and completed reviews
// must retain evidence and reject stale writes within the authorized plan.
var _ = Describe("weekly execution review persistence", Label("database"), func() {
	var database *db.DB
	var srv *server
	var plan db.PlanRow
	var claims auth.Claims
	encode := func(v any) []byte { b, err := json.Marshal(v); Expect(err).NotTo(HaveOccurred()); return b }
	call := func(method, path string, body any, viewer auth.Claims) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var payload []byte
		if body != nil {
			payload = encode(body)
		}
		srv.handlePlanItem(rec, httptest.NewRequest(method, path, bytes.NewReader(payload)), viewer)
		return rec
	}
	object := func(rec *httptest.ResponseRecorder) map[string]any {
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var value map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &value)).To(Succeed())
		return value
	}
	base := func() string { return "/api/plan/" + plan.ID }
	previewInput := func() map[string]any {
		return map[string]any{"reviewDate": "2026-11-01", "timezone": "America/Los_Angeles", "snapshotId": "", "filters": map[string]any{"team": "Team A", "initiative": "Atlas"}}
	}
	preview := func() map[string]any { return object(call("POST", base()+"/reviews/preview", previewInput(), claims)) }
	completion := func(fingerprint any) map[string]any {
		body := previewInput()
		body["expectedFingerprint"], body["outcomeNote"], body["nextCheckpoint"] = fingerprint, "Confirm Atlas readiness with its owner.", "2026-11-08"
		return body
	}
	decision := func() map[string]any {
		return object(call("POST", base()+"/decisions", map[string]any{"action": "Confirm upstream readiness", "owner": "Team lead", "reviewDate": "2026-10-30", "rationale": "The checkpoint needs explicit evidence.", "initiative": "Atlas"}, claims))
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
		claims = auth.Claims{Sub: "weekly-owner-" + newID(), Roles: []string{"manager"}}
		srv = &server{db: database}
		now := time.Now().Unix()
		plan = db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas weekly plan", HorizonWeeks: 26, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(plan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(plan.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 2}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Atlas", EpicKeys: []string{"PROJ-1"}, Work: map[string]planning.TeamWork{"Team A": {Weeks: 4, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		Expect(database.SavePlanScheduling(plan.ID, encode(planning.SchedulingParams{WipModel: "strict", EstimateModel: "effort", PeriodStart: "2026-09-07"}), now)).To(Succeed())
	})
	It("keeps the legacy decision API and original text while appending evidenced transitions", func() {
		action := decision()
		Expect(action["status"]).To(Equal("open"))
		Expect(action["version"]).To(Equal(float64(1)))
		path := base() + "/decisions/" + action["id"].(string) + "/transitions"
		bad := call("POST", path, map[string]any{"expectedVersion": 1, "status": "resolved", "evidence": "  "}, claims)
		Expect(bad.Code).To(Equal(400), bad.Body.String())
		resolved := object(call("POST", path, map[string]any{"expectedVersion": 1, "status": "resolved", "evidence": "PROJ-2 acceptance was confirmed at the checkpoint."}, claims))
		Expect(resolved["status"]).To(Equal("resolved"))
		Expect(resolved["version"]).To(Equal(float64(2)))
		reopened := object(call("POST", path, map[string]any{"expectedVersion": 2, "status": "open", "evidence": "A new follow-up is needed."}, claims))
		Expect(reopened["version"]).To(Equal(float64(3)))
		for _, key := range []string{"action", "owner", "reviewDate", "rationale", "initiative", "createdBy", "createdAt"} {
			Expect(reopened[key]).To(Equal(action[key]), key)
		}
		history := object(call("GET", path, nil, claims))["transitions"].([]any)
		Expect(history).To(HaveLen(2))
		Expect(string(encode(history))).To(ContainSubstring("PROJ-2 acceptance"))
		Expect(string(encode(history))).To(ContainSubstring(claims.Sub))
		listed := object(call("GET", base()+"/decisions", nil, claims))["decisions"].([]any)
		Expect(listed).To(HaveLen(1))
		Expect(listed[0].(map[string]any)["status"]).To(Equal("open"))
	})
	It("allows only one concurrent transition from the same action version", func() {
		action := decision()
		path := base() + "/decisions/" + action["id"].(string) + "/transitions"
		payload := encode(map[string]any{"expectedVersion": 1, "status": "in_progress", "evidence": "Owner accepted the follow-up."})
		results := make(chan int, 2)
		var workers sync.WaitGroup
		for i := 0; i < 2; i++ {
			workers.Add(1)
			go func() {
				defer GinkgoRecover()
				defer workers.Done()
				rec := httptest.NewRecorder()
				srv.handlePlanItem(rec, httptest.NewRequest("POST", path, bytes.NewReader(payload)), claims)
				results <- rec.Code
			}()
		}
		workers.Wait()
		close(results)
		codes := []int{}
		for code := range results {
			codes = append(codes, code)
		}
		Expect(codes).To(ConsistOf(200, 409))
		Expect(object(call("GET", path, nil, claims))["transitions"]).To(HaveLen(1))
	})
	It("preserves completed manual review observations and filters after later action and plan edits", func() {
		action := decision()
		prepared := preview()
		Expect(prepared["fingerprint"]).NotTo(BeEmpty())
		agenda := prepared["agenda"].(map[string]any)
		Expect(agenda["delivery"]).To(BeEmpty())
		Expect(agenda["gaps"]).NotTo(BeEmpty())
		Expect(prepared["context"].(map[string]any)["snapshot"]).To(BeNil())
		saved := object(call("POST", base()+"/reviews", completion(prepared["fingerprint"]), claims))
		Expect(saved["preview"]).To(Equal(prepared))
		Expect(saved["outcomeNote"]).To(Equal("Confirm Atlas readiness with its owner."))
		path := base() + "/reviews/" + saved["id"].(string)
		before := object(call("GET", path, nil, claims))
		Expect(object(call("POST", base()+"/decisions/"+action["id"].(string)+"/transitions", map[string]any{"expectedVersion": 1, "status": "resolved", "evidence": "Owner confirmed acceptance."}, claims))["status"]).To(Equal("resolved"))
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 3}}), time.Now().Unix())).To(Succeed())
		after := object(call("GET", path, nil, claims))
		Expect(after).To(Equal(before))
		Expect(after["preview"].(map[string]any)["filters"]).To(Equal(previewInput()["filters"]))
		Expect(object(call("GET", base()+"/reviews", nil, claims))["reviews"]).To(HaveLen(1))
		for _, method := range []string{"PATCH", "DELETE"} {
			rec := call(method, path, map[string]any{"outcomeNote": "Rewrite"}, claims)
			Expect(rec.Code).To(BeNumerically(">=", 400))
		}
	})
	It("rejects a stale completion after an action transition, plan edit or another completed review", func() {
		action := decision()
		first := preview()
		object(call("POST", base()+"/decisions/"+action["id"].(string)+"/transitions", map[string]any{"expectedVersion": 1, "status": "in_progress", "evidence": "Started."}, claims))
		Expect(call("POST", base()+"/reviews", completion(first["fingerprint"]), claims).Code).To(Equal(409))
		second := preview()
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Team A", Tracks: 3}}), time.Now().Unix())).To(Succeed())
		Expect(call("POST", base()+"/reviews", completion(second["fingerprint"]), claims).Code).To(Equal(409))
		third := preview()
		object(call("POST", base()+"/reviews", completion(third["fingerprint"]), claims))
		Expect(call("POST", base()+"/reviews", completion(third["fingerprint"]), claims).Code).To(Equal(409))
		Expect(object(call("GET", base()+"/reviews", nil, claims))["reviews"]).To(HaveLen(1))
	})
	It("keeps agreement bytes and selected snapshot context immutable and rejects foreign references", func() {
		row, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		in, err := srv.planScheduleFor(row, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		bid := newID()
		_, err = database.SaveBaseline(db.BaselineRow{ID: bid, PlanID: plan.ID, Name: "Agreed", Inputs: encode(in), Schedule: encode(in.Recompute()), Fingerprint: in.Fingerprint(), CreatedAt: time.Now().Unix()}, true)
		Expect(err).NotTo(HaveOccurred())
		before, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		snapshot := db.SnapshotRow{ID: newID(), Owner: claims.Sub, Name: "Observed review", Source: "jira", CreatedAt: time.Now().Unix()}
		Expect(database.CreateSnapshotWithData(snapshot, db.SnapshotData{})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(snapshot.ID)).To(Succeed()) })
		body := previewInput()
		body["snapshotId"] = snapshot.ID
		prepared := object(call("POST", base()+"/reviews/preview", body, claims))
		Expect(prepared["context"].(map[string]any)["baseline"].(map[string]any)["id"]).To(Equal(bid))
		body["expectedFingerprint"], body["outcomeNote"] = prepared["fingerprint"], "Reconcile missing evidence before claiming progress."
		saved := object(call("POST", base()+"/reviews", body, claims))
		Expect(saved["snapshotId"]).To(Equal(snapshot.ID))
		Expect(saved["baselineId"]).To(Equal(bid))
		after, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		foreign := snapshot
		foreign.ID, foreign.Owner = newID(), "another-owner"
		Expect(database.CreateSnapshotWithData(foreign, db.SnapshotData{})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(foreign.ID)).To(Succeed()) })
		body = previewInput()
		body["snapshotId"] = foreign.ID
		Expect(call("POST", base()+"/reviews/preview", body, claims).Code).To(Equal(404))
		outsider := auth.Claims{Sub: "another-owner", Roles: []string{"manager"}}
		Expect(call("GET", base()+"/reviews/"+saved["id"].(string), nil, outsider).Code).To(Equal(403))
		Expect(call("POST", base()+"/reviews/preview", previewInput(), outsider).Code).To(Equal(403))
	})
	It("rejects invalid and forged completion bodies without creating history", func() {
		prepared := preview()
		for _, mutation := range []func(map[string]any){
			func(body map[string]any) { body["outcomeNote"] = " " },
			func(body map[string]any) { body["nextCheckpoint"] = "2026-02-30" },
			func(body map[string]any) { body["summary"] = map[string]any{"delivery": "invented"} },
		} {
			body := completion(prepared["fingerprint"])
			mutation(body)
			rec := call("POST", base()+"/reviews", body, claims)
			Expect(rec.Code).To(Equal(400), rec.Body.String())
		}
		Expect(object(call("GET", base()+"/reviews", nil, claims))["reviews"]).To(BeEmpty())
	})
	It("refuses cross-plan review and action identifiers even for the same owner", func() {
		action := decision()
		prepared := preview()
		review := object(call("POST", base()+"/reviews", completion(prepared["fingerprint"]), claims))
		other := plan
		other.ID, other.Name = newID(), "Beacon separate plan"
		Expect(database.CreatePlan(other)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(other.ID)).To(Succeed()) })
		otherBase := "/api/plan/" + other.ID
		Expect(call("GET", otherBase+"/reviews/"+review["id"].(string), nil, claims).Code).To(Equal(404))
		path := otherBase + "/decisions/" + action["id"].(string) + "/transitions"
		Expect(call("GET", path, nil, claims).Code).To(Equal(404))
		Expect(call("POST", path, map[string]any{"expectedVersion": 1, "status": "resolved", "evidence": "Wrong plan."}, claims).Code).To(Equal(404))
		Expect(object(call("GET", base()+"/decisions/"+action["id"].(string)+"/transitions", nil, claims))["transitions"]).To(BeEmpty())
	})
	// specs/024-weekly-execution-review.md:114: the preceding completed review
	// is part of the context, including two clients completing concurrently.
	It("atomically permits only one completion of a shared prepared context", func() {
		prepared := preview()
		payload := encode(completion(prepared["fingerprint"]))
		results := make(chan int, 2)
		start := make(chan struct{})
		var workers sync.WaitGroup
		for i := 0; i < 2; i++ {
			workers.Add(1)
			go func() {
				defer GinkgoRecover()
				defer workers.Done()
				<-start
				rec := httptest.NewRecorder()
				srv.handlePlanItem(rec, httptest.NewRequest("POST", base()+"/reviews", bytes.NewReader(payload)), claims)
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
		Expect(object(call("GET", base()+"/reviews", nil, claims))["reviews"]).To(HaveLen(1))
	})
	// specs/024-weekly-execution-review.md:115: activating a different agreement
	// invalidates the reviewed context without mutating either saved agreement.
	It("rejects a stale completion after agreement activation", func() {
		row, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		in, err := srv.planScheduleFor(row, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		firstID, secondID := newID(), newID()
		for i, id := range []string{firstID, secondID} {
			_, err = database.SaveBaseline(db.BaselineRow{ID: id, PlanID: plan.ID, Name: "Agreement " + id, Inputs: encode(in), Schedule: encode(in.Recompute()), Fingerprint: in.Fingerprint(), CreatedAt: time.Now().Unix()}, i == 0)
			Expect(err).NotTo(HaveOccurred())
		}
		prepared := preview()
		Expect(prepared["context"].(map[string]any)["baseline"].(map[string]any)["id"]).To(Equal(firstID))
		changed, err := database.SetBaselineActive(plan.ID, secondID)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(call("POST", base()+"/reviews", completion(prepared["fingerprint"]), claims).Code).To(Equal(409))
		Expect(object(call("GET", base()+"/reviews", nil, claims))["reviews"]).To(BeEmpty())
	})
	// specs/024-weekly-execution-review.md:311: completion order cannot regress
	// when two records share a timestamp and later random IDs sort lower.
	It("advances the previous-review guard for equal timestamps regardless of ID order", func() {
		ctx := context.Background()
		row, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		actions, err := database.ExecutionDecisions(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		guard := db.ReviewGuard{Plan: row, Actions: actions}
		first := db.ExecutionReview{ID: "z-" + newID(), PlanID: plan.ID, ReviewDate: "2026-11-01", Timezone: "UTC", CreatedBy: claims.Sub, CreatedAt: 1793491200, OutcomeNote: "First completed review."}
		ok, err := database.CompleteExecutionReview(ctx, first, guard)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		guard.PreviousID = first.ID
		second := first
		second.ID, second.OutcomeNote = "a-"+newID(), "Second completed review in the same second."
		ok, err = database.CompleteExecutionReview(ctx, second, guard)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		latest, err := database.LatestExecutionReview(ctx, plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(latest.ID).To(Equal(second.ID))
		stale := first
		stale.ID = "b-" + newID()
		ok, err = database.CompleteExecutionReview(ctx, stale, guard)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
		reviews, err := database.ExecutionReviews(ctx, plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(reviews).To(HaveLen(2))
		Expect(reviews[0].ID).To(Equal(second.ID))
	})
})

var _ = Describe("weekly execution review authentication", func() {
	It("requires a manager account at the existing plan route boundary", func() {
		st := newMemStore()
		srv := &server{store: st}
		for _, path := range []string{"/api/plan/plan-a/reviews", "/api/plan/plan-a/reviews/preview", "/api/plan/plan-a/decisions/action-a/transitions"} {
			anonymous := httptest.NewRecorder()
			srv.withAuth(srv.handlePlanItem, "manager")(anonymous, httptest.NewRequest("POST", path, nil))
			Expect(anonymous.Code).To(Equal(401))
			for _, role := range []string{"player", "facilitator"} {
				user, _ := st.CreateUser("Weekly "+role+newID(), []string{role}, 1)
				req := httptest.NewRequest("POST", path, nil)
				req.Header.Set("Authorization", "Bearer "+st.Token(user))
				denied := httptest.NewRecorder()
				srv.withAuth(srv.handlePlanItem, "manager")(denied, req)
				Expect(denied.Code).To(Equal(403), role)
			}
		}
	})
})
