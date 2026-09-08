# Product delivery roadmap

Updated 2026-09-07. The maintainer selected the remaining delivery order below
on this date after reviewing the feature proposals. This order supersedes the
previous sequence and priority suggestions in
[the feature strategy](FEATURE-STRATEGY-2026-09-05.md). Numbered specifications
remain the source of truth for feature requirements.

Completed foundation: planning and scheduling corrections, execution evidence,
scenario copies and agreements, contextual documentation, feature announcements,
and linked Google Sheets (PR 80); weekly execution review and action follow-up
(spec 024) and the team ready-work queue (spec 025), both merged in PR 81;
reliable evidence capture, recovery and source-scoped identities (spec 026,
PR 82).
The first evidence-linked planning assistant increment (spec 027) is implemented:
saved-schedule explanations, agreement comparisons and review agendas, with
optional bounded question interpretation. Merged in PR 83;
live provider quality remains a deployment validation step.
Live connection validation remains a separate release check.

| Priority | Remaining item | Completion estimate | Status / dependency |
|---|---|---|---|
| Release | Live Google/Jira validation; startup changes | 0% | Pending live connection checks and review of local startup changes |
| 1 | Evidence-linked planning assistant | 100% | Merged PR 83; optional live model validation required before enabling |
| 2 | Calibrated portfolio forecasts | 85% | Scenarios merged in PR 84; prediction history merged in PR 85. Distinct-work historical validation and monthly descriptive coverage implemented in spec 029; release review pending. Held-out probability-model calibration remains |
| 3 | Flow improvement experiments | 0% | Planned; explicit hypotheses, comparable before/after measures and guardrails |
| 4 | Outcome and minimum-scope planning | 0% | Planned; extend lightweight review outcomes into initiative scope choices |
| 5 | Milestones and external dependencies | 0% | Planned; acceptance events, vendor commitments and release conditions |
| 6 | Execution history and aging work | 0% | Planned; observed transitions, workflow definitions and comparable cohorts |
| 7 | Operational capacity and disruption planning | 0% | Planned extension; dated partial capacity and disruption impact previews using existing assumptions and calendars |
| 8 | Scenario comparison and agreement review | 0% | Planned extension; common revision, displaced work and acknowledgements |
| 9 | Shared capacity across plans | 0% | Planned; canonical teams, committed plan allocations and conflicts in existing timelines and reports |
| 10 | Dependency agreements | 0% | Deferred to this position; accountable providers, shared records and acceptance conditions |

Percentages are rough scope estimates, not measured engineering effort. Live
release checks are separate from implementation completion.

Dependencies qualify the scope and claims of each increment; they do not silently
change this priority order. In particular, forecast ranges must not be called
calibrated without historical validation, and flow experiments must identify
missing or non-comparable observations. Include necessary enabling work in the
relevant specification and make any proposed change to delivery order explicit
to the maintainer.

An item is complete only with its usable workflow, appropriate behavioral and
browser checks, updated concepts/calculation documentation, and feature discovery.
After each completion, retain the ordered table and update status and dependencies.
