package planning

// In-app editing of the sequencing attributes, the other half of §10 Q9: an
// attribute may be entered in the uploaded sheet or on the plan itself. This is
// the plan-side path (§8's PATCH /api/plan/{id}/initiatives); ParseMatrix is the
// sheet-side one.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const isoDate = "2006-01-02"

// InitiativeEdit is a partial edit to one initiative's sequencing attributes.
//
// Every field is a pointer so that "not mentioned" is distinguishable from "set
// back to zero". A UI edits one cell at a time, and a PATCH that cleared a
// planner's target date because the form omitted the field would be exactly the
// silent data loss this feature exists to prevent.
type InitiativeEdit struct {
	Name               string    `json:"name"`
	StatedPriority     *int      `json:"statedPriority,omitempty"`
	PriorityLocked     *bool     `json:"priorityLocked,omitempty"`
	TargetDate         *string   `json:"targetDate,omitempty"`
	DateLocked         *bool     `json:"dateLocked,omitempty"`
	Tier               *int      `json:"tier,omitempty"`
	CostOfDelayPerWeek *float64  `json:"costOfDelayPerWeek,omitempty"`
	EarliestStart      *string   `json:"earliestStart,omitempty"`
	AfterInitiatives   *[]string `json:"afterInitiatives,omitempty"`
	KitPct             *float64  `json:"kitPct,omitempty"`
	InFlight           *bool     `json:"inFlight,omitempty"`
	ProgressPct        *float64  `json:"progressPct,omitempty"`
}

// PeriodBounds is the plan's first and last day: the period start, and the
// horizon's worth of weeks after it. ok is false when the plan has no period
// start, since nothing can be measured against a period that has none — and
// inventing one would reject a planner's date on a rule they never set.
func PeriodBounds(sp SchedulingParams, horizonWeeks float64) (start, end string, ok bool) {
	from := strings.TrimSpace(sp.PeriodStart)
	if from == "" {
		return "", "", false
	}
	t0, err := time.Parse(isoDate, from)
	if err != nil {
		return "", "", false
	}
	weeks := Params{HorizonWeeks: horizonWeeks}.WithDefaults().HorizonWeeks
	return t0.Format(isoDate), t0.AddDate(0, 0, int(weeks*7)).Format(isoDate), true
}

// ApplyInitiativeEdits returns the initiatives with the edits applied, leaving the
// caller's slice untouched.
//
// Nothing is applied unless every edit is acceptable. AC 2.4 requires that a
// rejected date leaves the plan alone ("and no schedule is recomputed"), and a
// batch that half-applied would leave a planner unable to say what was saved.
// Every problem in the batch is reported together, so a form with two bad dates
// takes one round trip rather than two.
func ApplyInitiativeEdits(inits []Initiative, edits []InitiativeEdit, sp SchedulingParams, horizonWeeks float64) ([]Initiative, error) {
	byName := map[string]int{}
	for i, it := range inits {
		byName[it.Name] = i
	}

	start, end, bounded := PeriodBounds(sp, horizonWeeks)
	var problems []string

	// Validate the whole batch before touching anything.
	type resolved struct {
		idx           int
		edit          InitiativeEdit
		targetDate    *string // normalised to ISO
		earliestStart *string // normalised to ISO
	}
	pending := make([]resolved, 0, len(edits))
	for _, e := range edits {
		idx, ok := byName[e.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("no initiative named %q in this plan", e.Name))
			continue
		}
		r := resolved{idx: idx, edit: e}
		bad := false
		if e.TargetDate != nil {
			iso, err := validDate(*e.TargetDate, start, end, bounded)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: target date %s", e.Name, err))
				bad = true
			} else {
				r.targetDate = &iso
			}
		}
		if e.EarliestStart != nil {
			// Readable, but not bounds-checked: AC 2.4 is about the target date, and
			// "cannot start until after this period" is a legitimate thing to record.
			// Readability still matters — an unreadable value would otherwise silently
			// replace a good date with nothing, which is the loss the pointers prevent.
			iso, err := validDate(*e.EarliestStart, "", "", false)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: earliest start %s", e.Name, err))
				bad = true
			} else {
				r.earliestStart = &iso
			}
		}
		if bad {
			continue
		}
		pending = append(pending, r)
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}

	out := make([]Initiative, len(inits))
	copy(out, inits)
	for _, r := range pending {
		// Copy the struct and keep its maps: Work and Leads belong to the caller's
		// initiative and are not what this endpoint edits.
		it := out[r.idx]
		e := r.edit
		assign(&it.StatedPriority, e.StatedPriority)
		assign(&it.PriorityLocked, e.PriorityLocked)
		assign(&it.DateLocked, e.DateLocked)
		assign(&it.Tier, e.Tier)
		assign(&it.CostOfDelayPerWeek, e.CostOfDelayPerWeek)
		assign(&it.KitPct, e.KitPct)
		assign(&it.InFlight, e.InFlight)
		assign(&it.ProgressPct, e.ProgressPct)
		if r.targetDate != nil {
			it.TargetDate = *r.targetDate
		}
		if r.earliestStart != nil {
			it.EarliestStart = *r.earliestStart
		}
		if e.AfterInitiatives != nil {
			it.AfterInitiatives = append([]string(nil), (*e.AfterInitiatives)...)
		}
		out[r.idx] = it
	}
	return out, nil
}

// validDate normalises a date and checks it against the period. An empty value is
// how a planner clears a date, so it is always allowed.
func validDate(in, start, end string, bounded bool) (string, error) {
	raw := strings.TrimSpace(in)
	if raw == "" {
		return "", nil
	}
	iso := parseSheetDate(raw)
	if iso == "" {
		return "", fmt.Errorf("%q is not a date this can read; use YYYY-MM-DD", raw)
	}
	if !bounded {
		return iso, nil
	}
	if iso < start || iso > end {
		return "", fmt.Errorf("%s is outside the plan's period, which runs %s to %s", iso, start, end)
	}
	return iso, nil
}

// assign writes through a pointer only when the caller supplied one, which is what
// makes an unmentioned attribute survive the edit.
func assign[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
