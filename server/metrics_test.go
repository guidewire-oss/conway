package main

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {
	It("counts incs and adds into named counters", func() {
		m := NewMetrics()
		m.Inc("plans_created")
		m.Inc("plans_created")
		m.Inc("logins")
		m.Add("http_2xx", 5)
		Expect(m.Snapshot()).To(HaveKeyWithValue("plans_created", int64(2)))
		Expect(m.Snapshot()).To(HaveKeyWithValue("logins", int64(1)))
		Expect(m.Snapshot()).To(HaveKeyWithValue("http_2xx", int64(5)))
	})

	It("is safe under concurrent increments", func() {
		m := NewMetrics()
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); m.Inc("logins") }()
		}
		wg.Wait()
		Expect(m.Snapshot()["logins"]).To(Equal(int64(100)), "concurrent inc lost a count")
	})

	It("folds item ids out of request paths for the buckets", func() {
		cases := map[string]string{
			"/api/plan":                                   "/api/plan",
			"/api/plan/PLAx123":                           "/api/plan/:id",
			"/api/plan/PLAx123/schedule":                  "/api/plan/:id/schedule",
			"/api/plan/PLAx123/baseline/b1":               "/api/plan/:id/baseline/:id",
			"/api/plan/PLAx123/baseline/b1/compare-to/b2": "/api/plan/:id/baseline/:id/compare-to/b2",
			"/api/roster/r9":                              "/api/roster/:id",
			"/api/admin/metrics":                          "/api/admin/metrics",
			"/api/game/g1/round":                          "/api/game/:id/round",
		}
		for in, want := range cases {
			Expect(bucketPath(in)).To(Equal(want), "bucketPath(%q)", in)
		}
	})

	It("buckets status codes into classes", func() {
		Expect(statusClass(200)).To(Equal("2xx"))
		Expect(statusClass(404)).To(Equal("4xx"))
		Expect(statusClass(500)).To(Equal("5xx"))
	})
})
