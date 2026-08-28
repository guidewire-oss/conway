# core.hooksPath must point at .githooks — or git runs zero hooks silently

## Lesson

The factory ships its push gate TRACKED in the repo (`.githooks/pre-push`) and
enables it with `git config core.hooksPath .githooks` (factory-init sets this;
install docs, step 6). A stale GLOBAL `core.hooksPath` pointing at a nonexistent
directory overrides it — and git runs zero hooks with no warning when the path
is missing. A direct push to main went through unnoticed (lesson 014).

Check and fix once per machine/clone:

    git config core.hooksPath          # stale global value? unset it:
    git config --global --unset core.hooksPath
    git config core.hooksPath .githooks

The hook being versioned with the repo means it survives re-clones — unlike
`.git/hooks/`, which is machine-local and was never the factory's mechanism.

## Provenance

- Observed 2026-08-26: `git config core.hooksPath` → `~/workspace/guidewire/git-hooks-core`
  (missing dir); direct push to main succeeded with no gate output. Correct enable step
  found in softwareaifactory.sh install docs (step 6) and the hook's own header comment.
- Related: lesson 014 (the TDZ-style silent-failure pattern in hooks), and the first
  attempt at this fix wired `.git/hooks/pre-push` by hand — duplicated the tracked hook
  and would have drifted.
