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

Prebuilt images are published to `ghcr.io/guidewire-oss/conway`. A merge to
`main` publishes `edge` and `sha-<commit>`; a `v*` tag publishes the semver tags
and `latest`. Pull requests build the image to check the Dockerfile but publish
nothing. `edge` is the tip of development and `latest` is the newest release —
pick accordingly. The package inherits this repository's visibility, and
`docker compose` builds from source rather than pulling, so local development
needs no registry access.

Postgres is required — Conway has no local-file fallback. Fastest way to try it:

```
docker compose up --build
# open http://localhost:8741 — admin password is printed to
# `docker compose logs conway` unless you set CONWAY_ADMIN_PASSWORD.
# Seeds a small synthetic demo org on first boot (CONWAY_SEED_BASELINE=true).
```

**Local Go development** (against a Postgres you already have running, e.g.
`docker compose up -d postgres` — the compose Postgres publishes 5432 on loopback
for exactly this. If that port is taken, `CONWAY_PG_PORT=5433` moves both the
published port and the `DATABASE_URL` that `make server` uses):
```
CONWAY_ADMIN_PASSWORD=letmein make server   # DATABASE_URL defaults to the compose Postgres
# sign in as admin -> ⚙ Admin -> mint one account per team (auto-expiring, default 48h)
```

`CONWAY_ADMIN_PASSWORD` sets the admin password on **every** boot, so it doubles
as the reset: set it and restart. Leave it unset and the existing password is kept;
on a first boot with nothing set, one is generated and printed to the log once.

`make server` is a **restart**: it stops whatever is running (including anything
else holding the port), rebuilds, and then waits for the server to actually answer
before reporting success. That last part matters more than it sounds — `app/` is
served from disk, so a checkout updates the page immediately, while routes are
compiled into the binary. A dev loop that says "started" when the old binary is
still bound to the port gives you a page calling endpoints the server does not
have, which arrives as a 405 with no obvious cause. If startup blocks (usually an
unreachable `DATABASE_URL` — the server dials Postgres before it binds), you get
the log tail and a non-zero exit instead of a success line.

Sign in gates everything: the admin panel creates/extends/revokes expiring
team accounts and shows a live board. Frontend has no external deps (vendored
d3); the Go server vendors pgx + goose, self-migrating on boot. The demo
dataset lives in `server/db/seed/baseline.sql` (applied when
`CONWAY_SEED_BASELINE` is unset or `true`; set to `false` for an empty
first-run and import your own org from Jira instead).

For your own real org's data, use **Import from Jira** in the app (OAuth or an
API token). The legacy offline mining pipeline it replaced — a one-shot Jira
crawl to JSON, predating the Postgres-backed import — was removed in favour of
that flow; `git log` has it if you ever want to look.

Tests:
```
node --test tests/sim.test.mjs   # engine (JS)
go test ./...                    # Go (module root is the repo root)
make test                        # both of the above
```

## Authentication

By default an **admin** manages accounts and shares credentials manually
(passwords are PBKDF2-hashed; sessions are short-lived HMAC-signed tokens).

For org sign-in, Conway supports **SSO via OpenID Connect** (Okta, Google,
Entra ID, Auth0). Staff roles (admin / facilitator / manager) are derived from
a group claim, and accounts are provisioned just-in-time on first login. The
built-in admin password stays as a break-glass fallback; teams still join games
by code. See [docs/sso-oidc.md](docs/sso-oidc.md) for setup and configuration.

## Contributing — the gates

This repo is governed by the [software factory](https://github.com/anoop2811/software-factory-template):
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

`make preflight` runs the strict internal-paths guard plus the factory gate suite,
locally, before you push. It is not identical to CI: the internal-paths guard is
local-only by design (its pattern list lives in `.git/info/exclude`, which is
never pushed), and CI additionally runs lint, security scanners and the image
build. `make test` runs the guard in its lenient mode, so an ignore file being
replaced is caught on the next test run.

What will block you:

- **Commit messages** are conventional (`feat|fix|chore|docs|refactor|test|ci|build|perf: subject`, no trailing period), with a body of at most 6 bullets of 25 words or fewer. Any claim of "verified" or "fixed" must cite the command run and its output; if you did not run it, write "written but NOT verified".
- **Direct pushes to `main`** are rejected. Push a branch and open a PR.
- **Specs** live in `specs/`, named `NNN-name.md`, following `specs/SPEC_TEMPLATE.md`.
- **Internal data stays out.** `scripts/hooks/internal-paths-ignored.sh` fails if any local-only internal path stops being git-ignored; `--strict` also requires a clean tree. Its patterns live in `.git/info/exclude` — per-clone and never pushed, so re-create that block after a fresh clone before copying working files in. It runs locally only, for that reason: a CI checkout has no list and no internal files.

Two gates are deliberately not armed, both recorded in
[specs/002-factory-adoption.md](specs/002-factory-adoption.md): the Go pack's
Ginkgo dialect check (this suite uses stdlib `testing`) and citation linting
(nothing cites specs by `file:line` yet).

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
