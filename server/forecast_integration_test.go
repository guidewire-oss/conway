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
	"net/http/httptest"
	"os"
	"strings"
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
	It("records immutable predictions, retries without duplicates and protects captured evidence", func() {
		callPath := func(method, path, body string, c auth.Claims) *httptest.ResponseRecorder {
			r := httptest.NewRecorder()
			srv.handlePlanItem(r, httptest.NewRequest(method, "/api/plan/"+plan.ID+path, bytes.NewBufferString(body)), c)
			return r
		}
		sourceID, snapshotID := newID(), newID()
		now := time.Now().Unix()
		_, err := pool.Exec(context.Background(), `INSERT INTO evidence_sources(id,owner,config,credential,next_at) VALUES($1,$2,'{"site":"https://atlas.atlassian.net","projects":["PROJ"]}','',0)`, sourceID, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, e := pool.Exec(context.Background(), `DELETE FROM evidence_sources WHERE id=$1`, sourceID)
			Expect(e).NotTo(HaveOccurred())
		})
		Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: snapshotID, Owner: claims.Sub, Name: "Atlas capture", Source: "jira", CreatedAt: now - 60}, db.SnapshotData{})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(snapshotID)).To(Succeed()) })
		_, err = pool.Exec(context.Background(), `INSERT INTO evidence_runs(id,source_id,status,started_at,finished_at,snapshot_id,config,version) SELECT $1,id,'succeeded',$3,$4,$5,config,1 FROM evidence_sources WHERE id=$2`, newID(), sourceID, now-120, now-60, snapshotID)
		Expect(err).NotTo(HaveOccurred())
		comparison := call("POST", settings, claims)
		Expect(comparison.Code).To(Equal(200))
		var f planning.PortfolioForecast
		Expect(json.Unmarshal(comparison.Body.Bytes(), &f)).To(Succeed())
		body := string(encode(map[string]any{"id": "prediction-fixture", "name": "September check", "settings": f.Settings, "fingerprint": f.Fingerprint, "snapshotId": snapshotID}))
		saved := callPath("POST", "/predictions", body, claims)
		Expect(saved.Code).To(Equal(200), saved.Body.String())
		Expect(callPath("POST", "/predictions", body, claims).Body.String()).To(Equal(saved.Body.String()))
		history := callPath("GET", "/predictions", "", claims)
		Expect(history.Code).To(Equal(200))
		Expect(history.Body.String()).To(ContainSubstring("September check"))
		Expect(callPath("GET", "/predictions/prediction-fixture", "", claims).Code).To(Equal(200))
		Expect(callPath("PATCH", "/predictions/prediction-fixture", "{}", claims).Code).To(Equal(405))
		conflict := string(encode(map[string]any{"id": "prediction-fixture", "name": "Other name", "settings": f.Settings, "fingerprint": f.Fingerprint, "snapshotId": snapshotID}))
		Expect(callPath("POST", "/predictions", conflict, claims).Code).To(Equal(409))
		before, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(database.SavePlanTeams(plan.ID, encode([]planning.Team{{Name: "Atlas", Tracks: 2}}), before.UpdatedAt)).To(Succeed())
		stale := string(encode(map[string]any{"id": "prediction-stale", "name": "Old comparison", "settings": f.Settings, "fingerprint": f.Fingerprint, "snapshotId": snapshotID}))
		Expect(callPath("POST", "/predictions", stale, claims).Code).To(Equal(409))
		Expect(callPath("GET", "/predictions/prediction-fixture", "", claims).Body.String()).To(Equal(saved.Body.String()))
		Expect(callPath("GET", "/predictions/absent", "", claims).Code).To(Equal(404))
		other := auth.Claims{Sub: "prediction-other-" + newID(), Roles: []string{"manager"}}
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,'Other manager','manager',ARRAY['manager'],'salt','hash',0,$2)`, other.Sub, now)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, e := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, other.Sub)
			Expect(e).NotTo(HaveOccurred())
		})
		otherPlan := db.PlanRow{ID: newID(), Owner: other.Sub, Name: "Other portfolio", HorizonWeeks: 12, CreatedAt: now}
		Expect(database.CreatePlan(otherPlan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(otherPlan.ID)).To(Succeed()) })
		Expect(callPath("GET", "/predictions", "", other).Code).To(Equal(403))
		for _, path := range []string{"/predictions/prediction-fixture", "/predictions/prediction-fixture/assessment"} {
			method := "GET"
			if strings.HasSuffix(path, "assessment") {
				method = "POST"
			}
			r := httptest.NewRecorder()
			srv.handlePlanItem(r, httptest.NewRequest(method, "/api/plan/"+otherPlan.ID+path, bytes.NewBufferString(`{"snapshotId":"`+snapshotID+`"}`)), other)
			Expect(r.Code).To(Equal(404))
		}
		assessed := callPath("POST", "/predictions/prediction-fixture/assessment", `{"snapshotId":"`+snapshotID+`"}`, claims)
		Expect(assessed.Code).To(Equal(200))
		Expect(assessed.Body.String()).To(ContainSubstring("Choose a capture started after"))
		stored, err := database.Prediction(context.Background(), plan.ID, "prediction-fixture")
		Expect(err).NotTo(HaveOccurred())
		currentPlan, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 51; i++ {
			copied := *stored
			copied.ID = newID()
			inserted, e := database.SavePrediction(context.Background(), currentPlan, copied)
			Expect(e).NotTo(HaveOccurred())
			Expect(inserted).To(BeTrue())
		}
		var firstPage struct {
			Predictions []db.PredictionRow `json:"predictions"`
			Next        int64              `json:"next"`
		}
		Expect(json.Unmarshal(callPath("GET", "/predictions", "", claims).Body.Bytes(), &firstPage)).To(Succeed())
		Expect(firstPage.Predictions).To(HaveLen(50))
		Expect(firstPage.Next).To(BeNumerically(">", 0))
		var secondPage struct {
			Predictions []db.PredictionRow `json:"predictions"`
			Next        int64              `json:"next"`
		}
		Expect(json.Unmarshal(callPath("GET", fmt.Sprintf("/predictions?before=%d", firstPage.Next), "", claims).Body.Bytes(), &secondPage)).To(Succeed())
		Expect(secondPage.Predictions).To(HaveLen(2))
		Expect(secondPage.Next).To(BeZero())
		Expect(secondPage.Predictions[1].ID).To(Equal("prediction-fixture"))
		_, err = pool.Exec(context.Background(), `UPDATE snapshots SET owner='other',public=false WHERE id=$1`, snapshotID)
		Expect(err).NotTo(HaveOccurred())
		Expect(callPath("GET", "/predictions/prediction-fixture", "", claims).Code).To(Equal(404))
		_, err = pool.Exec(context.Background(), `UPDATE accounts SET roles=ARRAY['player'],role='player' WHERE username=$1`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		Expect(callPath("GET", "/predictions", "", claims).Code).To(Equal(403))
	})
})
