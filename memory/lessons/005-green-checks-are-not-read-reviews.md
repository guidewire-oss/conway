# Lesson: green checks are not read reviews — fetch the threads after every push

## Date
2026-08-22

## Context
Closing PR #13's review threads, I replied and resolved all 15, landed fixes,
pushed, watched the GitHub check suite go green, and reported the PR "done."
The user then asked "did you look at any of the reviews?" — and PR #14 had one
unresolved thread I had never opened. It was a valid catch: cubic reviewed the
*doc* I had committed (memory/lessons/004) and found the recipe dropped the
follow-up commits (`git branch -f` leaves HEAD detached). A real defect, filed
as a review thread, invisible in `gh pr checks`.

## Root cause
AI reviewers (cubic, Copilot) file their findings as review threads and
review-summaries that do **not** gate the merge: `gh pr checks` shows them as
"pass" or "pending" regardless of unresolved comments. Treating a green check
suite as "the review is clean" conflates two different signals — CI says the
code runs, threads say a reviewer (human or AI) has an unresolved point.

## The fix
After every push to a PR, before reporting it done:

```
gh api graphql -f query='query { repository(owner: "guidewire-oss",
  name: "conway") { pullRequest(number: N) { reviewThreads(first: 50) {
  nodes { isResolved path originalLine comments(first: 20) {
  nodes { author { login } body } } } } } } }'
```

Read every unresolved thread and the latest review summary body, not just the
check status. Reply, fix, or leave open with the reason — then, and only then,
report the PR as closed-out.

## Provenance
Observed 2026-08-22 on conway PR #14: `gh pr checks 14` showed only
"cubic · AI code reviewer pass" while the thread list had 1 unresolved
thread (memory/lessons/004 recipe bug, filed by cubic-dev-ai, thread
PRRT_kwDOTMR4Hs6bWQ49). The thread was fixed in commit 65cd0eb after the
user's prompt surfaced the gap.
