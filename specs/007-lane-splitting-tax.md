# Lane Splitting with a Splitting Tax

**Status:** Draft
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-08-26
**Story/Ticket:** discovered planning GWCP Portfolio_Priority; extends spec 006 Decision 2
**Sprint/Cycle:** n/a

---

## 1. Overview

Under the effort model a slice takes **all** of its pod's lanes or waits —
all-or-nothing. Real teams split work: when a big slice runs on some lanes and
other lanes come free, the team moves part of the work across and finishes
sooner, paying a coordination overhead. This spec adds **partial lane
allocation with an org-level splitting tax**: a slice may occupy fewer lanes
than its effort suggests, free lanes accelerate in-flight work, and every
split pays `splitTaxWeeks` of pure overhead per split.

---

## 2. Problem

DevSpace/VAMOS at Okocim: 200 effort-weeks. Under wall-clock it ran 225 weeks
on one track while four sat idle. Under the current effort model it takes all
5 lanes for 45 weeks — but if it had started when only 2 lanes were free, it
would hold ALL five anyway (starving whoever held the other three), rather
than start on two and pick up the rest as they free. The schedule cannot
express "grow into free capacity", and it cannot express the cost of doing so.

---

## 3. User Stories

### Story 1: Big work starts on free lanes and grows

**As a** planning manager
**I want** a big slice to begin on whatever lanes are free and absorb more as
they free up, rather than wait for the whole pod
**So that** idle tracks shorten the wall clock of large initiatives

### Story 2: Splitting costs something

**As a** planning manager
**I want** an org-level splitting tax (e.g. 2 weeks per split) included when
work is divided
**So that** the plan doesn't claim perfect parallelism a real team can't
deliver, and I can turn splitting off entirely with a high tax

---

## 4. Acceptance Criteria

**AC 1.1: A slice grows into freed lanes**

> Given a 5-lane pod where lanes 1–2 are free at week 0 and lanes 3–5 free at
> week 10, and a 100-effort-week slice released at week 0
> When scheduled with splitting enabled
> Then the slice runs 2 lanes for weeks 0–10 and 5 lanes from week 10,
> finishing per the effort math across both phases, and the timeline shows
> one bar with the occupancy change on hover

**AC 1.2: The tax is charged per split**

> Given the scenario in AC 1.1 and a splitting tax of 2 weeks
> Then the finish moves 2 weeks later than the tax-free split, and the
> initiative's assumptions name the split and its tax

**AC 1.3: All-or-nothing remains available**

> Given a splitting tax of 0 and splitting disabled (absent)
> Then behaviour matches today exactly: the slice takes all lanes or waits

**AC 1.4: The tax is an assumption, not a per-pod knob**

> Given the assumptions form
> Then one org-level "splitting tax (weeks)" control with a tooltip; blank
> disables splitting

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | `SchedulingParams.SplitTaxWeeks`: org-level weeks of overhead per lane-split; absent/0 disables splitting | MUST |
| FR-002 | Under the effort model, placement MUST offer a slice the lanes free at its ready week, and MUST absorb lanes that free during its run, when the tax is set | MUST |
| FR-003 | Remaining work MUST be recomputed per phase: finish when Σ(lanes × weeks) over phases ≥ ceil(effort ÷ (1 − loss)) + taxWeeks | MUST |
| FR-004 | The WorkSlice MUST report its per-phase lanes (new `Phases` field) so the timeline and heatmap render growth honestly | MUST |
| FR-005 | The heatmap MUST count each week's actual occupancy (phased lanes), not a flat lanesUsed | MUST |
| FR-006 | The assumptions form MUST expose the tax with a tooltip; the value is labelled in the Order header's assumptions line | MUST |
| FR-007 | Wall-clock model and tax=0 MUST reproduce today's schedules bit-for-bit | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | No regression | suites green | `go test ./...`, `node --test` |
| NFR-002 | 29×35 GWCP plan with splitting | < 2s | schedbench |

---

## 7. Data Model

- `SchedulingParams.SplitTaxWeeks int` — org level, absent/0 = no splitting
- `WorkSlice.Phases []LanePhase` where `LanePhase{FromWeek, ToWeek, Lanes int}`
- `WorkSlice.LanesUsed` retained as the initial phase's lanes (back-compat)

---

## 8. API Contract

No new endpoints; the schedule payload gains `phases` per slice.

---

## 9. Out of Scope

- Per-pod or per-initiative tax overrides
- Splitting across pods (capacity transfer is spec 001 §11 D7 / FR-017 territory)
- Deciding WHICH initiative yields lanes — the engine never preempts; growth
  only absorbs lanes nothing else holds

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Should a slice also SHRINK (yield lanes) when other work wants them mid-run, or only grow? Preemption changes everything downstream. | Anoop | 2026-08-30 | pending — default: grow only |

---

## 11. Decision Record

### Decision 1: Growth only, never preemption

**Context:** Growth into free lanes is a pure win modulo tax; shrinking to
yield lanes would re-open every downstream start mid-run and make the
schedule non-monotonic — a planner reading week numbers could not trust them
across renders.

**Decision:** Slices grow into lanes that free during their run; nothing ever
loses lanes it holds. (Q1's default; flip only with strong reason.)

**Alternatives considered:**
- Full preemptive reallocation — rejected: non-monotonic schedules, and the
  human cost of explaining a date that moved backwards mid-period.

### Decision 2: The tax is weeks, org-level, per split

**Context:** You asked for "a buffer tax of maybe 2 weeks". The unit that
matters is time, not a percentage — coordination overhead is roughly a fixed
ramp (context, pairing up, hand-off), independent of slice size.

**Decision:** Flat weeks per split event, one knob for the org. A high value
economically disables splitting (the engine still may not split when the tax
outweighs the gain — FR-003's arithmetic decides naturally).

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| DevSpace@Okocim finish (from user's view) | w225 wall-clock / w45 all-lanes | earlier than w45 when lanes free early | schedbench + in-browser |
| Idle lane-weeks on hot pods | visible in heatmap | reduced with tax set | heatmap Σ |
