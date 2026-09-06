# A completed write must not override newer navigation

A server-side change can become observable before its response reaches the
browser. Waiting for that change through an API is not proof that the UI's
completion callback has finished. A later history or preview selection must
survive the earlier write's eventual completion.

Capture the dialog generation when starting the mutation, and refresh only if
that generation is still current. Test the ordering by holding the real write
response, navigating, and then releasing it; do not hide the race with a delay.

Provenance: observed 2026-09-06 in the failed browser acceptance job
https://github.com/guidewire-oss/conway/actions/runs/34045140515/job/101518756935.
The regression in `tests/browser/linked-features.mjs` holds the Check now
response while opening captured history. After the generation guard, the
isolated browser harness returned `ok conway/server 4.064s` with one passing
acceptance spec. The decision is `specs/017-planning-and-execution-usability.md:219`.
