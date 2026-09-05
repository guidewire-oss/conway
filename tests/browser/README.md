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
