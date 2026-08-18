#!/usr/bin/env bash
# Assert that every internal path is still ignored by git.
#
# Conway is slated for open-source release and holds internal planning data
# locally: mined
# Jira output, planning workbooks, an org roster, internal notes, a deployment
# tree with real hostnames, and files carrying secrets. Publishing any of them is
# irreversible, so it must not depend on anyone remembering.
#
# The patterns live in .git/info/exclude, between the conway-internal markers,
# for two reasons: that file is never touched by any installer (the software
# factory's factory-init replaces .gitignore wholesale, which is what motivated
# this hook), and it is not committed, so the list of sensitive names is not
# itself published when the repo opens up.
#
# The trade-off that makes this hook necessary: .git/info/exclude is per-clone
# and never pushed or backed up. A fresh clone has no protection at all, and
# nothing would say so. This hook is what turns that silence into a failure —
# hence a missing or empty block is an error, never a skip.
#
# WHERE THIS RUNS. It is a local gate, deliberately not a CI one: the pattern
# list lives in .git/info/exclude, which is never pushed, so in a CI checkout
# there is no list — and no internal files either, since a fresh clone only has
# tracked ones. Running it there could only ever fail for the wrong reason.
# `make test` runs it, and `make preflight` runs it in --strict mode.
#
# Registering it in factory.yaml `local_hooks` makes hook-existence-check assert
# it stays present and executable across upgrades; that registration does not
# execute it, which is why the Makefile wiring above matters.
#
# TWO LEVELS. Default: every pattern in the block still resolves as ignored —
# cheap, no false alarms, catches an installer replacing an ignore file.
# --strict: additionally require a clean tree, which closes the hole where
# deleting a pattern would delete its own assertion. Strict is for pre-push,
# where work in progress should already be committed.
set -euo pipefail

STRICT=0
case "${1:-}" in
  --strict) STRICT=1 ;;
  "") : ;;
  *) echo "usage: $0 [--strict]" >&2; exit 2 ;;
esac

# Event logging, so `factory metrics` can report what this gate blocked. Falls
# back to a no-op when the lib is absent, matching every shipped hook — a gate's
# job is enforcement, not bookkeeping, and missing bookkeeping must never be why
# enforcement fails to run.
_EVDIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../lib/events.sh
if [ -f "$_EVDIR/../lib/events.sh" ]; then . "$_EVDIR/../lib/events.sh"; else factory_log_event() { :; }; fi

EXCLUDE_FILE="$(git rev-parse --git-dir)/info/exclude"
BEGIN_MARKER='# BEGIN conway-internal'
END_MARKER='# END conway-internal'

fail() {
  factory_log_event "internal-paths-ignored" "internal path not ignored"
  echo "internal-paths-ignored: FAIL — $1" >&2
  exit 1
}

[ -f "$EXCLUDE_FILE" ] || fail "$EXCLUDE_FILE does not exist.
  A fresh clone starts without it. Re-create the conway-internal block before
  copying any internal working files into this repository."

grep -qF "$BEGIN_MARKER" "$EXCLUDE_FILE" \
  || fail "no '$BEGIN_MARKER' block in $EXCLUDE_FILE.
  Without it nothing stops these paths being committed and later published."

# Patterns between the markers, minus comments and blank lines.
PATTERNS="$(sed -n "/$BEGIN_MARKER/,/$END_MARKER/p" "$EXCLUDE_FILE" \
  | grep -v '^#' | grep -v '^[[:space:]]*$' || true)"

[ -n "$PATTERNS" ] || fail "the conway-internal block is empty.
  Either the patterns were removed, or the block was never populated."

checked=0
violations=0
while IFS= read -r pattern; do
  [ -n "$pattern" ] || continue
  # Only assert on paths that actually exist here: a pattern matching nothing is
  # protection held in reserve, not a violation. Globs are expanded by the shell;
  # nullglob keeps an unmatched glob from being treated as a literal filename.
  shopt -s nullglob dotglob
  # shellcheck disable=SC2206  # deliberate glob expansion of the pattern
  matches=( ${pattern} )
  shopt -u nullglob dotglob
  for path in "${matches[@]}"; do
    [ -e "$path" ] || continue
    checked=$((checked + 1))
    if ! git check-ignore -q -- "$path"; then
      echo "  NOT IGNORED: $path (pattern: $pattern)" >&2
      violations=$((violations + 1))
    fi
  done
done <<< "$PATTERNS"

if [ "$violations" -gt 0 ]; then
  fail "$violations internal path(s) are no longer ignored.
  Something replaced or edited an ignore file — factory-init does exactly this to
  .gitignore. Restore the rules before committing anything."
fi

# Backstop. The check above can only assert patterns that are still in the block,
# so deleting a pattern would delete its own assertion — the check would pass
# while the file it protected became visible. This closes that hole without
# committing the sensitive names anywhere: in this repository every file that
# legitimately exists is either tracked or deliberately ignored, so an untracked
# file is by definition something nobody has decided about yet.
#
# Intended for pre-push and CI, where the tree should already be clean. Running
# it mid-edit will flag work in progress, which is the point: stage it, or ignore
# it, but do not push with the question open.
# `git ls-files --others` alone would only see untracked files, so a staged or
# modified tracked file would still be reported as "tree clean" — a claim the
# check had not actually verified. --porcelain covers all three.
DIRTY=""
[ "$STRICT" = "1" ] && DIRTY="$(git status --porcelain --untracked-files=all)"
if [ -n "$DIRTY" ]; then
  echo "internal-paths-ignored: FAIL — tree is not clean:" >&2
  printf '%s\n' "$DIRTY" >&2
  fail "each entry above needs a decision before pushing, and which one depends
  on the status letter.
  Tracked changes (M, A, D, R): commit them or stash them. Ignoring cannot help —
  git is already tracking those paths.
  Untracked files (??): commit them, remove them, or — if the file is internal —
  add a pattern to the conway-internal block in .git/info/exclude. Not to
  .gitignore, which is tracked and which installers overwrite."
fi

if [ "$STRICT" = "1" ]; then
  echo "internal-paths-ignored: OK ($checked internal path(s) ignored, tree clean)"
else
  echo "internal-paths-ignored: OK ($checked internal path(s) ignored; --strict also requires a clean tree)"
fi
