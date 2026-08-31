# 020: a refactor replaced a wiring call and every control it wired went dead silently

**Date:** 2026-08-30
**Provenance:** observed 2026-08-30 (user report: "saving the baseline name does not save"); `git log -S 'wireBaselineControls()'` shows commit e3dd722 (spec 012, 2026-08-27) replacing the call with `wireCallouts()` during a renderOrder refactor; confirmed in-browser — clicking "✓ Save baseline" dispatched nothing.

`wireBaselineControls()` bound save/activate/compare/listeners, all in one function called once per renderOrder. A refactor swapped that one call line for another and deleted it — every baseline control went inert for three weeks, CI green, no error anywhere: an unbound button is not a failure, it is silence.

Rules:
- Controls that live in re-rendered markup should be wired by DELEGATION (one document-level listener with `closest()` matching), not per-render `addEventListener` — a wiring call that must be remembered on every render path is a single deleted line away from silent death.
- When deleting or replacing a function CALL, grep its name for other call sites AND for what the function was wiring; a `grep -c` of the definition minus call sites (`grep -n 'fnName' file | wc -l` == 1 means defined-never-called) is a one-second check.
- Symptom signature: a button click that does literally nothing — no request, no error, no state change. Treat "no network request on click" as a wiring bug before a server bug.
- The function-defined-never-called state is detectable: a lint/test asserting every exported-or-named wiring function has a call site would have caught this.
