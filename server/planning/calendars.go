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
		if to < 0 || from > horizon {
			continue // entirely before or after the period: inert
		}
		if from < 0 {
			from = 0 // started before the period: constrains from week 0
		}
		if to > horizon {
			to = horizon
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

// weekIndexOf maps a date to its week from the period start, rounding down —
// a date part-way through a week is in that week. Negative means before the
// period start.
func weekIndexOf(t0, t time.Time) int {
	days := t.Sub(t0).Hours() / 24
	return int(days / 7)
}

// calendarRules is the compiled constraint set the scheduler consults:
// the frozen weeks (per effect) and the reduced-capacity spans (per scope).
type calendarRules struct {
	blockStart  map[int]bool // org-wide block-start weeks
	blockFinish map[int]bool // org-wide block-finish weeks
	// reduced[scope] lists the week spans where that site/pod runs reduced.
	reduced map[string][]calendarWindow
}

func compileCalendars(ws []calendarWindow) *calendarRules {
	r := &calendarRules{blockStart: map[int]bool{}, blockFinish: map[int]bool{}, reduced: map[string][]calendarWindow{}}
	for _, w := range ws {
		switch w.effect {
		case EffectBlockStart:
			if w.scope == "" || w.scope == ScopeOrg {
				for k := w.fromWeek; k <= w.toWeek; k++ {
					r.blockStart[k] = true
				}
			}
		case EffectBlockFinish:
			if w.scope == "" || w.scope == ScopeOrg {
				for k := w.fromWeek; k <= w.toWeek; k++ {
					r.blockFinish[k] = true
				}
			}
		case EffectReduceCapacity:
			r.reduced[w.scope] = append(r.reduced[w.scope], w)
		}
	}
	return r
}

// firstStartFrom advances a candidate start week past any blocked-start week.
// The freeze is the binding constraint whenever it moved the slice, so the
// reason is returned alongside.
func (r *calendarRules) firstStartFrom(_ string, from int) (int, bool) {
	w := from
	for r.blockStart[w] {
		w++
	}
	return w, w != from
}

// firstFinishFrom advances a candidate finish week past any blocked-finish
// week, stretching the slice's duration by the same amount so the completion
// lands on legal ground rather than being quietly clipped.
func (r *calendarRules) firstFinishFrom(_ string, finish int) (int, bool) {
	w := finish
	for r.blockFinish[w] {
		w++
	}
	return w, w != finish
}

// reducedTracks returns the tracks a pod offers in week w, after the
// reduce-capacity windows that scope it. The current model reduces to zero —
// a site holiday is not a partial holiday — but the window shape allows a
// future fraction without changing the wire format.
func (r *calendarRules) reducedTracks(pod, site string, tracks, w int) int {
	if tracks <= 0 {
		return 0
	}
	for _, scope := range []string{strings.ToLower(strings.TrimSpace(pod)), strings.ToLower(strings.TrimSpace(site))} {
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
