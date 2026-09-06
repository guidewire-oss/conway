# Conway documentation

Start with the **[Planning and execution guide](../app/docs.html)**. It is the
canonical user manual, also available through **Help** in the running app. Open
`app/docs.html` in a browser to read it directly; its content and styles ship
locally. Repository viewers may show the HTML source instead of rendering it.

In the app, use **Help → Help with this view** for the current task or
**Browse or search the guide** for the full reference. The overlay's **Open
guide in new tab** keeps the selected section available alongside your plan.
Search stays visible while reading; on smaller screens, **Contents** opens the
section menu. Tables become labeled rows on phones. Escape dismisses search or
contents first, then closes the embedded guide on a subsequent press.

## Get started

- [Concepts and vocabulary](../app/docs.html#concepts): teams, tracks, initiatives,
  WIP, buffers, agreements, scenarios and evidence.
- [Ideas behind Conway](../app/docs.html#foundations): The Goal, Critical Chain,
  Goldratt's Rules of Flow, The Phoenix Project and The Unicorn Project, with
  author credits and publisher references.
- [First-plan tutorial](../app/docs.html#start): prepare inputs, choose assumptions,
  review the schedule and save an agreement.

## Choose a workflow

| Your task | Guide |
|---|---|
| Choose settings for the way your teams work | [Situation-based choices](../app/docs.html#howto) |
| Prepare planning and delivery reviews | [Planning and weekly review](../app/docs.html#planning-loop) |
| Identify which data a view uses | [Sources and switching](../app/docs.html#snapshots-picker) |
| Upload a roster and initiative matrix | [Workbook preparation](../app/docs.html#plan-setup) |
| Maintain plan inputs in a shared spreadsheet | [Linked Google Sheets](../app/docs.html#linked-sheets) |
| Find newly available capabilities | [Feature announcements](../app/docs.html#feature-news) |
| Inspect capacity or edit placement | [Timeline](../app/docs.html#timeline) |
| Preserve and compare a commitment | [Agreements](../app/docs.html#baselines) |
| Prepare and release a team's next work | [Next work and release decisions](../app/docs.html#next-work) |
| Connect Jira evidence to initiatives | [Execution review](../app/docs.html#execution) |
| Complete a review and follow accountable actions | [Weekly review and action states](../app/docs.html#weekly-review) |
| Facilitate a learning session | [Flow Game](../app/docs.html#learning) |

## Explain a calculation

| Model | Reference |
|---|---|
| Tracks, loss, effort, duration and buffer | [Planning arithmetic](../app/docs.html#capacity-calculations) |
| WIP, leads, readiness, splitting, dates and calendars | [Scheduling options](../app/docs.html#assumptions), [calendar effects](../app/docs.html#sites) |
| Ordering rules and weighted costs | [Ordering](../app/docs.html#order) |
| Progress, inferred dates, variance and calibration | [Execution calculations](../app/docs.html#execution-calculations) |
| Overdue actions, review context and comparison limits | [Weekly review calculations](../app/docs.html#weekly-review) |
| Readiness, contiguous placement and release eligibility | [Next work calculations](../app/docs.html#next-work) |
| Queue proxy, dependency ranking and Org Flow Index | [Org Network](../app/docs.html#network) |
| Throughput, sample filtering and cycle percentiles | [WIP Scoreboard](../app/docs.html#scoreboard) |
| Missing evidence and quality scores | [Data Quality](../app/docs.html#hygiene) |
| Full kit, sampled durations, dependencies and percentiles | [Feature Simulator](../app/docs.html#simulator) |
| WIP reduction projections and buffer risk | [Flow Actions](../app/docs.html#flow-actions), [three fever calculations](../app/docs.html#fever) |

The manual separates inputs, units, assumptions and current limitations.
Conway's heuristics apply ideas from the books; the numeric defaults are not
presented as formulas prescribed by those authors.

## Administration and development

- [Run locally and contribute](../README.md#run)
- [Snapshots, scenario files and API](snapshots-and-scenarios.md)
- [Single sign-on](sso-oidc.md)
- [Configure linked Google Sheets](linked-google-sheets.md)
- [UI component registry](COMPONENTS.md): Bootstrap-first controls, theme and responsive layout conventions
- [Feature specifications](../specs/): source of truth for requirements and decisions
- [Feature strategy](FEATURE-STRATEGY-2026-09-05.md): proposed planning and execution improvements, priorities, dependencies and pilot criteria; not implemented features
- [Factory rules](FACTORY_RULES.md) and [workflows](../workflows/README.md)

When a model changes, update the relevant guide section and its contextual help
together. Check the actual execution path, including server handlers: a utility
function or database method alone may not describe the behavior users see.

| Calculation source | Implementation |
|---|---|
| Finite scheduling, ranking, buffers and release limits | [schedule.go](../server/planning/schedule.go) |
| Planning demand and what-if levers | [simulate.go](../server/planning/simulate.go) |
| Weekly calendar enforcement | [calendars.go](../server/planning/calendars.go) |
| Evidence-based execution proxies | [actuals.go](../server/planning/actuals.go) |
| Historical cycle samples | [aggregate.go](../server/jira/aggregate.go) |
| Data quality components | [enrich.go](../server/jira/enrich.go) |
| Monte Carlo, full kit, merges and flow projections | [sim.js](../app/js/sim.js) |
| Snapshot-to-view load preparation | [main.js](../app/js/main.js) |

Keep the HTML guide as the single complete manual; this index provides routes
into it rather than a second copy of the explanations.
