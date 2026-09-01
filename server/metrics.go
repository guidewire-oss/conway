// Metrics: a tiny in-process counter registry for admin usage awareness
// (spec: telemetry). Zero dependencies — atomic counters, one mutex, a JSON
// snapshot. Not a time-series: counters are "since boot", which is exactly
// the question an admin asks first ("is this thing being used?"). Persistence
// is deliberately out: a restart resetting usage counts is honest, and a
// durable store would be a schema + retention problem this does not need.
package main

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Metrics groups named atomic counters. Names are stable strings the admin
// panel renders as-is (metric "plans_created" shows as "plans created").
type Metrics struct {
	mu       sync.Mutex
	counters map[string]*atomic.Int64
}

// NewMetrics returns an empty registry.
func NewMetrics() *Metrics {
	return &Metrics{counters: map[string]*atomic.Int64{}}
}

// Inc adds one to the named counter.
func (m *Metrics) Inc(name string) {
	m.Add(name, 1)
}

// Add adds delta to the named counter. A nil registry is a no-op, so test
// harnesses constructing bare &server{} need no metrics wiring to exercise
// handlers.
func (m *Metrics) Add(name string, delta int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	c, ok := m.counters[name]
	if !ok {
		c = &atomic.Int64{}
		m.counters[name] = c
	}
	m.mu.Unlock()
	c.Add(delta)
}

// Snapshot returns all counters sorted by name — the JSON shape the admin
// endpoint serves and the panel renders. A nil registry reports empty.
func (m *Metrics) Snapshot() map[string]int64 {
	if m == nil {
		return map[string]int64{}
	}
	m.mu.Lock()
	names := make([]string, 0, len(m.counters))
	for n := range m.counters {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make(map[string]int64, len(names))
	for _, n := range names {
		out[n] = m.counters[n].Load()
	}
	m.mu.Unlock()
	return out
}

// bucketPath normalizes a request path so per-ID routes count together:
// /api/plan/PLAx123/schedule becomes /api/plan/:id/schedule. The opaque ID is
// the first segment after a known item prefix; deeper segments (the action,
// and a second baseline id) stay, so distinct operations remain distinct.
func bucketPath(p string) string {
	for _, prefix := range []string{"/api/plan/", "/api/roster/", "/api/snapshot/", "/api/game/"} {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			continue
		}
		seg, tail, _ := strings.Cut(rest, "/")
		if seg == "" {
			return prefix
		}
		if tail == "" {
			return prefix + ":id"
		}
		// a baseline item route carries a second id: fold it too
		if brest, ok := strings.CutPrefix(tail, "baseline/"); ok {
			if _, btail, found := strings.Cut(brest, "/"); found {
				return prefix + ":id/baseline/:id/" + btail
			}
			return prefix + ":id/baseline/:id"
		}
		return prefix + ":id/" + tail
	}
	return p
}

// statusClass buckets a status code into 2xx/4xx/5xx for the request counters.
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	default:
		return "2xx"
	}
}
