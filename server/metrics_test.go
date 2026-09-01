package main

import (
	"sync"
	"testing"
)

func TestMetricsIncAndSnapshot(t *testing.T) {
	m := NewMetrics()
	m.Inc("plans_created")
	m.Inc("plans_created")
	m.Inc("logins")
	m.Add("http_2xx", 5)
	snap := m.Snapshot()
	if snap["plans_created"] != 2 || snap["logins"] != 1 || snap["http_2xx"] != 5 {
		t.Fatalf("counts wrong: %v", snap)
	}
}

func TestMetricsConcurrent(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Inc("logins") }()
	}
	wg.Wait()
	if got := m.Snapshot()["logins"]; got != 100 {
		t.Fatalf("concurrent inc lost: %d", got)
	}
}

func TestBucketPath(t *testing.T) {
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
		if got := bucketPath(in); got != want {
			t.Errorf("bucketPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusClass(t *testing.T) {
	if statusClass(200) != "2xx" || statusClass(404) != "4xx" || statusClass(500) != "5xx" {
		t.Fatal("status class bucketing wrong")
	}
}
