# Lesson: an ES module registry outlives a dev-server restart — force reload before trusting a live check

## Date
2026-08-22

## Context
Verifying the timeline's horizon clamp in the browser: after landing `barGeom`
(and its unit tests passing), the live page still showed 17 bars overflowing
past 100%. The served file had the fix (`fetch('/js/timeline.js')` contained
`barGeom`), and yet the running page computed `width:107.69%` — the unclamped
value. The same trap fired three times this session: after each `make server`
restart, a soft `page.goto('#plan')` re-ran the app against the PREVIOUS
session's module registry.

## Root cause
`app/` is served with no-cache headers, but the browser's ES module registry
keys modules by URL within a document/session — a same-document navigation
(hash change, history) does not refetch modules. `location.reload()` does.
Every "the fix didn't work" that was actually "the page never loaded the fix"
costs a debugging cycle and can send you rewriting correct code.

## The fix
Before trusting any negative live check against changed `app/js/*.js`:
1. `location.reload()` — not a hash navigation.
2. Re-authenticate if the reload dropped the token (`localStorage.conway_token`
   survives reloads, but a fresh profile does not).
3. Only then evaluate.

A cheap probe when in doubt: read the served file and compare against the
live behavior — if `fetch('/js/x.js')` contains the new symbol but the page
behaves old, it is the module registry, not the code.

## Provenance
Observed 2026-08-22 verifying PR #18's clamp fix: before reload, 17 bars
overflowed with `width:107.69%`; after `location.reload()`, 0 overflowed with
the same served file (`fetch('/js/timeline.js')` → `servedHasClamp: true` in
both probes). Same pattern earlier in the session: "still no timeline tab"
after the tab was wired and served.
