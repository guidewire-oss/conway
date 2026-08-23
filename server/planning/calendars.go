package planning

// FR-018: calendar constraints. A CalendarWindow is a dated span with a scope
// (org-wide, a site, or a named pod) and an effect:
//
//   - block-start: no slice may BEGIN in a frozen week — releases and starts
//     wait for the window to lift (AC X.3).
//   - block-finish: no slice may COMPLETE in a frozen week — the finish moves
//     to the first week after the window.
//   - reduce-capacity: the scoped pods run at reduced (typically zero) tracks
//     inside the window — site holidays, onboarding gaps, a pod's ramp.
//
// Windows are dates on the wire (§7) and weeks inside the engine, mapped off
// the period start with toDate inclusive: a freeze "to" Friday covers that
// Friday's week, and work resumes the week after. Windows that fall outside
// the period, or arrive without a period start to map against, are inert —
// there is no week they could describe, and inventing one would move a
// schedule on a schedule the planner cannot see.

import (
	"strings"
	"time"
)

const (
	CalSiteNonWorking = "site-nonworking"
	CalChangeFreeze   = "change-freeze"
	CalEvent          = "event"

	ScopeOrg = "org" // the empty scope is org-wide too; this is the explicit name

	EffectReduceCapacity = "reduce-capacity"
	EffectBlockStart     = "block-start"
	EffectBlockFinish    = "block-finish"
)

// CalendarWindow is §7's wire shape.
type CalendarWindow struct {
	Kind   string `json:"kind"`     // site-nonworking | change-freeze | event
	Scope  string `json:"scope"`    // "org" or a site/pod name; empty = org
	From   string `json:"fromDate"` // ISO date, inclusive
	To     string `json:"toDate"`   // ISO date, inclusive
	Effect string `json:"effect"`   // reduce-capacity | block-start | block-finish
}

// calendarWindow is the engine's week-mapped form. One CalendarWindow may be
// inert (outside the period); only windows that map become these.
type calendarWindow struct {
	kind     string
	scope    string
	effect   string
	fromWeek int
	toWeek   int
}

// parseCalendars maps the wire windows onto weeks. Scope normalisation happens
// here once: "" and "org" are the same thing, and site matching against roster
// names is case- and whitespace-insensitive the way every other pod-name join
// in the app already is.
func parseCalendars(sp SchedulingParams, horizon int) []calendarWindow {
	if strings.TrimSpace(sp.PeriodStart) == "" || len(sp.Calendars) == 0 {
		return nil
	}
	const iso = "2006-01-02"
	t0, err := time.Parse(iso, strings.TrimSpace(sp.PeriodStart))
	if err != nil {
		return nil
	}
	out := make([]calendarWindow, 0, len(sp.Calendars))
	for _, w := range sp.Calendars {
		f, err1 := time.Parse(iso, strings.TrimSpace(w.From))
		t, err2 := time.Parse(iso, strings.TrimSpace(w.To))
		if err1 != nil || err2 != nil {
			continue // an unparseable date is a data error, not a constraint
		}
		from := weekIndexOf(t0, f)
		to := weekIndexOf(t0, t)
		// The schedule exposes weeks 0..horizon-1; horizon itself is the first
		// week past the end, so a window starting there constrains nothing.
		if horizon <= 0 || to < 0 || from >= horizon {
			continue // entirely before or after the period: inert
		}
		if from < 0 {
			from = 0 // started before the period: constrains from week 0
		}
		if to >= horizon {
			to = horizon - 1
		}
		if from > to {
			continue
		}
		out = append(out, calendarWindow{
			kind: w.Kind, scope: strings.ToLower(strings.TrimSpace(w.Scope)),
			effect: w.Effect, fromWeek: from, toWeek: to,
		})
	}
	return out
}

// weekIndexOf maps a date to its week from the period start, rounding toward
// minus infinity — a date part-way through a week is in that week, and a date
// up to six days before the period start is week -1, not week 0. Truncation
// toward zero here once made an entirely pre-period window constrain week 0.
func weekIndexOf(t0, t time.Time) int {
	days := int(t.Sub(t0).Hours() / 24)
	if days >= 0 {
		return days / 7
	}
	return -(( -days + 6) / 7)
}

// calendarRules is the compiled constraint set the scheduler consults. Every
// effect is scope-aware: org-wide windows apply to all pods, and a window
// scoped to a site or pod applies only there — a Kraków freeze must not stop
// Toronto starting, and the reverse omission (an org window ignored because
// only site scopes were probed) is the bug this shape replaces.
type calendarRules struct {
	orgBlockStart  map[int]bool
	orgBlockFinish map[int]bool
	// scoped[scope] holds block-start and block-finish weeks for one site/pod.
	scopedBlockStart  map[string]map[int]bool
	scopedBlockFinish map[string]map[int]bool
	// reduced[scope] lists the week spans where that site/pod runs reduced.
	// The org scope means every pod.
	reduced map[string][]calendarWindow
}

func compileCalendars(ws []calendarWindow) *calendarRules {
	r := &calendarRules{
		orgBlockStart: map[int]bool{}, orgBlockFinish: map[int]bool{},
		scopedBlockStart: map[string]map[int]bool{}, scopedBlockFinish: map[string]map[int]bool{},
		reduced: map[string][]calendarWindow{},
	}
	isOrg := func(scope string) bool { return scope == "" || scope == ScopeOrg }
	for _, w := range ws {
		switch w.effect {
		case EffectBlockStart:
			if isOrg(w.scope) {
				for k := w.fromWeek; k <= w.toWeek; k++ {
					r.orgBlockStart[k] = true
				}
			} else {
				m := r.scopedBlockStart[w.scope]
				if m == nil {
					m = map[int]bool{}
					r.scopedBlockStart[w.scope] = m
				}
				for k := w.fromWeek; k <= w.toWeek; k++ {
					m[k] = true
				}
			}
		case EffectBlockFinish:
			if isOrg(w.scope) {
				for k := w.fromWeek; k <= w.toWeek; k++ {
					r.orgBlockFinish[k] = true
				}
			} else {
				m := r.scopedBlockFinish[w.scope]
				if m == nil {
					m = map[int]bool{}
					r.scopedBlockFinish[w.scope] = m
				}
				for k := w.fromWeek; k <= w.toWeek; k++ {
					m[k] = true
				}
			}
		case EffectReduceCapacity:
			r.reduced[w.scope] = append(r.reduced[w.scope], w)
		}
	}
	return r
}

// startBlocked reports whether week w refuses a start for this pod — org-wide
// or scoped to its site or pod name. Names are normalised the way parseCalendars
// normalised the scopes: lowercased and trimmed, the same join every other
// pod-name match in the app uses.
func (r *calendarRules) startBlocked(pod, site string, w int) bool {
	if r.orgBlockStart[w] {
		return true
	}
	pod, site = normScope(pod), normScope(site)
	return r.scopedBlockStart[pod][w] || r.scopedBlockStart[site][w]
}

// finishBlocked reports whether week w refuses a completion for this pod.
func (r *calendarRules) finishBlocked(pod, site string, w int) bool {
	if r.orgBlockFinish[w] {
		return true
	}
	pod, site = normScope(pod), normScope(site)
	return r.scopedBlockFinish[pod][w] || r.scopedBlockFinish[site][w]
}

func normScope(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// firstStartFrom advances a candidate start week past any blocked-start week.
// The freeze is the binding constraint whenever it moved the slice, so the
// move is reported alongside.
func (r *calendarRules) firstStartFrom(pod, site string, from int) (int, bool) {
	w := from
	for r.startBlocked(pod, site, w) {
		w++
	}
	return w, w != from
}

// firstFinishFrom advances a candidate finish week past any blocked-finish
// week, so the completion lands on legal ground rather than being quietly
// clipped.
func (r *calendarRules) firstFinishFrom(pod, site string, finish int) (int, bool) {
	w := finish
	for r.finishBlocked(pod, site, w) {
		w++
	}
	return w, w != finish
}

// reducedTracks returns the tracks a pod offers in week w, after the
// reduce-capacity windows that scope it: its own name, its site, or the org
// (the whole org on holiday is a real thing — a company-wide shutdown). The
// current model reduces to zero — a holiday is not a partial holiday — but the
// window shape allows a future fraction without changing the wire format.
func (r *calendarRules) reducedTracks(pod, site string, tracks, w int) int {
	if tracks <= 0 {
		return 0
	}
	scopes := []string{strings.ToLower(strings.TrimSpace(pod)), strings.ToLower(strings.TrimSpace(site)), ScopeOrg}
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		for _, win := range r.reduced[scope] {
			if w >= win.fromWeek && w <= win.toWeek {
				return 0
			}
		}
	}
	return tracks
}
