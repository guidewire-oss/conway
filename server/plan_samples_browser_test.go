package main

import (
	"context"
	"conway/server/db"
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
)

// specs/033-consistent-plan-controls-and-samples.md:42
var _ = Describe("plan samples browser", Label("database", "browser"), func() {
	It("downloads matching workbooks, recovers failures and imports the sample", func() {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 with isolated PostgreSQL and Playwright.")
		}
		database, err := db.Open(context.Background(), os.Getenv("CONWAY_TEST_DATABASE_URL"))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		st := newMemStore()
		user, password := st.CreateUser("Sample manager "+newID(), []string{"manager"}, 1)
		srv := &server{db: database, store: st, metrics: NewMetrics()}
		now := time.Now().Unix()
		plan := db.PlanRow{ID: newID(), Owner: user.Username, Name: "Atlas sample plan", HorizonWeeks: 26, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(plan)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(plan.ID)).To(Succeed()) })
		for _, name := range []string{"Atlas", "Beacon"} {
			pods, e := json.Marshal([]NetPod{{Name: name, Streams: 2}})
			Expect(e).NotTo(HaveOccurred())
			roster := db.RosterRow{ID: newID(), Owner: user.Username, Name: name + " roster", Pods: pods, CreatedAt: now}
			Expect(database.CreateRoster(roster)).To(Succeed())
			DeferCleanup(func() { Expect(database.DeleteRoster(roster.ID)).To(Succeed()) })
		}
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
		script, err := filepath.Abs("../tests/browser/plan-samples.mjs")
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := browserCommand(ctx, node, script)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL, "CONWAY_TEST_USERNAME="+user.Username, "CONWAY_TEST_PASSWORD="+password, "CONWAY_TEST_PLAN_ID="+plan.ID)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})
