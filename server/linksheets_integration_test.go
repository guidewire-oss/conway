package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"conway/server/planning"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This provider exists only in the test binary; no production endpoint can
// select fixture rows or replace Google's host.
type sourceFixtureProvider struct {
	mu    sync.Mutex
	rows  [][]string
	err   error
	calls int
}

type sourceFetchFunc func(context.Context, string, string) ([][]string, error)

func (f sourceFetchFunc) Fetch(ctx context.Context, id, tab string) ([][]string, error) {
	return f(ctx, id, tab)
}

func (f *sourceFixtureProvider) Fetch(context.Context, string, string) ([][]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	rows := make([][]string, len(f.rows))
	for i, row := range f.rows {
		rows[i] = append([]string(nil), row...)
	}
	return rows, f.err
}
func (f *sourceFixtureProvider) set(rows [][]string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
	f.err = err
}

var _ = Describe("linked Sheets plan integration", Label("database"), func() {
	var database *db.DB
	var srv *server
	var provider *sourceFixtureProvider
	var plan *db.PlanRow
	var claims auth.Claims
	var now time.Time
	encode := func(v any) []byte { b, err := json.Marshal(v); Expect(err).NotTo(HaveOccurred()); return b }
	roster := func(tracks string) [][]string {
		return [][]string{{"Name", "Tracks"}, {"Team A", tracks}, {"Team B", "1"}}
	}
	matrix := func(weeks string) [][]string {
		return [][]string{{"Initiative", "Full Kit Estimate", "Team A"}, {"Atlas", "4", weeks}}
	}
	request := func(method, path string, body any, viewer auth.Claims) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var raw []byte
		if body != nil {
			raw = encode(body)
		}
		srv.handlePlanItem(rec, httptest.NewRequest(method, path, bytes.NewReader(raw)), viewer)
		return rec
	}
	reload := func() *db.PlanRow { p, err := database.GetPlan(plan.ID); Expect(err).NotTo(HaveOccurred()); return p }
	fingerprint := func() string {
		in, err := srv.planScheduleFor(reload(), scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		return in.Fingerprint()
	}
	decodeCheck := func(rec *httptest.ResponseRecorder) sheetCheckResult {
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var result sheetCheckResult
		Expect(json.Unmarshal(rec.Body.Bytes(), &result)).To(Succeed())
		return result
	}
	create := func(kind, mode string) sheetCheckResult {
		return decodeCheck(request("POST", "/api/plan/"+plan.ID+"/sources", map[string]any{"kind": kind, "spreadsheetUrl": "https://docs.google.com/spreadsheets/d/generic-sheet-id/edit", "range": "Inputs!A1:Z100", "mode": mode}, claims))
	}
	path := func(sid, suffix string) string { return "/api/plan/" + plan.ID + "/sources/" + sid + suffix }
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set CONWAY_TEST_DATABASE_URL to run isolated PostgreSQL integration checks.")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		now = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
		claims = auth.Claims{Sub: "sheet-owner-" + newID(), Roles: []string{"manager"}}
		pool, err := pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		_, err = pool.Exec(context.Background(), `INSERT INTO accounts(username,display,role,roles,salt,hash) VALUES($1,'Sheet test owner','manager',ARRAY['manager'],'unused','unused')`, claims.Sub)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, err := pool.Exec(context.Background(), `DELETE FROM accounts WHERE username=$1`, claims.Sub)
			Expect(err).NotTo(HaveOccurred())
		})
		provider = &sourceFixtureProvider{}
		provider.set(roster("3"), nil)
		srv = &server{db: database, sheetsProvider: provider, sheetsNow: func() time.Time { return now }}
		row := db.PlanRow{ID: newID(), Owner: claims.Sub, Name: "Atlas linked inputs", HorizonWeeks: 26, CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
		Expect(database.CreatePlan(row)).To(Succeed())
		DeferCleanup(func() { Expect(database.DeletePlan(row.ID)).To(Succeed()) })
		teams := []planning.Team{{Name: "Team A", Tracks: 2}, {Name: "Team B", Tracks: 1}}
		inits := []planning.Initiative{{Name: "Atlas", Work: map[string]planning.TeamWork{"Team A": {Weeks: 4, Estimated: true, InPath: true}}, EpicKeys: []string{"PROJ-1"}}}
		Expect(database.SavePlanTeams(row.ID, encode(teams), now.Unix())).To(Succeed())
		Expect(database.SavePlanInitiatives(row.ID, encode(inits), now.Unix())).To(Succeed())
		plan, err = database.GetPlan(row.ID)
		Expect(err).NotTo(HaveOccurred())
	})
	It("captures in review mode without changing inputs and deduplicates only consecutive content", func() {
		before := fingerprint()
		first := create("teams", "review")
		Expect(first.Version).NotTo(BeNil())
		Expect(first.Version.Valid).To(BeTrue())
		Expect(first.Applied).To(BeFalse())
		Expect(first.Source.PollMinutes).To(Equal(15))
		Expect(fingerprint()).To(Equal(before))
		same := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(same.Unchanged).To(BeTrue())
		Expect(same.Version.ID).To(Equal(first.Version.ID))
		provider.set(roster("4"), nil)
		changed := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(changed.Version.ID).NotTo(Equal(first.Version.ID))
		provider.set(roster("3"), nil)
		again := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(again.Version.ID).NotTo(Equal(first.Version.ID))
		Expect(again.Version.ContentHash).To(Equal(first.Version.ContentHash))
		history, err := database.ListSourceVersions(context.Background(), first.Source.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(3))
		Expect(fingerprint()).To(Equal(before))
	})
	It("retains invalid raw captures, refuses apply, and never turns provider errors into empty replacements", func() {
		first := create("teams", "review")
		before := fingerprint()
		provider.set([][]string{{"Name", "Tracks"}, {"Team A", "unknown"}}, nil)
		bad := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(bad.Version.Valid).To(BeFalse())
		Expect(bad.Version.Errors).NotTo(BeEmpty())
		Expect(bad.Version.Rows[1][1]).To(Equal("unknown"))
		rec := request("POST", path(first.Source.ID, "/apply"), map[string]any{"versionId": bad.Version.ID, "expectedFingerprint": before}, claims)
		Expect(rec.Code).To(Equal(422), rec.Body.String())
		provider.set(nil, errors.New("temporary provider failure"))
		failed := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(failed.Source.LastError).NotTo(BeEmpty())
		history, err := database.ListSourceVersions(context.Background(), first.Source.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(2))
		Expect(fingerprint()).To(Equal(before))
	})
	It("applies and restores captured versions without modifying the opposite input kind or agreement", func() {
		in, err := srv.planScheduleFor(plan, scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		bid := newID()
		_, err = database.SaveBaseline(db.BaselineRow{ID: bid, PlanID: plan.ID, Name: "Agreed scope", Inputs: encode(in), Schedule: encode(in.Recompute()), Fingerprint: in.Fingerprint(), CreatedAt: now.Unix()}, true)
		Expect(err).NotTo(HaveOccurred())
		baseline, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		originalInits := append([]byte(nil), plan.Initiatives...)
		first := create("teams", "review")
		oldVersion, err := database.GetSourceVersion(context.Background(), first.Source.ID, first.Version.ID)
		Expect(err).NotTo(HaveOccurred())
		apply := func(vid string) {
			rec := request("POST", path(first.Source.ID, "/apply"), map[string]any{"versionId": vid, "expectedFingerprint": fingerprint()}, claims)
			Expect(rec.Code).To(Equal(200), rec.Body.String())
		}
		apply(first.Version.ID)
		afterFirst := fingerprint()
		provider.set(roster("4"), nil)
		second := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		apply(second.Version.ID)
		Expect(fingerprint()).NotTo(Equal(afterFirst))
		apply(first.Version.ID)
		Expect(fingerprint()).To(Equal(afterFirst))
		Expect(reload().Initiatives).To(Equal(originalInits))
		unchanged, err := database.GetSourceVersion(context.Background(), first.Source.ID, first.Version.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged).To(Equal(oldVersion))
		currentBaseline, err := database.GetBaseline(plan.ID, bid)
		Expect(err).NotTo(HaveOccurred())
		Expect(currentBaseline).To(Equal(baseline))
	})
	It("refuses stale explicit previews and automatic overwrites after local edits", func() {
		first := create("teams", "auto_apply")
		Expect(first.Applied).To(BeTrue())
		stale := fingerprint()
		in, err := srv.planScheduleFor(reload(), scheduleRequest{})
		Expect(err).NotTo(HaveOccurred())
		in.Initiatives[0].Description = "Local review note"
		Expect(database.SavePlanInitiatives(plan.ID, encode(in.Initiatives), now.Unix())).To(Succeed())
		local := fingerprint()
		provider.set(roster("4"), nil)
		pending := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(pending.Applied).To(BeFalse())
		Expect(pending.Conflict).NotTo(BeEmpty())
		Expect(fingerprint()).To(Equal(local))
		rec := request("POST", path(first.Source.ID, "/apply"), map[string]any{"versionId": pending.Version.ID, "expectedFingerprint": stale}, claims)
		Expect(rec.Code).To(Equal(409), rec.Body.String())
		Expect(fingerprint()).To(Equal(local))
	})
	It("requires explicit review of dropped scope even with automatic updates enabled", func() {
		provider.set(matrix("4"), nil)
		first := create("initiatives", "auto_apply")
		Expect(first.Applied).To(BeTrue())
		before := fingerprint()
		provider.set(matrix("No Dependency"), nil)
		removed := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(removed.Applied).To(BeFalse())
		Expect(removed.Version.Removals).NotTo(BeEmpty())
		Expect(fingerprint()).To(Equal(before))
		body := map[string]any{"versionId": removed.Version.ID, "expectedFingerprint": before}
		rec := request("POST", path(first.Source.ID, "/apply"), body, claims)
		Expect(rec.Code).To(Equal(409), rec.Body.String())
		body["allowRemovals"] = true
		rec = request("POST", path(first.Source.ID, "/apply"), body, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
	})
	It("polls due sources server-side, honors pause, and retains history after disconnect", func() {
		first := create("teams", "review")
		provider.set(roster("4"), nil)
		calls := provider.calls
		srv.pollLinkedSheets(context.Background())
		Expect(provider.calls).To(Equal(calls))
		now = now.Add(16 * time.Minute)
		srv.pollLinkedSheets(context.Background())
		Expect(provider.calls).To(Equal(calls + 1))
		rec := request("PATCH", path(first.Source.ID, ""), map[string]any{"status": "paused", "mode": "auto_apply"}, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		before := fingerprint()
		provider.set(roster("5"), nil)
		now = now.Add(16 * time.Minute)
		srv.pollLinkedSheets(context.Background())
		Expect(provider.calls).To(Equal(calls + 1))
		manual := decodeCheck(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims))
		Expect(manual.Applied).To(BeFalse())
		Expect(fingerprint()).To(Equal(before))
		rec = request("DELETE", path(first.Source.ID, ""), nil, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		Expect(request("GET", path(first.Source.ID, "/versions"), nil, claims).Code).To(Equal(200))
		Expect(request("POST", path(first.Source.ID, "/check"), map[string]any{}, claims).Code).To(Equal(409))
	})
	It("isolates plans and roles for every nested source operation", func() {
		first := create("teams", "review")
		other := auth.Claims{Sub: "another-account", Roles: []string{"manager"}}
		for _, suffix := range []string{"", "/versions", "/versions/" + first.Version.ID} {
			rec := request("GET", path(first.Source.ID, suffix), nil, other)
			Expect(rec.Code).To(BeElementOf(403, 404))
		}
		for _, suffix := range []string{"/check", "/apply"} {
			rec := request("POST", path(first.Source.ID, suffix), map[string]any{"versionId": first.Version.ID, "expectedFingerprint": fingerprint()}, other)
			Expect(rec.Code).To(BeElementOf(403, 404))
		}
		rec := request("GET", path(first.Source.ID, "/versions"), nil, auth.Claims{Sub: claims.Sub, Roles: []string{"player"}})
		Expect(rec.Code).To(Equal(403))
		Expect(request("GET", path("unrelated-source", "/versions/"+first.Version.ID), nil, claims).Code).To(Equal(404))
	})
	It("rechecks a source paused while another due fetch is in progress", func() {
		var mu sync.Mutex
		var gated bool
		var fetched []string
		started := make(chan string, 1)
		release := make(chan struct{})
		done := make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		DeferCleanup(unblock)
		srv.sheetsProvider = sourceFetchFunc(func(ctx context.Context, _ string, tab string) ([][]string, error) {
			mu.Lock()
			block := gated
			if block {
				fetched = append(fetched, tab)
			}
			mu.Unlock()
			if block {
				select {
				case started <- tab:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			if tab == "Teams" {
				return roster("3"), nil
			}
			return matrix("4"), nil
		})
		createTab := func(kind, tab string) sheetCheckResult {
			return decodeCheck(request("POST", "/api/plan/"+plan.ID+"/sources", map[string]any{"kind": kind, "spreadsheetUrl": "https://docs.google.com/spreadsheets/d/generic-sheet-id/edit", "range": tab}, claims))
		}
		teams := createTab("teams", "Teams")
		inits := createTab("initiatives", "Initiatives")
		now = now.Add(16 * time.Minute)
		mu.Lock()
		gated = true
		mu.Unlock()
		go func() { defer GinkgoRecover(); defer close(done); srv.pollLinkedSheets(context.Background()) }()
		var first string
		Eventually(started, 3*time.Second).Should(Receive(&first))
		other := teams.Source.ID
		if first == "Teams" {
			other = inits.Source.ID
		}
		rec := request("PATCH", path(other, ""), map[string]any{"status": "paused"}, claims)
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		unblock()
		Eventually(done, 3*time.Second).Should(BeClosed())
		mu.Lock()
		calls := append([]string(nil), fetched...)
		mu.Unlock()
		Expect(calls).To(Equal([]string{first}), "the second source was paused after due-list selection and must not fetch")
	})
})
