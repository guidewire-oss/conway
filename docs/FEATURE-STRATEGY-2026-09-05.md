# Feature strategy: planning, execution and organizational learning

Date: 2026-09-05. Reviewed product revision: `fe5bbb3`.

**Status: proposal for discussion, not an approved implementation roadmap.**
The rankings below are product judgments grounded in the repository and the
reported user journeys. They are not measured demand, delivery estimates or
promises of business improvement. No new product capabilities are implemented
by this document. Accepted features should receive numbered specifications and
decision records under `specs/` before implementation.

## 1. Recommendation

Conway's strongest opportunity is to help organizations repeatedly make and
follow through on delivery decisions: select outcomes, prepare work, agree a
feasible sequence, respond to exceptions and learn from the result.

The current product has many useful ingredients: finite team-track scheduling,
explanations for holds, scenario copies, remedy previews, agreements, explicit
Jira bindings, dated execution evidence and a review decision log. The next
investment should connect these into a regular management and team workflow.

Prioritize a **weekly execution review with accountable follow-up**, a **team
work queue that explains what is eligible to start**, and **explicit dependency
agreements between teams**. Establish trustworthy shared capacity before
claiming organization-wide portfolio feasibility. Add historical evidence and
calibrated forecasting in stages. Connect delivery to customer outcomes so
success is not reduced to schedule compliance.

Keep the product centered on decisions across work systems. A second detailed
ticket board would create duplicate upkeep and compete with the source of
execution evidence.

## 2. Evidence and boundaries

### Current capability map

| Area | Present today | Missing workflow or proposed extension |
|---|---|---|
| Planning | Work and team imports; finite scheduling; WIP, readiness, leads, calendars and optional dates | Replenishment decisions, evidence for readiness and future team capacity profiles |
| Alternatives | Independent named scenarios, ordering proposals and priced remedies | Compare packages of changes against a common reference; review and accept with stakeholders |
| Agreements | Immutable named baselines and active agreement selection | Review of changes, explicit acknowledgement and treatment of removed scope |
| Execution | Snapshot selection, confirmed epic bindings, inferred progress/dates, review decisions | Exception triage, action lifecycle, observed transition history and regular review cadence |
| Organization | Snapshot network, WIP, quality, flow suggestions and simulator | Shared commitments across plans, durable team identity and trends across comparable observations |
| Learning | Flow Game and documented management concepts | Record a real experiment after a game and review its observed result |

Source anchors: initiative fields in `server/planning/planning.go:55`;
scheduling controls in `server/planning/schedule.go:122`; execution derivation
in `server/planning/actuals.go:161`; review decision storage in
`server/db/execution.go:41`; execution form and its notification boundary in
`app/js/executionui.js:85`; scenario and execution requirements in
`specs/017-planning-and-execution-usability.md:83`.

Continuous Jira sync, changelog history, trends, configuration versioning and
before/after interventions were already proposed in
[the v2 document](v2-spec.md#4-functional-requirements-ears). They are existing
roadmap ideas, not newly discovered features or proof of implemented behavior.
The older architecture and deployment assumptions need reconciliation with
today's application before those requirements become implementation work.

The [manager audit](MANAGER-AUDIT-2026-09-05.md) records previously reproduced
defects and separate source-traced concerns. Its probes were not rerun for this
strategy. This analysis uses them as release prerequisites to recheck, not as
newly reproduced findings. No fresh customer interviews, production analytics,
concurrency load tests or competitive product trials were performed.

### Management principles

The [guide's foundations](../app/docs.html#foundations) record the product's
inspiration from The Goal, Critical Chain, Goldratt's Rules of Flow, The Phoenix
Project and The Unicorn Project. The proposed applications here are:

| Inspiration | Product consequence |
|---|---|
| The Goal | Make the limiting part of delivery visible; measure outcomes across the system |
| Critical Chain | Protect the agreed chain; make contention, uncertainty and buffer use explainable |
| Rules of Flow | Prepare work before starting; manage release and reduce harmful multitasking |
| The Phoenix Project | Include operational work and interruptions; shorten feedback from execution |
| The Unicorn Project | Reduce handoffs and friction; connect improvements to customer value |

These connections do not make Conway's existing numerical heuristics prescribed
book formulas. A claim of a complete critical-chain implementation would need
its own scope and validation.

External evidence supports the direction, not the specific feature ranking:

- DORA describes WIP limits together with visible work and feedback, including
  accounting for support and improvement work. This supports testing a release
  queue and interruption allowance, rather than rewarding full utilization.
  [DORA: WIP limits](https://dora.dev/capabilities/wip-limits/).
- DORA describes visibility from business intent through delivery, including
  upstream and downstream work. This supports explicit handoffs and a view of
  value beyond one team's schedule.
  [DORA: work visibility](https://dora.dev/capabilities/work-visibility-in-value-stream/).
- The Kanban Guide defines WIP, throughput, age and cycle time against explicit
  workflow boundaries. This supports agreement on what “started” and “finished”
  mean before adding aging alerts.
  [The Kanban Guide](https://kanbanguides.org/the-kanban-guide/).
- SPACE treats developer productivity as multidimensional. The proposal should
  therefore assess delivery, quality and team experience together, without
  converting activity counts into individual performance rankings.
  [SPACE research](https://www.microsoft.com/en-us/research/publication/the-space-of-developer-productivity-theres-more-to-it-than-you-think/).

External sources accessed 2026-09-05. Feature mechanisms and suggested pilot
criteria below are our design proposals, not findings from those publications.

## 3. Jobs to support

| Person | Recurring decision | An effective result |
|---|---|---|
| Portfolio leader | Which outcomes can we commit to with shared capacity? | A selected scope with explicit displaced work and accepted tradeoffs |
| Delivery manager | What changed, who needs to act, and by when? | A short review agenda with evidence and tracked follow-up |
| Team lead | What should we finish, unblock or pull next? | A team queue with eligibility, dependencies and capacity reasons |
| Product manager | What is the smallest useful outcome we can deliver? | A scope option tied to a customer measure and acceptance conditions |
| Platform or operations lead | How do interrupts affect the work we promised? | Visible service obligations, reserves and a negotiated response to overload |
| Team member | What dependency or decision is holding our work? | A named request, responding owner, expected handoff and escalation route |
| Facilitator | Did the lesson change how we work? | An improvement experiment with a review and observed evidence |

## 4. Ranked feature opportunities

Priority is ordinal, not a synthetic ROI score. “First” favors repeated value
and reuse of current capabilities. “Next” depends on better identity, evidence
or coordination. “Later” has substantial modeling or adoption uncertainty.
Effort is relative: S = bounded view/workflow, M = new persisted workflow,
L = changes across ingestion, model or authorization. These are not durations.

| ID | Opportunity | Type | Priority | Effort | Main dependency |
|---|---|---|---|---|---|
| F01 | Weekly execution review and action follow-up | Extend review decisions | First | M | Evidence/scope correctness and reliable saves |
| F02 | Team ready-work queue and release decisions | New workflow over existing scheduler | First | M | Clear eligibility and no silent scope loss |
| F03 | Dependency agreements and handoff inbox | New | First pilot | M | Stable identities and accountable owners |
| F04 | Shared capacity across active plans | New | Next; earlier for shared teams | L | Canonical teams, plan revisions and access scope |
| F05 | Scenario comparison and agreement review | Extend scenarios and baselines | Next | M | Truthful comparison and conflict handling |
| F06 | Outcome and minimum-scope planning | New | First lightweight pilot | M | Outcome owner and acceptance evidence |
| F07 | Evidence history, aging work and freshness | Existing v2 idea; extend execution | Start foundation early | L | Durable ingestion and workflow definitions |
| F08 | Operational capacity and disruption response | Extend capacity model | Next | L | Time-varying capacity and scenario comparison |
| F09 | Portfolio uncertainty and forecast calibration | Extend forecasting | Later | L | Comparable history, consistent units and finite-resource trials |
| F10 | Flow improvement experiments | Existing v2 idea plus learning link | Next | M | Baseline measures and comparable history |
| F11 | Delivery milestones and external dependencies | New scheduling objects | Next where needed | L | Explicit acceptance events and dependency validation |
| F12 | Evidence-linked planning assistant | New optional interface | Later | L | Stable explanations, permissions and evaluation cases |

### F01. Weekly execution review and action follow-up

**Problem:** Managers must assemble a review by reading several views. Existing
decisions record action, owner and review date, but the stored object has no
completion state, resolution evidence or follow-up history.

**Experience:** Open the chosen plan and select **Review this week**. See changes
since the last review: missing evidence, newly blocked work, scope changes,
forecast movement, agreement divergence and actions due. Selecting an exception
opens its initiative and supporting evidence. Conclude by recording an action,
owner, due review, and next checkpoint; resolve it later with evidence.

**First increment:** Manual review sessions; saved exception filters; decision
states Open, In progress, Resolved and Superseded; a before/after evidence link;
an accessible shareable review summary. Keep a data-gap section separate from
delivery-risk ranking. Label “no new capture” without inventing new progress.

**Calculation:** An action is overdue when its agreed review date precedes the
review's date in the organization's chosen timezone and it is not resolved or
superseded. Group repeated symptoms under one underlying blocker. Start with
explicit rules, not an opaque urgency score.

**Measure:** Review preparation time, unresolved action age and the fraction of
resolved actions with supporting evidence. Avoid rewarding resolution counts
that encourage closing records without solving the problem.

**Later:** Opt-in digests with one notice per material change, acknowledgement,
snooze and a visible notification history. Notifications should link to the
decision, never silently modify Jira or an agreement.

### F02. Team ready-work queue and release decisions

**Problem:** A team can see an empty track without understanding what it should
do next. A schedule describes placement; a team also needs an operational
decision about finishing, unblocking or starting work.

**Experience:** **My team → Next work** groups items into In progress, Ready to
pull, Waiting and Deliberately deferred. Every waiting item states the reason,
the owner who can remove it, and the earliest feasible placement under current
assumptions. An idle lane has a visible explanation rather than a blank surface.

**First increment:** Derive the queue from the existing schedule and release
rules. Add a small full-kit checklist with required evidence, owner and last
confirmation. Let the lead record a release decision. A planned start remains
distinct from an observed start in Jira.

**Logic:** Ready requires the configured readiness conditions, predecessor
acceptance, permitted release window, WIP/lead eligibility and sufficient
contiguous lane capacity. A zero-effort milestone follows its own acceptance
rule; it must not consume a fabricated work week. Evaluate whole-chain impact
before suggesting a start merely because a local track is free.

**Measure:** Time from ready to start; avoidable blocked starts; aging ready
work; completed outcomes. Track whether new releases starve important work
that needs more preparation. Do not score teams by occupied lanes.

**Interaction:** Default to three suggested decisions, with **Show all** for
the full queue. Actions say **Inspect hold**, **Request readiness** and
**Record release**, distinguishing them from editing the forecast.

### F03. Dependency agreements and handoff inbox

**Problem:** A dependency edge says work must precede other work. It does not
establish that another team has accepted the deliverable or the requested date.

**Experience:** Beacon requests an interface from Atlas with acceptance
conditions and a needed-by date. Atlas accepts, proposes another date or asks
for clarification. Both see the same dependency record. A changed promise
identifies affected initiatives and prompts review before either plan changes.

**First increment:** Link a provider team, consumer team, initiative/slice,
deliverable, requested date, proposed date, accountable owner and status:
Requested, Accepted, Ready for acceptance, Accepted delivery, Blocked or
Cancelled. Separate a planning agreement from evidence of delivery acceptance.

**Calculation:** Handoff exposure is the accepted delivery date minus the
consumer's latest usable date, with both dates and calendar basis shown.
Positive exposure prompts an impact preview, not an automatic infeasibility
verdict. If either date or commitment is absent, show Unknown.

**Measure:** Unacknowledged request age, late handoffs, acceptance rework and
downstream waiting. Avoid counting inbox replies as successful delivery.

**Boundary:** This is a narrow coordination record linked to Jira, not a new
ticket hierarchy. Begin with one shared service and two consuming teams to
discover whether acknowledgement adds value or administrative burden.

### F04. Shared capacity across active plans

**Problem:** Local plan feasibility does not establish that several plans can
share the same teams. A reusable roster is not a global reservation ledger.

**Experience:** A portfolio leader chooses which active commitments share
capacity. A team view overlays those commitments on one calendar. A new plan
shows conflicts with existing commitments and a preview of who moves if it is
accepted. A scenario is excluded from committed demand until explicitly chosen.

**First increment:** Canonical team IDs; explicit portfolio membership;
dated capacity allocations per plan; cross-plan conflict detection. Use a
federated conflict view before attempting a global optimizer. Renaming teams
must not duplicate capacity or disconnect history.

**Calculation:** For each team and dated interval, compare the sum of accepted
lane reservations across included plans with available physical lanes. Match
time origins before comparing. Recurring productivity loss affects work
duration; it must not also be deducted from physical lanes without an explicit
model. Detect exact overlapping lane claims as well as aggregate overload.

**Measure:** Double-booked intervals, commitments changed after discovery of a
shared-team conflict, and reconciliation time. Never merge scenario copies into
the numerator. Do not disclose hidden plan names to unauthorized viewers; an
authorized capacity allocation can represent externally reserved capacity.

### F05. Scenario comparison and agreement review

**Problem:** An independent scenario and a before/after remedy are useful, but
stakeholders need to compare a package of choices and agree why one is preferred.

**Experience:** Compare up to three scenarios against one named agreement:
scope delivered, unstarted work, calendar commitments, constraint demand,
changes to active work and assumptions. Highlight who benefits and who waits.
Collect team acknowledgements and the accountable manager's decision before
creating a new agreement.

**First increment:** A saved comparison with common reference and revision;
comments attached to a change; explicit Accept, Request changes and Withdraw
states. Keep lightweight acknowledgement as the default; formal approval chains
should be an organizational choice.

**Calculation:** Compare calendar dates when both origins exist. Preserve
Scheduled, Unplaced and Removed scope as different states. Present several
objectives instead of collapsing business value, lateness and disruption into
an arbitrary “best plan” score. If ranking by a weighted objective, show the
weights and a sensitivity comparison.

**Measure:** Time to reach agreement, unexplained changes after agreement, and
rework caused by stakeholders reviewing different input revisions.

### F06. Outcome and minimum-scope planning

**Problem:** Finishing initiatives is useful only if their intended outcomes
matter. A single initiative estimate also hides smaller options that could
deliver value sooner.

**Experience:** Attach a customer or operational outcome, owner, current measure,
target, review date and evidence source. Define Essential and Optional scope
packages. Compare the earliest useful release with the full initiative, showing
which acceptance conditions each package meets.

**First increment:** Optional outcome fields and manually defined scope options
on an initiative. Add an outcome review after delivery. Preserve child work and
dependency identity when options include overlapping scope; never count shared
work twice. Distinguish Discovery, Delivery and Outcome confirmed.

**Calculation:** Roll up the measure according to its meaning. Sum counts only
when populations do not overlap; do not sum percentages or story points across
teams as delivered value. Keep forecast benefits separate from observations.
Priority and current unitless cost-of-delay inputs are not automatically money.

**Measure:** Delivered initiatives reviewed against an outcome, time to first
useful release and stakeholder decisions changed by outcome evidence. Avoid
requiring a speculative revenue number for maintenance or risk-reduction work.

### F07. Evidence history, aging work and freshness

**Problem:** Dated captures and Created-to-Resolved proxies cannot answer all
questions about when active work stalled or how interventions changed flow.

**Experience:** Configure a read-only scheduled capture. See its last successful
run and coverage. In **Team flow**, inspect aging active work and waiting time
against a declared workflow definition. Every trend identifies source scope,
calculation version and any gap in history.

**First increment:** Deliver durable capture and failure recovery first. Add
status transitions and per-team workflow boundaries next. Use explicit “as of
capture” dates; distinguish an elapsed-age projection after capture from fresh
evidence that an item remains open.

**Calculation:** Age = observation time minus the observed start transition.
Cycle time = finish transition minus start transition under the selected
workflow. A service-level expectation is a chosen historical percentile for a
comparable cohort, with window and sample count shown. Define treatment of
reopened items and missing transitions before reporting. Time in an active
status does not establish actual hands-on work time.

**Measure:** Freshness attainment, incomplete capture frequency, transition
coverage and earlier detection of aging work. Published snapshots remain
immutable observations; a partial sync must not replace a complete capture.

### F08. Operational capacity and disruption response

**Problem:** A fixed loss percentage cannot express every support rotation,
capacity ramp, one-off outage or sudden urgent request.

**Experience:** A team edits a future capacity profile and a named support
reserve. An urgent item opens **Assess disruption**, showing effects on active
work and other commitments. The manager chooses to spend reserve, defer scope
or request capacity, recording why and for how long.

**First increment:** Dated team-level track changes, explicit operational
reserve and an expedite scenario. Skills or service capabilities are coarse
eligibility constraints; they are not individual productivity ratings. Add
cross-training ramp assumptions only when the team can justify them.

**Calculation:** Declare one representation for reserved support: unavailable
planning capacity or explicit scheduled work. Never subtract both. An expedite
preview reruns finite scheduling with the same dependencies and calendars,
shows displaced work, and preserves the active agreement until a new decision.
Count distinct moved initiatives and changed start/finish dates separately.

**Measure:** Interrupt-driven commitment changes, reserve forecast error and
time needed to negotiate a response. An initially configured reserve is a
hypothesis to inspect, not a universal recommended percentage.

### Remaining opportunities

| Feature | Smallest useful increment | Model or adoption boundary | Evidence of value |
|---|---|---|---|
| F09 Portfolio uncertainty and calibration | Estimate ranges and stress scenarios first; later sampled finite-resource schedules with replayable assumptions | Preserve dependency, WIP and capacity constraints in every trial; model correlated disruption; do not add initiative P85s or present Monte Carlo sampling error as model validity | Historical interval coverage and forecast usefulness on comparable completed cohorts; show missing scope and sample count |
| F10 Flow improvement experiments | Record hypothesis, intervention, owner, baseline window, review window and guardrail; link a game debrief to it | Before/after association is not causal proof; annotate scope, team and counting changes; use quality and team feedback alongside flow | Experiments reviewed, decisions retained/reversed, observed waiting and quality trends |
| F11 Delivery milestones and external dependencies | Zero-work acceptance events, vendor dependencies and release conditions alongside team work | A milestone is an event, not effort; unknown external dates remain uncertain; validate cycles, acceptance and dependency types before broad scheduling support | Earlier discovery of release blockers and fewer “development complete” items awaiting acceptance |
| F12 Evidence-linked planning assistant | Answer “why is this held?” and draft a review agenda from authorized records, with source links | Use scheduler results as the authority; never invent estimates or apply changes from prose; proposed actions require review; evaluate against known cases before release | Correct evidence citations, useful accepted suggestions, correction rate and time saved in repeated review tasks |

## 5. Interaction architecture

Add depth to existing routes rather than a new global menu for each feature.

| Place | Default content | Progressive detail |
|---|---|---|
| Home | Chosen portfolio/plan, decisions requiring attention, next review | Cross-plan comparison and source health |
| Plan → My plans | Active commitments, drafts and scenarios clearly identified | Portfolio membership and capacity allocations |
| Plan → Plan commitments | Scope, options, feasibility and agreement | Scenario comparison, acknowledgement and change rationale |
| Plan → Timeline | Initiative/team lenses and meaningful gaps | Candidate placement, dependency acceptance and milestone layers |
| Plan → Review execution | Exceptions and actions due | Source evidence, bindings, scope reconciliation and historical measures |
| Measure | Organization and team observations | History, cohort definitions, trends and experiments |
| Learn | Rehearsal and debrief | Create a proposed real-world experiment |

Use a shared context strip: portfolio or plan, team filter, working revision,
active agreement and evidence capture date. Distinguish **Plan assumptions**
from **Observed evidence** in every comparison. Preserve selections in shareable
links while continuing to enforce authorization.

The initiative inspector should be the common drill-down across views, with
Overview, Team work, Dependencies, Evidence and Decisions. A manager should not
re-enter the initiative name or lose the team filter when changing lenses.
Provide a breadcrumb back to the originating exception or scenario.

Use verbs with specific effects: **Preview impact**, **Request handoff**,
**Record release**, **Acknowledge change**, **Save agreement**, **Resolve action**.
Pair status icons with text and dates; show Unknown as its own state. Keep
advanced arithmetic expandable with a worked example and source context.
Keyboard users need a list/inspector equivalent for every timeline action.
Mobile should prioritize review and acknowledgement; precise lane editing can
use an explicit form. Do not make a tiny desktop Gantt the only mobile workflow.

No-evidence onboarding should still be useful: start with a workbook, a manual
readiness checklist and a review agenda. Add Jira association when available.
Show exactly which later capabilities need history. Offer role-based defaults
without hiding the common underlying plan.

## 6. Necessary foundations and release prerequisites

These are not feature launches. Recheck the earlier audit's relevant defects
and complete the required correction before releasing a dependent capability.

| Foundation | Why it matters | Prior audit to recheck |
|---|---|---|
| Scope and identity integrity | An omitted estimate or lost dependency invalidates a ready-work queue and capacity comparison | M01–M03, M11 |
| Truthful agreement comparisons | Unplaced or removed work must not look like an earlier delivery; date origins must align | M04, M10 |
| Whole-scope evidence rules | Incomplete capture must not establish completion, low risk or calibration | M05, M15–M16 |
| Revisioned writes and context-safe UI | Shared review and action work must survive concurrent edits and delayed responses | M06–M07, M12–M13 |
| Consistent units and result provenance | History and forecasts must agree about calendar/working time and current/previous results | M08, M14 |
| Safe rendering and access boundaries | Imported labels and shared portfolio views must respect content and access isolation | M09; new cross-plan authorization cases |

Architectural direction for eventual specs:

- Use stable IDs for initiatives and teams; names stay editable labels. Migrate
  historical references explicitly and report unresolved mappings.
- Separate working revisions, immutable agreements, immutable observations and
  append-only decision events. An action can have a current status plus history.
- Reuse one finite scheduler for forecasts, release eligibility and impact
  previews. Record input fingerprint and algorithm version with a saved result.
- Add a narrow contract for shared capacity before introducing cross-plan
  optimization. Preserve independent scenarios and separate access scopes.
- Make capture jobs resumable and idempotent. Do not infer a successful refresh
  merely from a running job or a recent failed attempt.
- Evolve current authentication and SSO with explicit reviewer, editor and
  portfolio-view permissions where needed. Existing login does not establish
  the right to reveal every plan or grant every user approval authority.

## 7. Sequence and pilot plan

This is a sequence of evidence gates, not a calendar estimate. Team size,
integration depth and deployment expectations have not been established.

| Stage | Deliver | Exit evidence |
|---|---|---|
| A. Trust and discovery | Recheck critical audit cases; observe manager/team reviews; reconcile current behavior with older v2 proposals | Known affected commitments cannot silently omit scope or misstate evidence; agreed pilot workflow and baseline measures |
| B. A repeatable weekly review | F01; lightweight F06 outcome fields; one selected plan on Home | Managers can identify the highest-priority exception, record an owner/action, return next review and explain the result |
| C. Start and hand off deliberately | F02 and a small F03 pilot; begin F07 capture foundation | Teams agree what Ready means, resolve dependency requests and use the queue without keeping a parallel spreadsheet |
| D. Coordinate commitments | F04, F05 and F08; F11 for portfolios with acceptance/vendor dependencies | Shared-team conflicts are visible before agreement; a scenario explains displaced work and acknowledgements refer to the same revision |
| E. Learn and forecast better | F07 history, F10 experiments, then F09 | Comparable historical cohorts support the claimed measures; forecasts retain scope/resource constraints and have calibration evidence |
| F. Add assistance selectively | F12 | The assistant retrieves authorized evidence accurately and saves time on repeated tasks; deterministic controls retain authority |

For a small autonomous team, begin with F01/F02 and keep portfolio controls out
of the default view. For several plans sharing platform or specialist teams,
move F04 into the earliest coordinated pilot. For operations-heavy organizations,
pair F02 with F08. For weak issue data, prioritize manual reviews and coverage
reconciliation before predictive analytics.

Suggested pilot: one delivery manager, two delivery teams and one shared service
team, across four weekly reviews. This is a proposed sample, not a statistical
validation study. Observe the existing process before introducing changes.

Record review preparation minutes, time to identify a blocker owner, ready-work
waiting time, unresolved action age, cross-plan conflicts found before agreement,
and observed delivery/quality context. Use consistent item populations and
capture dates. Short pilots can establish usability and workflow adoption;
they cannot prove long-term throughput improvement.

Proposed usability acceptance tasks:

1. Identify which plan, agreement and evidence date a result uses without help.
2. Explain an idle team lane and find an eligible action within two minutes.
3. Identify a dependency's responding owner and agreed deliverable.
4. Compare two scope choices and name both the beneficiaries and displaced work.
5. Reopen last week's review and determine whether its action was completed,
   superseded or remains unresolved, including supporting evidence.
6. Complete the review path by keyboard and at a narrow viewport.

Treat these time bounds as initial design targets. Adjust them after observing
representative users and keep success criteria distinct from claimed outcomes.

## 8. Defer or avoid

- **A universal organization health score:** it would conceal incompatible
  units and encourage optimizing the display. Keep the decision and its evidence
  explicit; use several interpretable measures.
- **A fully automatic portfolio optimizer:** priorities, ownership and external
  commitments need negotiation; modeling more constraints does not resolve that.
  Start with alternatives and explained consequences.
- **Individual productivity rankings or inferred morale:** team flow data does
  not establish individual contribution or personal state.
- **Broad two-way issue-tracker synchronization:** first establish ownership,
  conflict recovery and auditability. Read-only evidence and explicit links are
  sufficient for the first workflows proposed here.
- **More chart types without actions:** a new chart should support a specific
  decision and drill down to its inputs, owner and next step.
- **Large configuration surfaces at onboarding:** presets should be visible
  starting assumptions, with progressive detail and a scenario for comparison.

## 9. Questions before committing a roadmap

Resolve these through pilot observation and stakeholder discussion, rather than
inventing requirements:

1. How many active plans share the same teams, and who can negotiate allocations?
2. What is today's most costly recurring failure: starting unready work,
   cross-team waiting, scope churn, interrupts or late discovery of slippage?
3. What constitutes accepted delivery, and how often is it different from Jira Done?
4. Which decisions need acknowledgement, formal approval or only a recorded owner?
5. Which Jira history and workflow mappings are available and sufficiently reliable?
6. Which customer or operational measures establish value after delivery?

The first implementation specification should cover **weekly execution review
and action follow-up**, bounded to one plan and its existing evidence. It can
deliver a complete recurring workflow while revealing the identity, data and
coordination requirements for the larger portfolio capabilities.
