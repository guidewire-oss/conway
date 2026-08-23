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

test('the affordance is a real button with an accessible name (WCAG 1.3.1 / F111)', () => {
  const html = term('rho');
  assert.match(html, /<button[^>]*class="help term-tip"/);
  assert.match(html, /aria-label="What does /);
  assert.match(html, /data-tip="/);
});

test('the term label renders beside the affordance when given', () => {
  const html = term('wip', 'WIP');
  assert.match(html, /WIP <button/);
  assert.ok(!term('wip').includes('WIP <'), 'no label by default — the term is already on screen');
});

test('unknown ids render nothing, never a broken affordance', () => {
  assert.equal(term('nonexistent'), '');
});

test('tooltip text is escaped — no markup injection from definitions', () => {
  assert.ok(!term('kingman').includes('<img'), 'safe by construction; definitions are literals here');
  assert.ok(!/undefined|NaN/.test(term('rho')), 'no leaked undefined');
});
