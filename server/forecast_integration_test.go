package main

import (
	"bytes"
	"context"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"crypto/sha256"
	"encoding/hex"
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
	It("fingerprints extraction changes while ignoring operational and future metadata", func() {
		ctx := context.Background()
		sourceID, snapshotID, runID := newID(), newID(), newID()
		_, err := pool.Exec(ctx, `INSERT INTO evidence_sources(id,owner,config,credential,next_at) VALUES($1,$2,'{}','',0)`, sourceID, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, err := pool.Exec(ctx, `DELETE FROM evidence_sources WHERE id=$1`, sourceID)
			Expect(err).NotTo(HaveOccurred())
		})
		_, err = pool.Exec(ctx, `INSERT INTO evidence_runs(id,source_id,status,started_at,finished_at,snapshot_id,config,version) VALUES($1,$2,'succeeded',1,2,$3,'{}',1)`, runID, sourceID, snapshotID)
		Expect(err).NotTo(HaveOccurred())
		fingerprint := func(config map[string]any) string {
			_, err := pool.Exec(ctx, `UPDATE evidence_runs SET config=$2::jsonb WHERE id=$1`, runID, encode(config))
			Expect(err).NotTo(HaveOccurred())
			value, err := database.PredictionSnapshotSource(ctx, snapshotID)
			Expect(err).NotTo(HaveOccurred())
			Expect(value).NotTo(BeNil())
			return value.Fingerprint
		}
		for _, config := range []map[string]any{
			{}, {"name": "Atlas source", "enabled": true},
			{"site": "https://atlas.atlassian.net", "projects": []string{"PROJ"}},
			{"site": "https://atlas.atlassian.net", "projects": []string{"PROJ"}, "rosterId": "atlas-roster", "roster": map[string]any{"pods": []string{"Atlas"}}, "wipMode": "strict", "podField": "team", "teams": []string{"Atlas"}},
		} {
			baseline := fingerprint(config)
			var legacy []byte
			Expect(pool.QueryRow(ctx, `SELECT (config - ARRAY['name','intervalHours','freshnessHours','enabled'])::text FROM evidence_runs WHERE id=$1`, runID).Scan(&legacy)).To(Succeed())
			sum := sha256.Sum256(legacy)
			Expect(baseline).To(Equal(hex.EncodeToString(sum[:])), "Existing fingerprints stay compatible")
			for _, field := range []string{"name", "intervalHours", "freshnessHours", "enabled", "futureOperationalMetadata"} {
				changed := map[string]any{}
				for k, v := range config {
					changed[k] = v
				}
				changed[field] = "changed metadata"
				Expect(fingerprint(changed)).To(Equal(baseline), field)
			}
			for _, field := range []string{"site", "projects", "rosterId", "roster", "wipMode", "podField", "teams"} {
				changed := map[string]any{}
				for k, v := range config {
					changed[k] = v
				}
				changed[field] = "changed extraction"
				Expect(fingerprint(changed)).NotTo(Equal(baseline), field)
			}
		}
	})
	It("evaluates held-out forecasts with archived inputs and enforces strict requests and capture access", func() {
		f := seedEvaluationFixture(database, pool, claims.Sub)
		endpoint := "/api/plan/" + f.Plan + "/predictions/" + f.Reference + "/evaluation"
		body := string(encode(map[string]string{"trainingSnapshotId": f.Training, "snapshotId": f.Later}))
		request := func(method, path, body string, c auth.Claims) *httptest.ResponseRecorder {
			r := httptest.NewRecorder()
			srv.handlePlanItem(r, httptest.NewRequest(method, path, strings.NewReader(body)), c)
			return r
		}
		before, err := database.GetPlan(f.Plan)
		Expect(err).NotTo(HaveOccurred())
		r := request("POST", endpoint, body, claims)
		Expect(r.Code).To(Equal(200), r.Body.String())
		Expect(r.Header().Get("Cache-Control")).To(Equal("no-store"))
		var report planning.ForecastEvaluation
		Expect(json.Unmarshal(r.Body.Bytes(), &report)).To(Succeed())
		Expect(report.Training.Eligible).To(Equal(1))
		Expect(report.Test.Eligible).To(Equal(1))
		Expect(*report.Probability).To(BeNumerically("~", 2.0/3))
		Expect(*report.BrierScore).To(BeNumerically("~", 1.0/9))
		Expect(database.SavePlanInitiatives(f.Plan, []byte(`[]`), before.UpdatedAt)).To(Succeed())
		Expect(request("POST", endpoint, body, claims).Body.String()).To(Equal(r.Body.String()), "Current edits do not refit archived training evidence")
		Expect(database.SavePlanInitiatives(f.Plan, before.Initiatives, before.UpdatedAt)).To(Succeed())
		after, err := database.GetPlan(f.Plan)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		for _, bad := range []string{"null", "{}", body + body, strings.TrimSuffix(body, "}") + `,"apply":true}`} {
			Expect(request("POST", endpoint, bad, claims).Code).To(BeNumerically(">=", 400), bad)
		}
		Expect(request("GET", endpoint, body, claims).Code).To(Equal(405))
		Expect(request("POST", endpoint+"/unknown", body, claims).Code).To(Equal(405))
		reversed := string(encode(map[string]string{"trainingSnapshotId": f.Later, "snapshotId": f.Training}))
		Expect(request("POST", endpoint, reversed, claims).Code).To(Equal(400))
		for _, c := range []auth.Claims{{Sub: "other", Roles: []string{"manager"}}, {Sub: claims.Sub, Roles: []string{"player"}}, {Sub: claims.Sub, Roles: claims.Roles, GameID: "game"}} {
			Expect(request("POST", endpoint, body, c).Code).To(Equal(403))
		}
		for _, id := range []string{f.Initial, f.Training, f.Later} {
			_, err = pool.Exec(context.Background(), `UPDATE snapshots SET owner='other',public=false WHERE id=$1`, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(request("POST", endpoint, body, claims).Code).To(Equal(404))
			_, err = pool.Exec(context.Background(), `UPDATE snapshots SET owner=$2 WHERE id=$1`, id, claims.Sub)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(request("POST", endpoint, body, claims).Code).To(Equal(200))
		_, err = pool.Exec(context.Background(), `UPDATE accounts SET roles=ARRAY['player'],role='player' WHERE username=$1`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", endpoint, body, claims).Code).To(Equal(403))
	})
	It("validates the full history with current evidence access and rejects partial oversized reports", func() {
		ctx := context.Background()
		now := time.Now().Unix()
		sourceID, initialID, otherID, laterID := newID(), newID(), newID(), newID()
		_, err := pool.Exec(ctx, `INSERT INTO evidence_sources(id,owner,config,credential,next_at) VALUES($1,$2,'{"site":"https://atlas.atlassian.net","projects":["PROJ"]}','',0)`, sourceID, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, e := pool.Exec(ctx, `DELETE FROM evidence_sources WHERE id=$1`, sourceID)
			Expect(e).NotTo(HaveOccurred())
		})
		for _, id := range []string{initialID, otherID, laterID} {
			Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: id, Owner: claims.Sub, Name: "Atlas capture", Source: "jira", CreatedAt: now - 1}, db.SnapshotData{})).To(Succeed())
			DeferCleanup(func() { Expect(database.DeleteSnapshot(id)).To(Succeed()) })
			start, finish := now-120, now-60
			if id == laterID {
				start, finish = now-2, now-1
			}
			_, err = pool.Exec(ctx, `INSERT INTO evidence_runs(id,source_id,status,started_at,finished_at,snapshot_id,config,version) SELECT $1,id,'succeeded',$3,$4,$5,config,1 FROM evidence_sources WHERE id=$2`, newID(), sourceID, start, finish, id)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(database.SavePlanInitiatives(plan.ID, encode([]planning.Initiative{{Name: "Beacon", EpicKeys: []string{"PROJ-1"}, KitPct: 1, Work: map[string]planning.TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		savedPlan, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		inputs, err := srv.planScheduleFor(savedPlan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		forecast, err := planning.ComputeForecast(inputs, planning.ForecastSettings{LowerFactor: .8, UpperFactor: 1.3, Disruption: .1})
		Expect(err).NotTo(HaveOccurred())
		source, err := database.PredictionSnapshotSource(ctx, initialID)
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 52; i++ {
			capture := initialID
			if i == 1 {
				capture = otherID
			}
			pred := planning.ForecastPrediction{ID: fmt.Sprintf("history-%03d", i), Name: "September check", IssuedAt: now - 30, Inputs: inputs, Forecast: forecast, Evidence: planning.PredictionEvidence{SnapshotID: capture, SourceID: sourceID, ConfigFingerprint: source.Fingerprint}}
			inserted, e := database.SavePrediction(ctx, savedPlan, db.PredictionRow{ID: pred.ID, Name: pred.Name, IssuedAt: pred.IssuedAt, SnapshotID: capture, RequestHash: pred.ID, Data: encode(pred)})
			Expect(e).NotTo(HaveOccurred())
			Expect(inserted).To(BeTrue())
		}
		request := func(method, body string, c auth.Claims) *httptest.ResponseRecorder {
			r := httptest.NewRecorder()
			srv.handlePlanItem(r, httptest.NewRequest(method, "/api/plan/"+plan.ID+"/predictions/history-000/validation", strings.NewReader(body)), c)
			return r
		}
		body := string(encode(map[string]string{"snapshotId": laterID}))
		response := request("POST", body, claims)
		Expect(response.Code).To(Equal(200), response.Body.String())
		var report planning.PredictionValidation
		Expect(json.Unmarshal(response.Body.Bytes(), &report)).To(Succeed())
		Expect(report.TotalRecords).To(Equal(52))
		Expect(report.CandidateRecords).To(Equal(52))
		Expect(report.Repeated).To(Equal(51))
		Expect(report.Excluded).To(Equal(1))
		Expect(report.CoveragePercent).To(BeNil())
		after, err := database.GetPlan(plan.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(savedPlan))
		Expect(request("GET", body, claims).Code).To(Equal(405))
		Expect(request("POST", `{"snapshotId":"absent"}`, claims).Code).To(Equal(404))
		Expect(request("POST", body+body, claims).Code).To(Equal(400))
		for _, suffix := range []string{"validation/unknown", "validation/", "assessment/unknown"} {
			r := httptest.NewRecorder()
			srv.handlePlanItem(r, httptest.NewRequest("POST", "/api/plan/"+plan.ID+"/predictions/history-000/"+suffix, strings.NewReader(body)), claims)
			Expect(r.Code).To(Equal(405), suffix)
		}
		for _, c := range []auth.Claims{{Sub: "other", Roles: []string{"manager"}}, {Sub: claims.Sub, Roles: []string{"player"}}, {Sub: claims.Sub, Roles: claims.Roles, GameID: "game"}} {
			Expect(request("POST", body, c).Code).To(Equal(403))
		}
		_, err = pool.Exec(ctx, `UPDATE snapshots SET owner='other',public=false WHERE id=$1`, otherID)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", body, claims).Code).To(Equal(404))
		_, err = pool.Exec(ctx, `UPDATE plan_forecast_predictions SET issued_at=$2,data=jsonb_set(data,'{issuedAt}',to_jsonb($2::bigint)) WHERE plan_id=$1 AND id='history-001'`, plan.ID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", body, claims).Code).To(Equal(404), "Matching late records still require original capture access")
		_, err = pool.Exec(ctx, `UPDATE snapshots SET owner=$2 WHERE id=$1`, otherID, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		response = request("POST", body, claims)
		Expect(response.Code).To(Equal(200))
		Expect(json.Unmarshal(response.Body.Bytes(), &report)).To(Succeed())
		Expect(report.TooLateRecords).To(Equal(1))
		Expect(report.CandidateRecords).To(Equal(51))
		_, err = pool.Exec(ctx, `UPDATE evidence_runs SET config='{"site":"https://other.atlassian.net"}' WHERE snapshot_id=$1`, laterID)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", body, claims).Code).To(Equal(400))
		_, err = pool.Exec(ctx, `UPDATE evidence_runs SET config=(SELECT config FROM evidence_sources WHERE id=$2) WHERE snapshot_id=$1`, laterID, sourceID)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, `INSERT INTO plan_forecast_predictions(id,plan_id,issued_at,name,snapshot_id,request_hash,data) SELECT 'extra-'||n,plan_id,issued_at,name,snapshot_id,'hash',jsonb_set(data,'{id}',to_jsonb('extra-'||n)) FROM plan_forecast_predictions CROSS JOIN generate_series(1,148) n WHERE plan_id=$1 AND id='history-000'`, plan.ID)
		Expect(err).NotTo(HaveOccurred())
		response = request("POST", body, claims)
		Expect(response.Code).To(Equal(200), response.Body.String())
		Expect(json.Unmarshal(response.Body.Bytes(), &report)).To(Succeed())
		Expect(report.TotalRecords).To(Equal(200))
		_, err = pool.Exec(ctx, `INSERT INTO plan_forecast_predictions(id,plan_id,issued_at,name,snapshot_id,request_hash,data) SELECT 'overflow',plan_id,issued_at,name,snapshot_id,'hash',jsonb_set(data,'{id}','"overflow"') FROM plan_forecast_predictions WHERE plan_id=$1 AND id='history-000'`, plan.ID)
		Expect(err).NotTo(HaveOccurred())
		response = request("POST", body, claims)
		Expect(response.Code).To(Equal(422))
		Expect(response.Body.String()).To(ContainSubstring("No partial report"))
		_, err = pool.Exec(ctx, `UPDATE accounts SET roles=ARRAY['player'],role='player' WHERE username=$1`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", body, claims).Code).To(Equal(403))
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
		Expect(assessed.Body.String()).To(ContainSubstring(`"coveragePercent":null`))
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
