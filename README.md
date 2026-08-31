# Conway

**Org-flow modeling and planning, built on one idea:** organizations ship their
communication structure — so make that structure visible, price what it costs
you, and rehearse changing it before you change it for real.

Conway mines your issue tracker for the *actual* cross-team dependency network
(not the org chart), plans a period of work through a finite-capacity execution
scheduler that says which initiatives will fit and which will slip and **why**,
and lets you rehearse org changes — including the inverse Conway manoeuvre the
name promises — in a multi-team game before you commit them.

It exists because the most expensive delays in software are not caused by slow
coding. They are caused by waiting: a handoff across a time-zone seam, a pod at
ρ 1.1, a dependency nobody priced, WIP spread across five things at once. Those
costs are invisible in every planning spreadsheet. Conway makes them the first
thing you see.

## Three lenses — Observe · Plan · Train

One pod map and one flow engine, seen three ways:

### Observe — what is actually happening

Mined from Jira (OAuth or API token): the cross-pod dependency network, WIP and
queue time per pod, data-hygiene gaps that starve the model, Monte-Carlo
feature forecasts (P50/P85), and a CCPM buffer fever chart. Dated **snapshots**
can be captured, **compared** over time, and **published** so facilitators can
build games from them. See
[docs/snapshots-and-scenarios.md](docs/snapshots-and-scenarios.md).

### Plan — what should happen next

Upload a period's plan — a teams roster and an initiatives matrix — and the
execution-order scheduler answers the question every plan dodges: *what fits,
what slips, and what is the binding constraint?*

- **An execution order with reasons, not just dates.** Six dispatch rules
  compete on a portfolio objective; every start week carries its binding
  constraint (pod capacity, a dependency, a WIP limit, a date lock, a freeze
  window), and a fit line states plainly how much of the plan the period can
  absorb.
- **Drag-to-edit.** Grab a bar on the timeline: moving it pins the start,
  stretching an edge changes the estimate — and the edit re-runs the whole
  engine, so every view, the Order table, the heatmap and the health report
  agree by construction. One-level undo included.
- **Baselines.** Freeze the agreed order — schedule, roster, parameters — into
  a named, immutable baseline. Compare any two later; the chip tells you when
  the plan's inputs have drifted from the agreement.
- **Per-pod capacity loss.** An ops-heavy pod and a greenfield pod do not lose
  the same fraction of their tracks; each pod can override the plan's global
  figure.
- **Site timezone overlap.** Cross-site handoff cost is computed from the real
  working-hours overlap between two sites' IANA timezones on the modelled date
  — daylight saving included — instead of same-site-or-not string matching.
- **A health report.** One printable card: how many initiatives will not
  finish inside the period, which pods are the constraint, which date-locked
  initiatives contend, and the remedies the engine priced — each with its cost
  in moved initiatives.
- **What-if levers** priced before/after: add capacity, descope, defer, reduce
  WIP, un-pair — with the victims named.

### Train — rehearse it

A multi-team learning game teaching the same levers. Seed a game from a
difficulty preset, a published org snapshot, or an editable **scenario
template** (download a network as JSON, edit it, re-upload, share it), and play
the round: the levers you learned in Plan, under time pressure, with
consequences.

## How to read the Plan

- **Capacity = parallel work-tracks**, not headcount: a pairing pod runs
  ~ceil(devs/2) tracks. Effective capacity = `tracks × horizon × (1 − loss)`
  (defaults: 26-week half-year, 10% loss for PTO/attrition/ramp, overridable
  per pod).
- **Utilization ρ = demand ÷ capacity** is the primary, defensible signal —
  which pods will choke (red ≥ 1, amber ≥ .85).
- **Lead time is directional, not a forecast.** A busy pod makes work wait —
  it says *much-worse vs much-better*, not a date. Real software delay is
  dominated by waiting and variability, which this captures only directionally.
- **Every edit goes through the engine.** There is no client-side geometry: a
  drag persists a pin or an estimate change and the server recomputes, so the
  chart can never show a schedule the constraints refuse.

Upload formats — teams: a pod-directory CSV/XLSX (pod name, Developers,
location, optional `pairs`/`tracks`/`Capacity Loss %`); initiatives: the
FullKit matrix (paired `<Team> Sequence` dependency + `<Team>` weeks columns).
A demo plan seeds one on first boot.

## Run

Prebuilt images are published to `ghcr.io/guidewire-oss/conway`. A merge to
`main` publishes `edge` and `sha-<commit>`; a `v*` tag publishes the semver
tags and `latest`. `edge` is the tip of development and `latest` is the newest
release — pick accordingly. `docker compose` builds from source rather than
pulling, so local development needs no registry access.

Postgres is required — Conway has no local-file fallback. Fastest way to try
it:

```
docker compose up --build
# open http://localhost:8741 — the admin password is printed to
# `docker compose logs conway` unless you set CONWAY_ADMIN_PASSWORD.
# Seeds a small synthetic demo org on first boot (CONWAY_SEED_BASELINE=true).
```

**Local Go development** (against a Postgres you already have running, e.g.
`docker compose up -d postgres` — the compose Postgres publishes 5432 on
loopback for exactly this. If that port is taken, `CONWAY_PG_PORT=5433` moves
both the published port and the `DATABASE_URL` that `make server` uses):

```
CONWAY_ADMIN_PASSWORD=letmein make server   # DATABASE_URL defaults to the compose Postgres
# sign in as admin -> ⚙ Admin -> mint one account per team (auto-expiring, default 48h)
```

`CONWAY_ADMIN_PASSWORD` sets the admin password on **every** boot, so it
doubles as the reset: set it and restart. Leave it unset and the existing
password is kept; on a first boot with nothing set, one is generated and
printed to the log once.

`make server` is a **restart**: it stops whatever is running (including
anything else holding the port), rebuilds, and then waits for the server to
actually answer before reporting success. `app/` is served from disk, so a
checkout updates the page immediately, while routes are compiled into the
binary — a dev loop that says "started" when the old binary is still bound
would give you a page calling endpoints the server does not have.

For your own org's data, use **Import from Jira** in the app. Sign-in gates
everything: the admin panel creates/extends/revokes expiring team accounts and
shows a live board.

## Authentication

By default an **admin** manages accounts and shares credentials manually
(passwords are PBKDF2-hashed; sessions are short-lived HMAC-signed tokens).

For org sign-in, Conway supports **SSO via OpenID Connect** (Okta, Google,
Entra ID, Auth0). Staff roles (admin / facilitator / manager) are derived from
a group claim, and accounts are provisioned just-in-time on first login. The
built-in admin password stays as a break-glass fallback; teams still join games
by code. See [docs/sso-oidc.md](docs/sso-oidc.md) for setup and configuration.

## Project layout

```
app/            the frontend — vanilla JS + d3 (vendored), no build step
server/         the Go server: the planning engine, Jira mining, auth, the game
  planning/     the execution-order scheduler, capacity model, site overlap —
                pure computation, no I/O, table-driven tests
  db/           Postgres access + embedded goose migrations (self-migrating)
specs/          the decision records: every feature's WHAT and WHY, numbered
docs/           guides: snapshots & scenarios, SSO, the factory rulebook
tests/          engine and view specs (node --test)
scripts/        the software factory's gates (see Contributing)
```

The `specs/` directory is the project's memory: each feature's spec records the
problem, the decisions taken and the alternatives rejected, and stays alive as
the code evolves. Start with
[specs/001-plan-execution-order.md](specs/001-plan-execution-order.md).

## Status — v0.1.0

This is the first public release. The three lenses are real and used; the
rough edges are known and recorded. Expect: the planning engine and its views
to work end to end on imported orgs; the Jira mining to want a well-kept board
(it degrades honestly and tells you what it could not use); and the game to be
played with a facilitator. See the
[issue tracker](https://github.com/guidewire-oss/conway/issues) for what is
next.

## Contributing — the gates

This repo is governed by the
[software factory](https://github.com/anoop2811/software-factory-template):
the rules below are shell hooks and CI checks that reject a bad commit or push,
not conventions to remember. `docs/FACTORY_RULES.md` is the full rulebook and
`AGENTS.md` is the contract every coding agent reads; project-specific values
(test patterns, docs root, the check command) live in `factory.yaml`.

```
./factory doctor    # which gates are armed vs inert, then prove each one fires
make check          # the checks CI runs
./factory report    # what the gates have blocked
```

Arm the local push gate once per clone — it is not on by default:

```
git config core.hooksPath .githooks
```

`make preflight` runs the strict internal-paths guard plus the factory gate
suite, locally, before you push. It is not identical to CI: the internal-paths
guard is local-only by design (its pattern list lives in `.git/info/exclude`,
which is never pushed), and CI additionally runs lint, security scanners and
the image build. `make test` runs the guard in its lenient mode, so an ignore
file being replaced is caught on the next test run.

What will block you:

- **Commit messages** are conventional (`feat|fix|chore|docs|refactor|test|ci|build|perf: subject`, no trailing period), with a body of at most 6 bullets
  of 25 words or fewer. Any claim of "verified" or "fixed" must cite the
  command run and its output; if you did not run it, write "written but NOT
  verified".
- **Direct pushes to `main`** are rejected. Push a branch and open a PR.
- **Specs** live in `specs/`, named `NNN-name.md`, following
  [specs/SPEC_TEMPLATE.md](specs/SPEC_TEMPLATE.md). A feature's spec lands
  before or with the feature.
- **Internal data stays out.** `scripts/hooks/internal-paths-ignored.sh` fails
  if any local-only internal path stops being git-ignored; `--strict` also
  requires a clean tree. Its patterns live in `.git/info/exclude` — per-clone
  and never pushed, so re-create that block after a fresh clone before copying
  working files in. It runs locally only, for that reason: a CI checkout has no
  list and no internal files.

Two gates are deliberately not armed, both recorded in
[specs/002-factory-adoption.md](specs/002-factory-adoption.md): the Go pack's
Ginkgo dialect check (the planning suite has since adopted Ginkgo; the rest of
the Go tests are stdlib `testing`) and citation linting (specs are cited by
name, not yet by `file:line`).

## Views

- **Flow Actions** — the constraint (five focusing steps), WIP freeze
  candidates with per-issue drill-down, CCPM buffer fever chart
- **Network** — layered DAG of mined blocking edges + org-merge simulation
  (headcount transfer lever, Org Flow Index)
- **Feature Simulator** — epic import, full-kit check, P50/P85 forecasts,
  history-based dependency suggestions
- **Flow Scoreboard** / **Data Hygiene** — per-pod flow stats and the data
  gaps starving the model, drillable to individual Jiras
- **✦ Guide** — persona playbooks (exec / lead / PM) with live insights

See `SPEC.md` for the model and the path to a shareable v2.

## License

[MIT](LICENSE) — Copyright © 2026 Guidewire Software, LLC.
