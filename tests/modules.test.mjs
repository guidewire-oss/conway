// Every app module must be present and loadable. This is the smoke test that
// would have caught the drag.js deletion (lesson: a bulk message-rewrite
// emptied the module, and nothing imported it, so CI stayed green while the
// feature was dead — PR #41, 2026-08-26). Modules are enumerated at RUN time,
// so a future module can never be forgotten here.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';

const here = dirname(fileURLToPath(import.meta.url));
const jsDir = join(here, '..', 'app', 'js');
const modules = readdirSync(jsDir)
  .filter((f) => f.endsWith('.js'))
  .map((f) => `../app/js/${f}`);

assert.ok(modules.length > 0, 'no app modules found — the test is broken, not the app');

// Modules that touch the DOM at top level cannot execute under node. For
// those: presence + a compile-only parse (a SyntaxError fails; DOM access
// cannot happen because the source is never run). An emptied file — the
// drag.js incident — fails the presence check.
const DOM_AT_TOP_LEVEL = new Set(['main.js', 'sortable.js', 'docs.js']);

test(`every app module is present and loads (${modules.length} modules)`, async (t) => {
  for (const mod of modules) {
    await t.test(mod, async () => {
      const file = resolve(here, mod);
      assert.ok(statSync(file).size > 0, `${mod} is EMPTY — a deleted module breaks the app silently`);
      if (DOM_AT_TOP_LEVEL.has(mod.split('/').pop())) {
        // `node --check` parses the module without executing it: syntax
        // errors fail, DOM access cannot happen.
        execFileSync(process.execPath, ['--check', file], { stdio: 'pipe' });
        return;
      }
      const m = await import(mod);
      assert.ok(m && typeof m === 'object', `${mod} did not evaluate to a module namespace`);
    });
  }
});
