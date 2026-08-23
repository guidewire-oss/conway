import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  verdictBannerHTML, comparisonBarsHTML,
} from '../app/js/order.js';

const sched = JSON.parse(readFileSync(new URL('./fixtures/schedule-demo.json', import.meta.url)));

// IA #2: the verdict banner is the FIRST thing a reader sees — one sentence a
// VP can act on before any machinery. It must state how many dated initiatives
// miss, under the best order found, and name the worst case.
test('the verdict banner counts the misses and names the worst case', () => {
  const html = verdictBannerHTML(sched);
  assert.match(html, /verdict-banner/);
  // how many dated initiatives miss (computed from the fixture, not hardcoded)
  const dated = sched.initiatives.filter((i) => i.targetWeek !== null && i.targetWeek !== undefined);
  const missing = dated.filter((i) => i.verdict === 'late' || i.verdict === 'structurally-infeasible');
  assert.ok(missing.length > 0, 'fixture must have misses for this spec');
  assert.match(html, new RegExp(`${missing.length} of ${dated.length} dated initiatives miss`));
  // worst case named: the largest weeksLate among dated
  const worst = dated.reduce((a, b) => (b.weeksLate > a.weeksLate ? b : a));
  assert.match(html, new RegExp(worst.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
});

test('a clean plan reads as clean', () => {
  const fine = JSON.parse(JSON.stringify(sched));
  fine.initiatives.forEach((i) => { i.verdict = 'on-time'; i.weeksLate = 0; });
  const html = verdictBannerHTML(fine);
  assert.match(html, /every dated initiative holds/i);
});

test('the banner carries no jargon without a glossary affordance', () => {
  const html = verdictBannerHTML(sched);
  // The banner's plain sentence is the point — where it uses a domain term,
  // the shared glossary affordance rides along.
  if (html.includes('cannot fit')) assert.match(html, /term-tip/);
});

// The yours-vs-proposed comparison becomes two bars, not arithmetic. Numbers
// stay (WCAG 1.4.1: bars must carry their values, not just length).
test('the comparison renders both bars with their numbers', () => {
  const obj = { comparable: true, stated: 100, proposed: 40, delta: -60, better: true };
  const html = comparisonBarsHTML(obj);
  assert.match(html, /ord-bars/);
  assert.match(html, /yours/i);
  assert.match(html, /proposed/i);
  assert.match(html, /100/);
  assert.match(html, /40/);
  // bar widths derive from the values: proposed/stated ratio
  assert.match(html, /width:\s*40%/);
});

test('uncomparable plans get no bars, just the plain-language note', () => {
  const html = comparisonBarsHTML({ comparable: false });
  assert.ok(!html.includes('ord-bars'));
  assert.match(html, /no dates|no priorities/i);
});

test('zero-cost plans do not divide by zero', () => {
  const html = comparisonBarsHTML({ comparable: true, stated: 0, proposed: 0, delta: 0 });
  assert.ok(!html.includes('NaN') && !html.includes('Infinity'));
});
