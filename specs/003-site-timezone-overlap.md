# Site Timezone Overlap

**Status:** In Progress (engine + site table + API implemented; admin editor in the plan setup)
**Author(s):** Anoop Gopalakrishnan (with Claude)
**Date:** 2026-08-17
**Story/Ticket:** _TBD_
**Sprint/Cycle:** —

---

## 1. Overview

Deleting the v1 Python mining pipeline left exactly one capability behind: a
table of site → UTC offset, and the working-hours overlap matrix derived from it.
Conway's Go code has no equivalent, so every place that should price a cross-site
handoff instead approximates it as same-site or not.

This spec ports that capability into Go as first-class data a manager can edit,
and makes the overlap it produces the single source for cross-site latency across
Observe, Plan and Train. It also closes `[NEEDS CLARIFICATION]` Q1 of
`001-plan-execution-order.md`, which asks where a plan roster's site overlap comes
from.

---

## 2. Problem

Conway's whole argument is that cross-site seams are expensive. The README says
some of the worst dependencies "cross timezones with zero overlap — paid for in
someone's evening", and `SPEC.md` prices a handoff by overlap band: 4 hours or
more costs a quarter-day, 2 or more a half-day, any overlap a full day, none a
day and a half.

That banding needs overlap hours between two sites. The only real computation of
it lived in `scripts/build_pods.py`, with a five-entry hardcoded offset table and
an `overlap_hours` function, feeding a `pods.json` the running app no longer
reads. That script is now the last file of a deleted pipeline.

What the Go code does instead: `planWorld` assigns 8 hours if two pods share a
site string, 6 if the site is non-empty and equal, 2 otherwise. Taking the demo
roster's sites: Dublin and Warsaw are an hour apart and share almost a full
working day, while Warsaw and Denver share none at all. Both pairs score the same
2 hours. The model cannot tell a hard seam from an easy one, which is the
distinction the tool exists to make.

Concretely: a plan whose critical path hands off Warsaw → Dublin and one that
hands off Warsaw → Denver produce identical lead times today. The second should
be materially worse.

---

## 3. User Stories

### Story 1: Real overlap between sites

**As a** manager reading a plan or a network
**I want** cross-site handoff cost derived from the actual working-hours overlap of the two sites
**So that** a hard seam and an easy one are not priced the same

### Story 2: Maintain the site table

**As an** admin
**I want** to see and edit the sites Conway knows, with their timezone and working hours
**So that** the model matches where my teams actually are, without a code change

### Story 3: Know when overlap is guessed

**As a** manager
**I want** to see which pods have no usable site data
**So that** I know which handoff costs are real and which are a default

---

## 4. Acceptance Criteria

### Story 1: Real overlap between sites

**AC 1.1: Overlap is computed from working hours, not string equality**

> Given two sites with known timezones and working-hour windows
> When overlap is computed
> Then the result is the number of hours their working windows intersect
> And two sites in the same timezone yield a full working day

**AC 1.2: The existing latency bands consume it unchanged**

> Given a computed overlap in hours
> When a cross-site handoff cost is derived
> Then it uses the bands already specified: >= 4h a quarter-day, >= 2h a half-day, > 0h a full day, 0h a day and a half
> And in the weekly scheduling model the allowance rounds to whole weeks — today 0 — so the bands price the game world's coordination costs until the model gains sub-week resolution (amended 2026-08-30)

**AC 1.3: One computation, used everywhere**

> Given Observe, Plan and Train all price cross-site handoffs
> When any of them needs overlap between two pods
> Then all three resolve it through the same function and the same site table
> And no view carries its own approximation

**AC 1.4: A same-site pair needs no table entry**

> Given two pods whose site strings are equal
> When overlap is computed
> Then it is a full working day, whether or not the site is in the table

### Story 2: Maintain the site table

**AC 2.1: Sites are listed with their timezone**

> Given a roster whose pods carry site names
> When an admin opens the site table
> Then every distinct site in the roster is listed, with its timezone and working hours if known
> And sites present in the roster but absent from the table are flagged

**AC 2.2: A site can be added or corrected**

> Given a site missing or wrong
> When the admin sets its timezone and working hours
> Then the change persists
> And any view recomputing overlap uses it immediately

**AC 2.3: Timezones are named, not offsets**

> Given a site in a region observing daylight saving
> When its timezone is set
> Then it is stored as an IANA zone name
> And the offset used for overlap is the one in effect on the date being modelled

**AC 2.4: Working hours default sensibly**

> Given a site added with no working hours specified
> When overlap is computed
> Then a documented default window is used
> And the fact that it is a default is visible in the table

### Story 3: Know when overlap is guessed

**AC 3.1: Unknown sites are reported, not silently defaulted**

> Given a pod whose site is empty or absent from the site table
> When a view prices a handoff involving it
> Then the figure is marked as a default
> And the pod appears in a list of pods with unusable site data

**AC 3.2: The default is the pessimistic band**

> Given two pods where at least one site is unknown
> When overlap is computed
> Then it resolves to the no-overlap band
> And this is stated wherever the resulting cost is shown

---

## 5. Functional Requirements

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The system MUST hold a site table mapping each site name to an IANA timezone and a working-hours window | MUST |
| FR-002 | The system MUST compute overlap between two sites as the intersection of their working-hour windows, evaluated at the modelled date so daylight saving is correct | MUST |
| FR-003 | The system MUST expose one overlap function, consumed by every cross-site cost in Observe, Plan and Train; no view MAY carry its own approximation | MUST |
| FR-004 | The system MUST map overlap hours to handoff latency using the bands already documented in `SPEC.md` | MUST |
| FR-005 | The system MUST treat two pods with equal site strings as fully overlapping without requiring a table entry | MUST |
| FR-006 | The system MUST provide an admin view listing every site referenced by a roster, with its timezone, working hours, and whether each value is set or defaulted | MUST |
| FR-007 | The system MUST let an admin add or correct a site's timezone and working hours, persisted alongside the roster data | MUST |
| FR-008 | The system MUST resolve an unknown or empty site to the no-overlap band, and MUST mark every figure derived that way as a default | MUST |
| FR-009 | The system MUST list pods whose site data cannot produce a real overlap, consistent with the existing data-hygiene reporting | MUST |
| FR-010 | The system SHOULD seed the table with the sites of the roster being imported, leaving timezone unset for an admin to fill | SHOULD |
| FR-011 | The system MUST NOT infer a site's timezone from a pod name or from any personal data | MUST NOT |
| FR-012 | `scripts/build_pods.py` MUST be deleted once this port is verified, completing the v1 pipeline removal — deleted 2026-08-30 | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Pure computation | Overlap and banding live in a dependency-free package with no I/O, matching the `planning` package convention | Unit tests without a database |
| NFR-002 | Daylight-saving correctness | Overlap between a DST-observing and a non-observing site differs across a DST boundary, and matches hand-computed values | Table-driven tests at two dates |
| NFR-003 | Consistency | For any pod pair, every view reports the same overlap and the same latency | Cross-view assertion test |
| NFR-004 | No regression in existing figures | Same-site and unknown-site cases produce the same costs as today, so only genuinely-known cross-site pairs change | Golden-file comparison on the demo plan |
| NFR-005 | Honest defaults | Every defaulted overlap is labelled as such in the API response and the UI | Output review; assertion in tests |

---

## 7. Data Model

### Entities

**Site**
- name: string — matches the roster's site/location string
- timezone: IANA zone name (e.g. `Europe/Warsaw`); unset until an admin fills it
- workStartHour, workEndHour: local hours bounding the working window
- defaulted: boolean — working hours came from the default, not an admin

**SiteOverlap** _(derived)_
- siteA, siteB: references
- hours: number — intersection of the working windows at the modelled date
- band: enum (full, partial-4h, partial-2h, minimal, none)
- confidence: enum (computed, defaulted) with the reason when defaulted

### Relationships

- A Roster's pods each reference zero or one Site by name.
- A Site pair yields one SiteOverlap per modelled date.
- Cross-pod dependency edges in Observe, Plan and Train each resolve their latency through one SiteOverlap.

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | /api/sites | List known sites, plus sites referenced by rosters but not yet configured | — | list of Site with defaulted flags |
| PATCH | /api/sites | Set or correct a site's timezone and working hours | name, timezone, work hours | updated Site |
| GET | /api/sites/overlap | The overlap matrix for a given set of sites at a given date | site names, date | SiteOverlap entries |

---

## 9. Out of Scope

- Per-person or per-pod working hours. Sites carry the window; individuals are never modelled (consistent with the app's standing framing).
- Holiday calendars. Those are specified in `001-plan-execution-order.md` as calendar windows and are a separate input from timezone overlap.
- Automatic timezone detection from any external source. An admin sets it.
- Changing the latency bands themselves. This spec supplies real overlap hours to the bands `SPEC.md` already defines; retuning them is a modelling decision on its own evidence.
- Rebuilding any other part of the deleted v1 pipeline.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | What default working-hours window? The old script implied a standard business day; 09:00–17:00 local is the obvious choice but understates orgs that flex for overlap. | Anoop | — | resolved 2026-08-30: 09:00–17:00 local per site, editable per site later |
| Q2 | Does the site table live per-roster or globally? Per-roster keeps a snapshot honest as offices change; global avoids re-entering the same sites every import. | Anoop | — | resolved 2026-08-30: per-plan (the plan's roster is the snapshot), seeded from the roster's distinct sites on first read; a "use previous plan's sites" convenience is a later slice |
| Q3 | Which date does Plan model overlap at — the period start, or per-week so a DST shift mid-period is reflected? Per-week is more correct and more machinery. | Anoop | — | resolved 2026-08-30: the overlap function takes a date; consumers evaluate at the modelled date (the game world uses the current date) — per-week consumers can call it per week without changing the function |
| Q4 | The five sites in the deleted table were real offices. Should the shipped default table be empty, or seeded with common IANA zones as examples? Seeding with real office names in a public repo is the thing to avoid. | Anoop | — | resolved 2026-08-30: the table seeds from the roster's own site names with timezones unset; the editor offers common IANA zones as example choices — no real office names ship in the repo |

---

## 11. Decision Record

### Decision 1: Port the capability, not the script

**Context:** `build_pods.py` computed overlap as a side effect of building a
`pods.json` the app no longer reads. The valuable part is the offset table and
the overlap function; the rest is dead pipeline.

**Decision:** Reimplement the site table and overlap computation in Go as data a
manager owns, and delete the script once verified.

**Alternatives considered:**
- Keep the script and run it manually — rejected: it writes an artifact nothing consumes, and the whole point of removing the pipeline was to stop maintaining two data paths.
- Port it verbatim, hardcoded table included — rejected: a hardcoded table of one org's offices does not belong in a public repo, and cannot be corrected without a release.

**Consequences:** The table starts empty, so overlap is defaulted until an admin
fills it. That is honest but means no behaviour change until someone does the
data entry, which FR-009's hygiene reporting has to make visible or it will not
happen.

### Decision 2: Store IANA zones, not fixed offsets

**Context:** The deleted table stored fixed UTC offsets with a comment saying
"June / DST where applicable" — correct for half the year.

**Decision:** Store IANA zone names and resolve the offset at the modelled date.

**Alternatives considered:**
- Fixed offsets — rejected: silently wrong for half the year, and the error lands exactly on the cross-hemisphere pairs where overlap matters most.

**Consequences:** Overlap depends on the date being modelled (Q3), which is new
in a model that has so far been date-free. Plan gains dates in spec 001 anyway,
so the two land together.

### Decision 3: Unknown site means the pessimistic band

**Context:** Most rosters will have sites missing from the table at first.

**Decision:** Resolve unknown to the no-overlap band, and label the result.

**Alternatives considered:**
- Optimistic default (full overlap) — rejected: it would hide exactly the seams the tool exists to surface.
- Refuse to compute — rejected: the app must stay usable on a fresh import.

**Consequences:** A fresh import looks worse than reality until the table is
filled. Acceptable only because the figure is labelled as a default; without
FR-008's labelling this would be misleading rather than conservative.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Cross-site pairs priced by real overlap | 0 — all approximated as same-site or not | All pairs whose sites are configured | Share of pod pairs resolving to `computed` rather than `defaulted` |
| Distinguishable seams | Dublin–Warsaw priced identically to Warsaw–Denver | Different bands for different real overlaps | Overlap matrix on a configured roster |
| Approximations of overlap in the codebase | At least 2 (`planWorld`, the Python script) | 1 shared function | Code review; NFR-003 test |
| Python files in the repo | 1 (`build_pods.py`) | 0 | Repository listing after FR-012 |

---

## Review Checklist

- [x] Problem is clearly stated and justified
- [x] User stories represent real user value
- [x] Acceptance criteria are in Given/When/Then format
- [x] Edge cases and error scenarios are covered (unknown site, same-site, DST boundary, empty table)
- [x] Requirements use MUST/SHOULD/MAY language
- [x] Non-functional requirements have measurable thresholds
- [x] Out of Scope is explicit
- [x] Open questions resolved (Q1–Q4, 2026-08-30)
- [x] No implementation details in the requirements
- [x] AI can read this spec (markdown, in the repo)
