# Conway

Org-flow modeling: mines Jira for the real cross-pod dependency
network, forecasts feature completion with Monte Carlo over per-pod cycle-time
distributions, and simulates org changes — including the inverse Conway
manoeuvre the name promises.

Named for Conway's law: organizations ship their communication structure.
This tool makes that structure visible, prices it in days, and lets you
rehearse changing it before you change it for real.

## Three lenses — Observe · Plan · Train

One pod map and one flow engine, seen three ways:

- **Observe** — current state mined from Jira: the cross-pod dependency network,
  WIP, hygiene, and Monte-Carlo feature forecasts. A manager can **Import from
  Jira** to capture a dated **snapshot**, **compare** snapshots over time
  (now vs a past quarter), and **publish** a snapshot (public/private) so
  facilitators can build games from it. See
  [docs/snapshots-and-scenarios.md](docs/snapshots-and-scenarios.md).
- **Plan** — upload a period's plan (a teams roster + an initiatives matrix) and
  simulate it: a directed dependency network, per-pod utilization (ρ), the
  constraint pods, and **what-if levers** shown **before → after**.
  Manager-owned (admins see all). "Load demo plan" seeds a populated example.
- **Train** — the multi-team learning game that teaches the same levers. A
  facilitator seeds a game from a difficulty preset, a **public org snapshot**, or
  an editable **scenario template**. Templates are authored by downloading a
  network as JSON, editing it (pods = teams, edges, loads), and re-uploading — then
  named and optionally shared. See
  [docs/snapshots-and-scenarios.md](docs/snapshots-and-scenarios.md).

### How to read the Plan
- **Capacity = parallel work-tracks**, not headcount: a pairing pod runs
  ~ceil(devs/2) tracks. Effective capacity = `tracks × horizon × (1 − loss)`
  (defaults: 26-week half-year, 10% loss for PTO/attrition/ramp).
- **Utilization ρ = demand ÷ capacity** is the primary, defensible signal —
  which pods will choke (red ≥1, amber ≥.85).
- **Lead time is directional, not a forecast.** A busy pod makes work wait,
  `m(ρ)=1/(1−ρ)` (ρ .8 → 5×, .9 → 10×); an initiative's lead time is the critical
  path through its `dependency → team` chain. It says *much-worse vs much-better*,
  not a date — real software delay is dominated by waiting and variability, which
  this captures only directionally.
- **Levers** mutate the inputs and recompute: add capacity / un-pair (ρ↓),
  descope / defer (demand↓), reduce WIP (recovered multitasking waste).

Upload formats — teams: a pod-directory CSV/XLSX (pod name, Developers, location,
optional `pairs`/`tracks`); initiatives: the FullKit matrix (paired
`<Team> Sequence` dependency + `<Team>` weeks columns).

## Run

Postgres is required — Conway has no local-file fallback. Fastest way to try it:

```
docker compose up --build
# open http://localhost:8741 — admin password is printed to
# `docker compose logs conway` unless you set CONWAY_ADMIN_PASSWORD.
# Seeds a small synthetic demo org on first boot (CONWAY_SEED_BASELINE=true).
```

**Local Go development** (against a Postgres you already have running, e.g.
`docker compose up -d postgres`):
```
make server   # DATABASE_URL defaults to the docker-compose Postgres; see `make help`
# sign in as admin -> ⚙ Admin -> mint one account per team (auto-expiring, default 48h)
```

Sign in gates everything: the admin panel creates/extends/revokes expiring
team accounts and shows a live board. Frontend has no external deps (vendored
d3); the Go server vendors pgx + goose, self-migrating on boot. The demo
dataset lives in `server/db/seed/baseline.sql` (applied when
`CONWAY_SEED_BASELINE` is unset or `true`; set to `false` for an empty
first-run and import your own org from Jira instead).

For your own real org's data, either use **Import from Jira** in the app
(OAuth or an API token), or see `data/`/`scripts/` for the legacy offline
mining pipeline (a one-shot Jira crawl → JSON, pre-dating the Postgres-backed
import — kept for reference, not the primary path).

Tests:
```
node --test tests/sim.test.mjs   # engine (JS)
cd server && go test ./...       # Go
```

## Authentication

By default an **admin** manages accounts and shares credentials manually
(passwords are PBKDF2-hashed; sessions are short-lived HMAC-signed tokens).

For org sign-in, Conway supports **SSO via OpenID Connect** (Okta, Google,
Entra ID, Auth0). Staff roles (admin / facilitator / manager) are derived from
a group claim, and accounts are provisioned just-in-time on first login. The
built-in admin password stays as a break-glass fallback; teams still join games
by code. See [docs/sso-oidc.md](docs/sso-oidc.md) for setup and configuration.

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

MIT — see [`LICENSE`](LICENSE).
