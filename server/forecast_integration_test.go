package main

import (
	"bytes"
	"context"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
	"os"
	"time"
)

var _ = Describe("portfolio forecast API", Label("database"), func() {
	var database *db.DB
	var pool *pgxpool.Pool
	var srv *server
	var plan db.PlanRow
	var claims auth.Claims
	encode := func(v any) []byte { b, e := json.Marshal(v); Expect(e).NotTo(HaveOccurred()); return b }
	call := func(method, body string, c auth.Claims) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		srv.handlePlanItem(r, httptest.NewRequest(method, "/api/plan/"+plan.ID+"/forecast", bytes.NewBufferString(body)), c)
		return r
	}
	const settings = `{"lowerFactor":0.8,"upperFactor":1.3,"disruption":0.1}`
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set isolated PostgreSQL test database")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		pool, err = pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		claims = auth.Claims{Sub: "forecast-owner-" + newID(), Roles: []string{"manager"}}
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,'Forecast fixture','manager',ARRAY['manager'],'salt','hash',0,$2)`, claims.Sub, time.Now().Unix())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, err := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, claims.Sub)
			Expect(err).NotTo(HaveOccurred())
		})
		now := time.Now().Unix()
		plan = db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas portfolio", HorizonWeeks: 12, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(plan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(plan.ID)).To(Succeed()) })
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Atlas", Tracks: 1}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Beacon", KitPct: 1, Work: map[string]planning.TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		Expect(database.SavePlanScheduling(plan.ID, encode(planning.SchedulingParams{WipModel: "strict", EstimateModel: "effort", PeriodStart: "2026-09-07"}), now)).To(Succeed())
		srv = &server{db: database}
	})
	It("returns reproducible scenarios without changing saved inputs", func() {
		before, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		r := call("POST", settings, claims)
		Expect(r.Code).To(Equal(200), r.Body.String())
		Expect(r.Header().Get("Cache-Control")).To(Equal("no-store"))
		var value planning.PortfolioForecast
		Expect(json.Unmarshal(r.Body.Bytes(), &value)).To(Succeed())
		Expect(value.Scenarios).To(HaveLen(3))
		Expect(value.Fingerprint).NotTo(BeEmpty())
		again := call("POST", settings, claims)
		Expect(again.Body.String()).To(Equal(r.Body.String()))
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	It("refuses malformed, extra, missing and out-of-range settings", func() {
		for _, body := range []string{"null", "{}", settings + settings, `{"lowerFactor":0.8,"upperFactor":1.3,"disruption":0.1,"apply":true}`, `{"lowerFactor":1,"upperFactor":1,"disruption":1}`} {
			r := call("POST", body, claims)
			Expect(r.Code).To(Equal(400), r.Body.String())
		}
		Expect(call("GET", "", claims).Code).To(Equal(405))
	})
	It("rejects other owners, game identities and revoked roles", func() {
		for _, c := range []auth.Claims{{Sub: "other", Roles: []string{"manager"}}, {Sub: claims.Sub, Roles: []string{"player"}}, {Sub: claims.Sub, Roles: claims.Roles, GameID: "game"}} {
			Expect(call("POST", settings, c).Code).To(Equal(403))
		}
		_, err := pool.Exec(context.Background(), `UPDATE accounts SET roles=ARRAY['player'],role='player' WHERE username=$1`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		Expect(call("POST", settings, claims).Code).To(Equal(403))
	})
	It("separates unreadable saved inputs from actionable validation errors", func() {
		for _, field := range []string{"teams", "initiatives"} {
			before, err := database.GetPlan(plan.ID)
			Expect(err).NotTo(HaveOccurred())
			if field == "teams" {
				Expect(database.SavePlanTeams(plan.ID, []byte(`{"internal":"unreadable"}`), before.UpdatedAt)).To(Succeed())
			} else {
				Expect(database.SavePlanInitiatives(plan.ID, []byte(`{"internal":"unreadable"}`), before.UpdatedAt)).To(Succeed())
			}
			r := call("POST", settings, claims)
			Expect(r.Code).To(Equal(500), r.Body.String())
			Expect(r.Body.String()).To(Equal("Saved planning inputs could not be read. Retry or contact an administrator.\n"))
			Expect(database.SavePlanTeams(plan.ID, before.Teams, before.UpdatedAt)).To(Succeed())
			Expect(database.SavePlanInitiatives(plan.ID, before.Initiatives, before.UpdatedAt)).To(Succeed())
		}
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Beacon"}, {Name: "Beacon"}}), time.Now().Unix())).To(Succeed())
		r := call("POST", settings, claims)
		Expect(r.Code).To(Equal(400), r.Body.String())
		Expect(r.Body.String()).To(ContainSubstring("Beacon"))
	})
	It("does not present an account-query outage as forbidden access", func() {
		unavailable, err := db.Open(context.Background(), os.Getenv("CONWAY_TEST_DATABASE_URL"))
		Expect(err).NotTo(HaveOccurred())
		unavailable.Close()
		unavailableServer := &server{db: unavailable}
		r := httptest.NewRecorder()
		unavailableServer.handleForecast(r, httptest.NewRequest("POST", "/api/plan/"+plan.ID+"/forecast", bytes.NewBufferString(settings)), &plan, claims)
		Expect(r.Code).To(Equal(500), r.Body.String())
		Expect(r.Body.String()).To(Equal("Account access could not be checked. Retry when the service is available.\n"))
	})
})
