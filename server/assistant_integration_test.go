package main

import (
	"bytes"
	"context"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// specs/027-evidence-linked-planning-assistant.md:46
var _ = Describe("planning assistant evidence", Label("database"), func() {
	var database *db.DB
	var pool *pgxpool.Pool
	var srv *server
	var plan db.PlanRow
	var claims auth.Claims
	encode := func(v any) []byte { b, e := json.Marshal(v); Expect(e).NotTo(HaveOccurred()); return b }
	call := func(method string, body any, c auth.Claims) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var data []byte
		if body != nil {
			data = encode(body)
		}
		srv.handlePlanItem(rec, httptest.NewRequest(method, "/api/plan/"+plan.ID+"/assistant", bytes.NewReader(data)), c)
		return rec
	}
	input := func(task string) map[string]any {
		return map[string]any{"task": task, "initiative": "Beacon", "reviewDate": "2026-09-14", "timezone": "UTC", "snapshotId": "manual"}
	}
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set isolated PostgreSQL test database")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		claims = auth.Claims{Sub: "assistant-owner-" + newID(), Roles: []string{"manager"}}
		pool, err = pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,'Assistant fixture','manager',ARRAY['manager'],'salt','hash',0,$2)`, claims.Sub, time.Now().Unix())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, e := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, claims.Sub)
			Expect(e).NotTo(HaveOccurred())
		})
		srv = &server{db: database}
		now := time.Now().Unix()
		plan = db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas portfolio", HorizonWeeks: 12, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(plan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(plan.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Atlas", Tracks: 1}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Beacon", Work: map[string]planning.TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		Expect(database.SavePlanScheduling(plan.ID, encode(planning.SchedulingParams{WipModel: "strict", EstimateModel: "effort", PeriodStart: "2026-09-07"}), now)).To(Succeed())
	})
	It("explains saved schedule results without changing any planning inputs", func() {
		before, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		rec := call("POST", input("schedule"), claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		Expect(rec.Body.String()).To(ContainSubstring("Modeled"))
		Expect(rec.Body.String()).To(ContainSubstring("Beacon"))
		Expect(rec.Body.String()).To(ContainSubstring("planView=timeline"))
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	It("prepares a manual agenda without inventing measurements or saving a review", func() {
		rec := call("POST", input("review"), claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		Expect(rec.Body.String()).To(ContainSubstring("Manual"))
		reviews, err := database.ExecutionReviews(context.Background(), plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(reviews).To(BeEmpty())
		decisions, err := database.ExecutionDecisions(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(decisions).To(BeEmpty())
	})
	It("reports the missing agreement instead of inventing a change comparison", func() {
		rec := call("POST", input("changes"), claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		Expect(rec.Body.String()).To(ContainSubstring("No active agreement"))
	})
	It("refuses unknown scope and unsupported tasks", func() {
		for _, field := range []string{"team", "initiative", "task"} {
			body := input("schedule")
			body[field] = "Not a saved choice"
			rec := call("POST", body, claims)
			Expect(rec.Code).To(Equal(400), rec.Body.String())
		}
	})
	It("requires an explicit manual or snapshot choice for review", func() {
		body := input("review")
		delete(body, "snapshotId")
		rec := call("POST", body, claims)
		Expect(rec.Code).To(Equal(400), rec.Body.String())
	})
	It("refuses stale saved context", func() {
		body := input("schedule")
		body["expectedFingerprint"] = "obsolete"
		rec := call("POST", body, claims)
		Expect(rec.Code).To(Equal(409), rec.Body.String())
	})
	It("refuses other owners and game or nonmanager identities", func() {
		for _, c := range []auth.Claims{{Sub: "another-owner", Roles: []string{"manager"}}, {Sub: claims.Sub, Roles: []string{"player"}}, {Sub: claims.Sub, Roles: []string{"manager"}, GameID: "game"}} {
			rec := call("POST", input("schedule"), c)
			Expect(rec.Code).To(Equal(403), rec.Body.String())
			Expect(rec.Body.String()).NotTo(ContainSubstring("Atlas portfolio"))
		}
	})
	It("does not reinterpret a forbidden snapshot as a manual review", func() {
		body := input("review")
		body["snapshotId"] = "missing-private-capture"
		rec := call("POST", body, claims)
		Expect(rec.Code).NotTo(Equal(200))
	})
	It("does not call an external service for an unconsented question", func() {
		body := input("question")
		body["question"] = "Explain Beacon"
		rec := call("POST", body, claims)
		Expect(rec.Code).To(BeElementOf(400, 503))
		Expect(rec.Body.String()).NotTo(ContainSubstring("Modeled"))
	})
	It("refuses a demoted account before using old token roles", func() {
		_, err := pool.Exec(context.Background(), `UPDATE accounts SET roles=ARRAY['player'],role='player' WHERE username=$1`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		for _, method := range []string{"GET", "POST"} {
			rec := call(method, input("schedule"), claims)
			Expect(rec.Code).To(Equal(403))
			Expect(rec.Body.String()).NotTo(ContainSubstring("Atlas portfolio"))
		}
	})
	It("does not treat a nonparticipating team cell as assigned work", func() {
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Beacon", Work: map[string]planning.TeamWork{"Atlas": {InPath: false}}}}), time.Now().Unix())).To(Succeed())
		body := input("schedule")
		body["team"] = "Atlas"
		rec := call("POST", body, claims)
		Expect(rec.Code).To(Equal(400), rec.Body.String())
	})

	for _, invalid := range []string{"blank question", "too many initiatives", "too many teams", "oversized names", "oversized names while busy"} {
		It("returns an input error without contacting the provider for "+invalid, func() {
			var calls atomic.Int32
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer host.Close()
			srv.assistantModel = &assistantInterpreter{endpoint: host.URL, client: host.Client(), slots: make(chan struct{}, 4)}
			if invalid == "oversized names while busy" {
				for range cap(srv.assistantModel.slots) {
					srv.assistantModel.slots <- struct{}{}
				}
			}
			body := input("question")
			body["initiative"], body["question"], body["allowExternal"] = "", "Explain the schedule", true
			switch invalid {
			case "blank question":
				body["question"] = " \t\n "
			case "too many initiatives", "oversized names", "oversized names while busy":
				count := 301
				if strings.HasPrefix(invalid, "oversized names") {
					count = 200
				}
				initiatives := make([]planning.Initiative, count)
				for i := range initiatives {
					name := fmt.Sprintf("Initiative %d", i)
					if strings.HasPrefix(invalid, "oversized names") {
						name += strings.Repeat("x", 700)
					}
					initiatives[i] = planning.Initiative{Name: name}
				}
				Expect(database.SavePlanInitiatives(plan.ID, encode(initiatives), time.Now().Unix())).To(Succeed())
			case "too many teams":
				teams := make([]planning.Team, 301)
				for i := range teams {
					teams[i] = planning.Team{Name: fmt.Sprintf("Team %d", i), Tracks: 1}
				}
				Expect(database.SavePlanTeams(plan.ID, encode(teams), time.Now().Unix())).To(Succeed())
			}
			rec := call("POST", body, claims)
			Expect(rec.Code).To(Equal(http.StatusBadRequest), rec.Body.String())
			Expect(calls.Load()).To(BeZero())
		})
	}
	It("retains service-unavailable status for an actual provider outage", func() {
		var calls atomic.Int32
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer host.Close()
		srv.assistantModel = &assistantInterpreter{endpoint: host.URL, client: host.Client(), slots: make(chan struct{}, 4)}
		body := input("question")
		body["question"], body["allowExternal"] = "Explain Beacon", true
		Expect(call("POST", body, claims).Code).To(Equal(http.StatusServiceUnavailable))
		Expect(calls.Load()).To(Equal(int32(1)))
	})
	It("uses configured interpretation only with consent and returns canonical schedule facts", func() {
		var calls atomic.Int32
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			calls.Add(1)
			writeJSON(w, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(encode(map[string]string{"task": "schedule", "initiative": "Beacon", "team": "Atlas"}))}}}}})
		}))
		defer host.Close()
		srv.assistantModel = &assistantInterpreter{endpoint: host.URL, key: "fixture", model: "fixture", client: host.Client(), slots: make(chan struct{}, 4)}
		body := input("question")
		body["question"] = "Explain Beacon"
		Expect(call("POST", body, claims).Code).To(Equal(400))
		Expect(calls.Load()).To(BeZero())
		body["allowExternal"] = true
		rec := call("POST", body, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		Expect(calls.Load()).To(Equal(int32(1)))
		var interpreted, direct assistantAnswer
		Expect(json.Unmarshal(rec.Body.Bytes(), &interpreted)).To(Succeed())
		plain := input("schedule")
		plain["team"] = "Atlas"
		Expect(json.Unmarshal(call("POST", plain, claims).Body.Bytes(), &direct)).To(Succeed())
		Expect(interpreted.Facts).To(Equal(direct.Facts))
	})
	for _, change := range []string{"plan", "access"} {
		It("rejects changed "+change+" after a model wait", func() {
			started, release := make(chan struct{}), make(chan struct{})
			var released atomic.Bool
			DeferCleanup(func() {
				if released.CompareAndSwap(false, true) {
					close(release)
				}
			})
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				case <-time.After(5 * time.Second):
					return
				}
				writeJSON(w, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "{\"task\":\"schedule\",\"initiative\":\"Beacon\",\"team\":\"Atlas\"}"}}}}})
			}))
			defer host.Close()
			srv.assistantModel = &assistantInterpreter{endpoint: host.URL, key: "fixture", model: "fixture", client: host.Client(), slots: make(chan struct{}, 4)}
			body := input("question")
			body["question"] = "Explain Beacon"
			body["allowExternal"] = true
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { defer GinkgoRecover(); result <- call("POST", body, claims) }()
			Eventually(started, 3*time.Second).Should(BeClosed())
			expected := 409
			if change == "plan" {
				Expect(database.SavePlanScheduling(plan.ID, encode(planning.SchedulingParams{WipModel: "strict", PeriodStart: "2026-10-05"}), time.Now().Unix())).To(Succeed())
			} else {
				_, err := pool.Exec(context.Background(), `UPDATE accounts SET role='player',roles=ARRAY['player'] WHERE username=$1`, claims.Sub)
				Expect(err).NotTo(HaveOccurred())
				expected = 403
			}
			if released.CompareAndSwap(false, true) {
				close(release)
			}
			var response *httptest.ResponseRecorder
			Eventually(result, 3*time.Second).Should(Receive(&response))
			Expect(response.Code).To(Equal(expected), response.Body.String())
			Expect(response.Body.String()).NotTo(ContainSubstring("Modeled start"))
		})
	}
	It("retains the review evidence fingerprint and exact scoped agenda", func() {
		snapshot := newID()
		Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: snapshot, Owner: claims.Sub, Name: "Observed Atlas", Source: "jira", CreatedAt: time.Now().Unix()}, db.SnapshotData{Issues: []db.IssueRow{{Key: "PROJ-1", IssueType: "Epic", StatusCat: "indeterminate", Pod: "Atlas"}, {Key: "PROJ-2", IssueType: "Story", ParentKey: "PROJ-1", StatusCat: "indeterminate", Pod: "Atlas"}}})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(snapshot)).To(Succeed()) })
		body := input("review")
		body["snapshotId"] = snapshot
		body["team"] = "Atlas"
		rec := call("POST", body, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var answer assistantAnswer
		Expect(json.Unmarshal(rec.Body.Bytes(), &answer)).To(Succeed())
		saved, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		expected, _, code, _ := srv.prepareWeeklyReview(context.Background(), saved, claims, weeklyReviewRequest{ReviewDate: "2026-09-14", Timezone: "UTC", SnapshotID: snapshot, Filters: planning.ReviewFilters{Team: "Atlas", Initiative: "Beacon"}})
		Expect(code).To(Equal(200))
		Expect(answer.Context.EvidenceFingerprint).To(Equal(expected.Fingerprint))
		Expect(*answer.Context.Review).To(Equal(expected.Context))
		Expect(answer.Facts).To(HaveLen(len(expected.Agenda.Delivery) + len(expected.Agenda.Gaps) + len(expected.Actions)))
	})

})
