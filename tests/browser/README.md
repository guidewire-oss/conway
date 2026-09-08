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
The same harness runs `weekly-review.mjs` inside Review execution: manual review,
action creation, delayed catalog/preview responses, a real completion conflict,
evidence-preserving action conflicts, resolution, immutable completed summaries,
review-link reload, and older review selection during a delayed history refresh.
The `ready-queue.mjs` workflow uses the same real server to assess team/week
context, confirm full kit, release/defer/reconsider work, inspect history, and
retain decision evidence across conflicts and delayed responses. Desktop and
360px screenshots cover the queue in the existing plan theme.
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
`conway-announcements-mobile.png`, `conway-linked-sources-mobile.png`,
`conway-weekly-review-desktop.png`, `conway-weekly-review-mobile.png`,
`conway-ready-queue-desktop.png` and `conway-ready-queue-mobile.png`.

CI provisions PostgreSQL, Go, Node and Playwright/Chromium before running this
opt-in command. The ordinary Go suite skips the browser test unless explicitly
enabled; an enabled browser run fails if its dependencies or database are absent.

The authenticated linked-feature journey additionally switches timeline lenses
using the keyboard and checks that selected state and the initiative filter
survive the real application rerender.

## Bootstrap adoption acceptance

The same CI browser command also selects the Bootstrap adoption Ginkgo spec.
It mounts actual operational view modules in a temporary HTTP server, checks
framework controls, keyboard disclosures and retained drafts, and exercises
the documentation reader's search and contents navigation. The game helper
uses the actual renderer with sanitized API fixtures to exercise keyboard
staging, round submission and result/pause dialogs. It checks light and
dark themes at desktop and 360px widths and writes `conway-bootstrap-*.png`
screenshots to the artifact directory. This focused spec needs Playwright but
no database:

```sh
CONWAY_TEST_BROWSER=1 go test ./server -ginkgo.focus='Bootstrap adoption' -ginkgo.fail-on-empty -timeout 5m
```

## Planning assistant and forecast acceptance

The `planning assistant browser` Ginkgo harness runs the authenticated assistant
journey and `portfolio-forecast.mjs` against an isolated database and temporary
server. The forecast journey compares scenarios, retries a failed request,
inspects imported evidence, changes settings during a pending response, leaves
and returns through Home, and follows an initiative into the saved timeline.
Successful runs write `conway-assistant-dark.png`, `conway-assistant-light.png`
and `conway-forecast-mobile.png` plus `conway-prediction-history-mobile.png` to `CONWAY_TEST_ARTIFACT_DIR` (or the system
temporary directory). The assistant uses a deterministic provider fixture; no
live model or external credentials are needed.

`prediction-history.mjs` extends this journey with recording, lost-response
retry without duplicates, reload, visible capture failures, later outcome
assessment and capture-selection invalidation. It temporarily uses a future
planning period and restores the original settings, so time passing does not
turn a prospective prediction fixture into an already-expired one.

The same journey validates complete prediction history, suppresses repeated
work, recovers a failed validation request, ignores a result after capture
selection changes, and opens the representative record. It writes
`conway-validation-mobile.png` with expanded selection explanations at 360px.

`prediction-evaluation.mjs` continues on a separate generic history fixture. It
uses the real evaluation endpoint to fit earlier completed evidence, score later
work, reject reversed captures, recover after an outage, preserve selections,
ignore late responses after either capture disappears or navigation changes, and
open original records. Keyboard submission and light/dark 360px screenshots
(`conway-evaluation-*-mobile.png`) cover the progressive model section.

Use `waitForAsyncFunction` from `async-condition.mjs` for conditions that fetch
server state. It awaits each result and bounds both repeated false values and
unsettled requests. Keep Playwright `waitForFunction` predicates synchronous.
After a backend change, also wait for the corresponding rendered state before
interacting with an existing control that may still refer to older data.
