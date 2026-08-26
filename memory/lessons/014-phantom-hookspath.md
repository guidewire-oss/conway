# core.hooksPath can point at a directory that does not exist

## Lesson

`git config core.hooksPath` was set to `~/workspace/guidewire/git-hooks-core`, which does not
exist on this machine (the org dir holds `org-chart`, no hooks). Git silently runs zero hooks
when the path is missing — so `direct-main-push-block`, `pending-lessons-push-block` and every
other pre-push gate were dead locally. A direct push to main went through unnoticed (a chore
commit removing a stray screenshot, harmless but rule-breaking).

Check `git config core.hooksPath` AND that the directory exists before trusting local hook
enforcement; `make preflight` runs the same gates explicitly and is the reliable last line.

## Provenance

- Observed 2026-08-26: `git config core.hooksPath` → `~/workspace/guidewire/git-hooks-core`;
  `ls` → ENOENT; `ls ~/workspace/guidewire` → only `org/org-chart`. Commit 8db5461 pushed to
  main directly with no hook output.
