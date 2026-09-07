# Request keys must match supported browser contexts

Do not make retry-safe recording depend on a secure-context-only browser API
when the application also supports plain HTTP deployments. An opaque request
key can use `crypto.getRandomValues`; retain the generated key across unchanged
retries and put generation inside the operation's visible error boundary.

Canon: specs/028-portfolio-forecasts.md:250. API provenance: MDN
[getRandomValues](https://developer.mozilla.org/en-US/docs/Web/API/Crypto/getRandomValues)
and [randomUUID](https://developer.mozilla.org/en-US/docs/Web/API/Crypto/randomUUID),
read 2026-09-07. Behavioral provenance: observed 2026-09-07 via
`go test -v -count=1 ./server -ginkgo.focus='planning assistant browser'
-ginkgo.no-color -timeout=3m` with isolated PostgreSQL and Chrome:
`SUCCESS! -- 1 Passed | 0 Failed`. The journey removes `randomUUID`, exercises
unavailable randomness recovery, and retries a committed save whose response
was lost; history contains one record. This simulates API availability, rather
than serving the test page from an insecure remote origin.
