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

test('an unschedulable dated initiative is a miss, not a silent pass (the every-holds guard)', () => {
  const odd = JSON.parse(JSON.stringify(sched));
  // one dated row unschedulable (weeksLate omitted as the server does), rest on-time
  const dated = odd.initiatives.filter((i) => i.targetWeek !== null && i.targetWeek !== undefined);
  odd.initiatives.forEach((i) => { i.verdict = 'on-time'; i.weeksLate = 0; });
  dated[0].verdict = 'unschedulable'; dated[0].weeksLate = 0;
  const html = verdictBannerHTML(odd);
  assert.match(html, /1 of \d+ dated initiatives miss/);
  assert.match(html, /cannot be scheduled as entered/);
  assert.ok(!html.includes('undefined'), 'no undefined weeks in the banner');
});

test('the worst case names a miss, never an on-time row', () => {
  const mixed = JSON.parse(JSON.stringify(sched));
  const dated = mixed.initiatives.filter((i) => i.targetWeek !== null && i.targetWeek !== undefined);
  mixed.initiatives.forEach((i) => { i.verdict = 'on-time'; delete i.weeksLate; });
  const lateOne = dated[1];
  lateOne.verdict = 'late'; lateOne.weeksLate = 5;
  const html = verdictBannerHTML(mixed);
  assert.match(html, new RegExp(lateOne.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
  assert.ok(!html.includes('undefinedw'), 'no undefined leaked into the weeks');
});

test('a miss banner carries the glossary affordance unconditionally', () => {
  // The banner's worst-case clause names a verdict; the shared glossary
  // affordance rides along whenever there IS a miss.
  const html = verdictBannerHTML(sched);
  assert.match(html, /verdict-miss/);
  assert.match(html, /term-tip/);
});

// The yours-vs-proposed comparison becomes two bars, not arithmetic. Numbers
// stay (WCAG 1.4.1: bars must carry their values, not just length).
test('the comparison renders both bars with their numbers', () => {
  const obj = { comparable: true, stated: 100, proposed: 40, delta: -60, better: true };
  const html = comparisonBarsHTML(obj);
  assert.match(html, /ord-bars/);
  assert.match(html, /yours/i);
  assert.match(html, /proposed/i);
  // values ride ON the bars (WCAG 1.4.1) — assert them inside .ord-bar-val,
  // not anywhere in the markup.
  const vals = [...html.matchAll(/class="ord-bar-val">([^<]+)</g)].map((m) => m[1].trim());
  assert.deepEqual(vals, ['100', '40']);
  // bar widths derive from the values: proposed/stated ratio
  assert.match(html, /width:\s*40%/);
});

test('uncomparable plans get no bars, just the plain-language note', () => {
  const html = comparisonBarsHTML({ comparable: false });
  assert.ok(!html.includes('ord-bars'));
  assert.match(html, /no dates|no priorities/i);
});

test('zero-cost plans get the plain-language fallback, not zero-value bars', () => {
  const html = comparisonBarsHTML({ comparable: true, stated: 0, proposed: 0, delta: 0 });
  assert.ok(!html.includes('ord-bars'), 'no bars when both orders cost nothing');
  // The copy must not claim 'every date holds' — comparable covers priority-only
  // plans where no date can be missed either.
  assert.match(html, /nothing is late/i);
  assert.ok(!html.includes('every date holds'), 'no unsupported all-held claim');
  assert.ok(!html.includes('NaN') && !html.includes('Infinity'));
});
