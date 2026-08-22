# Lesson: a branch grown off an open PR branch re-audits the PR's own commits

## Date
2026-08-22

## Context
PR #13's branch (`feat/001-baselines`) had review follow-ups to land. The
natural move — `git checkout -b fix/001-review-followups feat/001-baselines`,
commit, push, PR — fails this repo's preflight. `scripts/pre-push-check.sh`
step 6 runs `decision-log-gate.sh origin/main..HEAD`, and that range includes
the PR branch's own commits. One of them (2b7c5a9, which introduced
`scripts/wait-for-http.sh` before a Decision existed) failed the gate, and
amending it meant rewriting an already-pushed branch.

Complication: `git rebase -i` opens an editor and cannot run from this
non-interactive harness.

## Root cause
Preflight ranges are measured against the remote main, not the PR base. Any
descendant branch inherits the PR's governance audit. The gate is working as
designed; the branch topology was wrong for it.

## The fix
Rewrite the offending ancestor in place, then rebuild the line on top:

1. `git checkout <sha> && git commit --amend -m "<msg with Decision: N>"` —
   reword non-interactively. This leaves HEAD detached.
2. `git branch -f feat/001-baselines HEAD && git checkout feat/001-baselines` —
   move the PR branch label to the reword AND get on the branch. `branch -f`
   only relabels the ref; HEAD stays detached, so cherry-picks run from step 3
   would land on the detached HEAD and the step-4 push would ship the branch
   without them (caught in review of this lesson, 2026-08-22).
3. `git cherry-pick` the follow-up commits (cherry-pick, not rebase:
   rebase's cherry-pick detection silently skips already-applied commits and
   leaves the branch at the wrong place).
4. `git push --force-with-lease origin feat/001-baselines`.

For future review-followup work on an open PR, commit directly on the PR
branch (or cherry-pick onto it) instead of stacking a sibling branch — the
stack only works if every ancestor already passes the gates.

## Provenance
Observed 2026-08-22 while closing PR #13's review threads on conway.
The failing output: `DECISION-LOG-GATE FAIL: commit 2b7c5a9 touches
governance-sensitive paths` from `make preflight`, with the branch topology at
`git log --oneline main..HEAD` showing the PR's commits under the fix commits.
The cherry-pick-skip behaviour: `git rebase --onto` printed "warning: skipped
previously applied commit" and left fix/001-review-followups at 2dde428.
