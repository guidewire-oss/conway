# Conway v2 — Product & Requirements Spec

**Status:** Draft for review · **Date:** 2026-06-19 · **Owner:** the tool owner
**Companion:** [`v2-architecture.md`](./v2-architecture.md) (the *how*). This doc is the *what/why*.

---

## 1. Purpose & vision

Conway today is a **snapshot tool**: a one-time Jira crawl produces static JSON, the
app renders a point-in-time picture, and an offsite game runs on that snapshot.

Conway v2 is a **continuous flow-observability platform for engineering leaders**:
it connects to Jira on its own, keeps itself up to date, preserves history, and shows
not just *where flow stands today* but *how it has changed over time* — so a leader can
make an intervention and watch the needle move. It must be usable by **any** engineering
leader, not just one org — nothing org-specific may be hardcoded.

The thesis is unchanged from v1 (Goldratt/TOC + Team Topologies + queueing): make flow,
constraints, dependencies, and toil visible. v2 adds the **time axis** and turns a
demo into a product.

## 2. Goals / non-goals

**Goals**
- Continuous, incremental Jira sync (not a one-time scrape).
- Durable history → trends, before/after, constraint migration over time.
- Point-in-time metrics *and* time-series, both trustworthy.
- Multi-tenant / fully configurable: any Jira, any projects, any team model.
- Carry forward the existing analytics, forecasts, and (optional) strategy game.
- Honest about data quality — surface confidence, never fake precision.

**Non-goals (v2)**
- Replacing Jira or being a work-tracker. Conway observes; it does not manage tickets.
- Individual performance measurement (explicitly forbidden — see NFR-PRIV).
- Real-time/streaming freshness. Hourly–daily sync is sufficient for flow analysis.
- Mining sources beyond Jira at launch (Slack/PagerDuty/git are future extensions).

## 3. Personas & primary use cases

- **Executive / VP (org owner):** "Is the org getting healthier or just busier?" Trends,
  constraint movement, flow scorecard, ROI of interventions.
- **Engineering lead:** "Did freezing my pod's WIP actually help?" Before/after on a pod.
- **PM / delivery:** epic forecasts (P85) and buffer-fever trends over the quarter.
- **Admin / operator:** connect Jira, configure the team model, manage access, watch sync health.
- **Facilitator (offsite):** run the strategy game from the live snapshot.

## 4. Functional requirements (EARS)

### 4.1 Onboarding & configuration
- **R-CFG-1** WHEN an admin first runs Conway, THE SYSTEM SHALL provide a setup flow to enter
  the Jira base URL, authentication, the project keys to mine, and the custom-field id that
  identifies a team (e.g. "Assigned Pod" = a custom field in your Jira instance).
- **R-CFG-2** THE SYSTEM SHALL let an admin define the **team model**: each team's site/timezone,
  member roster (for size and pairing), SRE/platform classification, and work area — by upload
  or in-app editing.
- **R-CFG-3** THE SYSTEM SHALL expose **model parameters** (flow-efficiency assumption, healthy-WIP
  per stream, handoff-latency-by-overlap, waiting-status set) as tunable config with documented
  defaults — never hardcoded.
- **R-CFG-4** WHERE a configuration value affects a historical metric, THE SYSTEM SHALL version it
  so trends remain interpretable and history can be recomputed (see R-HIST-4).

### 4.2 Jira connection & sync
- **R-SYNC-1** THE SYSTEM SHALL authenticate to Jira using a configurable credential: a read-only
  **service account / API token** for the background crawler, and/or **OAuth** for interactive setup.
- **R-SYNC-2** THE SYSTEM SHALL perform an initial **backfill** of issues, their fields, blocking
  links, and **changelogs** for the configured projects.
- **R-SYNC-3** THE SYSTEM SHALL thereafter sync **incrementally** on a schedule, fetching only issues
  changed since the last successful sync (watermark on `updated`).
- **R-SYNC-4** THE SYSTEM SHALL respect Jira rate limits with backoff/retry, and a sync SHALL be
  **idempotent** (re-running produces no duplicates or drift).
- **R-SYNC-5** IF a sync fails partway, THE SYSTEM SHALL resume from the last durable watermark
  without data loss or double-counting.
- **R-SYNC-6** THE SYSTEM SHALL record per-sync health (start/end, issues processed, errors, rate-limit
  hits) and expose a "data freshness / last successful sync" indicator in the UI.

### 4.3 Historization
- **R-HIST-1** THE SYSTEM SHALL retain raw issues and changelog events as an append-only source of truth.
- **R-HIST-2** THE SYSTEM SHALL compute and store **point-in-time rollups** (per pod, per epic, org-wide)
  at a configurable cadence (default daily), enabling trend queries without re-scanning raw data.
- **R-HIST-3** THE SYSTEM SHALL reconstruct historical state (e.g. WIP, status, age on a past date) from
  changelogs, so trends extend back to issue history — not only forward from install.
- **R-HIST-4** WHEN a metric definition or config parameter changes, THE SYSTEM SHALL be able to
  **recompute history from raw**, and SHALL label which definition-version produced each stored series.

### 4.4 Metrics & analysis
- **R-MET-1** THE SYSTEM SHALL compute the v1 metrics per sync: per-pod load (ρ), Kingman wait factor,
  cycle-time percentiles, throughput, WIP (active vs waiting), dependency edges, constraint ranking,
  hygiene, epic full-kit/outcome/due-date status, and the buffer-fever position.
- **R-MET-2** WHERE changelog data is available, THE SYSTEM SHALL compute **measured** flow metrics —
  time-in-status, flow efficiency, aging WIP, rollover, reopen/bounce rates — superseding the v1 proxies.
- **R-MET-3** THE SYSTEM SHALL keep the Monte-Carlo feature forecaster and the org-merge simulation,
  fed from the historized distributions.

### 4.5 Dashboards & trends
- **R-UI-1** THE SYSTEM SHALL render the v1 views (Flow Actions, Network, Feature Simulator, Flow
  Scoreboard, Data Hygiene, Guide) against live, continuously-updated data.
- **R-UI-2** THE SYSTEM SHALL add **trend views**: flow-index over time, per-pod load/cycle-time/morale-
  proxy trends, hygiene trend, constraint-migration timeline, dependency-graph evolution, and
  before/after framing around a marked intervention date.
- **R-UI-3** EVERY metric on screen SHALL carry a tooltip with its definition, formula, and book grounding,
  and EVERY figure SHALL be traceable to its source (Jira link for specific issues; derivation otherwise).
- **R-UI-4** THE SYSTEM SHALL show a confidence/quality indicator wherever data hygiene materially affects
  a number.

### 4.6 Strategy game (carry-over, optional per tenant)
- **R-GAME-1** THE SYSTEM SHALL retain the server-authoritative flow game, seeded from the live snapshot,
  with admin-managed expiring team logins and a projector leaderboard.
- **R-GAME-2** Game rules SHALL remain server-side only (never shipped to the client).

### 4.7 Access & administration
- **R-AUTH-1** THE SYSTEM SHALL require login for all non-public views; roles at minimum {admin, viewer}.
- **R-AUTH-2** THE SYSTEM SHALL store all secrets (Jira tokens, signing keys) encrypted at rest.
- **R-AUTH-3** THE SYSTEM SHALL provide an audit trail of admin actions (config changes, credential
  changes, account management).

## 5. Non-functional requirements

- **NFR-SCALE** Handle Jira instances with 10^5+ issues across many projects via batched incremental
  sync; a routine incremental sync SHALL complete within minutes, not hours.
- **NFR-RATE** Never exceed Jira rate limits; degrade gracefully (slower sync) rather than fail.
- **NFR-SEC** Least-privilege, read-only Jira scope; secrets encrypted; transport over TLS.
- **NFR-PRIV** *Anti-surveillance:* all people-adjacent signals SHALL be aggregated to team level. No
  per-individual performance metric, ranking, or dashboard. The product SHALL state "system diagnosis,
  not individual evaluation" in-app. This is a hard requirement, not a guideline.
- **NFR-REL** Sync is resumable and idempotent; a failed sync never corrupts the historized series.
- **NFR-OBS** Sync health, freshness, and error rates are observable to operators.
- **NFR-PORT** Self-hostable as a single service + embedded database with no external dependencies for a
  single-tenant deployment; cloud/multi-tenant is an additive deployment mode.
- **NFR-COST** Storage and compute scale with issue volume, not wall-clock; no unbounded growth (retention
  policy on raw, rollups kept long-term).

## 6. Data-quality & confidence

- **R-DQ-1** THE SYSTEM SHALL measure and trend Jira hygiene itself (sizing, staleness, assignment, link
  density, outcome presence) as a first-class output — improving hygiene is a goal, and its trend proves
  whether the data (and the org) is getting better.
- **R-DQ-2** THE SYSTEM SHALL present aggregate figures as floors/estimates where hygiene is poor, and
  SHALL document the known biases (single-project vs all-projects, under-reported links, etc.).

## 7. Phased roadmap (exit criteria)

| Phase | Scope | Exit criteria |
|---|---|---|
| **P1 — Continuous sync** | DB + incremental watermark sync replacing the one-shot scripts; app reads via API; same metrics, now fresh. | A scheduled sync keeps the existing dashboards current with no manual scripts. |
| **P2 — History & trends** | Changelog ingestion; daily rollups; trend views; before/after. | A leader can see a metric's trajectory over ≥1 quarter and mark an intervention. |
| **P3 — Multi-tenant config** | Remove all org-specific hardcoding; onboarding/settings; team-model editor. | A different team can stand up Conway against their own Jira with zero code changes. |
| **P4 — SaaS hardening** | Postgres, multi-org isolation, scheduled-routine UX, regression alerting. | Multiple orgs run isolated; flow regressions can alert. |

## 8. Product success metrics
- Time-to-first-insight for a new leader (install → first trustworthy board).
- % of figures traceable to source (target 100%).
- Hygiene trend across onboarded orgs (does using Conway improve the data?).
- Interventions tracked with measurable before/after.

## 9. Open questions (need stakeholder decision)
1. Single-tenant self-host first, or multi-tenant SaaS from the start? (Affects storage choice & isolation.)
2. Jira **Cloud**, **Data Center**, or both? (Auth and API differ.)
3. Service-account token vs Jira Connect app vs OAuth 3LO as the *primary* crawl credential?
4. Is the strategy game in-scope for the productized app, or does it stay an offsite-only mode?
5. Retention: how long to keep raw issues/changelogs vs rollups?
6. Which non-Jira sources (PagerDuty, Slack, git) are on the roadmap, and at what phase?

## 10. Out of scope (v2)
Writing back to Jira; replacing sprint planning; non-Jira trackers (Azure DevOps, Linear) — design the
ingestion port to allow them later, but don't build them now.
