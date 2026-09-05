# Manager planning and execution audit

Audit date: 2026-09-05. Product source revision: `d349e23`.

The most valuable next improvements are **trustworthy scope, truthful agreement
comparisons and reliable save behavior**. Conway supports a useful manager loop:
import a plan, explore capacity, agree a baseline, bind execution evidence and
record decisions. Several boundary cases still break that loop or make its
numbers look more certain than the evidence permits.

This is a follow-up audit, not an implementation report. No product fixes are
included. Earlier resolved findings are excluded. Examples use generic data.

## Scope and method

Independent reviews covered planning/backend, timeline/UI, execution and docs.
A separate verification pass tried to refute candidates. Accepted finding IDs
were deduplicated and ranked with deterministic code. The retained
[diagnostic probes](audits/2026-09-05/README.md) and their
[recorded output](audits/2026-09-05/observations.txt) make the main cases repeatable.

| Manager journey | Reviewed surfaces | Evidence and limits |
|---|---|---|
| Start and orient | Home, navigation, roles, onboarding, source context | Source and automated checks; no fresh live visual validation |
| Establish scope | Roster/matrix imports, unknown estimates, identity matching, snapshot associations | Generic Go probes and source traces; no new external import |
| Schedule and explore | Capacity, dependencies, leads, calendars, lanes, Fit, remedies | Go probes and uncached server checks; mutation-path concern below |
| Read and edit | Order, both timeline lenses, search, precise edits, undo, uploads, exports, health report | JS suite and handler probes; pixel rendering and drag ergonomics not rechecked live |
| Agree and compare | Baseline drawer, activation, divergence, calendar changes, comparison | Go/JS probes; deletion policy cross-checked against spec and docs |
| Execute and review | Bindings, snapshot choice, progress, finish, variance, calibration, decision log | Real evidence derivation and request-timing probes; concurrent DB case source-traced |
| Measure and simulate | Network, WIP Scoreboard, Data Quality, Feature Simulator, Levers, fever chart | Source, provenance, markup, state and time-basis probes |
| Learn and operate | Guide, game source selection, README, manual, contextual help, supporting docs/specs | Documentation/code comparison; no facilitated multiplayer or live identity-provider exercise |

P1 means address before relying on the affected output for commitments or shared
editing. P2 means repair in the next usability/reliability pass. A diagnostic
exiting successfully means it ran, not that the product behavior is correct.

## Product defects and evidence

### M01 · P1 · Explicit unknown work can disappear during import

An Atlas estimate of 4 and a Beacon estimate of `TBD`, both with sequence `NONE`, produce only Atlas work. The result says `provisional=false` and `unestimated=[]`. A manager can therefore agree a forecast that omits an explicitly named unknown estimate. Blank cells may legitimately mean no involvement; a nonblank unknown estimate needs a separate state.

**Evidence:** Go probe: `TBD known plus unknown work=map[Atlas:{4 true true []}] provisional=false unestimated=[]`.

**Source:** `server/planning/planning.go:478`.

**Recommended change:** Preserve explicit unknown work, surface a cell-level import warning, and require an estimate or an explicit exclusion before treating the scope as fully estimated.

### M02 · P1 · Duplicate team identities corrupt demand and capacity totals

Two Atlas estimate columns containing 3 and 7 retain only 7. Two Atlas roster rows with 2 and 5 tracks are accepted: scheduling uses 5 tracks, while Fit counts 182 available track-weeks instead of the 130 represented by those tracks over 26 weeks. These are conflicting capacity answers for the same plan.

**Evidence:** Go probes: `duplicate team columns ... Weeks:7`; `duplicate roster scheduleTracks=5 fitAvailable=182 expectedForUsedTracks=130`.

**Source:** `server/planning/planning.go:481`; `server/planning/teams.go:180`; `server/planning/schedule.go:643`; `server/planning/schedule.go:2219`.

**Recommended change:** Reject ambiguous duplicate normalized identities before saving; identify the conflicting rows or columns. Use the same validated team collection for Fit and scheduling.

### M03 · P1 · Accepted dependency capitalization can change execution order

Strict import accepts `ATLAS` as matching the roster team Atlas, but preserves the uppercase spelling. The scheduler then fails to enforce it. In the same fixture, Beacon starts at week 4 for `Atlas` and week 0 for `ATLAS`. The provisional warning does not restore the required sequence.

**Evidence:** Go probe: `dependency"Atlas"` gives Beacon 4–5; `dependency"ATLAS"` gives Beacon 0–1.

**Source:** `server/planning/planning.go:146`.

**Recommended change:** Resolve accepted references to canonical roster identities during import and validate all scheduling references against those identities.

### M04 · P1 · Agreement comparisons can report false delivery improvements

A previously scheduled initiative held outside the horizon has a missing commit represented by zero. Comparison subtracts the old commit and the UI renders a green improvement such as `w12 → w0`, `−12w`. Separately, moving the entire period start forward eight weeks produces zero date deltas and `Moved=0` because comparison uses relative week offsets. These affect both agreement review and consumers of the same comparison arithmetic.

**Evidence:** Go probe: shifted period gives zero deltas; held work gives `CommitDeltaWeeks:-4`. UI probe: `fabricatedWeekZero:true, falseGreenImprovement:true`.

**Source:** `server/planning/baseline.go:194`; `app/js/baseline.js:98`.

**Recommended change:** Represent unplaced dates explicitly. Compare actual calendar dates when period origins exist; label relative comparisons when they do not. Never color losing a placement as an improvement.

### M05 · P1 · Missing bound epics still produce completion and calibration claims

Bind one initiative to PROJ-1 and PROJ-9 in both the agreement and current plan. A snapshot containing only PROJ-1 and one completed child reports 100% complete, an actual finish, on-track status and a calibration factor of 0.5, while also reporting that PROJ-9 is missing. The missing-evidence guard withholds the conditional forecast but does not withhold these whole-scope conclusions.

**Evidence:** Go probe: `percentComplete:100`, `actualFinishWeek:3`, `status:"on-track"`, calibration `factor:0.5, sampleCount:1`, alongside the missing-epic gap.

**Source:** `server/planning/actuals.go:259`; `server/planning/actuals.go:378`; `server/planning/actuals.go:445`.

**Recommended change:** Keep observed child counts, but distinguish captured-subset progress from complete-scope progress. Withhold whole-scope finish, comparative risk and calibration when agreed evidence is missing.

### M06 · P1 · Concurrent initiative saves have a lost-update path

Each request loads a whole initiatives document, patches its copy and replaces the stored document. Two overlapping binding saves can both succeed while the later write restores the earlier version of the other initiative. The UI disables only the individual submitting form. This finding is source-traced; a concurrent PostgreSQL HTTP reproduction was not run.

**Evidence:** Read/modify/write trace: both requests read `[Atlas:A, Beacon:B]`; their writes are `[Atlas:A2, Beacon:B]` and `[Atlas:A, Beacon:B2]`. The SQL update has no revision predicate.

**Source:** `server/planhandlers.go:156`; `server/planhandlers.go:818`; `server/db/plans.go:106`; `app/js/executionui.js:107`.

**Recommended change:** Add transactional atomic patching or a revision precondition with conflict recovery. Serializing one browser’s saves helps locally but cannot protect against another tab.

### M07 · P1 · A late upload response can discard another plan’s draft

Start an upload in plan A, switch to plan B and begin a draft. When A’s request completes, the handler reads the current plan ID, clears undo state and reloads B. That reload replaces B’s working inputs with stored inputs. The request itself still targets A; the error is in its late UI completion.

**Evidence:** UI probe: `reopenedPlans:['plan-b'], clearedOtherPlanUndo:true` after an upload initiated in A.

**Source:** `app/js/planui.js:1940`; `app/js/planui.js:263`.

**Recommended change:** Capture the initiating plan and draft revision before awaiting. Apply completion effects only to that context, preserving newer work and its undo history.

### M08 · P1 · Simulator date conversion mixes calendar and working days

Jira cycle samples are elapsed calendar days. Their lognormal parameters pass unchanged into the simulator, whose date formatter multiplies by 7/5 as though the result were working days. An isolated one-task fixture with a seven-calendar-day median returns P50 about 6.985 but displays a date ten calendar days away. Handoff durations also use working days, so multi-team forecasts mix units internally.

**Evidence:** Time probe: `inputCalendarMedian:7, simulatedP50:6.985135321498676, displayedCalendarDays:10`.

**Source:** `server/jira/aggregate.go:140`; `app/js/main.js:63`; `app/js/sim.js:107`; `app/js/simulator.js:282`.

**Recommended change:** Choose and document one duration basis; convert mined samples and handoffs consistently. Add a single-task date sanity check and a cross-team calendar case.

### M09 · P1 · Imported labels are emitted as HTML in Measure

An inert team name `<b data-audit="probe">Atlas</b>` and a formatted site label survive roster parsing and are interpolated directly into Scoreboard and Levers markup. Similar sinks exist in Data Quality and network details. At minimum this changes layout and displayed labels; script execution and deployed exploitability were not tested.

**Evidence:** Measure probe: `rawNameMarkup:true, rawSiteMarkup:true`; independent parser probe accepted and preserved both labels.

**Source:** `app/js/scoreboard.js:111`; `app/js/flow.js:35`; `app/js/hygiene.js:125`; `app/js/graph.js:398`.

**Recommended change:** Render imported values as text or escape them at every HTML/attribute boundary. Keep intentionally generated UI markup separate from external labels.

### M10 · P2 · The agreement drawer and status chip do not stay current

Compare stores its result but repaints Order instead of the open agreement drawer; from Timeline the manager sees no comparison appear. Saving changed assumptions or a timeline edit also leaves cached baseline divergence untouched, so the chip can still say “matches.” These are two related omissions in refreshing agreement state.

**Evidence:** UI probe: `orderPaints:1, drawerPaints:0`; after changed assumptions, `baselineLoads:0, chipStillMatches:true`.

**Source:** `app/js/planui.js:807`; `app/js/planui.js:569`; `app/js/planui.js:1069`.

**Recommended change:** Refresh the drawer after comparison actions and refresh agreement divergence after every persisted scheduling-input change. Preserve the selected view and focused control.

### M11 · P2 · Reassigning work can remove uncertainty without estimating it

Merging an unestimated Beacon slice into estimated Atlas combines their estimation flags with OR. The resulting work becomes estimated and the initiative loses its provisional status, although the unknown work has not been sized. A proposed organizational remedy can therefore look more certain for an accounting reason.

**Evidence:** Go probe: `reassign unknown beforeProvisional=true afterProvisional=false`.

**Source:** `server/planning/simulate.go:263`.

**Recommended change:** Preserve the presence of unknown contributing work when combining slices, with an explicit partial-estimate state or equivalent provenance.

### M12 · P2 · Saving a decision erases newer typing

The decision form remains editable during the request, then resets unconditionally after success. Text entered while the first decision saves disappears even though it was never submitted.

**Evidence:** Execution UI probe: `savedAction:'Initial action', actionAfterResponse:''` after a different action was typed during the request.

**Source:** `app/js/executionui.js:152`; `app/js/executionui.js:156`.

**Recommended change:** Reset only an unchanged submitted draft, or disable all form fields during saving with clear feedback.

### M13 · P2 · Execution Refresh cannot recover a failed snapshot list

After the initial snapshot catalog request fails, the selector contains “Snapshots unavailable.” Refresh requests evidence only; with no selected snapshot it returns without retrying the catalog. The message changes to “Choose a snapshot” although none can be chosen.

**Evidence:** Execution UI probe after Refresh: `snapshotCalls:1`, unavailable option still present.

**Source:** `app/js/executionui.js:141`; `app/js/executionui.js:162`.

**Recommended change:** Retry the failed catalog before loading evidence; retain an actionable error until recovery succeeds.

### M14 · P2 · Failed simulator runs retain the previous successful result

An empty task list, missing team statistics or a simulation error leaves `lastResult` and previous charts intact. Missing-team and invalid-graph errors replace only the stat cards. Revisiting the simulator redraws the old successful result and can overwrite the error. The dirty-source message helps, but the figures still need their own valid-result state.

**Evidence:** Execution UI probe: `keepsPreviousResult:true` for empty, missing-team and invalid cases.

**Source:** `app/js/simulator.js:84`; `app/js/simulator.js:209`.

**Recommended change:** Invalidate or explicitly label the previous result and bind it to the task-input revision. Preserve the current error across tab changes.

### M15 · P2 · Frozen-snapshot fever status changes without new evidence

The fever chart combines captured completion with elapsed time calculated from today. An unchanged snapshot at 50% completion is green when viewed on its capture date and red four weeks later in the fixture. A current-date projection could be useful, but it must be distinguished from the captured state; the source panel currently describes snapshot evidence.

**Evidence:** Time probe: same frozen snapshot gives `zone:'green'` on September 8 and `zone:'red'` on October 6.

**Source:** `app/js/flow.js:216`; `app/js/flow.js:234`.

**Recommended change:** Default historical evidence to the capture timestamp. Offer a separately labeled “assuming no further progress” projection if managers need that comparison.

### M16 · P2 · Synthetic estimates can generate real-looking intervention advice

A team with no mined history receives synthetic WIP. Scoreboard can show both “no data” and “over capacity”; Levers can rank that team as the number-one constraint and prescribe a WIP cap without a local synthetic label. The source banner discloses synthetic teams in aggregate, but does not make the individual recommendation evidence-based.

**Evidence:** Measure probe: `overCapacityFlag:true, noDataFlag:true`; Levers `syntheticRankedAsConstraint:true, syntheticLabel:false, suggestsWipCap:true`.

**Source:** `app/js/main.js:21`; `app/js/scoreboard.js:55`; `app/js/flow.js:23`.

**Recommended change:** Keep illustrative scenarios available, but exclude synthetic-only teams from observed constraint rankings and attach provenance to each metric or recommendation.

## Documentation and guidance defects

These survived comparison with current source. The conflicting baseline policy
needs a specification decision, not a silent choice of behavior.

| ID | Problem | Recommended correction | Source |
|---|---|---|---|
| D01 | Contextual Help from Plan → Dependencies targets nonexistent `plan-network`. | Map views explicitly to manual topics and check every destination. | `app/js/docs.js:75` |
| D02 | Manual promises newest remaining agreement activates after deleting the active one. Code leaves none. Spec FR-004 agrees with code; Q1 contradicts it. | Reconcile the decision record and manual. | `app/docs.html:527`; `server/db/plans.go:315`; `specs/015-baselines-drawer.md:117`; `specs/015-baselines-drawer.md:165` |
| D03 | Guide points to removed “Save as baseline (Order header)” control; its action only opens Order. | Name and open the agreement chip/drawer. | `app/js/guide.js:270`; `app/js/guide.js:392`; `specs/015-baselines-drawer.md:119` |
| D04 | Edge dragging is described universally as estimate editing. Split/display-split and in-flight work instead lack resizing and their edges act as moves. | Explain exceptions and inspector controls; show resize affordances only where supported. | `app/docs.html:494`; `app/js/timeline.js:414`; `app/js/timeline.js:450`; `app/js/drag.js:57` |
| D05 | Simulator manual says every team uses its real distribution and the forecast “reflects reality,” despite synthetic fallbacks. | Explain conditional forecasts, coverage and assumptions consistently. | `app/docs.html:287`; `app/js/main.js:67` |
| D06 | Scoreboard P85 tooltip prescribes a date to promise with confidence. | Explain this is a historical single-item percentile, not an epic or portfolio commitment. | `app/js/scoreboard.js:34` |
| D07 | Learn groups the baseline example under “Live org snapshots”; supporting docs call it a mined seed. | Distinguish examples from dated imported captures throughout the app. | `app/js/gamesui.js:83`; `docs/snapshots-and-scenarios.md:12`; `app/js/measure-context.js:24` |
| D08 | Manual locates period start in Plan setup; README says Undo has one level. | Point to Assumptions and describe the retained undo history. | `app/docs.html:316`; `app/js/order.js:905`; `README.md:46`; `app/js/planui.js:992` |
| D09 | Empty assignee is described as work nobody is doing. The query only establishes missing assignment data. | Ask the manager to confirm ownership instead of asserting inactivity. | `app/docs.html:272`; `server/db/execution.go:10` |

## Manager workflow and layout improvements

These are proposals, not reproduced defects. Validate the interaction choices
with managers using realistic portfolios.

| Priority | Improvement | Concrete interaction and benefit |
|---|---|---|
| First | Weekly execution review | Agreement and snapshot dates → coverage gaps → exceptions → action, owner and review date. Let managers run the meeting without reconstructing context across unrelated screens. |
| First | Import reconciliation | Before accepting a workbook, show recognized teams, participating slices, missing estimates, ignored columns, unmatched references and duplicates. Link warnings to their source row/column and resulting initiative. |
| First | One visible agreement state | Distinguish working plan, saved agreement, unsaved edits and comparison target. Refresh “changed since agreement” after saves; keep comparisons in the drawer that launched them. |
| Next | Unplaced-work queue beside Gantt | List unscheduled and beyond-period slices with reason, remaining effort and a direct action: estimate, release a constraint, extend the period or defer. Apply the same initiative search to this queue. |
| Next | Team-capacity inspector | Selecting a gap should explain tracks, loss, dependencies, calendars and lead policy, including why waiting work cannot occupy it. Offer a precise scheduling action from that explanation. |
| Next | Exception-focused execution | Filter changed scope, missing evidence, late slices, missing owners and overdue decisions. Retain removed agreement initiatives as removed scope until explicitly reviewed. |
| Next | Results linked to inputs | Put plan/agreement/snapshot identity, date, input revision and key assumptions beside results and in exports. Label a retained successful simulation as a previous run. |
| Next | Predictable controls and icons | Use consistent text for Save, Compare, Activate, Defer and Record decision. Give secondary icon-only controls accessible names. Separate destructive actions; show save state beside the control used. |
| Next | Progressive disclosure | Keep plan identity, period, save state and agreement status visible. Collapse detailed assumptions/source metadata after a summary answers what is shown. Avoid requiring managers to reread a large source block at each navigation. |
| Later | Portfolio review Home | Support choosing the monitored plan or reviewing several active plans. Home currently discloses that it checks only the most recently updated populated plan. |
| Later | Plain-language terminology | Introduce “Team (pod),” “Agreed plan (baseline)” and “capacity bottleneck (drum).” Explain tracks as concurrent capacity. Teach advanced terms beside the actions that need them. |

The next visual pass should cover desktop and narrow layouts with long generic
names, many teams, missing dates, split work and unplaced slices. Check sticky
labels, chart navigation, visible focus, keyboard alternatives, touch targets
and warnings readable without color. This audit does not claim those checks passed.

## Further concerns requiring end-to-end validation

- **Saved lane validation across mutations:** initiative edits invoke
  `ValidateLanePinsWithParams`, while general metadata PATCH at
  `server/planhandlers.go:1321` changes capacity loss without that check.
  Loss can lengthen pinned work. Exercise the complete HTTP save/recompute path
  against an isolated DB before counting a persisted-lane defect. The governing
  decision covers all scheduling edits:
  `specs/019-scheduling-audit-and-gantt-integrity.md:184`.
- **Removed agreement scope:** derivation iterates current initiatives at
  `server/planning/actuals.go:211`. Decide how removed initiatives remain visible
  in execution review. This is a scope-history proposal, not an assertion that
  current requirements mandate that behavior.
- **Integration recovery:** exercise concurrent saves, authentication expiry,
  inaccessible/deleted snapshots, retries and cross-tab edits against an isolated
  server. Pure functions and DOM stubs cannot establish complete persistence behavior.

## Validation record and limits

RAN `node --test tests/*.test.mjs`:

```text
tests 374
pass 374
fail 0
skipped 0
```

RAN uncached `go test -race -count=1 ./server/...` with a task-specific Go cache:

```text
ok  conway/server          8.653s
ok  conway/server/auth     5.134s
?   conway/server/db       [no test files]
ok  conway/server/game     3.596s
ok  conway/server/jira     1.936s
ok  conway/server/logging  2.266s
ok  conway/server/oidc     2.915s
ok  conway/server/planning 6.795s
```

The isolated PostgreSQL environment was not configured. Its integration specs
skip without `CONWAY_TEST_DATABASE_URL`
(`server/execution_integration_test.go:37`). The passing command does not
establish concurrent database correctness.

RAN all five retained commands in the
[reproduction README](audits/2026-09-05/README.md); each exited 0 and recorded
the adverse behavior above. Handler probes use controlled DOM/request stubs.
The existing suites passing does not refute those reproductions.

A fresh read-only local browser inspection was attempted, but automatic approval
review timed out and denied access to the local app. No alternate browser route
was used. Fresh screenshots, end-to-end interaction and live visual validation
remain uncompleted. External Jira/OIDC services, a live multi-user learning game
and production deployment behavior were not exercised. No stored user plans
were changed.

## Suggested implementation order

1. Preserve imported scope and identities; correct comparison semantics, date
   units and missing-evidence conclusions (M01–M05, M08).
2. Prevent lost edits and unsafe label rendering; repair agreement refresh and
   execution recovery (M06–M07, M09–M14).
3. Align Measure clocks, synthetic-data behavior and manager guidance
   (M15–M16, D01–D09).
4. Add weekly review, import reconciliation and unplaced-work workflows, then
   complete the visual and isolated integration checks.

For each fix, add a regression proving the manager-visible failure is gone.
Use model checks for arithmetic and interaction checks for request timing,
focus and persistence.
