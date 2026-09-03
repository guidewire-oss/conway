# Usage Analytics

**Status:** In Progress (page implemented; deeper slicing later)
**Author(s):** opencode (implementer), Anoop (product owner)
**Date:** 2026-09-01

## 1. Overview

A dedicated analytics page for admins: durable business events, aggregated
KPIs (not high-cardinality request logs), the plan-setup adoption funnel, and
per-user activity — sliceable by range, to answer "is the system being used,
where do people drop off, who are the power users."

## 2. Framework

Google's HEART (Happiness, Engagement, Adoption, Retention, Task success) via
the Goals-Signals-Metrics model (see ProductPlan's glossary; Rodden, Google).
Conway's read:

| HEART | Question | Signal → Metric |
|---|---|---|
| Adoption | Do people complete the plan-setup journey? | Funnel: plan_created → teams_uploaded → initiatives_uploaded → schedules_computed → baselines_saved, distinct users per step |
| Engagement | Is it used, and by whom? | Daily actives (zero-filled), per-user events + distinct features |
| Retention | Do they come back? | This week's actives vs last week's |
| Task success | Is the core job done? | schedules_computed, baselines_saved, jira_import_done |
| Happiness | (future) | A per-user satisfaction survey — out of scope |

## 3. Implementation

- **Events**: append-only `analytics_events` Postgres table (migration 0019):
  ts, username, event, plan_id, meta jsonb. Written fire-and-forget at the
  same call sites as the metrics counters (spec 013's taxonomy extended) — an
  analytics failure must never fail the request it describes.
- **Aggregation**: `GET /api/admin/analytics?days=7|30|90` (admin role) —
  aggregates in Go from the range query: daily actives/events (zero-filled —
  gaps are signal), event counts, per-user table (events, distinct features,
  plans touched, last seen), funnel (distinct users per step), week-over-week
  actives. Anonymous events (login_failed with no user) count toward volume
  but never user activity.
- **Page**: 📊 Usage (admin nav) — KPI cards, a d3 line chart (daily actives +
  events, vendored d3, zero new frontend deps), funnel bars, per-user table.
  Range buttons 7/30/90d.

## 4. Decisions

**Decision 1: durable events in Postgres, aggregated in Go.** Counters alone
lose the time dimension the drop-off questions need; a time-series service is
a deployment this tool doesn't warrant. One small append-only table, Go
aggregation, no SQL analytics dialect to drift.

**Decision 2: funnel counts distinct users per step, not raw events.** One
user scheduling forty times is one conversion; the funnel is about people.

**Decision 3: the read side is an overlay page (📊 Usage, admin nav), not a
section of the Admin users panel** — per review.
