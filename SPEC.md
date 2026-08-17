# Dependency Flow Model — Prototype Spec

> **This document describes the v1 prototype** (one-shot Jira crawl → static JSON → SPA + game).
> For the productized, continuously-synced, multi-tenant, time-series direction, see
> [`docs/v2-spec.md`](docs/v2-spec.md) (product/requirements, *what/why*) and
> [`docs/v2-architecture.md`](docs/v2-architecture.md) (technical architecture, *how*).

## Problem

Features span multiple pods. Each pod has its own backlog, sites span
5+ timezones, and SRE is engaged late. Leadership needs a way to (a) see the
real dependency network, (b) forecast feature completion probabilistically,
and (c) locate where calendar time is lost (queues, handoffs, timezone gaps).

## Requirements (EARS)

- R1: WHEN the app loads, THE SYSTEM SHALL render a pod dependency graph
  where nodes are pods (sized by WIP, colored by utilization proxy) and
  directed edges are Jira-mined blocking relationships (width ∝ count).
- R2: WHEN a user defines a feature as a set of tasks (pod, size, precedence),
  THE SYSTEM SHALL run a Monte Carlo simulation (≥5,000 trials) and report
  P50/P85/P95 completion in working days, a critical-path Gantt, and a
  per-pod criticality index.
- R3: WHEN a dependency edge crosses pods, THE SYSTEM SHALL add a handoff
  delay derived from the two pods' site timezone overlap.
- R4: WHEN a user adjusts a pod's utilization or capacity in what-if mode,
  THE SYSTEM SHALL rescale that pod's queue-wait component using Kingman's
  ρ/(1−ρ) factor and re-simulate, showing the delta.
- R5: WHEN the scoreboard view opens, THE SYSTEM SHALL show per-pod flow
  stats from mined data: throughput/week, cycle time P50/P85, WIP,
  utilization proxy, criticality across simulated features.
- R6: All simulation math SHALL live in a pure, dependency-free JS module
  (`app/js/sim.js`) covered by node:test unit tests.

## Model

- Task lead time ~ Lognormal(mu, sigma) fitted per pod from 180d of resolved
  Jira cycle times (created→resolved), scaled by task size factor
  (S=0.5, M=1, L=2, XL=4 of the pod's typical issue).
- Lead time decomposes as wait + touch with default flow efficiency 0.15
  (industry range 5–15%; configurable). What-if utilization ρ rescales the
  wait part by [ρ/(1−ρ)] / [ρ0/(1−ρ0)], capped at ρ=0.97.
- Baseline ρ0 per pod = min(0.92, WIP / (devs × 2)) — heuristic: ~2
  concurrently healthy items per dev; documented as a tunable assumption.
- Handoff delay per cross-pod dependency edge = roundtrips × latency(overlap):
  overlap ≥ 4h → 0.25d; ≥ 2h → 0.5d; > 0h → 1.0d; 0h → 1.5d. Default
  roundtrips = 3. Same-pod edges: 0.
- Completion = PERT network: start(t) = max(finish(deps)) + handoff;
  finish = start + duration. Monte Carlo N trials → percentiles,
  criticality index (fraction of trials a task/pod lies on the critical path),
  sensitivity (per-pod: mean completion with that pod accelerated 25% vs base).

## Data pipeline (v1, removed — superseded by the Postgres-backed Jira import)

The v1 pipeline was a background Jira crawl producing `data/*.jsonl`, aggregated
into `data/edges.json` and `data/pod_stats.json`, plus `scripts/build_pods.py`
turning the pod CSV into `pods.json` (name, area, location, devCount, tz offset,
site overlap hours matrix).

Its output was never wired into the running app (see `docs/v2-architecture.md`):
the demo/seed dataset lives in `server/db/seed/baseline.sql`, and a real org's
data comes from the in-app **Import from Jira** flow, which writes straight to
Postgres. The crawl and aggregation scripts were therefore deleted; `git log` has
them.

One capability has not yet been ported to Go: the site → UTC-offset table and the
overlap-hours matrix derived from it, which is why cross-site handoff latency in
the plan model is still approximated by same-site/different-site. `build_pods.py`
is kept until that port lands — see `specs/003-repo-layout-and-pipeline.md`.

## UI (single-page, vanilla JS + vendored d3, no build step)

Tabs: 1) Network — force-directed graph + pod detail panel.
2) Simulator — feature task editor, run button, completion histogram/CDF,
   Gantt of a representative P85 trial, criticality + tornado charts,
   what-if sliders per involved pod.
3) Scoreboard — sortable pod table with flow stats and red flags.

## Out of scope (prototype)

Auth, persistence of features beyond localStorage, live Jira sync, rework
loops (DSM feedback), Brooks-law penalties.

## Path to a shareable v2 (deferred by decision 2026-06-12 — personal tool for now)

- **Settings**: configurable Jira base URL + project(s) + custom-field ids
  (Assigned Pod = cf 10026 is an example — yours will differ), pod directory upload, site/
  timezone table, SRE pod list, model knobs (flow efficiency, WIP-per-dev cap,
  handoff roundtrips) — all currently hardcoded.
- **Incremental sync**: replace one-shot crawls with watermark-based pulls
  (`updated >= <last sync>` per project), scheduled (cron), with backfill jobs.
  Jira REST via API token or OAuth app; rate-limit aware.
- **Database**: SQLite/DuckDB for raw issues + changelogs; derived tables
  (edges, pod stats, hygiene, epic snapshots) rebuilt per sync; app reads a
  small API layer instead of static JSON. Changelog ingestion unlocks
  time-in-status (measured flow efficiency), interrupt ratio, and CFDs.
- **Multi-user**: read-only share first; per-persona landing (guide already
  persona-aware); audit note on every leadership-visible metric ("system
  diagnosis, not individual performance").

