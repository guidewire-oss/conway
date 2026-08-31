// Site timezone overlap (spec 003): the ported capability from the deleted v1
// pipeline. A table of site → IANA timezone + working-hours window, and the
// overlap function every cross-site cost resolves through — one computation,
// so a hard seam and an easy one are never priced the same again.
//
// Pure computation, no I/O (NFR-001): the table is data a manager owns, and
// the functions here are deterministic given it.
package planning

import (
	"fmt"
	"math"
	"time"
)

// The default working-hours window (spec 003 Q1, resolved 2026-08-30): a
// standard local business day. Sites that flex for overlap set their own.
const (
	DefaultWorkStart = 9.0
	DefaultWorkEnd   = 17.0
)

// Site is one entry in a plan's site table: a roster site name, the IANA zone
// its people work in (never a fixed offset — Decision 2), and the local
// working-hours window. An unset timezone means the site is unconfigured:
// overlap involving it resolves to the pessimistic default, and says so.
type Site struct {
	Name      string  `json:"name"`
	Timezone  string  `json:"timezone"`                // IANA zone name; "" until an admin sets it
	WorkStart float64 `json:"workStartHour,omitempty"` // local hour, 0..24
	WorkEnd   float64 `json:"workEndHour,omitempty"`   // local hour; <= start means the default window
	Defaulted bool    `json:"defaulted,omitempty"`     // working hours came from the default (AC 2.4)
	Unknown   bool    `json:"unknown,omitempty"`       // referenced by the roster but not yet configured (AC 2.1)
}

// workWindow clamps a site's configured window to the documented default when
// unset or inverted (an overnight shift is not modelled — spec 003 §9).
func (s Site) workWindow() (float64, float64, bool) {
	start, end := s.WorkStart, s.WorkEnd
	if start <= 0 && end <= 0 || end <= start || start < 0 || end > 24 {
		return DefaultWorkStart, DefaultWorkEnd, true
	}
	return start, end, false
}

// OverlapHours is the number of working hours two sites share on a given date,
// computed in UTC so daylight saving is correct at that date (FR-002, NFR-002).
// The second return is false when either site is unconfigured — the caller
// resolves that to the pessimistic band and labels it (Decision 3).
func OverlapHours(a, b Site, at time.Time) (float64, bool, error) {
	if a.Name == b.Name {
		// Same site: a full working day, no table entry needed (FR-005) — and
		// real, not defaulted, whether or not the site is configured.
		start, end, def := a.workWindow()
		if def {
			return DefaultWorkEnd - DefaultWorkStart, true, nil
		}
		return end - start, true, nil
	}
	if a.Timezone == "" || b.Timezone == "" {
		return 0, false, nil
	}
	locA, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return 0, false, fmt.Errorf("site %q: unknown timezone %q", a.Name, a.Timezone)
	}
	locB, err := time.LoadLocation(b.Timezone)
	if err != nil {
		return 0, false, fmt.Errorf("site %q: unknown timezone %q", b.Name, b.Timezone)
	}
	startA, endA, _ := a.workWindow()
	startB, endB, _ := b.workWindow()
	// Each site's window expressed on the same UTC timeline at the modelled
	// date. Zero minutes/seconds: the model works in whole hours.
	utc := func(loc *time.Location, localHour float64) time.Time {
		hour := int(localHour)
		minute := int(math.Round((localHour - float64(hour)) * 60))
		t := time.Date(at.Year(), at.Month(), at.Day(), hour, minute, 0, 0, loc)
		return t.UTC()
	}
	aStart, aEnd := utc(locA, startA), utc(locA, endA)
	bStart, bEnd := utc(locB, startB), utc(locB, endB)
	lo := maxTime(aStart, bStart)
	hi := minTime(aEnd, bEnd)
	if !hi.After(lo) {
		return 0, true, nil // no intersection that day
	}
	return math.Round(hi.Sub(lo).Hours()*10) / 10, true, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// HandoffLatencyDays maps overlap hours to the latency bands SPEC.md already
// defines (FR-004): >= 4h a quarter-day, >= 2h a half-day, any overlap a full
// day, none a day and a half. Unconfigured pairs resolve here too — Decision
// 3's pessimistic default is this 1.5.
func HandoffLatencyDays(hours float64, configured bool) float64 {
	if !configured {
		return 1.5
	}
	switch {
	case hours >= 4:
		return 0.25
	case hours >= 2:
		return 0.5
	case hours > 0:
		return 1.0
	default:
		return 1.5
	}
}

// SitesFromTeams seeds the table from a plan's roster (FR-010): every distinct
// site appears, timezone unset for an admin to fill. Sites already configured
// in existing are preserved.
func SitesFromTeams(teams []Team, existing []Site) []Site {
	configured := map[string]Site{}
	for _, s := range existing {
		configured[s.Name] = s
	}
	seen := map[string]bool{}
	out := existing
	for _, t := range teams {
		name := t.Site
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := configured[name]; !ok {
			out = append(out, Site{Name: name, Unknown: true})
		}
	}
	return out
}
