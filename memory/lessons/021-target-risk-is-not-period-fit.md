# Target risk is not period fit

Observed 2026-09-05 via direct Node invocation of `fitSentence` during the
interaction review: an initiative targeting week 5 and committing in week 6
of a 26-week period produced a headline saying it would not finish inside the
period. Target lateness alone does not establish period overrun.

Provenance: `app/js/report.js:34` classifies every non-good verdict as outside
the period; `server/planning/schedule.go:2112` evaluates target-date verdicts.
The exact reproduction command and output are retained in
`docs/UX-REVIEW-2026-09-05.md` under Evidence log and limits.

When summarizing scheduling results, preserve the difference between target
risk, horizon fit and missing evidence. A shared computation source does not
guarantee that a new headline describes that computation accurately.
