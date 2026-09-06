package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type announcementTestProvider struct{}

func (announcementTestProvider) Fetch(context.Context, string, string) ([][]string, error) {
	return nil, nil
}

var _ = Describe("feature announcement API", func() {
	It("requires account authentication even if persistence is unavailable", func() {
		srv := &server{}
		for _, claims := range []auth.Claims{{}, {Sub: "guest", GameID: "example-game"}} {
			get := httptest.NewRecorder()
			srv.handleAnnouncements(get, httptest.NewRequest("GET", "/api/announcements", nil), claims)
			Expect(get.Code).To(Equal(401))
			ack := httptest.NewRecorder()
			srv.handleAnnouncementAck(ack, httptest.NewRequest("POST", "/api/announcements/ack", strings.NewReader(`{"id":"guide-navigation-v1","kind":"visited"}`)), claims)
			Expect(ack.Code).To(Equal(401))
		}
	})
	It("filters roles and provider availability before acknowledging catalog IDs", func() {
		srv := &server{}
		player := auth.Claims{Sub: "account-a", Roles: []string{"player"}}
		manager := auth.Claims{Sub: "account-b", Roles: []string{"manager"}}
		Expect(srv.announcementCatalog(player)).To(HaveLen(1))
		Expect(srv.announcementCatalog(manager)).To(HaveLen(3))
		srv.sheetsProvider = announcementTestProvider{}
		Expect(srv.announcementCatalog(manager)).To(HaveLen(4))
		ids := []string{}
		for _, feature := range srv.announcementCatalog(manager) {
			ids = append(ids, feature.ID)
			if feature.ID == "weekly-execution-review-v1" {
				Expect(feature.Action.Target).To(Equal("view-execution"))
			}
		}
		Expect(ids).To(ContainElements("execution-review-v1", "weekly-execution-review-v1"))
		Expect(srv.announcementCatalog(player)).To(HaveLen(1))
		ack := httptest.NewRecorder()
		srv.handleAnnouncementAck(ack, httptest.NewRequest("POST", "/api/announcements/ack", strings.NewReader(`{"id":"execution-review-v1","kind":"visited"}`)), player)
		Expect(ack.Code).To(Equal(404))
	})
	DescribeTable("rejects invalid acknowledgement bodies", func(body string) {
		rec := httptest.NewRecorder()
		(&server{}).handleAnnouncementAck(rec, httptest.NewRequest("POST", "/api/announcements/ack", strings.NewReader(body)), auth.Claims{Sub: "account-a"})
		Expect(rec.Code).To(Equal(400))
	}, Entry("invalid kind", `{"id":"guide-navigation-v1","kind":"dismissed"}`), Entry("client-selected identity", `{"id":"guide-navigation-v1","kind":"visited","subject":"another-account"}`), Entry("trailing JSON", `{"id":"guide-navigation-v1","kind":"visited"} {}`))
})

var _ = Describe("announcement persistence", Label("database"), func() {
	var database *db.DB
	var pool *pgxpool.Pool
	var srv *server
	var accountA, accountB string
	BeforeEach(func() {
		url := os.Getenv("CONWAY_TEST_DATABASE_URL")
		if url == "" {
			Skip("Set CONWAY_TEST_DATABASE_URL to run isolated PostgreSQL integration checks.")
		}
		var err error
		database, err = db.Open(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(database.Close)
		pool, err = pgxpool.New(context.Background(), url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)
		accountA = "announcement-test-" + newID()
		accountB = "announcement-test-" + newID()
		DeferCleanup(func() {
			_, err := pool.Exec(context.Background(), `DELETE FROM feature_announcement_state WHERE subject = ANY($1)`, []string{accountA, accountB})
			Expect(err).NotTo(HaveOccurred())
		})
		srv = &server{db: database}
	})
	ack := func(subject, kind string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.handleAnnouncementAck(rec, httptest.NewRequest("POST", "/api/announcements/ack", strings.NewReader(`{"id":"guide-navigation-v1","kind":"`+kind+`"}`)), auth.Claims{Sub: subject})
		return rec
	}
	load := func(subject string) featureAnnouncement {
		rec := httptest.NewRecorder()
		srv.handleAnnouncements(rec, httptest.NewRequest("GET", "/api/announcements", nil), auth.Claims{Sub: subject})
		Expect(rec.Code).To(Equal(200), rec.Body.String())
		var body struct{ Features []featureAnnouncement }
		Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Features).To(HaveLen(1))
		return body.Features[0]
	}
	It("persists independent states across server recreation without exposing another account", func() {
		Expect(load(accountA).Announced).To(BeFalse())
		Expect(load(accountA).Visited).To(BeFalse())
		Expect(ack(accountA, "announced").Code).To(Equal(200))
		srv = &server{db: database}
		Expect(load(accountA).Announced).To(BeTrue())
		Expect(load(accountA).Visited).To(BeFalse())
		Expect(load(accountB).Announced).To(BeFalse())
		Expect(load(accountB).Visited).To(BeFalse())
		Expect(ack(accountB, "visited").Code).To(Equal(200))
		Expect(load(accountB).Visited).To(BeTrue())
		Expect(load(accountB).Announced).To(BeFalse())
		Expect(load(accountA).Visited).To(BeFalse())
	})
	It("keeps first timestamps on repeated acknowledgements", func() {
		Expect(ack(accountA, "announced").Code).To(Equal(200))
		var first, after time.Time
		Expect(pool.QueryRow(context.Background(), `SELECT announced_at FROM feature_announcement_state WHERE subject=$1 AND feature_id=$2`, accountA, "guide-navigation-v1").Scan(&first)).To(Succeed())
		Expect(ack(accountA, "announced").Code).To(Equal(200))
		Expect(ack(accountA, "visited").Code).To(Equal(200))
		Expect(pool.QueryRow(context.Background(), `SELECT announced_at FROM feature_announcement_state WHERE subject=$1 AND feature_id=$2`, accountA, "guide-navigation-v1").Scan(&after)).To(Succeed())
		Expect(after).To(Equal(first))
		Expect(load(accountA).Announced).To(BeTrue())
		Expect(load(accountA).Visited).To(BeTrue())
	})
})
