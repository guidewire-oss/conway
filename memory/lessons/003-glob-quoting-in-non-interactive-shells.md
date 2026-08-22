# Lesson: shells don't expand globs inside quotes, and interactive shells are not `sh -c`

## Date
2026-08-21

## Context
`make check` was red on this machine with "Could not find
'.../tests/*.test.mjs'" while `node --test 'tests/*.test.mjs'` succeeded when
typed into an interactive shell. The quoted glob works interactively because
readline/zsh performs its own expansion (or passes it through only after
globbing the pattern), which hides the difference. The factory's `check_command`
in `factory.yaml` runs through `sh -c`, where quoting means the asterisk reaches
`node` as a literal filename and nothing matches.

Same trap from the other direction on the same day: writing a GraphQL reply
containing a backtick-quoted SQL fragment through a nested shell heredoc ate
the closing quote and the mutation never ran. Both failures look like the
*tool* being wrong and are the *quoting context* being wrong.

## Root cause
A shell expands unquoted globs; a shell does not expand quoted ones; a different
shell, a different invocation mode (`sh -c`, `$( )`, a make recipe, a heredoc)
changes which expansions apply. The same string literal can be correct in one
of those contexts and silently wrong in the next.

## The fix
For factory check commands, leave the glob unquoted and rely on the shell to
expand it (paths with spaces would need a different answer — a small wrapper
script — but this repo has none under `tests/`):

```
check_command: "... && node --test tests/*.test.mjs"
```

For anything nontrivial sent to `gh api`, `gcloud`, `aws` etc. from a script:
build the payload with a JSON encoder (`python3 -c json.dumps(...)`, `jq -n`)
and pass it as a single argument, instead of interpolating text that contains
quotes and backticks into a shell-quoted string.

## Provenance
Observed 2026-08-21 while running `make check` on conway @ fix/001-review-followups
(factory.yaml:23) and replying to PR #13 review threads. The make failure's
paste: `Could not find '/Users/anoopgopalakrishnan/workspace/conway/tests/*.test.mjs'`.
Verified by `sh -c "node --test 'tests/*.test.mjs'"` failing where
`sh -c "node --test tests/*.test.mjs"` passes.
