package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Set CONWAY_TEST_DATABASE_URL to an isolated PostgreSQL database. Every test
// creates unique generic fixtures and removes only its own rows.
var _ = Describe("planning database integration", Label("database"), func() {
	var database *db.DB
	var srv *server
	var plan *db.PlanRow
	var claims auth.Claims
	marshal := func(v any) []byte { b, err := json.Marshal(v); Expect(err).NotTo(HaveOccurred()); return b }
	request := func(method, path string, body any, viewer auth.Claims) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var data []byte
		if body != nil {
			data = marshal(body)
		}
		srv.handlePlanItem(rec, httptest.NewRequest(method, path, bytes.NewReader(data)), viewer)
		return rec
	}
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set CONWAY_TEST_DATABASE_URL to run isolated PostgreSQL integration checks.")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		database, err = db.Open(ctx, url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		claims = auth.Claims{Sub: "ux-integration-owner", Roles: []string{"manager"}}
		srv = &server{db: database}
		teams, inits := planning.Demo()
		inits[0].EpicKeys = []string{"PROJ-1"}
		row := db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas integration plan", HorizonWeeks: 26, CapacityLoss: 0.1, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}
		Expect(database.CreatePlan(row)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(row.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(row.ID, marshal(teams), row.UpdatedAt)).To(Succeed())
		Expect(database.SavePlanInitiatives(row.ID, marshal(inits), row.UpdatedAt)).To(Succeed())
		Expect(database.SavePlanScheduling(row.ID, marshal(planning.DemoScheduling()), row.UpdatedAt)).To(Succeed())
		plan, err = database.GetPlan(row.ID)
		Expect(err).NotTo(HaveOccurred())
	})
	It("migrates and clones independent inputs with no inherited agreement or review history", func() {
		in, err := srv.planScheduleFor(plan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		bid := newID()
		_, err = database.SaveBaseline(db.BaselineRow{ID: bid, PlanID: plan.ID, Name: "Agreed", Inputs: marshal(in), Schedule: marshal(in.Recompute()), Fingerprint: in.Fingerprint(), CreatedAt: time.Now().Unix()}, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(database.AppendExecutionDecision(db.ExecutionDecision{ID: newID(), PlanID: plan.ID, Action: "Review baseline", Owner: "Team lead", ReviewDate: "2026-09-12", Rationale: "Confirm scope", CreatedBy: claims.Sub, CreatedAt: time.Now().Unix()})).To(Succeed())
		rec := request("POST", "/api/plan/"+plan.ID+"/scenario", map[string]any{"name": "Beacon scenario"}, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var result struct{ ID string }
		Expect(json.Unmarshal(rec.Body.Bytes(), &result)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(result.ID)).To(Succeed()) })
		clone, err := database.GetPlan(result.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(clone.Owner).To(Equal(claims.Sub))
		Expect(clone.Teams).To(Equal(plan.Teams))
		Expect(clone.Initiatives).To(Equal(plan.Initiatives))
		baselines, err := database.ListBaselines(clone.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(baselines).To(BeEmpty())
		decisions, err := database.ExecutionDecisions(clone.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(decisions).To(BeEmpty())
		Expect(database.SavePlanInitiatives(clone.ID, []byte("[]"), time.Now().Unix())).To(Succeed())
		unchanged, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.Initiatives).To(Equal(plan.Initiatives))
	})
	// specs/033-consistent-plan-controls-and-samples.md:51
	It("downloads samples from authorized plan teams without exposing real work", func() {
		path := "/api/plan/" + plan.ID + "/sample/initiatives.xlsx"
		teams := []planning.Team{{Name: "Atlas", Tracks: 1}, {Name: "Beacon", Tracks: 2}}
		Expect(database.SavePlanTeams(plan.ID, marshal(teams), time.Now().Unix())).To(Succeed())
		rec := request("GET", path, nil, claims)
		Expect(rec.Code).To(Equal(200))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("private, no-store"))
		Expect(rec.Header().Get("Content-Disposition")).To(ContainSubstring("attachment;"))
		Expect(rec.Body.Bytes()).To(Equal(planning.WriteSampleInitiativesXLSX(teams)))
		Expect(request("GET", path, nil, auth.Claims{Sub: "other-manager", Roles: []string{"manager"}}).Code).To(Equal(403))
		unauthenticated := httptest.NewRecorder()
		srv.store = newMemStore()
		srv.withAuth(srv.handlePlanItem, "manager")(unauthenticated, httptest.NewRequest("GET", path, nil))
		Expect(unauthenticated.Code).To(Equal(401))
		Expect(request("GET", "/api/plan/missing/sample/initiatives.xlsx", nil, claims).Code).To(Equal(404))
		Expect(request("POST", path, nil, claims).Code).To(Equal(405))
		Expect(database.SavePlanTeams(plan.ID, marshal([]planning.Team{}), time.Now().Unix())).To(Succeed())
		Expect(request("GET", path, nil, claims).Body.Bytes()).To(Equal(planning.WriteSampleInitiativesXLSX(nil)))
		saved, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Initiatives).To(Equal(plan.Initiatives))
	})
	It("checks roster read access before copying teams into a plan sample", func() {
		roster := db.RosterRow{ID: newID(), Owner: "another-roster-owner", Name: "Atlas private roster", Pods: marshal([]NetPod{{Name: "Cedar", Streams: 1}}), CreatedAt: time.Now().Unix()}
		Expect(database.CreateRoster(roster)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteRoster(roster.ID)).To(Succeed()) })
		path := "/api/plan/" + plan.ID + "/roster"
		Expect(request("POST", path, map[string]string{"rosterId": roster.ID}, claims).Code).To(Equal(403))
		saved, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Teams).To(Equal(plan.Teams))
		Expect(database.SetRosterPublic(roster.ID, true)).To(Succeed())
		Expect(request("POST", path, map[string]string{"rosterId": roster.ID}, claims).Code).To(Equal(200))
		Expect(request("GET", "/api/plan/"+plan.ID+"/sample/initiatives.xlsx", nil, claims).Body.Bytes()).To(Equal(planning.WriteSampleInitiativesXLSX([]planning.Team{{Name: "Cedar", Tracks: 1}})))
	})
	It("reads complete snapshot evidence, protects scope, and persists append-only review decisions", func() {
		in, err := srv.planScheduleFor(plan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		bid := newID()
		_, err = database.SaveBaseline(db.BaselineRow{ID: bid, PlanID: plan.ID, Name: "Agreed", Inputs: marshal(in), Schedule: marshal(in.Recompute()), Fingerprint: in.Fingerprint(), CreatedAt: time.Now().Unix()}, true)
		Expect(err).NotTo(HaveOccurred())
		captured := time.Now()
		created := captured.AddDate(0, 0, -21)
		resolved := captured.AddDate(0, 0, -7)
		snapshot := db.SnapshotRow{ID: newID(), Owner: claims.Sub, Name: "Atlas observed sample", Source: "jira", CreatedAt: captured.Unix()}
		issues := []db.IssueRow{{Key: "PROJ-1", IssueType: "Epic", Summary: in.Initiatives[0].Name}, {Key: "PROJ-2", ParentKey: "PROJ-1", Pod: in.Teams[0].Name, IssueType: "Story", StatusCat: "done", Created: &created, Resolved: &resolved}}
		for i := 0; i < 601; i++ {
			issues = append(issues, db.IssueRow{Key: fmt.Sprintf("PROJ-%d", 100+i), Pod: in.Teams[0].Name, IssueType: "Story", StatusCat: "new"})
		}
		Expect(database.CreateSnapshotWithData(snapshot, db.SnapshotData{Issues: issues})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(snapshot.ID)).To(Succeed()) })
		baseBefore, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		rec := request("GET", "/api/plan/"+plan.ID+"/actuals?snapshot="+snapshot.ID, nil, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var actuals struct {
			Snapshot map[string]any
			Baseline map[string]any
			Coverage planning.ExecutionCoverage
		}
		Expect(json.Unmarshal(rec.Body.Bytes(), &actuals)).To(Succeed())
		Expect(actuals.Snapshot["source"]).To(Equal("jira"))
		Expect(actuals.Baseline["periodStart"]).To(Equal(in.Scheduling.PeriodStart))
		Expect(actuals.Coverage.Bound).To(Equal(1))
		counts, err := database.HygieneIssueCounts(snapshot.ID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(counts["unsized"]).To(Equal(601))
		denied := request("GET", "/api/plan/"+plan.ID+"/actuals?snapshot="+snapshot.ID, nil, auth.Claims{Sub: "other-owner"})
		Expect(denied.Code).To(Equal(403))
		foreign := snapshot
		foreign.ID = newID()
		foreign.Owner = "other-owner"
		Expect(database.CreateSnapshotWithData(foreign, db.SnapshotData{})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(foreign.ID)).To(Succeed()) })
		denied = request("GET", "/api/plan/"+plan.ID+"/actuals?snapshot="+foreign.ID, nil, claims)
		Expect(denied.Code).To(Equal(404))
		for _, action := range []string{"Review dependency", "Confirm next checkpoint"} {
			rec = request("POST", "/api/plan/"+plan.ID+"/decisions", db.ExecutionDecision{Action: action, Owner: "Team lead", ReviewDate: "2026-09-12", Rationale: "Confirm upstream readiness", SnapshotID: snapshot.ID, BaselineID: bid}, claims)
			Expect(rec.Code).To(Equal(200), rec.Body.String())
		}
		history, err := database.ExecutionDecisions(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(2))
		Expect(history[0].CreatedBy).To(Equal(claims.Sub))
		baseAfter, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		Expect(baseAfter).To(Equal(baseBefore))
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(plan))
		rec = request("PATCH", "/api/plan/"+plan.ID+"/initiatives", map[string]any{"initiatives": []map[string]any{{"name": in.Initiatives[0].Name, "epicKeys": []string{"PROJ-1", "PROJ-3"}}}}, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		edited, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		var editedInits []planning.Initiative
		Expect(json.Unmarshal(edited.Initiatives, &editedInits)).To(Succeed())
		Expect(editedInits[0].EpicKeys).To(Equal([]string{"PROJ-1", "PROJ-3"}))
		unchangedBase, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchangedBase).To(Equal(baseBefore))
	})
	It("applies only the previewed current proposal and rejects concurrent same-second edits", func() {
		in, err := srv.planScheduleFor(plan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		choices := planning.ComputeRemedies(in.Teams, in.Initiatives, in.Params, in.Scheduling, nil)
		Expect(choices).NotTo(BeEmpty())
		body := remedyProposalRequest{Remedy: choices[0], Fingerprint: in.Fingerprint()}
		rec := request("POST", "/api/plan/"+plan.ID+"/schedule/remedies/preview", body, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		afterPreview, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterPreview).To(Equal(plan))
		proposal, _, _, err := planning.PreviewRemedy(in, choices[0])
		Expect(err).NotTo(HaveOccurred())
		rec = request("POST", "/api/plan/"+plan.ID+"/schedule/remedies/apply", body, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		applied, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		var actual []planning.Initiative
		Expect(json.Unmarshal(applied.Initiatives, &actual)).To(Succeed())
		Expect(actual).To(Equal(proposal.Initiatives))
		ok, err := database.SavePlanProposalIfUnchanged(plan, plan.Teams, plan.Initiatives, plan.UpdatedAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse(), "stale pre-apply row must not overwrite the applied change")
		Expect(database.SavePlanInitiatives(plan.ID, []byte("[]"), applied.UpdatedAt)).To(Succeed())
		ok, err = database.SavePlanProposalIfUnchanged(applied, applied.Teams, applied.Initiatives, applied.UpdatedAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse(), "same-second edits must be caught by JSON equality")
		rec = request("POST", "/api/plan/"+plan.ID+"/schedule/remedies/apply", body, claims)
		Expect(rec.Code).To(Equal(409), rec.Body.String())
	})
})
