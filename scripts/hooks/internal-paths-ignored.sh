#!/usr/bin/env bash
# Assert that every internal path is still ignored by git.
#
# Conway is a public repository that holds internal planning data locally: mined
# Jira output, planning workbooks, an org roster, internal notes, a deployment
# tree with real hostnames, and files carrying secrets. Publishing any of them is
# irreversible, so it must not depend on anyone remembering.
#
# The patterns live in .git/info/exclude, between the conway-internal markers,
# for two reasons: that file is never touched by any installer (the software
# factory's factory-init replaces .gitignore wholesale, which is what motivated
# this hook), and it is not committed, so the list of sensitive names is not
# itself published in a public repo.
#
# The trade-off that makes this hook necessary: .git/info/exclude is per-clone
# and never pushed or backed up. A fresh clone has no protection at all, and
# nothing would say so. This hook is what turns that silence into a failure —
# hence a missing or empty block is an error, never a skip.
#
# Registered via factory.yaml `local_hooks` so `factory upgrade` preserves it.
# Runs standalone too: it reads no factory config.
set -euo pipefail

EXCLUDE_FILE="$(git rev-parse --git-dir)/info/exclude"
BEGIN_MARKER='# BEGIN conway-internal'
END_MARKER='# END conway-internal'

fail() {
  echo "internal-paths-ignored: FAIL — $1" >&2
  exit 1
}

[ -f "$EXCLUDE_FILE" ] || fail "$EXCLUDE_FILE does not exist.
  A fresh clone starts without it. Re-create the conway-internal block before
  copying any internal working files into this repository."

grep -qF "$BEGIN_MARKER" "$EXCLUDE_FILE" \
  || fail "no '$BEGIN_MARKER' block in $EXCLUDE_FILE.
  Without it the internal paths are unprotected in a public repository."

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
UNTRACKED="$(git ls-files --others --exclude-standard)"
if [ -n "$UNTRACKED" ]; then
  echo "internal-paths-ignored: FAIL — untracked files present:" >&2
  printf '  %s\n' $UNTRACKED >&2
  fail "each of these must be either committed or ignored.
  If one is internal, add it to the conway-internal block in .git/info/exclude —
  do not add it to .gitignore, which is public and which installers overwrite."
fi

echo "internal-paths-ignored: OK ($checked existing internal path(s) verified ignored, no untracked files)"
