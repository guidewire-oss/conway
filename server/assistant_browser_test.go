package main

import (
	"context"
	"conway/server/db"
	"conway/server/planning"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
)

// Opt-in real-browser acceptance; all fixture injection is confined to this
// test binary (specs/027-evidence-linked-planning-assistant.md:46).
var _ = Describe("planning assistant browser", Label("database", "browser"), func() {
	It("explains evidence, preserves context and recovers accessibly", func() {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 with isolated PostgreSQL and Playwright.")
		}
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		Expect(url).NotTo(BeEmpty())
		database, err := db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		st := newMemStore()
		user, password := st.CreateUser("Browser manager "+newID(), []string{"manager"}, 1)
		pool, err := pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,$2,'manager',$3,$4,$5,$6,$7)`, user.Username, user.Display, user.Roles, user.Salt, user.Hash, user.ExpiresAt, time.Now().Unix())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, e := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, user.Username)
			Expect(e).NotTo(HaveOccurred())
		})
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": `{"task":"schedule","initiative":"Beacon","team":""}`}}}}})
		}))
		DeferCleanup(provider.Close)
		srv := &server{db: database, store: st, metrics: NewMetrics(), assistantModel: &assistantInterpreter{endpoint: provider.URL, key: "fixture-key", model: "configured-model", client: provider.Client(), slots: make(chan struct{}, 4)}}
		now := time.Now().Unix()
		row := db.PlanRow{ID: newID(), Owner: user.Username, Name: "Atlas browser plan", HorizonWeeks: 26, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(row)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(row.ID)).To(Succeed()) })
		encode := func(v any) []byte { b, e := json.Marshal(v); Expect(e).NotTo(HaveOccurred()); return b }
		Expect(database.SavePlanTeams(row.ID, encode([]planning.Team{{Name: "Team A", Tracks: 2}, {Name: "Team B", Tracks: 1}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(row.ID, encode([]planning.Initiative{{Name: "Beacon", EpicKeys: []string{"PROJ-1"}, Work: map[string]planning.TeamWork{"Team A": {Weeks: 4, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		Expect(database.SavePlanScheduling(row.ID, encode(planning.SchedulingParams{WipModel: "strict", EstimateModel: "effort", PeriodStart: "2026-09-07"}), now)).To(Succeed())
		saved, err := database.GetPlan(row.ID)
		Expect(err).NotTo(HaveOccurred())
		inputs, err := srv.planScheduleFor(saved, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		_, err = database.SaveBaseline(db.BaselineRow{ID: newID(), PlanID: row.ID, Name: "Initial agreement", Active: true, CreatedAt: now, CreatedBy: user.Username, Fingerprint: inputs.Fingerprint(), Inputs: encode(inputs), Schedule: encode(inputs.Recompute())}, true)
		Expect(err).NotTo(HaveOccurred())
		snapshotID := newID()
		Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: snapshotID, Owner: user.Username, Name: "Atlas observed evidence", Source: "jira", CreatedAt: now, Scope: json.RawMessage(`["PROJ"]`)}, db.SnapshotData{Pods: []db.PodRow{{Name: "Team A", Streams: 2}}, Issues: []db.IssueRow{{Key: "PROJ-1", IssueType: "Epic", StatusCat: "indeterminate", Pod: "Team A"}, {Key: "PROJ-2", ParentKey: "PROJ-1", IssueType: "Story", StatusCat: "indeterminate", Pod: "Team A"}}})).To(Succeed())
		DeferCleanup(func() { Expect(database.DeleteSnapshot(snapshotID)).To(Succeed()) })
		mux := http.NewServeMux()
		mux.HandleFunc("/api/config", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"server": true, "authRequired": true, "serverGame": false, "oidc": false})
		})
		mux.HandleFunc("/api/login", srv.handleLogin)
		mux.HandleFunc("/api/me", srv.withAuth(srv.handleMe, ""))
		mux.HandleFunc("/api/announcements", srv.withAuth(srv.handleAnnouncements, ""))
		mux.HandleFunc("/api/announcements/ack", srv.withAuth(srv.handleAnnouncementAck, ""))
		mux.HandleFunc("/api/plan", srv.withAuth(srv.handlePlans, "manager"))
		mux.HandleFunc("/api/plan/", srv.withAuth(srv.handlePlanItem, "manager"))
		mux.HandleFunc("/api/snapshots", srv.withAuth(srv.handleSnapshots, ""))
		mux.HandleFunc("/api/rosters", srv.withAuth(srv.handleRosters, "manager"))
		mux.HandleFunc("/api/jira/status", srv.withAuth(srv.handleJiraStatus, "manager"))
		app, err := filepath.Abs("../app")
		Expect(err).NotTo(HaveOccurred())
		mux.Handle("/", http.FileServer(http.Dir(app)))
		host := httptest.NewServer(mux)
		DeferCleanup(host.Close)
		node := os.Getenv("CONWAY_TEST_NODE")
		if node == "" {
			node = "node"
		}
		script, err := filepath.Abs("../tests/browser/planning-assistant.mjs")
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := browserCommand(ctx, node, script)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL, "CONWAY_TEST_USERNAME="+user.Username, "CONWAY_TEST_PASSWORD="+password, "CONWAY_TEST_PLAN_ID="+row.ID, "CONWAY_TEST_SNAPSHOT_ID="+snapshotID)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})
