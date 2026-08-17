# AGENTS.md — project instructions for every coding agent

This is the canonical instruction file. `CLAUDE.md` is a symlink to it, kept that
way deliberately: the software factory's `sync-claude.sh` runs `rm -f CLAUDE.md`
and symlinks it, without backing it up first, so any content held there would be
lost silently. A symlink that already exists makes that step a no-op.

If a factory install or upgrade overwrites this file with the template's own
`AGENTS.md`, re-merge the sections below from the `.factory-backup` copy it
leaves alongside.

## Working agreement

Before continuing, name the deliverable in one sentence (PR title, file changed, command output you expect). Stop and check with me if (a) you spend more than 10 minutes without an edit, or (b) the same approach fails twice. Do not retry past two attempts on any single fix.

## Specs

Specs live in this repo under `specs/`, one file per feature. They do **not**
live in a sibling specs repository — that convention does not apply here.

- **Format:** follow `specs/SPEC_TEMPLATE.md` (local copy of
  <https://github.com/anoop2811/ai-craft/blob/main/SPEC_TEMPLATE.md>). Use its
  section order and numbering as written.
- **File naming:** `NNN-kebab-case-name.md`, numbered so specs can be
  cross-referenced by ID (`001-plan-execution-order.md`).
- **Requirements say WHAT and WHY.** Algorithms, alternatives considered, and
  their consequences belong in §11 Decision Record, not in the requirement
  tables.
- **Mark unknowns** `[NEEDS CLARIFICATION]` in §10 rather than assuming.
- Keep specs alive: they are the decision record for why the code is shaped the
  way it is. Tests capture what the system does.

`SPEC.md` (v1 prototype model), `GAME-SPEC.md` and `docs/v2-spec.md` predate
this convention and stay where they are; new feature specs go in `specs/`.
