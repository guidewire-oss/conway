# Retained evidence must preserve decision intent

A refreshed workflow can change which actions are available. Restoring an old
form's owner and evidence while defaulting to the first new action can silently
turn a reconsideration into a release. Require an explicit new choice when the
original action disappears; retain the evidence without granting new intent.

A successful mutation must also refresh its still-visible context even if an
intervening read completed before the write committed. Request-generation guards
protect late reads, but should not suppress the authoritative post-write refresh.

Provenance: observed 2026-09-06 through independent current-source probes
`node /private/tmp/conway-ready-draft-probe.mjs` and
`node /private/tmp/conway-ready-save-refresh.mjs`. The first reproduced a
reconsideration becoming release; after correction it retained an empty required
choice and sent zero POST requests. The second reproduced a stale ready state
following successful release; after correction it requested the updated queue.
Production recovery lives in `app/js/readyqueueui.js:153` and
`app/js/readyqueueui.js:188`; specification 025 Decision 4 remains canonical for
context guards and retained evidence.
