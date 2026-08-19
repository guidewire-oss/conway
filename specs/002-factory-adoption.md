# Software Factory Adoption

**Status:** Draft
**Author(s):** Anoop Gopalakrishnan (with Claude)
**Date:** 2026-08-17
**Story/Ticket:** _TBD_
**Sprint/Cycle:** —

---

## 1. Overview

Installing the software factory into Conway is destructive today: an attempt on
2026-08-17 replaced `.gitignore` and `Makefile` wholesale, deleted `CLAUDE.md`,
and armed language gates that cannot pass against this codebase. The install was
reverted. The repository is private today but headed for open-source release,
which is what makes the ignore-file damage the most severe of the four.

This spec says what has to change in Conway — and what has to be understood
about the installer — for a second attempt to be safe and to leave every armed
gate genuinely working. It is preparation work, not a product feature: no user
sees its output, and its success criterion is that `./factory doctor` reports a
healthy install over a repository that still behaves exactly as it did before.

Analysis is against template **v0.1.5**, the pinned release `install.sh` fetches.

---

## 2. Problem

The install of 2026-08-17 caused four failures, in descending order of severity.

**Internal data became committable.** `factory-init` copies its own `.gitignore`
over the adopter's. Conway's excluded `data/*.jsonl` (mined Jira), the planning
workbook, the pod-directory CSV, `deploy/` (internal hostnames), `.envrc.local`
and `pd.txt` (secrets). After the install, all of these showed as untracked, and
a single `git add -A` would have committed them. A backup was written, so this
was recoverable — but only by someone who noticed.

The remote (`guidewire-oss/conway`) is **private today**, so nothing was
disclosed. It carries an MIT licence, an `OSS_READINESS_PLAN.md` and a
release-oriented README, so it is headed for public release — at which point
anything already committed goes with it, including through history. That is why
this is treated as the most severe item: the window between committing internal
data and noticing is unbounded, and the fix is only cheap before the repo opens.

This one is now largely defused ahead of the install: the ignore rules have been
split by audience (Decision 6), so the local-only internal paths live in
`.git/info/exclude`, which no installer reads or writes. What remains in
`.gitignore` is build artifacts and pipeline output — still worth restoring after
an overwrite, but no longer a disclosure risk.

**Agent instructions were deleted, not backed up.** `scripts/sync-claude.sh`
runs `rm -f CLAUDE.md` and symlinks it to `AGENTS.md`. `CLAUDE.md` is absent
from the installer's backup list, so its contents are simply gone. This is not
init-only: it runs on every `make sync-harnesses`.

**The project's task interface disappeared.** `Makefile` was replaced with the
factory's, so `make server`, `build`, `test`, `stop`, `status`, `logs` and
`clean` — the interface the README documents and a fresh clone depends on —
stopped existing. Backed up, again recoverable only if noticed.

**The armed gates could not pass, and one failed open.** Conway's Go module is
in `server/`, not the repository root, so the Go pack's `check_command` and CI
steps (`go test -race ./...`, `gosec ./...`, `govulncheck ./...`) all run where
there is no `go.mod`. The pack's dialect gate requires Ginkgo/Gomega; Conway has
19 test files using stdlib `testing` and zero using Ginkgo. That gate reported
success anyway, because it shells out to `rg`, ripgrep is not installed here, and
the script treats "command not found" as "no violations":

```
./scripts/hooks/ginkgo-only-check.sh: line 44: rg: command not found
ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega
```

A gate that fails open is worse than an absent one: `factory doctor` lists it as
`[ARMED]`, and the install attestation counts it as proven.

The cost of not fixing this: the factory's governance is worth having on a repo
that is increasingly agent-authored, but adoption currently trades a
data-exposure risk and a broken build interface for it, and pays in a gate that
lies.

---

## 3. User Stories

### Story 1: Install without losing project files

**As the** maintainer of Conway
**I want** the factory install to leave every file the project owns intact, or trivially restorable
**So that** adopting governance does not cost me my build interface or my ignore rules

### Story 2: Never re-expose internal data

**As the** maintainer of a repository slated for release, holding internal planning data locally
**I want** a check that fails if the internal paths stop being ignored
**So that** no install, upgrade or careless edit can quietly make them committable

### Story 3: Arm only gates that can pass

**As the** maintainer
**I want** the installed gates to match how this codebase is actually built and tested
**So that** `factory doctor` reporting "armed" means something, and CI is not red on day one

### Story 4: Know that a gate is real

**As the** maintainer
**I want** any gate that cannot run its own check to fail rather than report success
**So that** the attestation at the end of an install is trustworthy

---

## 4. Acceptance Criteria

### Story 1: Install without losing project files

**AC 1.1: Ignore rules survive**

> Given Conway's `.gitignore` before an install
> When `factory-init` has run and the post-install checklist is complete
> Then every internal path excluded before the install is still excluded
> And `git status --porcelain` lists no internal data file as untracked

**AC 1.2: The build interface survives**

> Given Conway's Makefile targets before an install
> When the install is complete
> Then `make help` lists `build`, `test`, `server`, `stop`, `status`, `logs` and `clean`
> And the factory's own targets are additionally available

**AC 1.3: Agent instructions survive the symlink**

> Given Conway's project instructions
> When `sync-claude.sh` replaces `CLAUDE.md` with a symlink to `AGENTS.md`
> Then no instruction content is lost, because it already lives in `AGENTS.md`
> And a later `make sync-harnesses` is a no-op with respect to that content

**AC 1.4: The whole install is one reviewable diff**

> Given a clean working tree on a non-default branch
> When `factory-init` has run
> Then the entire install appears as a single reviewable change
> And reverting it requires no manual file surgery

### Story 2: Never re-expose internal data

**AC 2.1: A guard fails when an internal path becomes committable**

> Given the internal-path guard is installed and registered
> When any change causes a path on the internal list to stop being git-ignored
> Then the guard fails with the offending path named
> And it fails in CI as well as locally

**AC 2.2: The guard survives upgrades**

> Given the guard is registered in `factory.yaml` under `local_hooks`
> When `./factory upgrade` runs
> Then the guard is still registered and still executable

**AC 2.3: The guard is proven, not assumed**

> Given the guard
> When the self-test runs
> Then it introduces the exact violation, asserts the guard fires, reverts, and asserts it passes

### Story 3: Arm only gates that can pass

**AC 3.1: Go checks run where the module is**

> Given the Go module at the repository root, with packages under `server/`
> When the pack's unmodified check command and CI workflow run from the root
> Then every Go step resolves the module
> And none reports "go.mod not found"

**AC 3.2: No pack is installed without a codebase to act on**

> Given a repository with no `package.json`, `tsconfig.json` or TypeScript source
> When packs are chosen
> Then the TypeScript pack is not installed
> And no TypeScript step appears in the check command or CI

**AC 3.3: The dialect gate matches the project's actual test convention**

> Given Conway's stdlib `testing` suite
> When the install is complete
> Then either the Ginkgo dialect gate is not armed, or the suite has been converted to satisfy it
> And `factory doctor` does not report an armed gate that the current suite violates

**AC 3.4: Citation linting is not armed before specs are cited**

> Given specs named `NNN-name.md` and a citation prefix of `SPEC_`
> When the install is complete
> Then citation linting is either disabled or its prefix matches the actual file naming
> And no citation can silently fail to resolve because no file could ever match

### Story 4: Know that a gate is real

**AC 4.1: A missing dependency fails the gate**

> Given a gate whose check requires a tool that is not installed
> When the gate runs
> Then it exits non-zero naming the missing tool
> And it does not report the check as passed

**AC 4.2: The install attestation reflects reality**

> Given an install on a machine missing a gate's dependency
> When the post-install self-test runs
> Then it does not report that gate as proven

---

## 5. Functional Requirements

Requirements on Conway, unless marked upstream.

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | Conway's agent instructions MUST live in `AGENTS.md` before any install, with `CLAUDE.md` treated as a generated symlink | MUST |
| FR-002 | Conway's Makefile targets MUST be extracted into a separate included file, so a factory overwrite of `Makefile` costs one `include` line rather than the whole interface | MUST |
| FR-003 | The repository MUST carry a guard that fails when any internal path stops being git-ignored, determined via `git check-ignore` so it is indifferent to which mechanism provides the ignore, registered under `local_hooks` so it survives upgrades | MUST |
| FR-004 | The internal-path guard MUST have a break/fix case in the self-test | MUST |
| FR-004a | The guard MUST read its path list from `.git/info/exclude`, so the sensitive names are never committed, and MUST fail — not skip — when that file is missing or carries no Conway block | MUST |
| FR-004b | Ignore rules MUST be split by audience: paths a fresh clone can produce stay in `.gitignore`; paths that exist only on one maintainer's machine live in `.git/info/exclude` | MUST |
| FR-005 | The install MUST be performed on a non-default branch with a clean tree, and reviewed as a diff before merge | MUST |
| FR-006 | The install MUST be followed by a documented post-install checklist that verifies ignore rules, build interface, doctor health and the existing test suite | MUST |
| FR-007 | ~~`check_command` and the Go pack's CI workflow MUST execute Go steps inside `server/`~~ **Superseded 2026-08-17 (Decision 7):** the module moved to the repo root, so the pack's unmodified `go test -race ./...` works as shipped. No wrapping needed. | — |
| FR-008 | The TypeScript pack MUST NOT be installed while the repository has no TypeScript sources | MUST |
| FR-009 | The Go dialect gate MUST either be disarmed or satisfied; it MUST NOT be left armed against a suite that violates it | MUST |
| FR-010 | `citation_prefix` MUST be either empty or consistent with the actual spec file naming | MUST |
| FR-011 | The repository MUST resolve the two spec templates down to one, and record which | MUST |
| FR-012 | The `.claude/` directory MUST be either tracked (as the factory's drift check assumes) or the drift check disarmed; the current state, ignored-but-drift-checked, MUST NOT persist | MUST |
| FR-013 | A decision MUST be recorded on whether Conway adopts the factory's pull-request flow, since `direct-main-push-block` forbids the direct-to-main pushes used today | MUST |
| FR-014 | The install MUST be run with a controlling terminal, or with answers supplied on stdin; the piped `curl … \| sh -s -- init` form MUST NOT be used, as its prompts read `/dev/tty` and abort with blank answers | MUST |
| FR-015 | Conway's remaining `.gitignore` rules SHOULD be restored from the installer's backup after an install; with the internal paths moved out (FR-004b) this is a correctness fix, not a disclosure fix | SHOULD |
| FR-016 | (upstream) `sync-claude.sh` SHOULD back up a non-symlink `CLAUDE.md` before replacing it, or refuse when it has content | SHOULD |
| FR-017 | (upstream) `factory-init` SHOULD append to an existing `.gitignore` in a marked block rather than replacing it | SHOULD |
| FR-018 | (upstream) `ginkgo-only-check.sh` MUST fail closed when `rg` is absent | MUST |
| FR-019 | (upstream) Packs SHOULD NOT overwrite each other's `Makefile.pack`; a polyglot install currently keeps only the last pack's copy | SHOULD |
| FR-020 | (upstream) `Makefile.pack` SHOULD be included by the generated Makefile, or not installed; today it is inert | SHOULD |
| FR-021 | (upstream) `pre-push-check.sh` SHOULD be satisfiable on a first install. Step 5 (`diff-aware-check`) writes `memory/.parity-stale` whenever `.opencode/plugin/factory-hooks.ts` is in the diff range, and step 7 blocks on that flag — so on the install commit itself, which adds that file, the suite cannot reach 7/7 in a single run regardless of what the adopter does | SHOULD |
| FR-023 | (upstream) The Go pack's CI workflow MUST pin a `golangci-lint-action` major that matches the `golangci-lint` major it installs. It ships `golangci-lint-action@v6` (golangci-lint v1 only) with `version: v2.12.2`, so the step fails before the linter runs — observed on this repo's first CI run, 2026-08-17 | MUST |
| FR-022 | (upstream) `factory-init` SHOULD NOT overwrite the adopter's `README.md` with the template's own product README. It names no adopter and links to eight docs the installer does not copy, so every link is broken on arrival | SHOULD |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | Reversibility | The complete install can be reverted with `git checkout` plus `git clean`, with no manual file surgery and no loss of project content | Rehearse the revert on the install branch before merging |
| NFR-002 | Data safety | Zero internal paths committable at any point during or after the install | The FR-003 guard, run before every commit on the install branch |
| NFR-003 | Gate honesty | Every gate `factory doctor` reports as armed passes on a clean tree, and fails when its violation is introduced | `./factory doctor`, which runs the break/fix proof |
| NFR-004 | Build continuity | The existing test suite passes unchanged after the install | `make test` before and after |
| NFR-005 | Upgrade durability | A subsequent `./factory upgrade` does not reintroduce any of the four failures in §2 | Rehearse an upgrade on a branch after the install lands |

---

## 7. Data Model

Not applicable — this spec changes repository configuration and layout, not a
domain model. The one durable list it introduces is the set of internal paths the
FR-003 guard asserts remain ignored. Per Decision 6 that list lives in
`.git/info/exclude` and is deliberately not committed: planning workbooks,
internal notes and decks, `deploy/`, `pd.txt`, `*.vtt`, `claude-progress.md` and
the readiness plan. The shared half — build output, `data/` pipeline output,
`pod-directory*.csv`, `.envrc.local` — stays in `.gitignore`, where it protects
every clone.

---

## 8. API Contract

Not applicable — no API boundary. The relevant contract is `factory.yaml`, whose
keys are documented in the template's `docs/ADAPTING.md`; the values Conway sets
are specified in §5 and §11.

---

## 9. Out of Scope

- Rewriting the 19 stdlib `testing` files into Ginkgo. If that conversion is wanted it is its own spec with its own justification, not a side effect of adopting a governance tool.
- Moving Conway's Go module to the repository root. Configuring around the layout is cheaper and less disruptive than restructuring the repo to suit an installer.
- Installing the TypeScript pack, or adding a TypeScript toolchain to a frontend that is deliberately dependency-free vanilla JS with vendored d3.
- The adversarial review lane, the eval harness, and the wiki. All are opt-in and orthogonal to a safe install; adopt them later per the template's incremental guidance.
- Forking or vendoring the template. The upstream fixes in FR-016 to FR-020 are proposals to a repository the same author owns, not changes Conway carries.

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | Does Conway adopt the pull-request flow? `direct-main-push-block` rejects direct pushes to `main`, which is how the repo is pushed today. Adopting it means branch-and-PR for every change, including one-line doc edits. | Anoop | — | [NEEDS CLARIFICATION] |
| Q2 | Is the Ginkgo dialect gate disarmed permanently, or is the suite converted later? The Go pack is "battle-tested" specifically under that stack, so disarming it means adopting the pack minus its main opinion. | Anoop | 2026-08-19 | **Partly resolved 2026-08-19 — new tests are Ginkgo, the existing 19 stay stdlib.** Behavioural tests written from now on use Ginkgo/Gomega (`github.com/onsi/ginkgo/v2 v2.32.1`, `github.com/onsi/gomega v1.42.1`, vendored), starting with `server/planning/schedule_test.go` under the `TestPlanningSuite` bootstrap. `ginkgo-only-check.sh` stays disarmed while the stdlib files remain, because arming it would fail on 19 files this decision does not convert — Decision 4's reasoning is unchanged. What is still open is whether those 19 are converted, and by whom. |
| Q3 | Should `.claude/` become tracked? The factory generates it deterministically from `opencode.json` and drift-checks it; Conway currently ignores it. Tracking it is the factory's assumption, but it puts harness config in a repo headed for public release. | Anoop | — | [NEEDS CLARIFICATION] |
| Q4 | Which spec template survives — the ai-craft `SPEC_TEMPLATE.md` chosen for spec 001, or the factory's shorter `specs/TEMPLATE.md`? Keeping both guarantees drift. | Anoop | — | [NEEDS CLARIFICATION] |
| Q5 | Do the upstream fixes (FR-016 to FR-020) get raised against `software-factory-template` before Conway installs, or does Conway work around them? Fixing upstream first means every future adopter benefits and Conway's install gets simpler. | Anoop | — | [NEEDS CLARIFICATION] |
| Q6 | `protected_paths` was left empty. `server/game/` is the natural candidate — the game rules must stay server-side per `GAME-SPEC.md` — but empty means the decision-log gate treats only factory surfaces as governance-sensitive. Revisit after the install is stable? | Anoop | — | [NEEDS CLARIFICATION] |

---

## 11. Decision Record

### Decision 1: Prepare the repository, rather than fork the installer

**Context:** Four of the failures in §2 originate in the installer. Conway could
fork or patch it, or could change its own layout so the installer's behaviour is
harmless.

**Decision:** Change Conway. Move instructions into `AGENTS.md`, extract Makefile
targets into an included file, keep the `.gitignore` additions in a marked block,
and add a guard that makes the one unrecoverable failure detectable. Raise the
installer issues upstream separately.

**Alternatives considered:**
- Fork the template — rejected: it inherits maintenance of a project that is actively developed, to fix problems that are one-time costs at install.
- Patch `factory-init` locally before running it — rejected: the patch is not carried forward, so the next adopter of this repo, or the next machine, hits the same install.

**Consequences:** Conway carries a small amount of structure it would not
otherwise need (a separate Makefile include, a marked ignore block). All of it is
independently defensible, so the cost is low.

### Decision 2: The ignore guard is a test, not a note

**Context:** The `.gitignore` overwrite was caught by inspection. Inspection does
not scale, and the consequence — internal data committed to a repo headed for release — is
permanent once it happens.

**Decision:** Add a hook asserting that every internal path is still ignored,
register it in `local_hooks` so upgrades preserve the registration, and give it a
break/fix case in the self-test so it is proven rather than assumed.

The guard asks `git check-ignore` rather than reading `.gitignore`, so it does
not care which mechanism provides the ignore — and it therefore also catches the
residual risk introduced by Decision 6, a `.git/info/exclude` lost to a fresh
clone. It reads its path list from that same exclude file, so the list of
sensitive names is never itself committed.

**Alternatives considered:**
- A checklist item in `AGENTS.md` — rejected: the same class of protection that just failed.
- Hard-coding the sensitive paths in the guard — rejected: it would publish in a committed script exactly the names Decision 6 moves out of the committed `.gitignore`.

**Consequences:** One more hook to maintain, and it is inert on a clone whose
exclude file was never re-applied — which is precisely the condition it must
report, so it MUST fail rather than skip when the file is missing or empty. It is
the only gate on this list that protects against something irreversible.

### Decision 6: Split the ignore rules by audience

**Context:** Conway's `.gitignore` mixed two unrelated things: artifacts every
clone produces, and files that exist only on one maintainer's machine. The second
group is what made a `.gitignore` overwrite dangerous — and, separately, listing
those names in a tracked file discloses internal document and programme naming to
anyone reading the repository.

**Decision:** Split by one test — would a fresh clone by an outside contributor
ever produce this path? Rules that pass stay in `.gitignore`, where they protect
everyone: build output, `data/` pipeline output, `pod-directory*.csv` (which
stops the *next* adopter committing their own org's roster). Rules that fail move
to `.git/info/exclude`: the planning workbooks, internal notes and decks,
`deploy/`, `pd.txt`, `claude-progress.md`, `*.vtt`.

**Alternatives considered:**
- Leave everything in `.gitignore` — rejected: it keeps the disclosure and keeps the overwrite dangerous.
- Move everything local-ish, including `data/*` and `pod-directory*.csv` — rejected: those rules protect future adopters running the mining pipeline against their own Jira, which is exactly what a shared ignore file is for.
- A global `core.excludesFile` — rejected: it would apply these rules to every repository on the machine.

**Consequences:** A `.gitignore` overwrite by any installer is no longer a
disclosure risk. In exchange, protection for the internal paths now lives in a
file that is never pushed and never backed up: **re-clone the repository and it
is gone, silently**. The exclude file carries a warning header saying so, and the
Decision 2 guard is what turns that silence into a failure.

### Decision 3: ~~Configure around the module layout~~ — superseded by Decision 7

Originally: wrap the Go steps so they execute inside `server/`, and do not move
the module. Reversed on 2026-08-17 — see Decision 7.

### Decision 7: Move the Go module to the repository root

**Context:** The Go pack's `check_command` and CI workflow run `go test -race
./...`, `gosec ./...` and `govulncheck ./...` at the repository root with no
working directory. Conway's module was in `server/`, so every one of those steps
failed. Decision 3 proposed wrapping each step; that leaves two files carrying
the layout knowledge and every future pack step needing the same treatment.

**Decision:** Move `go.mod`, `go.sum` and `vendor/` to the repository root,
keeping the Go packages under `server/`. Internal imports become
`conway/server/*`. The pack then works unmodified.

**Alternatives considered:**
- Wrapping every Go step (Decision 3) — rejected once the cost was clear: it is per-step, forever, and silently wrong for any step added later by an upgrade.
- Moving the Go packages themselves to the root — rejected: it would scatter Go packages among `app/`, `data/`, `docs/` and `specs/`. Keeping them under `server/` with the module root above them is the smaller change.

**Consequences:** A one-time import rewrite across 19 files, done and verified
(`go build`/`vet` clean, all packages' tests pass, `make test` green). `go test
./...` now works from the root, which is both the Go convention and what every
pack step assumes. The Dockerfile needed its copy paths and build target
updated; that edit is **not** verified, as no Docker daemon was available.

### Decision 4: Do not arm a gate this codebase violates

**Context:** The Ginkgo dialect gate is the Go pack's central opinion, and
Conway's 19 test files violate it. It currently appears to pass only because
ripgrep is missing.

**Decision:** Disarm it at install time. Revisit as a deliberate testing
decision (Q2), never as an install side effect.

**Alternatives considered:**
- Convert the suite during the install — rejected: it makes a governance install into a test-framework migration, and couples two changes that should be reviewable separately.
- Leave it armed and red — rejected: a permanently failing gate trains everyone to ignore gate failures.

**Consequences:** Conway adopts the Go pack without its blessed test stack. That
should be stated plainly rather than glossed, since it is most of what makes the
pack "battle-tested".

### Decision 5: Install interactively, on a branch

**Context:** The documented one-liner pipes the installer into `sh`, whose
prompts read `/dev/tty`. Under automation there is no tty, every answer comes
back blank, and the installer aborts — which is how the first attempt failed
before it wrote anything.

**Decision:** Run the installer with a real terminal, from a clean tree on a
non-default branch, and review the whole install as a diff before merging.

**Consequences:** The install cannot be fully automated by an agent. Given what
it overwrites, a human at the terminal is the right level of ceremony.

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Internal paths committable after an install | All of them, until manually restored | Zero, enforced by a gate | The FR-003 guard in CI |
| Project files lost to an install | `.gitignore`, `Makefile`, `CLAUDE.md` | None | Post-install checklist |
| Armed gates that can actually pass | 1 of 2 language gates; 1 falsely passing | All armed gates pass on a clean tree and fail on violation | `./factory doctor` |
| Install attempts needed | 1 attempt, reverted | 1 attempt, merged | Install branch history |
| Existing test suite after install | Not run | Unchanged and green | `make test` |

---

## 13. Adoption Sequence

The order matters: every step before the install exists to make the install
boring.

**Before**

1. Move Conway's project instructions from `CLAUDE.md` into `AGENTS.md`. `CLAUDE.md` becomes a symlink at install; content there is deleted without backup.
2. Extract Conway's Makefile targets into `Makefile.conway` and reduce `Makefile` to an `include`. The overwrite then costs one line.
3. ~~Wrap Conway's `.gitignore` exclusions in a marked block.~~ **Done 2026-08-17:** the rules were split by audience instead (Decision 6). Local-only internal paths now live in `.git/info/exclude`, which no installer touches; `.gitignore` keeps only what every clone needs.
4. Write the internal-path guard, register it in `local_hooks`, and add its self-test case. It cannot be registered before `factory.yaml` exists, so write it now and register it in step 7.
5. Commit all of the above. Branch.

**Install**

6. Run `factory-init.sh` from a terminal with `--pack go` only. Answer: docs root `specs`, empty citation prefix, empty protected paths, provider `inherit`, review lane off, Go 1.26.

**After, before committing**

7. Register the guard in `local_hooks`; restore the `include` line in `Makefile`; re-merge the `.gitignore` block from the backup.
8. ~~Point the Go steps at `server/`.~~ **Done 2026-08-17:** the module moved to the repo root (Decision 7), so the pack's Go steps work unmodified. Verify `go test ./...` from the root after install rather than patching anything.
9. Disarm the Ginkgo dialect gate (Q2).
10. Resolve the two spec templates (Q4) and the `.claude/` tracking question (Q3).
11. Run the checklist: internal paths ignored; `make help` complete; `make test` green; `./factory doctor` healthy with no armed gate violated; `git status` clean of internal data.
12. Review the diff. Merge, or revert and revise this spec.

---

## Review Checklist

- [x] Problem is clearly stated and justified
- [x] User stories represent real user value
- [x] Acceptance criteria are in Given/When/Then format
- [x] Edge cases and error scenarios are covered (missing tool, no tty, polyglot pack collision, upgrade durability)
- [x] Requirements use MUST/SHOULD/MAY language
- [x] Non-functional requirements have measurable thresholds
- [x] Out of Scope is explicit
- [ ] Open questions are marked, owned, and time-bound — owners assigned, target dates pending
- [x] No implementation details in the requirements (installer mechanics confined to the Problem and Decision Record)
- [x] AI can read this spec (markdown, in the repo)
