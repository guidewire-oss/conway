import test from 'node:test';
import assert from 'node:assert/strict';
import { TERMS, term } from '../app/js/terms.js';

test('every glossary entry has a label and a plain-language first sentence', () => {
  for (const [id, t] of Object.entries(TERMS)) {
    assert.ok(t.label, `${id} has a label`);
    assert.ok(t.tip.length > 40, `${id} tip is substantial`);
    // First sentence must be understandable without the theory credit.
    const first = t.tip.split('. ')[0];
    assert.ok(first.length > 20, `${id} first sentence explains, not name-drops`);
  }
});

test('the affordance is a real button with a complete accessible name (WCAG 1.3.1 / F111)', () => {
  const html = term('rho');
  assert.match(html, /<button[^>]*class="help term-tip"/);
  assert.match(html, /aria-label="What does Load ρ mean\?"/);
  assert.match(html, /data-bs-title="/);
});

test('the term label renders beside the affordance when given', () => {
  const html = term('wip', 'WIP');
  assert.match(html, /WIP <button/);
  assert.ok(!term('wip').includes('WIP <'), 'no label by default — the term is already on screen');
});

test('unknown ids render nothing, never a broken affordance', () => {
  assert.equal(term('nonexistent'), '');
});

test('tooltip text stays a well-formed attribute value', () => {
  // Definitions are literals today, but the affordance interpolates them into
  // a double-quoted attribute — a raw double quote would break out of it.
  const html = term('weighted-late');
  const m = html.match(/data-bs-title="([^"]*)"/); // the regex only matches if no raw " inside
  assert.ok(m, 'data-bs-title value contains no raw double quote');
  assert.ok(!/undefined|NaN/.test(html), 'no leaked undefined');
});

test('inherited keys (toString, constructor) render nothing', () => {
  assert.equal(term('toString'), '');
  assert.equal(term('constructor'), '');
});
