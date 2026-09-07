package main

import (
	"bytes"
	"context"
	"conway/server/auth"
	"conway/server/db"
	"conway/server/evidence"
	"conway/server/jira"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"
)

// per specs/026-reliable-evidence-foundation.md:34
var _ = Describe("reliable evidence captures", Label("database"), func() {
	var s *server
	var database *db.DB
	var pool *pgxpool.Pool
	var owner auth.Claims
	var roster db.RosterRow
	var now int64
	encode := func(v any) []byte { b, e := json.Marshal(v); Expect(e).NotTo(HaveOccurred()); return b }
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set isolated CONWAY_TEST_DATABASE_URL")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		pool, err = pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		now = time.Now().Unix()
		st := newMemStore()
		st.Secret = []byte("durable evidence test secret")
		u, _ := st.CreateUser("Evidence manager "+newID(), []string{"manager"}, 24)
		owner = auth.Claims{Sub: u.Username, Roles: u.Roles}
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) VALUES($1,$2,'manager',$3,$4,$5,$6,$7)`, u.Username, u.Display, u.Roles, u.Salt, u.Hash, u.ExpiresAt, now)
		Expect(err).NotTo(HaveOccurred())
		s = &server{db: database, store: st, metrics: NewMetrics()}
		roster = db.RosterRow{ID: newID(), Owner: owner.Sub, Name: "Atlas roster", Pods: encode([]NetPod{{Name: "Atlas", Streams: 2, DevCount: 4}}), CreatedAt: now}
		Expect(database.CreateRoster(roster)).To(Succeed())
	})
	AfterEach(func() {
		if pool == nil {
			return
		}
		if s != nil {
			Eventually(func() int { return len(s.evidenceSlotsChannel()) }, 5*time.Second).Should(BeZero(), "fixture workers finish before database teardown")
		}
		ctx := context.Background()
		for _, table := range []string{"evidence_sources", "snapshots", "rosters"} {
			_, err := pool.Exec(ctx, "DELETE FROM "+table+" WHERE owner=$1", owner.Sub)
			Expect(err).NotTo(HaveOccurred())
		}
		_, err := pool.Exec(ctx, `DELETE FROM accounts WHERE username=$1`, owner.Sub)
		Expect(err).NotTo(HaveOccurred())
		pool.Close()
		database.Close()
		pool = nil
	})
	request := func(method, path string, body any, c auth.Claims) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(string(encode(body))))
		w := httptest.NewRecorder()
		if path == "/api/evidence-sources" {
			s.handleEvidenceSources(w, r, c)
		} else {
			s.handleEvidenceSource(w, r, c)
		}
		return w
	}
	create := func() *db.EvidenceSource {
		w := request("POST", "/api/evidence-sources", evidenceInput{Config: evidence.Config{Name: "Atlas evidence", Site: "https://atlas.atlassian.net", Projects: []string{"PROJ"}, RosterID: roster.ID, WipMode: "leaf", IntervalHours: 24, FreshnessHours: 24, Enabled: true}, Email: "reader@example.test", Token: "private-fixture-token", Consent: true}, owner)
		Expect(w.Code).To(Equal(201), w.Body.String())
		var row db.EvidenceSource
		Expect(json.Unmarshal(w.Body.Bytes(), &row)).To(Succeed())
		saved, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		return saved
	}
	// per specs/026-reliable-evidence-foundation.md:56
	It("requires consent, protects private rosters and never returns credentials", func() {
		cfg := evidence.Config{Name: "Atlas", Site: "https://atlas.atlassian.net", Projects: []string{"PROJ"}, RosterID: roster.ID, WipMode: "leaf", FreshnessHours: 24}
		Expect(request("POST", "/api/evidence-sources", evidenceInput{Config: cfg, Email: "reader@example.test", Token: "private"}, owner).Code).To(Equal(400))
		outsider := auth.Claims{Sub: "other", Roles: []string{"manager"}}
		Expect(request("POST", "/api/evidence-sources", evidenceInput{Config: cfg, Email: "reader@example.test", Token: "private", Consent: true}, outsider).Code).To(Equal(403))
		private := db.RosterRow{ID: newID(), Owner: "other", Name: "Private", Pods: roster.Pods, CreatedAt: now}
		Expect(database.CreateRoster(private)).To(Succeed())
		cfg.RosterID = private.ID
		response := request("POST", "/api/evidence-sources", evidenceInput{Config: cfg, Email: "reader@example.test", Token: "private", Consent: true}, owner)
		Expect(database.DeleteRoster(private.ID)).To(Succeed())
		Expect(response.Code).To(Equal(404))
		row := create()
		Expect(string(row.Credential)).NotTo(ContainSubstring("private-fixture-token"))
		Expect(request("GET", "/api/evidence-sources/"+row.ID, nil, outsider).Code).To(Equal(403))
		Expect(request("GET", "/api/evidence-sources", nil, auth.Claims{Sub: "player"}).Code).To(Equal(403))
		out := request("GET", "/api/evidence-sources/"+row.ID, nil, owner)
		Expect(out.Code).To(Equal(200))
		Expect(out.Body.String()).NotTo(ContainSubstring("private-fixture-token"))
		Expect(out.Body.String()).NotTo(ContainSubstring("reader@example.test"))
	})
	// per specs/026-reliable-evidence-foundation.md:47
	// per specs/026-reliable-evidence-foundation.md:61
	It("persists complete captures and identities across renamed keys while failures retain prior evidence", func() {
		row := create()
		s.evidenceNow = func() time.Time { return time.Unix(now, 0) }
		key := "PROJ-1"
		s.evidenceFetch = func(context.Context, evidence.Config, evidenceCredential) ([]jira.DetailedIssue, error) {
			return []jira.DetailedIssue{{ID: "123", Key: key, Pod: "Atlas", IssueType: "Epic", StatusCat: "new"}}, nil
		}
		run := newID()
		claimed, err := database.ClaimEvidence(context.Background(), row.ID, run, now, true)
		Expect(err).NotTo(HaveOccurred())
		s.runEvidence(claimed)
		one, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(one.LastStatus).To(Equal("succeeded"))
		body, err := database.GetSnapshotDoc(one.LastSnapshot, "identities.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring(evidence.Identity(row.ID, "issue", "123")))
		key = "PROJ-2"
		claimed, err = database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		s.runEvidence(claimed)
		two, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(two.LastSnapshot).NotTo(Equal(one.LastSnapshot))
		body, err = database.GetSnapshotDoc(two.LastSnapshot, "identities.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("PROJ-2"))
		Expect(string(body)).To(ContainSubstring(evidence.Identity(row.ID, "issue", "123")))
		var logs bytes.Buffer
		logger := zerolog.New(&logs)
		s.log = &logger
		s.evidenceFetch = func(context.Context, evidence.Config, evidenceCredential) ([]jira.DetailedIssue, error) {
			return nil, errors.New("private-fixture-token reader@example.test provider error")
		}
		claimed, err = database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		s.runEvidence(claimed)
		failed, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(failed.LastStatus).To(Equal("failed"))
		Expect(failed.LastSnapshot).To(Equal(two.LastSnapshot))
		Expect(failed.LastError).NotTo(ContainSubstring("secret token"))
		history, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(3))
		Expect(history[0].Status).To(Equal("failed"), "same-second attempts remain newest first")
		Expect(history[0].SnapshotID).To(BeEmpty())
		Expect(history[1].SnapshotID).To(Equal(two.LastSnapshot))
		Expect(history[2].SnapshotID).To(Equal(one.LastSnapshot))
		// per specs/026-reliable-evidence-foundation.md:56
		for _, attempt := range history {
			Expect(attempt.Error).NotTo(ContainSubstring("private-fixture-token"))
			Expect(attempt.Error).NotTo(ContainSubstring("reader@example.test"))
			Expect(attempt.FinishedAt).To(BeNumerically(">=", attempt.StartedAt))
		}
		Expect(logs.String()).NotTo(ContainSubstring("private-fixture-token"))
		Expect(logs.String()).NotTo(ContainSubstring("reader@example.test"))
		Expect(request("GET", "/api/evidence-sources/"+row.ID, nil, owner).Body.String()).NotTo(ContainSubstring("private-fixture-token"))
		w := httptest.NewRecorder()
		s.patchSnapshot(w, httptest.NewRequest("PATCH", "/api/snapshots/"+one.LastSnapshot, strings.NewReader(`{"rosterId":"other"}`)), one.LastSnapshot, owner)
		Expect(w.Code).To(Equal(409))
		w = httptest.NewRecorder()
		s.deleteSnapshot(w, one.LastSnapshot, owner)
		Expect(w.Code).To(Equal(409))
	})
	// per specs/026-reliable-evidence-foundation.md:51
	It("claims once under concurrency, recovers an expired lease and rejects obsolete success or failure", func() {
		row := create()
		var wg sync.WaitGroup
		wins := make(chan *db.EvidenceSource, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
				if e == nil {
					wins <- r
				}
			}()
		}
		wg.Wait()
		close(wins)
		Expect(len(wins)).To(Equal(1))
		old := <-wins
		Expect(database.UpdateEvidenceSource(context.Background(), *row, now)).To(MatchError(db.ErrEvidenceConflict))
		replacement, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now+1201, false)
		Expect(err).NotTo(HaveOccurred(), "expired claim recovers before daily nextAt")
		Expect(database.FinishEvidence(context.Background(), row.ID, old.ActiveRun, now+1202, db.SnapshotRow{}, db.SnapshotData{}, nil, "Late error")).To(MatchError(db.ErrEvidenceConflict))
		lateSnapshot := db.SnapshotRow{ID: newID(), Name: "Obsolete capture", CreatedAt: now + 1202}
		Expect(database.FinishEvidence(context.Background(), row.ID, old.ActiveRun, now+1202, lateSnapshot, db.SnapshotData{}, []byte(`{}`), "")).To(MatchError(db.ErrEvidenceConflict))
		missing, err := database.GetSnapshot(lateSnapshot.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(missing).To(BeNil())
		Expect(replacement.ActiveRun).NotTo(Equal(old.ActiveRun))
		runs, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(runs).To(ContainElement(HaveField("Status", "interrupted")))
	})
	// per specs/026-reliable-evidence-foundation.md:56
	It("rejects stale configuration writes and checks owner revocation before publication", func() {
		row := create()
		s.evidenceNow = func() time.Time { return time.Unix(now+123, 0) }
		cfg := row.Config
		cfg.Teams[0].Name = "Atlas platform"
		cfg.Teams[0].Aliases = append(cfg.Teams[0].Aliases, "Atlas platform")
		w := request("PUT", "/api/evidence-sources/"+row.ID, evidenceInput{Config: cfg, Version: row.Version}, owner)
		Expect(w.Code).To(Equal(200))
		updated, e := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(e).NotTo(HaveOccurred())
		Expect(updated.NextAt).To(Equal(row.NextAt), "a name or alias edit must not postpone the schedule")
		Expect(request("PUT", "/api/evidence-sources/"+row.ID, evidenceInput{Config: cfg, Version: row.Version}, owner).Code).To(Equal(409))
		claimed, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(context.Background(), `UPDATE accounts SET roles=ARRAY['player'] WHERE username=$1`, owner.Sub)
		Expect(err).NotTo(HaveOccurred())
		s.evidenceFetch = func(context.Context, evidence.Config, evidenceCredential) ([]jira.DetailedIssue, error) {
			return nil, nil
		}
		s.runEvidence(claimed)
		after, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.LastStatus).To(Equal("failed"))
		Expect(after.LastSnapshot).To(BeEmpty())
		Expect(request("GET", "/api/evidence-sources/"+row.ID, nil, owner).Code).To(Equal(403), "a previously issued manager token cannot bypass current roles")
		_, err = database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).To(MatchError(db.ErrEvidenceOwner))
	})
	// per specs/026-reliable-evidence-foundation.md:77
	It("does not publish partial data when a transactional insert fails", func() {
		row := create()
		claim, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		snapshot := db.SnapshotRow{ID: newID(), Name: "Incomplete", CreatedAt: now}
		data := db.SnapshotData{Issues: []db.IssueRow{{Key: "PROJ-1"}, {Key: "PROJ-1"}}}
		Expect(database.FinishEvidence(context.Background(), row.ID, claim.ActiveRun, now, snapshot, data, []byte(`{}`), "")).To(HaveOccurred())
		saved, err := database.GetSnapshot(snapshot.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved).To(BeNil())
		after, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.LastSuccess).To(BeZero())
	})
	// per specs/026-reliable-evidence-foundation.md:34
	// per specs/026-reliable-evidence-foundation.md:89
	DescribeTable("respects due time, pause and manual-only cadence", func(interval int, enabled, scheduled bool) {
		row := create()
		row.Config.IntervalHours, row.Config.Enabled = interval, enabled
		row.NextAt = now + 60
		Expect(database.UpdateEvidenceSource(context.Background(), *row, now)).To(Succeed())
		_, err := pool.Exec(context.Background(), `UPDATE evidence_sources SET next_at=$2 WHERE id=$1`, row.ID, now+60)
		Expect(err).NotTo(HaveOccurred(), "advance the fixture to one minute before its scheduled due time")
		_, err = database.ClaimEvidence(context.Background(), row.ID, newID(), now+59, false)
		Expect(err).To(MatchError(db.ErrEvidenceConflict))
		history, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(BeEmpty(), "not-yet-due scans must not create attempts")
		claim, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now+60, false)
		if scheduled {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(MatchError(db.ErrEvidenceConflict))
			claim, err = database.ClaimEvidence(context.Background(), row.ID, newID(), now+60, true)
			Expect(err).NotTo(HaveOccurred(), "capture now remains available while automatic capture is paused or manual")
		}
		Expect(database.FinishEvidence(context.Background(), row.ID, claim.ActiveRun, now+90, db.SnapshotRow{}, db.SnapshotData{}, nil, "Provider unavailable; retry capture.")).To(Succeed())
		after, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.LastStatus).To(Equal("failed"))
		if scheduled {
			Expect(after.NextAt).To(Equal(now+90+int64(interval)*3600), "retry cadence starts at completion")
			_, err = database.ClaimEvidence(context.Background(), row.ID, newID(), after.NextAt-1, false)
			Expect(err).To(MatchError(db.ErrEvidenceConflict))
		}
	}, Entry("six hourly", 6, true, true), Entry("daily", 24, true, true), Entry("weekly", 168, true, true), Entry("paused", 24, false, false), Entry("manual only", 0, true, false))

	// per specs/026-reliable-evidence-foundation.md:51
	It("recovers a durable interrupted claim after reopening the database and server", func() {
		row := create()
		old, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		database.Close()
		database, err = db.Open(context.Background(), os.Getenv("CONWAY_TEST_DATABASE_URL"))
		Expect(err).NotTo(HaveOccurred())
		restartedStore := newMemStore()
		restartedStore.Secret = append([]byte{}, s.store.Secret...)
		s = &server{db: database, store: restartedStore, metrics: NewMetrics(), evidenceNow: func() time.Time { return time.Unix(now+1201, 0) }}
		s.evidenceFetch = func(_ context.Context, config evidence.Config, credential evidenceCredential) ([]jira.DetailedIssue, error) {
			if credential.Token != "private-fixture-token" || credential.Email != "reader@example.test" || config.RosterID != roster.ID {
				return nil, errors.New("durable configuration or credential did not survive restart")
			}
			return []jira.DetailedIssue{{ID: "123", Key: "PROJ-1", Pod: "Atlas", IssueType: "Epic", StatusCat: "new"}}, nil
		}
		s.captureDueEvidence(context.Background())
		Eventually(func(g Gomega) {
			after, e := database.GetEvidenceSource(context.Background(), row.ID)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(after.LastStatus).To(Equal("succeeded"))
			g.Expect(after.LastSnapshot).NotTo(BeEmpty())
		}, 5*time.Second).Should(Succeed())
		history, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(2))
		Expect(history).To(ContainElement(And(HaveField("ID", old.ActiveRun), HaveField("Status", "interrupted"))))
		Expect(history).To(ContainElement(HaveField("Status", "succeeded")))
	})

	// per specs/026-reliable-evidence-foundation.md:42
	// per specs/026-reliable-evidence-foundation.md:67
	It("keeps successful capture scope and lineage immutable while current freshness ages", func() {
		_, err := pool.Exec(context.Background(), `UPDATE accounts SET expires_at=$2 WHERE username=$1`, owner.Sub, now+172800)
		Expect(err).NotTo(HaveOccurred())
		row := create()
		s.evidenceNow = func() time.Time { return time.Unix(now, 0) }
		s.evidenceFetch = func(context.Context, evidence.Config, evidenceCredential) ([]jira.DetailedIssue, error) {
			return []jira.DetailedIssue{{ID: "123", Key: "PROJ-1", Pod: "Unmapped team", IssueType: "Epic", StatusCat: "new"}}, nil
		}
		claim, err := database.ClaimEvidence(context.Background(), row.ID, newID(), now, true)
		Expect(err).NotTo(HaveOccurred())
		s.runEvidence(claim)
		captured, err := database.GetEvidenceSource(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		original, err := database.GetSnapshotDoc(captured.LastSnapshot, "identities.json")
		Expect(err).NotTo(HaveOccurred())
		var lineage struct {
			UnmappedTeams map[string]bool `json:"unmappedTeams"`
			Issues        []struct {
				TeamID string `json:"teamId"`
			} `json:"issues"`
		}
		Expect(json.Unmarshal(original, &lineage)).To(Succeed())
		Expect(lineage.UnmappedTeams).To(HaveKeyWithValue("Unmapped team", true))
		Expect(lineage.Issues).To(HaveLen(1))
		Expect(lineage.Issues[0].TeamID).To(BeEmpty())
		cfg := captured.Config
		cfg.Name, cfg.Projects = "Revised evidence", []string{"NEXT"}
		cfg.Teams[0].Name = "Atlas platform"
		Expect(request("PUT", "/api/evidence-sources/"+row.ID, evidenceInput{Config: cfg, Version: captured.Version}, owner).Code).To(Equal(200))
		unchanged, err := database.GetSnapshotDoc(captured.LastSnapshot, "identities.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged).To(Equal(original))
		snapshot, err := database.GetSnapshot(captured.LastSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(snapshot.Scope)).To(MatchJSON(`["PROJ"]`))
		Expect(snapshot.Public).To(BeFalse())
		history, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history[0].Config.Name).To(Equal("Atlas evidence"))
		Expect(history[0].Config.Projects).To(Equal([]string{"PROJ"}))
		for _, elapsed := range []int64{86399, 86400} {
			s.evidenceNow = func() time.Time { return time.Unix(now+elapsed, 0) }
			w := request("GET", "/api/evidence-sources/"+row.ID, nil, owner)
			Expect(w.Code).To(Equal(200))
			var state struct {
				Source    db.EvidenceSource `json:"source"`
				Freshness string            `json:"freshness"`
			}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state.Source.LastStatus).To(Equal("succeeded"))
			if elapsed < 86400 {
				Expect(state.Freshness).To(Equal("Fresh"))
			} else {
				Expect(state.Freshness).To(Equal("Stale"))
			}
		}
	})

	// per specs/026-reliable-evidence-foundation.md:134
	It("does not roll back the next scheduled capture when an unrelated edit races completion", func() {
		loaded := create()
		claim, err := database.ClaimEvidence(context.Background(), loaded.ID, newID(), now+10, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(database.FinishEvidence(context.Background(), loaded.ID, claim.ActiveRun, now+90, db.SnapshotRow{}, db.SnapshotData{}, nil, "Provider unavailable; retry capture.")).To(Succeed())
		completed, err := database.GetEvidenceSource(context.Background(), loaded.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(completed.NextAt).NotTo(Equal(loaded.NextAt))
		loaded.Config.Name = "Renamed evidence"
		Expect(database.UpdateEvidenceSource(context.Background(), *loaded, now+100)).To(Succeed())
		saved, err := database.GetEvidenceSource(context.Background(), loaded.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Config.Name).To(Equal("Renamed evidence"))
		Expect(saved.NextAt).To(Equal(completed.NextAt), "unrelated configuration saves retain the latest completion-based due time")
	})

	// per specs/026-reliable-evidence-foundation.md:56
	It("denies another current manager and expired owner access without exposing private sources", func() {
		row := create()
		outsider := auth.Claims{Sub: "other-" + newID(), Roles: []string{"manager"}}
		_, err := pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash,expires_at,created_at) SELECT $2,'Other manager','manager',ARRAY['manager'],salt,hash,expires_at,created_at FROM accounts WHERE username=$1`, owner.Sub, outsider.Sub)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_, e := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, outsider.Sub)
			Expect(e).NotTo(HaveOccurred())
		}()
		for _, actor := range []auth.Claims{outsider, owner} {
			status := 404
			if actor.Sub == owner.Sub {
				_, err = pool.Exec(context.Background(), `UPDATE accounts SET expires_at=$2 WHERE username=$1`, owner.Sub, now)
				Expect(err).NotTo(HaveOccurred())
				status = 403
			}
			Expect(request("GET", "/api/evidence-sources/"+row.ID, nil, actor).Code).To(Equal(status))
			Expect(request("PUT", "/api/evidence-sources/"+row.ID, evidenceInput{Config: row.Config, Version: row.Version}, actor).Code).To(Equal(status))
			Expect(request("POST", "/api/evidence-sources/"+row.ID+"/capture", nil, actor).Code).To(Equal(status))
			listed := request("GET", "/api/evidence-sources", nil, actor)
			Expect(listed.Body.String()).NotTo(ContainSubstring(row.ID))
		}
		runs, err := database.EvidenceRuns(context.Background(), row.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(runs).To(BeEmpty(), "denied requests cannot create attempts")
	})
})
