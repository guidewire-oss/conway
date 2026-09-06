# Browser workflow checks

`planning-ux.mjs` runs the real login, demo plan, precise timeline edit/undo,
agreement validation/save, execution view, scenario copy and 360px layout.
It creates generic demo plans and deletes only those plans in its cleanup.
Run against an isolated server/database. It needs a locally available
Playwright installation and Chrome; no dependencies are installed by the test.

```sh
CONWAY_TEST_BASE_URL=http://127.0.0.1:8766 \
CONWAY_TEST_USERNAME=admin \
CONWAY_TEST_PASSWORD='replace-with-test-server-password' \
node tests/browser/planning-ux.mjs
```

Set `PLAYWRIGHT_MODULE` to the installed module path if it is not resolvable as
`playwright`. Screenshots go to the system temporary directory; override with
`CONWAY_TEST_ARTIFACT_DIR` pointing to an existing directory.

Database API checks run inside the Go suite when `CONWAY_TEST_DATABASE_URL`
points to an isolated PostgreSQL database. Without it they report skipped
rather than claiming persistence coverage.

## Linked features acceptance

The Go test starts its own temporary HTTP server with the real authenticated
handlers, application files and an internal deterministic Sheets provider.
Playwright signs in, checks the introduction and replay, opens linked sources,
captures and applies a roster, refuses a stale preview, and restores an earlier
capture. It also checks mobile overflow and announcement retry/focus behavior.
The provider seam exists only in the test binary; no production fixture routes
or Google credentials are required. This does not test live Google access.

From the repository root, with PostgreSQL available and Playwright plus its
Chromium browser already installed:

```sh
CONWAY_TEST_DATABASE_URL='postgres://test-user:test-password@localhost:5432/conway_test?sslmode=disable' \
CONWAY_TEST_BROWSER=1 \
PLAYWRIGHT_MODULE='/path/to/node_modules/playwright/index.mjs' \
go test ./server -ginkgo.focus='linked features browser' -ginkgo.fail-on-empty -timeout 5m
```

Use only an isolated test database. The suite migrates it and creates temporary
generic accounts/plans. `PLAYWRIGHT_BROWSER_CHANNEL=chrome` selects an installed
Chrome; leave it unset for Playwright's bundled Chromium, as CI does.
`CONWAY_TEST_NODE` optionally selects a Node executable outside `PATH`.
`CONWAY_TEST_ARTIFACT_DIR` selects an existing screenshot directory; otherwise
screenshots use the system temporary directory. Successful runs produce
`conway-announcements-mobile.png` and `conway-linked-sources-mobile.png`.

CI provisions PostgreSQL, Go, Node and Playwright/Chromium before running this
opt-in command. The ordinary Go suite skips the browser test unless explicitly
enabled; an enabled browser run fails if its dependencies or database are absent.
