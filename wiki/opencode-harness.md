# opencode harness parity

**Parity claim status: OPEN — never verified in this repository.**

*Provenance:* observed 2026-08-17 — the factory install wrote
`.opencode/plugin/factory-hooks.ts`, `scripts/hooks/pending-lessons-push-block.sh:46`
raised `memory/.parity-stale` on it, and `make check-drift` reported no adapter
drift. No opencode run has ever happened here.

## What the claim would be

The factory's roles are canonical in `opencode.json`, and the Claude Code and
Codex adapters are generated from it. Parity means all three harnesses enforce
the same gates — in particular that an agent running as `implementer` is denied
edits to `*_test.go`, whichever harness it runs in. `.opencode/plugin/factory-hooks.ts`
is the enforcement point on the opencode side.

## Why it is open here, and why that matters

Which harness is in use varies by machine and by person: Claude Code, Codex and
opencode are all in play on this project. So parity is not academic — it decides
whether the gates hold for the next contributor, whoever they are and whatever
they run. An unverified harness is an unenforced one for everybody using it.

No opencode run has happened in this repository yet. The plugin arrived with the
factory install on 2026-08-17 and has not been exercised, so there is no OBSERVED
verification to go stale — the `memory/.parity-stale` flag fired because the file
was newly written, not because a previously verified claim was invalidated.

The same question applies to the other two, and only one of the three has been
seen working here: `test-edit-denial.sh` is wired for Claude Code via the
`PreToolUse` hook in `.claude/settings.json`, and the factory's own break/fix
self-test exercises the shared script. Codex and opencode enforcement remain
structurally configured but unobserved.

Per the Verification Contract this is a WROTE-level claim at best: the adapter
exists and `make check-drift` reports no drift between the generated Claude and
Codex configs and the opencode canon. That is a structural check on config files,
not evidence that the opencode plugin blocks a test-file edit at runtime.

## What would close it

Run the live parity eval on each harness someone actually uses — as
`implementer`, attempt an edit to any `*_test.go`, observe the block — and record
the command and its output here, dated, per harness. Until then, treat
enforcement as verified only on the harness that has been observed.

Whoever first works on this repo in opencode or Codex is the right person to
close the corresponding line, since they have the harness in front of them.

## Related

- `docs/FACTORY_RULES.md` — the Verification Contract and its three claim levels
- `specs/002-factory-adoption.md` — what the factory install did and did not prove
- Decision 8 in `docs/DECISION_LOG.md` — the two gates deliberately left unarmed

The drift check was weak here until 2026-08-17: `.claude/` was git-ignored by
Conway's own rule while `make check-drift` diffs files inside it, so the check
passed without comparing anything, and a teammate cloning on a Claude Code
machine received no generated roles and no `PreToolUse` hook. `.claude/` is now
tracked alongside `.opencode/` and `.codex/` — see Decision 10 in
`docs/DECISION_LOG.md`.
