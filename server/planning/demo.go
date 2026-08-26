package planning

import "time"

// Demo returns a realistic, self-contained sample plan (fictional pod names, a
// year's worth of platform initiatives) engineered so a couple of pods are
// clearly over capacity — so the network/constraints light up out of the box
// without an upload. Delta (a small pod that everything depends on) ends up
// the red constraint; Ember runs hot amber.
func Demo() ([]Team, []Initiative) {
	teams := []Team{
		{Name: "Atlas", Tracks: 6, Site: "Austin"},
		{Name: "Beacon", Tracks: 7, Site: "Dublin"},
		{Name: "Cascade", Tracks: 7, Site: "Toronto"},
		{Name: "Delta", Tracks: 2, Site: "Remote"},
		{Name: "Ember", Tracks: 2, Site: "Warsaw"},
		{Name: "Fjord", Tracks: 4, Site: "Oslo"},
		{Name: "Granite", Tracks: 5, Site: "Denver"},
		{Name: "Harbor", Tracks: 6, Site: "Singapore"},
		{Name: "Ibis", Tracks: 5, Site: "Remote"},
		{Name: "Juniper", Tracks: 4, Site: "Berlin"},
	}
	w := func(weeks float64, deps ...string) TeamWork {
		return TeamWork{Weeks: weeks, Estimated: true, InPath: true, DependsOn: deps}
	}
	mk := func(name string, work map[string]TeamWork) Initiative {
		return Initiative{Name: name, Work: work}
	}
	inits := []Initiative{
		mk("Self-service app platform", map[string]TeamWork{"Delta": w(10), "Atlas": w(5, "Delta"), "Cascade": w(4, "Atlas")}),
		mk("Telemetry GA", map[string]TeamWork{"Delta": w(12), "Granite": w(4, "Delta")}),
		mk("Autoscaling rollout", map[string]TeamWork{"Delta": w(10), "Fjord": w(3, "Delta")}),
		mk("Managed database MVP", map[string]TeamWork{"Delta": w(9), "Harbor": w(5, "Delta")}),
		mk("Disaster recovery for event streaming", map[string]TeamWork{"Delta": w(8), "Ibis": w(4, "Delta")}),
		mk("Bring-your-own-auth (early access)", map[string]TeamWork{"Ember": w(10), "Atlas": w(4, "Ember"), "Beacon": w(3)}),
		mk("SCIM provisioning", map[string]TeamWork{"Beacon": w(6), "Ember": w(11, "Beacon")}),
		mk("Secrets rotation", map[string]TeamWork{"Ember": w(12), "Juniper": w(4, "Ember")}),
		mk("Tenant isolation", map[string]TeamWork{"Ember": w(13, "Cascade"), "Cascade": w(5), "Granite": w(3)}),
		mk("Access-control rollout", map[string]TeamWork{"Granite": w(6), "Harbor": w(4), "Ibis": w(3)}),
	}
	return teams, withDemoSequencing(inits)
}

// DemoPeriodStart is the demo plan's week 0. A fixed date rather than "today", so
// the demo's dates and verdicts are the same for everyone who opens it and do not
// quietly rot as the calendar moves.
const DemoPeriodStart = "2026-01-05"

// DemoScheduling is the scheduling policy the demo plan is created with, so the
// execution order has a period to be measured against out of the box.
//
// maxConcurrentInitiatives is deliberately left at 0: Decision 22 derives it from
// the tracks at the drum pod, and the demo exists partly to show that derivation
// happening and being labelled.
func DemoScheduling() SchedulingParams {
	// The demo plan shows off the engine's proposal, so it carries the accepted-
	// engine marker; a fresh planner plan defaults to the stated order instead
	// (spec 006 Decision 1).
	return SchedulingParams{PeriodStart: DemoPeriodStart, KitGate: 0, TargetUtilization: 0,
		AcceptedOrdering: "engine"}
}

// withDemoSequencing gives the demo plan the sequencing attributes spec 001 adds,
// so the execution order shows the things it exists to show: a ranking that
// disagrees with the stated order, a date that holds, a date that misses on
// contention, one that no ordering could meet, a lock that costs something, and
// carryover already in flight.
//
// Without these the demo schedules as ten undated initiatives, which is a correct
// but very boring screen — and Decision 8 already noted the demo data needs
// extending for any of this to be visible.
func withDemoSequencing(inits []Initiative) []Initiative {
	// Weeks from DemoPeriodStart, so the intent of each date is readable here rather
	// than requiring the reader to do calendar arithmetic.
	day := func(weeks int) string {
		t, err := time.Parse("2006-01-02", DemoPeriodStart)
		if err != nil {
			return ""
		}
		return t.AddDate(0, 0, weeks*7).Format("2006-01-02")
	}
	type seq struct {
		priority int
		locked   bool
		targetWk int // 0 = no date
		dateLock bool
		tier     int
		cod      float64
		inFlight bool
		progress float64
	}
	// Ordered to match inits above. The dates are chosen against what the plan can
	// actually deliver — a chain inflated by capacityLoss, plus a 25% buffer — so the
	// demo shows a range of verdicts rather than ten impossible ones. A date nobody
	// could ever hit teaches nothing except that the tool says no.
	attrs := []seq{
		// Chain runs past the horizon however it is ordered: no in-period date fits.
		{priority: 1, targetWk: 24, tier: 2, cod: 6}, // Self-service app platform
		// Lands exactly on its buffered commit, so the contractual date holds.
		{priority: 2, targetWk: 24, dateLock: true, tier: 1, cod: 9}, // Telemetry GA
		{priority: 10, tier: 3, cod: 3},                              // Autoscaling rollout: undated
		// Would make week 22 on its own; loses it to contention at the drum.
		{priority: 4, targetWk: 22, tier: 2, cod: 7}, // Managed database MVP
		// Asked for week 12 against an 18-week chain: no ordering meets it.
		{priority: 5, targetWk: 12, tier: 1, cod: 8}, // Disaster recovery
		// Already 40% done, so it holds a WIP slot from week 0 and lands early.
		{priority: 6, targetWk: 14, tier: 2, cod: 4, inFlight: true, progress: 0.4}, // BYO-auth
		{priority: 7, tier: 4, cod: 1},                                              // SCIM provisioning: undated
		{priority: 8, targetWk: 26, tier: 3, cod: 2},                                // Secrets rotation: misses on contention
		{priority: 9, targetWk: 22, tier: 2, cod: 5},                                // Tenant isolation: chain past the horizon
		// The cheapest work in the plan, pinned near the top. This is the lock that
		// costs something, and the reconciliation report exists to price it.
		{priority: 3, locked: true, tier: 4, cod: 1}, // Access-control rollout
	}
	out := make([]Initiative, len(inits))
	copy(out, inits)
	for i := range out {
		if i >= len(attrs) {
			break
		}
		a := attrs[i]
		out[i].StatedPriority = a.priority
		out[i].PriorityLocked = a.locked
		out[i].DateLocked = a.dateLock
		out[i].Tier = a.tier
		out[i].CostOfDelayPerWeek = a.cod
		out[i].InFlight = a.inFlight
		out[i].ProgressPct = a.progress
		if a.targetWk > 0 {
			out[i].TargetDate = day(a.targetWk)
		}
	}
	return out
}
