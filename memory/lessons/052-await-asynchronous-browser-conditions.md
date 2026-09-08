# Await asynchronous browser conditions

An asynchronous predicate returns a Promise before its condition has a value.
Check whether the polling API awaits that value: otherwise a truthy Promise can
end polling on the first attempt. Use the bounded helper in
`tests/browser/async-condition.mjs` for API-backed conditions, then wait for the
rendered control to reference the completed resource before navigating.

Provenance: observed 2026-09-07 while investigating PR 86 integration failures
in linked-feature acknowledgement and refreshed capture navigation. The
installed Playwright 1.56.1 implementation matches its
[official source](https://github.com/microsoft/playwright/blob/v1.56.1/packages/playwright-core/src/server/frames.ts#L1372-L1375)
(fetched 2026-09-07): the polling loop tests the predicate return value directly.
`node --test tests/async-condition.test.mjs` exercises repeated false promises,
predicate failures and never-settling requests. After replacing asynchronous
predicates and waiting for the refreshed capture link, the browser gate reported
`6 Passed | 0 Failed`.
