package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"conway/server/db"
	"conway/server/planning"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type browserSheetProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *browserSheetProvider) Fetch(context.Context, string, string) ([][]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return [][]string{{"Name", "Tracks"}, {"Team A", strconv.Itoa(2 + p.calls)}, {"Team B", "1"}}, nil
}

// Opt-in real-browser acceptance; all fixture injection is confined to this
// test binary (specs/023-linked-google-sheets.md:272).
var _ = Describe("linked features browser", Label("database", "browser"), func() {
	It("introduces features once and reviews, conflicts and restores captured inputs", func() {
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
		srv := &server{db: database, store: st, metrics: NewMetrics(), sheetsProvider: &browserSheetProvider{}, sheetsAccountEmail: "sheet-reader@example.test"}
		now := time.Now().Unix()
		row := db.PlanRow{ID: newID(), Owner: user.Username, Name: "Atlas browser plan", HorizonWeeks: 26, CreatedAt: now, UpdatedAt: now}
		Expect(database.CreatePlan(row)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(row.ID)).To(Succeed()) })
		encode := func(v any) []byte { b, e := json.Marshal(v); Expect(e).NotTo(HaveOccurred()); return b }
		Expect(database.SavePlanTeams(row.ID, encode([]planning.Team{{Name: "Team A", Tracks: 2}, {Name: "Team B", Tracks: 1}}), now)).To(Succeed())
		Expect(database.SavePlanInitiatives(row.ID, encode([]planning.Initiative{{Name: "Atlas", Work: map[string]planning.TeamWork{"Team A": {Weeks: 4, Estimated: true, InPath: true}}}}), now)).To(Succeed())
		Expect(database.SavePlanScheduling(row.ID, encode(planning.SchedulingParams{WipModel: "strict", EstimateModel: "effort", PeriodStart: "2026-09-07"}), now)).To(Succeed())
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
		script, err := filepath.Abs("../tests/browser/linked-features.mjs")
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := browserCommand(ctx, node, script)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL, "CONWAY_TEST_USERNAME="+user.Username, "CONWAY_TEST_PASSWORD="+password, "CONWAY_TEST_PLAN_ID="+row.ID)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})
