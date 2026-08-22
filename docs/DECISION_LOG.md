# Decision Log

Architectural and governance decisions for Conway, newest last. A commit that
touches a governance-sensitive path (`specs/`, `scripts/`, `Makefile`,
`factory.yaml`, `.github/workflows/`, the harness adapters) must reference a
Decision here by number — `decision-log-gate.sh` enforces it.

Each entry records the context, the decision, what was rejected and why, and what
the decision costs. A decision with no stated cost usually means the cost was not
found yet. Entries are append-only: supersede by adding a new Decision that says
which one it replaces, rather than editing history.

Feature-level design decisions live in the relevant spec's Decision Record
section (`specs/NNN-*.md` §11). This log is for decisions about the repository
itself and its governance.

---

## Decision 1

**Specs live in this repository, in `specs/`, following the ai-craft template.**

*Context:* Conway had `SPEC.md`, `GAME-SPEC.md` and `docs/v2-spec.md` with no
convention for new feature specs, and the surrounding workspace convention points
at a sibling specs repository that does not apply here.

*Decision:* New feature specs go in `specs/NNN-kebab-case-name.md`, following
`specs/SPEC_TEMPLATE.md` (a local copy of the ai-craft template). Requirements
state what and why; algorithms and alternatives go in the spec's own Decision
Record. The three pre-existing spec documents stay where they are.

*Rejected:* A sibling specs repository — the workspace-level convention, which
would split Conway's specs from its code for no benefit on a single-repo project.
The factory's shorter `specs/TEMPLATE.md`, which arrived later with the install
and was removed to avoid two templates drifting.

*Cost:* Two spec formats exist in the repo's history; only new specs follow this
one.

## Decision 2

**Ignore rules are split by audience.**

*Context:* `.gitignore` mixed artifacts every clone produces with files that exist
only on one maintainer's machine. Conway is a repository slated for open-source release, holding internal
planning data locally, and `factory-init` replaces `.gitignore` wholesale — so
that mixture made an installer a disclosure risk. Listing internal filenames in a
public file was itself a disclosure.

*Decision:* Apply one test — would a fresh clone by an outside contributor ever
produce this path? If yes it stays in `.gitignore` (build output, `data/`
pipeline output, `pod-directory*.csv`, which stops the *next* adopter committing
their own org's roster). If no, it moves to `.git/info/exclude`, which no
installer reads or writes.

*Rejected:* Leaving everything in `.gitignore`, which keeps both problems. Moving
`data/*` and `pod-directory*.csv` as well, which would remove protection future
adopters need. A global `core.excludesFile`, which would leak these rules into
every repository on the machine.

*Cost:* `.git/info/exclude` is per-clone and never pushed or backed up. A fresh
clone has no protection and nothing says so — which is why Decision 3 exists.

## Decision 3

**An internal-path guard, reading its list from `.git/info/exclude`.**

*Context:* The `.gitignore` overwrite of 2026-08-17 was caught by inspection.
Inspection does not scale, and publishing internal data from a repository headed for release
is irreversible.

*Decision:* `scripts/hooks/internal-paths-ignored.sh` asserts, via
`git check-ignore`, that every internal path is still ignored — indifferent to
which file provides the ignore. It reads its patterns from the `conway-internal`
block in `.git/info/exclude`, so the sensitive names are never committed, and it
fails rather than skips when that block is missing. An untracked-files backstop
closes the self-reference hole: deleting a pattern would otherwise delete its own
assertion. Registered in `factory.yaml` `local_hooks` so upgrades preserve it.

*Rejected:* A checklist item in `AGENTS.md` — the same class of protection that
just failed. Hard-coding the paths in the guard, which would republish exactly
the names Decision 2 moved out of the public `.gitignore`.

*Cost:* The guard is inert on a clone whose exclude block was never re-created,
which is the condition it exists to report — hence failing closed on a missing
file.

## Decision 4

**The Go module root is the repository root; packages stay under `server/`.**

*Context:* The Go pack's check command and CI run `go test -race ./...`,
`gosec ./...` and `govulncheck ./...` at the repository root with no working
directory. Conway's module was in `server/`, so every one of those steps failed.

*Decision:* Move `go.mod`, `go.sum` and `vendor/` to the repository root, keeping
the Go packages under `server/`. Internal imports become `conway/server/*`.

*Rejected:* Wrapping each Go step in a subshell that enters `server/` — a
per-step cost forever, and silently wrong for any step a future upgrade adds.
Moving the Go packages themselves to the root, which would scatter them among
`app/`, `data/`, `docs/` and `specs/`.

*Cost:* A one-time import rewrite across 19 files. The Dockerfile needed new copy
paths and a `./server` build target, and that edit is **not** verified — no
Docker daemon was available.

## Decision 5

**Runtime output goes in one directory, `var/`.**

*Context:* Runtime files were scattered across `server/store.json`,
`server/game-state.json`, `server/conway`, `conway-server` and `.run/`, needing
four ignore rules.

*Decision:* Everything written at runtime — binary, pid, log, local JSON store,
game state — goes under `var/`. `CONWAY_STORE` defaults to `./var/store.json`,
and the game state is derived beside it. One ignore rule; `make clean` removes
the directory.

*Cost:* The store's parent directory is no longer guaranteed to exist, so the
server now creates it. Nothing in the codebase called `MkdirAll` before, so this
was a latent failure for the container's `/data/store.json` too.

## Decision 6

**The v1 Python mining pipeline is deleted, not ported.**

*Context:* Fifteen Python scripts implemented a Jira crawl superseded by the
in-app Go import. `SPEC.md` already recorded the pipeline as superseded and its
output as unwired, and nothing referenced the scripts — not the Makefile,
Dockerfile, compose file, frontend or Go.

*Decision:* Delete 14 of them. Keep `scripts/build_pods.py` until its site
timezone table is ported to Go, which is the one capability the Go path lacks;
that port is specified in `specs/003-site-timezone-overlap.md`.

*Rejected:* Porting the pipeline to Go behind an API, which would reimplement the
existing Import from Jira flow.

*Cost:* Until the port lands, cross-site handoff latency stays on the
same-site/different-site approximation, and one Python file remains.

## Decision 7

**Adopt the software factory by preparing the repository, not by forking it.**

*Context:* The first install attempt replaced `.gitignore`, `Makefile` and
`README.md` wholesale, deleted `CLAUDE.md` without a backup, and armed language
gates this codebase cannot pass. `factory upgrade` never overwrites those files;
only `factory-init` does, making the damage a one-time cost.

*Decision:* Change Conway so the installer's behaviour is harmless — instructions
in `AGENTS.md` with `CLAUDE.md` as a pre-existing symlink (so `sync-claude.sh`
skips its unbacked `rm`), Makefile targets in `Makefile.conway` behind an
`include`, ignore rules split per Decision 2, and the guard from Decision 3.
Install on a branch with `--pack go` only, then re-merge from the backups. Full
analysis in `specs/002-factory-adoption.md`.

*Rejected:* Forking or vendoring the template, which inherits maintenance of an
actively developed project to fix one-time install costs. Patching
`factory-init` locally, which the next machine or adopter would not have.

*Cost:* Conway carries a little structure it would not otherwise need — a
separate Makefile include, a marked exclude block. Each is independently
defensible.

## Decision 8

**Two factory gates are deliberately left unarmed.**

*Context:* The Go pack's dialect gate requires Ginkgo/Gomega; Conway has 19 test
files using stdlib `testing` and none using Ginkgo. Citation linting resolves
`SPEC_*.md:NN` references, but Conway's specs are named `NNN-name.md`, so no
citation could ever resolve.

*Decision:* Keep `ginkgo-only-check.sh` out of `check_command` and CI, and set
`citation_prefix` empty. Both are recorded rather than hidden: a permanently
failing gate trains everyone to ignore gate failures, and a gate that can only
pass vacuously is worse than an absent one. Converting the test suite is a
separate decision, open as `specs/002-factory-adoption.md` Q2.

*Rejected:* Converting 19 test files during a governance install, which couples
two changes that deserve separate review. Leaving either gate armed and red.

*Cost:* Conway runs the Go pack without its blessed test stack, which is most of
what makes that pack "battle-tested". `factory doctor` still reports the dialect
gate as armed, because it fails when a selected pack's hook is absent — so the
hook stays on disk, unwired, and doctor's report is misleading on that one line.

## Decision 9

**A gate that cannot run its check must fail, never pass.**

*Context:* `ginkgo-only-check.sh` built its file list with
`rg --files` inside a process substitution. `set -e` cannot observe a failure
there, so on any machine without ripgrep the loop iterated zero times and the
gate printed success. `factory doctor` listed it as armed and the install
attestation counted it as proven.

*Decision:* Remove the ripgrep dependency — enumerate test files with
`git ls-files`, match with `grep -E`, and spell out word boundaries rather than
using GNU `\b`, which BSD grep does not reliably support. Verified by running the
old and new scripts with `rg` removed from `PATH`: old exits 0 claiming success,
new exits 1 reporting 19 violations either way.

*Rejected:* Checking `command -v rg` and erroring — better than failing open, but
it keeps a dependency no gate needs.

*Cost:* The fix lives in a pack file that `factory upgrade` overwrites, so it is
also applied to the local template clone and should go upstream to
`software-factory-template` to be durable.

## Decision 10

**All three harness configurations are tracked, because all three are in use.**

*Context:* Which coding harness runs varies by machine and by person on this
project — Claude Code, Codex and opencode are all in play. The factory generates
adapters for each from the `opencode.json` canon. After the install, `.opencode/`
had 8 files tracked and `.codex/` 6, while `.claude/` had none: Conway's original
`.gitignore` excluded it, from before any of this existed.

*Decision:* Track `.claude/` alongside the other two. Ignore only genuinely local
state (`settings.local.json`, `scheduled_tasks.lock`).

*Rejected:* Leaving `.claude/` ignored and disarming `make check-drift` instead.
That check diffs `.claude/settings.json` and `.claude/agents/`; with those files
ignored it compared nothing and passed vacuously — the same fail-open shape as
Decision 9, and worth removing for the same reason.

*Consequences:* Two real gaps close. A teammate cloning on a Claude Code machine
now gets the generated roles and, more importantly, the `PreToolUse` hook in
`.claude/settings.json` that wires `test-edit-denial.sh` — without it that
harness had no enforcement at all. And the drift check now compares something,
so it can fail.

*Cost:* Harness configuration is now public. It contains permissions and hook
wiring, no credentials — reviewed before tracking.

*Still open:* Enforcement has only been observed on one harness. See
`wiki/opencode-harness.md`; the parity claim stays OPEN per harness until someone
runs the eval on the one in front of them.

## Decision 11

**The container image is built on every pull request and published to GHCR.**

*Context:* Nothing in the repository built the image. `docker compose up --build`
builds locally, and `deploy/build-push.sh` pushes to AWS ECR from a maintainer's
machine with AWS credentials — and `deploy/` is git-ignored, so that script is not
even part of the repo. The consequence showed up during the factory adoption: the
Dockerfile was edited three times (copy paths for the module move, a pinned Go
patch) with nothing verifying it built.

*Decision:* A separate `.github/workflows/image.yml` builds on every pull request
without pushing, and publishes `ghcr.io/guidewire-oss/conway` on `main` and on
`v*` tags. Authentication uses the built-in `GITHUB_TOKEN` with `packages: write`
— no PAT, no secret to rotate. Tags are the branch name, a full-SHA tag for exact
traceability, `latest` on the default branch only, and bare semver on version
tags.

*Rejected:* Adding the steps to the factory's `go-pack.yml`, which is a pack file
the factory owns and refreshes. Publishing on every branch, which fills the
registry with images nobody deploys. Pushing from pull requests, which cannot work
from forks and would let an unmerged branch publish.

*Consequences:* The Dockerfile is now covered by CI, which is what closes the
verification gap above. The image is single-architecture: the Dockerfile hardcodes
`GOARCH=amd64`, so a multi-arch build needs `TARGETARCH` plumbing first.

*Amended 2026-08-17, tagging:* the first version pinned `latest` to `main`. That
is against the convention `docker/metadata-action` documents — `edge` means the
last commit of the default branch, `latest` means the newest release — and would
have made `latest` mean "tip of development" to anyone pulling it. Main now
publishes `edge` plus a full-SHA tag; `latest` arrives with the semver tags on a
`v*` push, via the action's default `latest=auto` flavour.

*Surveyed for precedent:* `guidewire-oss/fern-platform` builds images in CI on
pull requests and main but publishes only from a tag-triggered `release.yml`,
which is the stricter end of the same idea. Conway keeps an `edge` image from main
because it has no releases yet and a deployable tip is useful; moving to
publish-on-tag-only later is a one-line change to the trigger.

*Not settled:* ECR remains the deploy path, so there are now two registries with
different purposes — GHCR for consumption, ECR for internal deploys. Whether
`deploy/` should pull from GHCR instead of building locally is a separate
decision. The GHCR package inherits the repository's visibility, which is private
today.

## Decision 12

**New Go behavioural tests are Ginkgo/Gomega; the existing 19 stdlib files stay,
and the dialect gate stays disarmed until they are converted.**

*Context:* Decision 4 of `specs/002-factory-adoption.md` disarmed the Go pack's
`ginkgo-only-check.sh` at install time, because Conway's 19 test files use the
standard `testing` package and an armed gate the whole suite violates trains
everyone to ignore gate failures. That left Q2 open: disarmed permanently, or
converted later? Writing the execution-order scheduler forced the question,
because it needed new tests either way.

*Decision:* Behavioural tests written from now on are Ginkgo specs under a single
`RunSpecs` bootstrap per package — `server/planning/planning_suite_test.go` and
`server/server_suite_test.go` are the first two. `github.com/onsi/ginkgo/v2`
v2.32.1 and `github.com/onsi/gomega` v1.42.1 are vendored. The pre-existing stdlib
tests are left alone, and `ginkgo-only-check.sh` stays out of the gate chain while
they remain, since arming it would fail on the 23 files this decision does not
convert (counted the way the gate counts on 2026-08-19, the two new bootstraps
excluded; spec 002 Decision 4's "19" was accurate when written).
This partly answers spec 002 Q2; whether those 19 files are ever converted, and by
whom, is still open.

*Rejected:* Converting all 19 files in the same change — it turns a feature PR into
a test-framework migration, which is the coupling Decision 4 rejected for the
install and which is no more reviewable here. Staying on stdlib — it adopts the Go
pack while permanently declining its central opinion, and the choice was the
user's to make, not the scheduler's to assume.

*Cost:* Two dialects in `server/` and `server/planning/` until the conversion
happens, so a reader has to recognise both. Vendoring onsi adds 750 files and
about 462k lines under `vendor/`, and pulled `golang.org/x/sys` from v0.43.0 to
v0.46.0 as a transitive upgrade. `revive`'s `dot-imports` rule is relaxed for
`_test.go` only, because dot-importing Ginkgo and Gomega is how their DSL is meant
to read; the rule stays armed for production code.


## Decision 13

**`wait-for-http.sh` waits on a wall-clock deadline, not an attempt count.**

*Context:* The script's contract is "a `make server` success line means serving".
Its first version looped `i < timeout` with `--max-time 2` per probe and a 1s
interprobe sleep, so a listener that accepts TCP but never answers (a database
waiting on a lock, a proxy holding the port) made each probe cost about 3s and
the nominal 30s wait ran about 90s — while the failure message still reported
"within 30s". A dev loop that lies about how long it waited is the same failure
class as a dev loop that reports an unlistening server as started, which is the
bug this script exists to prevent (spec 001 §11, trap 2 in the 2026-08-21
handoff).

*Decision:* Compute `deadline = start + timeout` once from `date +%s`, clamp
each curl `--max-time` and each sleep to the remaining budget, and break when the
deadline passes. The probe semantics are unchanged: curl's exit status decides,
not `%{http_code}` with a fallback (the `000`/`000000` concatenation trap that
the script's header documents).

*Rejected:* Keeping the iteration count and dividing timeout by probe cost —
probe cost is not constant (a fast refusal and a 2s hang differ by 100x), so any
fixed arithmetic still lies. A `--connect-timeout`/`--max-time` pair tuned per
case — two knobs for one deadline, and the connect case is already covered by
exit status.

*Cost:* One more variable and two arithmetic clamps in a 47-line script; the
`kill -0` liveness check per iteration is unchanged.

## Decision 14

**Slice latest-start anchors on the initiative's raw finish; slack can never
eat the buffer.**

*Context:* FR-041 defines a slice's latest start as "the last week the slice
can begin without moving its initiative's commit date", but the commit date
includes a flat buffer (spec 001 §11 D20), so the definition is ambiguous
about whether a slice may consume buffer weeks while "not moving the commit".
The timeline (Stories 8-9) and the pod sheet both render this number, so the
ambiguity had to be settled before the code.

*Decision:* The backward pass anchors terminal slices on `rawFinishWeek`, not
`commitWeek`. A slice that slips into the buffer's weeks moves the raw finish
and therefore the commit — the buffer protects the promise, it is not slack a
slice may spend. Consequences that fall out and are asserted by spec:
slack is never negative (a capacity-waited slice reads zero slack — "you can
wait no more"), and latest start never exceeds the raw finish.

*Rejected:* Anchoring on commitWeek — it would label buffer weeks as slack and
the pod sheet would tell leads they can start weeks they cannot; the buffer
would silently stop protecting anything. Per-slice buffers (SSQ-style, §10 Q5)
— rejected upstream by D20 and would not change the anchoring question.

*Cost:* One more pass over each initiative's slices (`annotateSliceSlack`,
a single reverse walk — the slices are already topologically sorted), and the
`WorkSlice` shape grows three fields (dependsOn, latestStartWeek, slackWeeks),
recorded in spec 001 §7 alongside.
