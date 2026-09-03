package main

import (
	"time"

	"conway/server/db"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("analytics aggregation", func() {
	It("computes daily actives, counts, users, funnel, and weekly retention", func() {
		now := time.Now()
		mk := func(username, event, plan string, ts time.Time) db.AnalyticsEventRow {
			return db.AnalyticsEventRow{Username: username, Event: event, PlanID: plan, TS: ts}
		}
		events := []db.AnalyticsEventRow{
			mk("ana", "plan_created", "P1", now),
			mk("ana", "teams_uploaded", "P1", now),
			mk("ana", "schedules_computed", "P1", now),
			mk("bob", "plan_created", "P2", now),             // bob created but never scheduled: funnel drop
			mk("ana", "login", "", now.Add(-9*24*time.Hour)), // last week's active
			mk("cat", "login", "", now),
		}
		sum := &analyticsSummary{}
		sum.aggregate(events, now.AddDate(0, 0, -30), now, 30)

		Expect(sum.EventCounts["plan_created"]).To(Equal(2))
		// funnel counts DISTINCT USERS per step — one user scheduling 40 times is one conversion
		Expect(sum.Funnel[0].Event).To(Equal("plan_created"))
		Expect(sum.Funnel[0].Users).To(Equal(2), "ana and bob both created")
		Expect(sum.Funnel[3].Event).To(Equal("schedules_computed"))
		Expect(sum.Funnel[3].Users).To(Equal(1), "only ana scheduled — bob dropped off")
		Expect(sum.Users[0].User).To(Equal("ana"), "most active first")
		Expect(sum.ActiveThisWk).To(Equal(3)) // ana, bob, cat all have events this week
		Expect(sum.ActiveLastWk).To(Equal(1)) // ana's 9-day-old event (last week only)
		Expect(sum.DailyActives).To(HaveLen(30), "zero-filled across the range")
	})

	It("does not count anonymous events as user activity", func() {
		now := time.Now()
		sum := &analyticsSummary{}
		sum.aggregate([]db.AnalyticsEventRow{{Event: "login_failed", TS: now}}, now.AddDate(0, 0, -7), now, 7)
		Expect(sum.Users).To(BeEmpty())
		Expect(sum.ActiveThisWk).To(Equal(0))
		Expect(sum.EventCounts["login_failed"]).To(Equal(1))
	})
})
