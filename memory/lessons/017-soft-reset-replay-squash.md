# 002: `git reset --soft` + sequential `-F` commits squash the whole index into the first commit

**Date:** 2026-08-29
**Provenance:** observed 2026-08-29 during the spec-008-S4 branch history fix on feat/spec-008-s4-edge-undo; the reviewer agent flagged commit 7adeba8 carrying the full S4 diff under the previous feature's message, and `git log origin/main..HEAD --stat` showed two empty follow-up commits.

A commit-message replay (`git reset --soft <base>` then re-creating N commits from saved messages with `git commit -F`) does NOT split changes by file: the soft reset stages the entire cumulative diff, so the FIRST `git commit` contains everything and the remaining N−1 commits are empty (or worse, misattributed). The branch then passes push and CI while its history lies.

Rules going forward:
- After any history rewrite, run `git log <base>..HEAD --stat` (not just `--oneline`) and check every commit has the intended files and no commit is empty before pushing.
- To replay per-commit messages, use `git commit --only <paths>` per commit, or re-commit with explicit `git add <paths>` after a mixed (`--no-soft`) reset.
- A reviewer pass on the *commit list*, not just the net diff, is what caught this.
