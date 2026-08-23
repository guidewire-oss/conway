package planning

// Baselines: the agreed order for a period, frozen with the inputs that produced
// it (Story 7, FR-029 to FR-033, and §11 Decision 27).
//
// The point of storing the inputs rather than only the schedule is that "why did
// it say week 4" has to be answerable a quarter later, when the roster has moved
// and the sheet has been edited twice. Decision 13 makes the same argument.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// BaselineInputs is everything ComputeSchedule was given, frozen.
//
// It is deliberately the arguments themselves rather than a curated subset: a
// field list would have to be maintained, and the failure mode of forgetting one
// is that a real change stops being detected (Decision 27).
type BaselineInputs struct {
	Teams       []Team           `json:"teams"`
	Initiatives []Initiative     `json:"initiatives"`
	Params      Params           `json:"params"`
	Scheduling  SchedulingParams `json:"scheduling"`
}

// NewBaselineInputs deep-copies the inputs, so a later edit to anything the caller
// still holds cannot reach into a saved baseline (FR-030: baselines are immutable).
//
// Deep, not shallow. A shallow copy leaves the Work maps, the AfterInitiatives
// slices, the lead-capacity map and the buffer pointers shared with the caller —
// and a snapshot that changes underneath is not a snapshot. The first version of
// this function argued that every edit path replaces maps wholesale, which was
// true of the paths that existed that day and is exactly the kind of claim that
// rots without anyone noticing.
func NewBaselineInputs(teams []Team, inits []Initiative, params Params, sp SchedulingParams) BaselineInputs {
	return BaselineInputs{
		Teams:       append([]Team(nil), teams...), // Team holds no references
		Initiatives: copyInitiatives(inits),
		Params:      params,
		Scheduling:  copySchedulingParams(sp),
	}
}

func copyInitiatives(inits []Initiative) []Initiative {
	out := make([]Initiative, len(inits))
	for i, it := range inits {
		c := it
		if it.Work != nil {
			c.Work = make(map[string]TeamWork, len(it.Work))
			for pod, w := range it.Work {
				w.DependsOn = append([]string(nil), w.DependsOn...)
				c.Work[pod] = w
			}
		}
		if it.Leads != nil {
			c.Leads = make(map[string]string, len(it.Leads))
			for k, v := range it.Leads {
				c.Leads[k] = v
			}
		}
		c.AfterInitiatives = append([]string(nil), it.AfterInitiatives...)
		out[i] = c
	}
	return out
}

func copySchedulingParams(sp SchedulingParams) SchedulingParams {
	c := sp
	// Calendars is a slice: without its own copy, a saved baseline would share
	// its backing array with the caller, and a later window edit would reach
	// into a frozen baseline (FR-030).
	if sp.Calendars != nil {
		c.Calendars = make([]CalendarWindow, len(sp.Calendars))
		copy(c.Calendars, sp.Calendars)
	}
	if sp.LeadCapacity != nil {
		c.LeadCapacity = make(map[string]int, len(sp.LeadCapacity))
		for k, v := range sp.LeadCapacity {
			c.LeadCapacity[k] = v
		}
	}
	// The percentage pointers exist so an explicit 0 is distinguishable from absent
	// (Decision 20); copying the value keeps that distinction without the sharing.
	c.BufferPct = copyFloatPtr(sp.BufferPct)
	c.FeedingBufferPct = copyFloatPtr(sp.FeedingBufferPct)
	return c
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Recompute rebuilds the schedule from the stored inputs. AC 7.1 requires this to
// reproduce the stored schedule exactly, which is a property a spec asserts rather
// than a claim this comment makes.
func (in BaselineInputs) Recompute() *Schedule {
	return ComputeSchedule(in.Teams, in.Initiatives, in.Params, in.Scheduling)
}

// Fingerprint identifies this exact set of inputs, so a plan whose inputs have
// moved can be flagged as diverged from a baseline (FR-030).
//
// SHA-256 over the canonical JSON of the whole struct. encoding/json sorts map
// keys, so a pod map built in a different order fingerprints the same — without
// that, the value would depend on Go's randomised map iteration and every boot
// would report divergence.
func (in BaselineInputs) Fingerprint() string {
	blob, err := json.Marshal(in)
	if err != nil {
		// Marshalling these types cannot fail — no channels, no functions, and NaN is
		// refused at every entry point. An empty fingerprint would silently read as
		// "never diverged", so return something that never matches instead.
		return "unfingerprintable"
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// BaselineDelta is one initiative's movement against a baseline (AC 7.4).
type BaselineDelta struct {
	Name             string `json:"name"`
	ProposedRank     int    `json:"proposedRank"`
	BaselineRank     int    `json:"baselineRank"`
	StartWeek        int    `json:"startWeek"`
	BaselineStart    int    `json:"baselineStartWeek"`
	StartDeltaWeeks  int    `json:"startDeltaWeeks"`
	CommitWeek       int    `json:"commitWeek"`
	BaselineCommit   int    `json:"baselineCommitWeek"`
	CommitDeltaWeeks int    `json:"commitDeltaWeeks"`
	Verdict          string `json:"verdict"`
	BaselineVerdict  string `json:"baselineVerdict"`
	VerdictChanged   bool   `json:"verdictChanged"`
}

// BaselineComparison is a computed order measured against a baseline. Additions
// and removals are listed separately rather than shown as movement, because an
// initiative that did not exist has not moved (AC 7.4).
type BaselineComparison struct {
	Initiatives []BaselineDelta `json:"initiatives"`
	Added       []string        `json:"added,omitempty"`
	Removed     []string        `json:"removed,omitempty"`
	Moved       int             `json:"moved"` // how many changed start or commit
}

// CompareToBaseline reports what has changed between a baseline's schedule and a
// freshly computed one. A nil baseline makes everything an addition, which is the
// truthful answer for a plan with no baseline yet.
func CompareToBaseline(baseline, current *Schedule) BaselineComparison {
	cmp := BaselineComparison{}
	was := map[string]ScheduledInitiative{}
	if baseline != nil {
		for _, si := range baseline.Initiatives {
			was[si.Name] = si
		}
	}
	seen := map[string]bool{}

	if current != nil {
		for _, si := range current.Initiatives {
			seen[si.Name] = true
			old, existed := was[si.Name]
			if !existed {
				cmp.Added = append(cmp.Added, si.Name)
				continue
			}
			d := BaselineDelta{
				Name: si.Name, ProposedRank: si.ProposedRank, BaselineRank: old.ProposedRank,
				StartWeek: si.StartWeek, BaselineStart: old.StartWeek,
				StartDeltaWeeks: si.StartWeek - old.StartWeek,
				CommitWeek:      si.CommitWeek, BaselineCommit: old.CommitWeek,
				CommitDeltaWeeks: si.CommitWeek - old.CommitWeek,
				Verdict:          si.Verdict, BaselineVerdict: old.Verdict,
				VerdictChanged: si.Verdict != old.Verdict,
			}
			if d.StartDeltaWeeks != 0 || d.CommitDeltaWeeks != 0 {
				cmp.Moved++
			}
			cmp.Initiatives = append(cmp.Initiatives, d)
		}
	}
	// Baseline order, so the removals read in the sequence they were agreed in.
	if baseline != nil {
		for _, si := range baseline.Initiatives {
			if !seen[si.Name] {
				cmp.Removed = append(cmp.Removed, si.Name)
			}
		}
	}
	sortDeltasByRank(cmp.Initiatives)
	return cmp
}

// sortDeltasByRank puts the table in the current order's sequence, so it reads the
// same way the Order view does rather than in whatever order the map produced.
func sortDeltasByRank(ds []BaselineDelta) {
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j].ProposedRank < ds[j-1].ProposedRank; j-- {
			ds[j], ds[j-1] = ds[j-1], ds[j]
		}
	}
}
