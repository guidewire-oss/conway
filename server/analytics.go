package main

// Usage analytics (spec 016): the read side of the telemetry. Events are
// recorded fire-and-forget at the same call sites as the metrics counters;
// this file answers the HEART questions the admin page asks:
//   Adoption — the plan-setup funnel (created → teams → initiatives →
//   period start → schedule → baseline)
//   Engagement — daily/weekly actives, per-user feature breadth
//   Retention — this week's actives vs last week's
//   Task success — schedules computed, baselines saved, Jira imports

import (
	"encoding/json"
	"net/http"
	"time"

	"conway/server/auth"
	"conway/server/db"
	"github.com/rs/zerolog/log"
)

// recordEvent appends one business event to the analytics stream. Never
// blocks or fails the request: analytics is an observer of the system, not
// a participant — a full disk or dead connection costs nothing.
func (s *server) recordEvent(username, event, planID string, meta map[string]any) {
	if s.db == nil {
		return
	}
	b, _ := json.Marshal(meta)
	ev := db.AnalyticsEvent{Username: username, Event: event, PlanID: planID, Meta: b}
	go func() {
		if err := s.db.InsertAnalyticsEvent(ev); err != nil {
			log.Debug().Str("event", event).Err(err).Msg("analytics insert failed (ignored)")
		}
	}()
}

// analyticsSummary is the aggregate the admin page renders.
type analyticsSummary struct {
	From         string         `json:"from"`
	To           string         `json:"to"`
	TotalEvents  int            `json:"totalEvents"`
	DailyActives []dailyPoint   `json:"dailyActives"`   // engagement
	DailyEvents  []dailyPoint   `json:"dailyEvents"`    // volume
	EventCounts  map[string]int `json:"eventCounts"`    // top events
	Users        []userActivity `json:"users"`          // by-user table
	Funnel       []funnelStep   `json:"funnel"`         // adoption (HEART)
	ActiveThisWk int            `json:"activeThisWeek"` // retention
	ActiveLastWk int            `json:"activeLastWeek"`
}

type dailyPoint struct {
	Day   string `json:"day"`
	Users int    `json:"users"`
	Count int    `json:"count"`
}

type userActivity struct {
	User     string `json:"user"`
	Events   int    `json:"events"`
	Distinct int    `json:"distinctEvents"`
	Last     string `json:"lastSeen"`
	Plans    int    `json:"plansTouched"`
}

type funnelStep struct {
	Event string `json:"event"`
	Users int    `json:"users"`
}

// funnelOrder is the plan-setup journey whose completion rate is the drop-off
// detector (spec 016): a plan that was created but never scheduled is the
// story the funnel tells without interpretation.
var funnelOrder = []string{
	"plan_created", "teams_uploaded", "initiatives_uploaded",
	"schedules_computed", "baselines_saved",
}

// handleAdminAnalytics aggregates the event stream for the requested range.
// The heavy thinking happens in SQL's absence on purpose: one code path, and
// the page's range selectors (7/30/90 days) bound the rows.
func (s *server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request, _ auth.Claims) {
	if s.db == nil {
		http.Error(w, "analytics requires the database", http.StatusServiceUnavailable)
		return
	}
	days := 30
	switch r.URL.Query().Get("days") {
	case "7":
		days = 7
	case "90":
		days = 90
	}
	to := time.Now()
	from := to.AddDate(0, 0, -days)
	events, err := s.db.ListAnalyticsEvents(from, to)
	if err != nil {
		s.logger().Error().Err(err).Msg("analytics query failed")
		http.Error(w, err.Error(), 500)
		return
	}

	sum := analyticsSummary{
		From:        from.Format(time.RFC3339),
		To:          to.Format(time.RFC3339),
		TotalEvents: len(events),
		EventCounts: map[string]int{},
	}
	sum.aggregate(events, from, to, days)
	writeJSON(w, sum)
}

// aggregate computes the daily series, event counts, per-user table, funnel,
// and week-over-week actives from the raw events in one pass plus small
// second passes (all in-memory; the row count is bounded by the selector).
func (a *analyticsSummary) aggregate(events []db.AnalyticsEventRow, from, to time.Time, days int) {
	a.EventCounts = map[string]int{} // the zero summary has nil maps: aggregate owns initialization
	dayUsers := map[string]map[string]bool{}
	dayCounts := map[string]int{}
	userEvents := map[string]int{}
	userDistinct := map[string]map[string]bool{}
	userLast := map[string]time.Time{}
	userPlans := map[string]map[string]bool{}
	weekThis := map[string]bool{}
	weekLast := map[string]bool{}
	wkAgo := to.AddDate(0, 0, -7)

	for _, e := range events {
		a.EventCounts[e.Event]++
		day := e.TS.Format("2006-01-02")
		if dayUsers[day] == nil {
			dayUsers[day] = map[string]bool{}
		}
		if e.Username != "" {
			dayUsers[day][e.Username] = true
			userEvents[e.Username]++
			if userDistinct[e.Username] == nil {
				userDistinct[e.Username] = map[string]bool{}
			}
			userDistinct[e.Username][e.Event] = true
			if e.TS.After(userLast[e.Username]) {
				userLast[e.Username] = e.TS
			}
			if e.PlanID != "" {
				if userPlans[e.Username] == nil {
					userPlans[e.Username] = map[string]bool{}
				}
				userPlans[e.Username][e.PlanID] = true
			}
			if e.TS.After(wkAgo) {
				weekThis[e.Username] = true
			} else {
				weekLast[e.Username] = true
			}
		}
		dayCounts[day]++
	}

	// Daily series across the whole range, zero-filled: gaps are signal, not
	// missing data — a flat zero week says "nobody opened Conway."
	for i := 0; i < days; i++ {
		day := from.AddDate(0, 0, i).Format("2006-01-02")
		a.DailyActives = append(a.DailyActives, dailyPoint{Day: day, Users: len(dayUsers[day]), Count: dayCounts[day]})
	}

	// Users table, most active first.
	for u, n := range userEvents {
		a.Users = append(a.Users, userActivity{
			User: u, Events: n,
			Distinct: len(userDistinct[u]),
			Last:     userLast[u].Format(time.RFC3339),
			Plans:    len(userPlans[u]),
		})
	}
	for i := range a.Users {
		for j := i + 1; j < len(a.Users); j++ {
			if a.Users[j].Events > a.Users[i].Events {
				a.Users[i], a.Users[j] = a.Users[j], a.Users[i]
			}
		}
	}

	// Funnel: distinct users per ordered step (not raw counts — one user
	// scheduling 40 times must not read as 40 conversions).
	funnelUsers := map[string]map[string]bool{}
	for _, e := range events {
		if funnelUsers[e.Event] == nil {
			funnelUsers[e.Event] = map[string]bool{}
		}
		if e.Username != "" {
			funnelUsers[e.Event][e.Username] = true
		}
	}
	for _, step := range funnelOrder {
		a.Funnel = append(a.Funnel, funnelStep{Event: step, Users: len(funnelUsers[step])})
	}

	a.ActiveThisWk = len(weekThis)
	a.ActiveLastWk = len(weekLast)
}
