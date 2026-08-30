import test from 'node:test';
import assert from 'node:assert/strict';
import {
  fitSentence, verdictSectionHTML, capacitySectionHTML,
  conflictsSectionHTML, remediesSectionHTML, healthReportHTML,
} from '../app/js/report.js';

// Spec 013: the plan health report. The card is a SUMMARIZING LENS (Decision
// 1): every function here takes the rendered schedule / remedies response and
// only aggregates — counts, sorting, top-N. No verdict, rho, or schedule is
// ever computed here, which is what makes the card unable to contradict the
// Order view.

const sched = {
  initiatives: [
    { name: 'Alpha rollout', verdict: 'on-time' },
    { name: 'Beta migration', verdict: 'beyond-horizon', weeksLate: 12 },
    { name: 'Gamma cutover', verdict: 'beyond-horizon' },
    { name: 'Delta portal', verdict: 'structurally-infeasible' },
    { name: 'Epsilon ui', verdict: 'no-date' },
  ],
  podWeeks: [
    { pod: 'Atlas', flatRho: 1.2, tracks: 3 },
    { pod: 'Beacon', flatRho: 0.9, tracks: 2 },
    { pod: 'Cascade', flatRho: 0.4, tracks: 4 },
  ],
  drumPods: ['Atlas'],
  conflicts: [{ a: 'Beta migration', b: 'Gamma cutover', pod: 'Atlas', note: 'both date-locked; contend for Atlas in weeks 4-9' }],
  rule: 'critical-path-first', objectiveScore: 430,
  horizonWeeks: 26, periodStart: '2026-09-01',
};

test('the fit sentence states the headline (AC 1.2)', () => {
  assert.equal(fitSentence(sched), '3 of 5 initiatives will not finish inside the period.');
  const green = fitSentence({ initiatives: [{ name: 'A', verdict: 'on-time' }] });
  assert.equal(green, 'All 1 initiatives commit inside the period.');
  assert.equal(fitSentence({ initiatives: [] }), 'No initiatives are scheduled yet.');
});

test('verdict counts list problem initiatives by name, with lateness (AC 1.1)', () => {
  const html = verdictSectionHTML(sched);
  assert.match(html, /<b>2<\/b> past the horizon: Beta migration \(\+12w\), Gamma cutover/);
  assert.match(html, /<b>1<\/b> structurally infeasible: Delta portal/);
  assert.match(html, /<b>1<\/b> on time/);
  assert.match(html, /<b>1<\/b> no target date/);
});

test('capacity names over-capacity and hot pods, hottest first, drums marked (AC 1.3)', () => {
  const html = capacitySectionHTML(sched);
  assert.match(html, /Over capacity/);
  assert.match(html, /<b>Atlas<\/b> ρ 1\.20 · 3 tracks · <b>drum<\/b>/);
  const hotIdx = html.indexOf('Queue hot'), atlasIdx = html.indexOf('Atlas');
  assert.match(html, /<b>Beacon<\/b> ρ 0\.90 · 2 tracks/);
  assert.ok(atlasIdx < hotIdx, 'over-capacity lists before hot');
});

test('a comfortable plan says so instead of an empty list', () => {
  const html = capacitySectionHTML({
    podWeeks: [{ pod: 'Atlas', flatRho: 0.5, tracks: 2 }], drumPods: [],
  });
  assert.match(html, /Every pod is comfortably inside capacity\./);
});

test('conflicts render with pod and note, or their absence is stated (AC 1.4)', () => {
  const withC = conflictsSectionHTML(sched);
  assert.match(withC, /<b>Beta migration<\/b> \+ <b>Gamma cutover<\/b> on Atlas/);
  assert.match(withC, /both date-locked/);
  const none = conflictsSectionHTML({ conflicts: [] });
  assert.match(none, /No date-locked initiatives contend/);
});

test('top remedies sort by portfolio improvement, cap at 3, name their cost (AC 1.5)', () => {
  const data = { remedies: [
    { kind: 'add-capacity', target: 'Beta migration', resultingVerdict: 'on-time', objectiveDelta: -40, affectedInitiatives: [{}, {}] },
    { kind: 'descope', target: 'Gamma cutover', resultingVerdict: 'late', objectiveDelta: -15, affectedInitiatives: [] },
    { kind: 'relax-date', target: 'Delta portal', resultingVerdict: 'on-time', objectiveDelta: -99 },
    { kind: 'raise-priority', target: 'Epsilon ui', resultingVerdict: 'late', objectiveDelta: -3 },
    { kind: 'defer-other', target: 'Zeta', resultingVerdict: 'late', objectiveDelta: 5 },
  ] };
  const html = remediesSectionHTML(data);
  const order = ['Delta portal', 'Beta migration', 'Gamma cutover'];
  const idx = order.map((n) => html.indexOf(n));
  assert.ok(idx.every((i) => i > -1) && idx[0] < idx[1] && idx[1] < idx[2], 'best improvement first');
  assert.ok(!html.includes('Epsilon ui') && !html.includes('Zeta'), 'capped at the top 3');
  assert.match(html, /moves 2 other initiatives/, 'the victims are named as a count');
  assert.match(html, /portfolio −99\.0/);
});

test('a failed remedies fetch degrades to a named note (NFR-003)', () => {
  assert.match(remediesSectionHTML({ error: 'the engine is unreachable' }), /the engine is unreachable/);
  assert.match(remediesSectionHTML(null), /computing remedies/);
});

test('the card names the active baseline, or the lack of one (AC 2.2)', () => {
  const withBl = healthReportHTML(sched, { baselines: [{ name: 'Kickoff', active: true, savedAt: 1756500000 }] });
  assert.match(withBl, /Baseline: <b>Kickoff<\/b>/);
  const noBl = healthReportHTML(sched, { baselines: [] });
  assert.match(noBl, /No baseline saved/);
});

test('the card carries the fit sentence, rule, and generated-at (FR-003, FR-011)', () => {
  const html = healthReportHTML(sched, { planName: 'Portfolio', generatedAt: '2026-08-30 10:00' });
  assert.match(html, /3 of 5 initiatives will not finish inside the period\./);
  assert.match(html, /Dispatch rule: critical-path-first · portfolio objective 430/);
  assert.match(html, /generated 2026-08-30 10:00/);
  assert.match(html, /period starts 2026-09-01, horizon 26w/);
});

test('a plan without a schedule states that and offers nothing else (FR-010)', () => {
  const html = healthReportHTML(null);
  assert.match(html, /No schedule yet/);
  assert.ok(!html.includes('Verdicts'), 'no sections render');
});
