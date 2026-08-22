import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  remedyKindLabel, remedyRowHTML, remediesPanelHTML, optionsExpanderHTML,
  remediesErrorMessage, REMEDY_KINDS,
} from '../app/js/remedyui.js';

// Real output from the Go endpoint for the demo plan (regenerate with
// `go run ./tools/fixgen`, which writes both fixtures), so the JS view and the
// Go Remedy shape are checked against each other, not against guesses.
const body = JSON.parse(readFileSync(new URL('./fixtures/remedies-demo.json', import.meta.url)));
const remedies = body.remedies;
const warnings = body.warnings || [];

test('the fixture is the shape the view expects', () => {
  assert.ok(remedies.length > 0, 'the demo plan misses dates, so it has remedies');
  for (const r of remedies) {
    assert.equal(typeof r.kind, 'string');
    assert.equal(typeof r.target, 'string');
    assert.equal(typeof r.resultingVerdict, 'string');
    assert.equal(typeof r.objectiveDelta, 'number');
  }
  assert.ok(warnings.length > 0, 'the transfer-capacity gap is carried, not silent');
});

test('every shipped kind has a human label', () => {
  for (const k of REMEDY_KINDS) {
    const label = remedyKindLabel(k);
    // The intent: machine names read as phrases, so no hyphenated enum leaks
    // into the page. A word that is already its own label ('descope') is fine.
    assert.ok(label && !/\w-\w/.test(label), `kind ${k} must read as a phrase, got "${label}"`);
  }
});

test('an unknown kind from a newer server renders generically instead of breaking', () => {
  // Upgrade tolerance: app/js is served from disk with no-cache while routes
  // are compiled into the binary, so a page CAN be older than the server it
  // talks to. A future remedy kind must render, not throw or vanish.
  const label = remedyKindLabel('quantum-reallocate');
  assert.equal(label, 'quantum-reallocate', 'the raw kind is the honest fallback label');
  const html = remedyRowHTML({
    kind: 'quantum-reallocate', target: 'X', magnitude: 3,
    resultingVerdict: 'on-time', targetWeeksLate: 0, objectiveDelta: -2,
  });
  assert.match(html, /quantum-reallocate/);
  assert.ok(!html.includes('undefined'), 'no undefined leaks for an unknown kind');
});

test('a kind named after an inherited property still gets the raw fallback', () => {
  // KIND_LABELS['constructor'] would otherwise be the inherited Object
  // function — truthy, so the || fallback never fires, and the row would
  // render a function instead of a kind.
  assert.equal(remedyKindLabel('constructor'), 'constructor');
  assert.equal(remedyKindLabel('__proto__'), '__proto__');
  assert.equal(remedyKindLabel('toString'), 'toString');
});

test('one priced option shows what it costs and what it lands', () => {
  const r = remedies[0];
  const html = remedyRowHTML(r);
  assert.match(html, new RegExp(escRe(r.target)));
  assert.match(html, new RegExp(escRe(remedyKindLabel(r.kind))));
  assert.match(html, /objective/);
  // A cheaper portfolio is a saving, and the sign must survive the render.
  const signed = r.objectiveDelta < 0 ? `−${Math.abs(r.objectiveDelta)}` : `+${r.objectiveDelta}`;
  assert.ok(html.includes(signed), `the objective delta ${signed} must appear`);
});

test('victims are named with their week deltas', () => {
  const withVictims = remedies.find((r) => (r.affectedInitiatives || []).length > 0);
  assert.ok(withVictims, 'the demo plan has at least one remedy with victims');
  const html = remedyRowHTML(withVictims);
  for (const v of withVictims.affectedInitiatives.slice(0, 3)) {
    assert.match(html, new RegExp(escRe(v.initiative)));
  }
});

test('a row with no victims says so, in one word, rather than nothing', () => {
  const bare = remedies.find((r) => !(r.affectedInitiatives || []).length);
  if (!bare) return; // not every plan has one; the demo plan usually does
  const html = remedyRowHTML(bare);
  assert.match(html, /no other initiatives move|nothing else moves/i);
});

test('a remedy row never leaks undefined for fields it does not have', () => {
  for (const r of remedies) {
    const html = remedyRowHTML(r);
    assert.ok(!html.includes('undefined'), `kind ${r.kind} leaked undefined`);
    assert.ok(!html.includes('NaN'), `kind ${r.kind} leaked NaN`);
  }
});

test('the panel carries the warnings, so a deferred remedy kind is never silent', () => {
  const html = remediesPanelHTML(remedies, warnings);
  assert.match(html, /transfer/i, 'the Q1 deferral note must stay visible');
  assert.match(html, new RegExp(escRe(remedies[0].target)));
});

test('the panel survives empty input without lying', () => {
  const html = remediesPanelHTML([], []);
  assert.ok(!html.includes('undefined'));
  assert.match(html, /no/i, 'it says there are no options, not a blank');
});

test('the expander appears only on a missed date', () => {
  const late = { name: 'Late one', verdict: 'late' };
  const stuck = { name: 'Stuck one', verdict: 'structurally-infeasible' };
  const fine = { name: 'Fine one', verdict: 'on-time' };
  const undated = { name: 'No date', verdict: 'no-date' };

  assert.match(optionsExpanderHTML(late), /options/);
  assert.match(optionsExpanderHTML(stuck), /options/);
  assert.equal(optionsExpanderHTML(fine), '', 'an on-time date has nothing to rescue');
  assert.equal(optionsExpanderHTML(undated), '', 'no-date is not a miss');
});

test('the expander carries the initiative name for the fetch wiring', () => {
  const html = optionsExpanderHTML({ name: 'Managed database MVP', verdict: 'late' });
  assert.match(html, /data-init="Managed database MVP"/);
});

test('the expander never leaks an initiative name into markup unsafely', () => {
  const html = optionsExpanderHTML({ name: '<img src=x onerror=alert(1)>', verdict: 'late' });
  assert.ok(!html.includes('<img'), 'the name must be escaped');
});

// The stale-binary seam: app/ is served from disk, routes are compiled in, so
// this page can be newer than the server. The expander's fetch will 405 and
// the copy must name the cause — the same trap the baselines panel hit once.
test('a 405 from an older server explains itself rather than echoing the body', () => {
  const msg = remediesErrorMessage(405, 'method');
  assert.match(msg, /restart|rebuild/i);
  assert.match(msg, /older/i);
  assert.ok(!/^method$/i.test(msg), 'never surface the raw server word');
});

test('a 405 keeps the route detail the server sent', () => {
  const msg = remediesErrorMessage(405, 'POST /api/plan/p1/schedule/remedies is not a route this server has.');
  assert.match(msg, /schedule\/remedies/);
});

test('a dead connection and a rejection read differently', () => {
  assert.match(remediesErrorMessage(0, ''), /reach the server/i);
  assert.match(remediesErrorMessage(500, 'boom'), /could not|failed|boom/i);
  assert.ok(!remediesErrorMessage(400, '<img src=x>').includes('<img'));
});

function escRe(s) {
  return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
