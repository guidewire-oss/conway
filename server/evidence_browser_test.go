package main

import (
	"context"
	"conway/server/db"
	"conway/server/evidence"
	"conway/server/jira"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// per specs/026-reliable-evidence-foundation.md:34
var _ = Describe("evidence sources browser", Label("database", "browser"), func() {
	// per specs/026-reliable-evidence-foundation.md:38
	// per specs/026-reliable-evidence-foundation.md:91
	It("sets up, recovers and explicitly selects captured evidence", func() {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 and an isolated test database")
		}
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		Expect(url).NotTo(BeEmpty())
		ctx := context.Background()
		database, err := db.Open(ctx, url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		pool, err := pgxpool.New(ctx, url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		st := newMemStore()
		st.Secret = []byte("browser evidence fixture durable key")
		u, password := st.CreateUser("Evidence browser "+newID(), []string{"manager"}, 1)
		_, err = pool.Exec(ctx, `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,$2,'manager',$3,$4,$5,$6,$7)`, u.Username, u.Display, u.Roles, u.Salt, u.Hash, u.ExpiresAt, time.Now().Unix())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			for _, table := range []string{"evidence_sources", "snapshots", "rosters"} {
				_, e := pool.Exec(ctx, "DELETE FROM "+table+" WHERE owner=$1", u.Username)
				Expect(e).NotTo(HaveOccurred())
			}
			_, e := pool.Exec(ctx, `DELETE FROM accounts WHERE username=$1`, u.Username)
			Expect(e).NotTo(HaveOccurred())
		})
		rosterID := newID()
		pods, _ := json.Marshal([]NetPod{{Name: "Atlas", Streams: 2, DevCount: 4}})
		Expect(database.CreateRoster(db.RosterRow{ID: rosterID, Owner: u.Username, Name: "Atlas browser roster", Pods: pods, CreatedAt: time.Now().Unix()})).To(Succeed())
		// per specs/026-reliable-evidence-foundation.md:38
		selectedSnapshotID := newID()
		Expect(database.CreateSnapshotWithData(db.SnapshotRow{ID: selectedSnapshotID, Owner: u.Username, Name: "Previously selected evidence", Source: "jira", RosterID: rosterID, Scope: json.RawMessage(`["PROJ"]`), CreatedAt: time.Now().Unix() - 3600}, db.SnapshotData{Pods: []db.PodRow{{Name: "Atlas", Streams: 2, DevCount: 4}}})).To(Succeed())
		var calls atomic.Int32
		srv := &server{db: database, store: st, metrics: NewMetrics(), evidenceFetch: func(context.Context, evidence.Config, evidenceCredential) ([]jira.DetailedIssue, error) {
			call := calls.Add(1)
			if call == 1 {
				return nil, errors.New("browser-private-token provider failure")
			}
			key, pod := "PROJ-1", "Atlas"
			if call > 2 {
				key, pod = "PROJ-2", "Atlas platform"
			}
			return []jira.DetailedIssue{{ID: "123", Key: key, Pod: pod, Summary: "Atlas initiative", IssueType: "Epic", StatusCat: "new"}}, nil
		}}
		DeferCleanup(func() {
			Eventually(func() int { return len(srv.evidenceSlotsChannel()) }, 5*time.Second).Should(BeZero(), "capture workers finish before fixture deletion")
		})
		mux := http.NewServeMux()
		mux.HandleFunc("/api/config", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"server": true, "authRequired": true, "serverGame": false, "oidc": false})
		})
		mux.HandleFunc("/api/login", srv.handleLogin)
		mux.HandleFunc("/api/me", srv.withAuth(srv.handleMe, ""))
		mux.HandleFunc("/api/announcements", srv.withAuth(srv.handleAnnouncements, ""))
		mux.HandleFunc("/api/announcements/ack", srv.withAuth(srv.handleAnnouncementAck, ""))
		mux.HandleFunc("/api/evidence-sources", srv.withAuth(srv.handleEvidenceSources, "manager"))
		mux.HandleFunc("/api/evidence-sources/", srv.withAuth(srv.handleEvidenceSource, "manager"))
		mux.HandleFunc("/api/snapshots", srv.withAuth(srv.handleSnapshots, ""))
		mux.HandleFunc("/api/snapshots/", srv.withAuth(srv.handleSnapshotItem, ""))
		mux.HandleFunc("/api/rosters", srv.withAuth(srv.handleRosters, "manager"))
		mux.HandleFunc("/api/plan", srv.withAuth(srv.handlePlans, "manager"))
		mux.HandleFunc("/api/jira/status", srv.withAuth(srv.handleJiraStatus, "manager"))
		app, err := filepath.Abs("../app")
		Expect(err).NotTo(HaveOccurred())
		mux.Handle("/", http.FileServer(http.Dir(app)))
		host := httptest.NewServer(mux)
		DeferCleanup(host.Close)
		script, err := filepath.Abs("../tests/browser/evidence-sources.mjs")
		Expect(err).NotTo(HaveOccurred())
		node := os.Getenv("CONWAY_TEST_NODE")
		if node == "" {
			node = "node"
		}
		work, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		cmd := browserCommand(work, node, script)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL, "CONWAY_TEST_USERNAME="+u.Username, "CONWAY_TEST_PASSWORD="+password, "CONWAY_TEST_ROSTER_ID="+rosterID, "CONWAY_TEST_SNAPSHOT_ID="+selectedSnapshotID)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(work.Err()).NotTo(HaveOccurred(), "browser exceeded its bounded workload: %s", output)
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})
