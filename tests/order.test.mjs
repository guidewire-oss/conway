import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  esc, zoneOf, weekLabel, verdictView, statedCell, objectiveView, wipLimitNote,
  orderRows, orderTableHTML, infeasibleNote, heatmapWeeks, overrunNote,
  podHeatmapHTML, podQueueHTML, noticesHTML, orderViewHTML, rowTraceHTML,
} from '../app/js/order.js';

// The fixture is real output from the Go scheduler for the demo plan, not a
// hand-written guess, so these tests fail if the two sides of the contract drift.
// Regenerate it the way tests/fixtures/README.md describes.
const sched = JSON.parse(readFileSync(new URL('./fixtures/schedule-demo.json', import.meta.url)));

test('the fixture is the shape the view expects', () => {
  assert.ok(sched.initiatives.length > 0);
  assert.ok(sched.podWeeks.length > 0);
  assert.equal(typeof sched.rule, 'string');
  assert.equal(typeof sched.wipLimit.derived, 'boolean');
});

test('zoneOf uses the app-wide utilization thresholds', () => {
  assert.equal(zoneOf(0), 'idle');
  assert.equal(zoneOf(0.5), 'green');
  assert.equal(zoneOf(0.84), 'green');
  assert.equal(zoneOf(0.85), 'amber');
  assert.equal(zoneOf(0.99), 'amber');
  assert.equal(zoneOf(1), 'red');
  assert.equal(zoneOf(2), 'red');
  assert.equal(zoneOf(Infinity), 'red', 'demand with no capacity is the hottest case, not a blank');
});

test('weekLabel is the wireframe form', () => {
  assert.equal(weekLabel(0), 'w0');
  assert.equal(weekLabel(17), 'w17');
});

test('verdictView carries the weeks late, not just the word late', () => {
  assert.deepEqual(verdictView({ verdict: 'late', weeksLate: 7 }),
    { symbol: '▲', text: 'late 7w', zone: 'red' });
  assert.equal(verdictView({ verdict: 'on-time' }).text, 'on time');
  assert.equal(verdictView({ verdict: 'no-date' }).zone, 'idle');
  assert.equal(verdictView({ verdict: 'unschedulable' }).symbol, '⚠');
});

test('verdictView marks a provisional verdict wherever it appears (AC 2.5)', () => {
  const v = verdictView({ verdict: 'on-time', provisional: true });
  assert.match(v.text, /provisional/);
  assert.equal(v.zone, 'green', 'provisional qualifies the verdict, it does not replace it');
});

test('verdictView never leaves colour as the only signal (FR-044)', () => {
  for (const verdict of ['on-time', 'at-risk', 'late', 'no-date', 'structurally-infeasible', 'unschedulable']) {
    const v = verdictView({ verdict, weeksLate: 2 });
    assert.ok(v.symbol && v.symbol.length > 0, `${verdict} has no symbol`);
    assert.ok(v.text && v.text.length > 0, `${verdict} has no text`);
  }
});

test('statedCell shows the move the engine made, and any lock', () => {
  assert.match(statedCell({ statedRank: 2, proposedRank: 1 }), /2 .*→1/);
  assert.match(statedCell({ statedRank: 2, proposedRank: 1 }), /ord-up/);
  assert.match(statedCell({ statedRank: 1, proposedRank: 3 }), /ord-down/);
  assert.equal(statedCell({ statedRank: 4, proposedRank: 4 }), '4', 'no arrow when nothing moved');
  assert.match(statedCell({ statedRank: 0, proposedRank: 2 }), /—/, 'unranked shows a dash');
  assert.match(statedCell({ statedRank: 3, proposedRank: 3, priorityLocked: true }), /locked/);
});

test('objectiveView prices the planner order against the proposal', () => {
  const v = objectiveView({
    statedOrderObjectiveScore: 14,
    objectiveScore: 3.5,
    initiatives: [{ targetWeek: 20, weeksLate: 3 }],
  });
  assert.equal(v.stated, 14);
  assert.equal(v.proposed, 3.5);
  assert.equal(v.delta, -10.5);
  assert.equal(v.better, true);
  assert.equal(v.comparable, true);
  assert.equal(v.allOnTime, false, 'something is late, so the scores are the story');
});

test('objectiveView does not claim a win when there is no order to compare', () => {
  const v = objectiveView({ statedOrderObjectiveScore: 0, objectiveScore: 0, initiatives: [] });
  assert.equal(v.comparable, false, 'a plan with no dates or priorities has nothing to argue about');
  assert.equal(v.delta, 0);
});

// The objective is weighted lateness, so a plan where every date holds scores 0 on
// both runs. Reading that as "no dates set" tells the planner the opposite of what
// just happened: they got everything they asked for.
test('objectiveView treats an all-on-time plan as comparable, not as undated', () => {
  const v = objectiveView({
    statedOrderObjectiveScore: 0,
    objectiveScore: 0,
    initiatives: [{ targetWeek: 12, verdict: 'on-time' }],
  });
  assert.equal(v.comparable, true, 'a date that held is still a date');
  assert.equal(v.allOnTime, true);
});

test('objectiveView is comparable when priorities exist without any dates', () => {
  const v = objectiveView({ initiatives: [{ statedRank: 1 }, { statedRank: 2 }] });
  assert.equal(v.comparable, true, 'a stated order can be argued with even undated');
});

test('wipLimitNote says whether the limit was derived, and from where (Decision 22)', () => {
  const derived = wipLimitNote({ value: 2, derived: true, fromPod: 'Delta' });
  assert.match(derived, /derived/);
  assert.match(derived, /Delta/);
  const explicit = wipLimitNote({ value: 5, derived: false });
  assert.match(explicit, /set on this plan/);
  assert.doesNotMatch(explicit, /derived/);
});

test('orderRows is sorted by proposed rank and carries each reason', () => {
  const rows = orderRows(sched);
  assert.equal(rows.length, sched.initiatives.length);
  for (let i = 1; i < rows.length; i++) {
    assert.ok(rows[i].si.proposedRank > rows[i - 1].si.proposedRank, 'rows must climb by rank');
  }
  const moved = rows.filter((r) => r.si.statedRank && r.si.statedRank !== r.si.proposedRank);
  assert.ok(moved.length > 0, 'the demo plan reorders something');
  for (const r of moved) {
    assert.ok(r.reason.length > 0, `${r.si.name} moved with no reason attached (FR-012)`);
  }
});

test('orderTableHTML renders one row per initiative, each traceable', () => {
  const html = orderTableHTML(sched);
  const rows = html.match(/class="ord-row"/g) || [];
  assert.equal(rows.length, sched.initiatives.length);
  for (const si of sched.initiatives) {
    assert.ok(html.includes(esc(si.name)), `${si.name} missing from the table`);
  }

  // Every row in this fixture carries ranking terms, so every row has a trace line;
  // the ones the engine moved additionally show their reason on it (FR-012).
  const whys = html.match(/class="ord-why"/g) || [];
  assert.equal(whys.length, sched.initiatives.length);
  for (const r of orderRows(sched)) {
    if (r.si.statedRank && r.si.statedRank !== r.si.proposedRank) {
      assert.ok(html.includes(esc(r.reason)), `${r.si.name} moved without its reason shown`);
    }
  }
  const traces = html.match(/class="ord-terms"/g) || [];
  assert.equal(traces.length, sched.initiatives.length, 'the arithmetic is available per row');
});

test('a row with nothing to explain gets no trace line', () => {
  const html = orderTableHTML({
    initiatives: [{ name: 'Bare', proposedRank: 1, startWeek: 0, commitWeek: 2, verdict: 'no-date' }],
  });
  assert.ok(!html.includes('ord-why'), 'no reason, no terms, no assumptions — no extra row');
  assert.equal(rowTraceHTML({ si: { name: 'Bare' }, reason: '' }), '');
});

test('orderTableHTML says something useful for an empty plan', () => {
  assert.match(orderTableHTML({ initiatives: [] }), /No initiatives/);
});

test('infeasibleNote lists the dates no ordering can meet, and only those', () => {
  const note = infeasibleNote(sched);
  const stuck = sched.initiatives.filter((si) => si.verdict === 'structurally-infeasible');
  assert.ok(stuck.length > 0, 'the demo plan has at least one impossible date');
  for (const si of stuck) assert.ok(note.includes(esc(si.name)));
  for (const si of sched.initiatives.filter((s) => s.verdict === 'late')) {
    assert.ok(!note.includes(esc(si.name)), `${si.name} is late, not infeasible (Decision 12)`);
  }
  assert.equal(infeasibleNote({ initiatives: [{ verdict: 'on-time' }] }), '');
});

test('heatmapWeeks bounds the grid to the period, not the whole overrun', () => {
  // The demo plan overruns badly: pods carry ~90 weeks of schedule against a
  // 26-week period, and drawing all of it would make every cell unreadable.
  assert.ok(sched.podWeeks[0].weeks.length > 26, 'fixture should overrun, or this proves nothing');
  assert.equal(heatmapWeeks(sched, 26), 26);
});

test('overrunNote counts what the grid does not show', () => {
  const note = overrunNote(sched, 26);
  assert.match(note, /commit after w26/);
  assert.equal(overrunNote({ initiatives: [{ commitWeek: 4 }] }, 26), '',
    'nothing to say when everything fits');
});

test('podHeatmapHTML never draws more tracks busy than a pod has', () => {
  const html = podHeatmapHTML(sched, 26);
  for (const ps of sched.podWeeks) {
    assert.ok(html.includes(`data-pod="${esc(ps.pod)}"`), `${ps.pod} missing from the heatmap`);
    for (const wk of ps.weeks.slice(0, 26)) {
      assert.ok(wk.busy <= ps.tracks,
        `${ps.pod} w${wk.week} shows ${wk.busy} busy on ${ps.tracks} tracks`);
    }
  }
  const cells = html.match(/class="ord-cell/g) || [];
  assert.equal(cells.length, sched.podWeeks.length * 26, 'one cell per pod per week of the period');
});

test('podHeatmapHTML marks the drum and puts the count in the cell (FR-044)', () => {
  const html = podHeatmapHTML(sched, 26);
  assert.match(html, /class="tag">drum/);
  assert.match(html, /class="ord-cell ord-red"[^>]*>[1-9]/, 'a red cell still shows its number');
});

test('podQueueHTML lists a pod queue in start order with its waits (AC 4.3)', () => {
  const drum = sched.wipLimit.fromPod;
  const html = podQueueHTML(sched, drum);
  const ps = sched.podWeeks.find((p) => p.pod === drum);
  assert.ok(ps.slices.length > 1, 'the drum should have a queue worth showing');
  let last = -1;
  for (const s of ps.slices.slice().sort((a, b) => a.startWeek - b.startWeek)) {
    assert.ok(html.includes(esc(s.initiative)));
    assert.ok(s.startWeek >= last, 'slices listed in scheduled start order');
    last = s.startWeek;
  }
  assert.equal(podQueueHTML(sched, 'Nonexistent pod'), '', 'an unknown pod renders nothing');
});

test('noticesHTML surfaces warnings and assumptions (FR-021)', () => {
  const html = noticesHTML({ warnings: ['Ghost has no capacity'], assumptions: ['broke a cycle'] });
  assert.match(html, /Ghost has no capacity/);
  assert.match(html, /1 assumption/);
  assert.equal(noticesHTML({}), '', 'a clean schedule adds no noise');
});

test('orderViewHTML composes without leaving undefined in the markup', () => {
  const html = orderViewHTML(sched, { horizonWeeks: 26 });
  assert.ok(!html.includes('undefined'), 'a missing field leaked into the page');
  assert.ok(!html.includes('NaN'));
  assert.match(html, /Execution order/);
  assert.match(html, /Pod load, week by week/);
});

test('orderViewHTML shows a pod queue only when a pod is selected', () => {
  assert.ok(!orderViewHTML(sched, { horizonWeeks: 26 }).includes('ord-queue'));
  assert.match(orderViewHTML(sched, { horizonWeeks: 26, pod: 'Delta' }), /ord-queue/);
});

// Initiative names come from a spreadsheet, so they reach this lookup as
// user-controlled keys. A plain object would let "__proto__" reach the prototype.
test('a lookup keyed by initiative name cannot be poisoned', () => {
  const rows = orderRows({
    initiatives: [{ name: '__proto__', proposedRank: 1, statedRank: 2, startWeek: 0, commitWeek: 3, verdict: 'no-date' }],
    reconciliation: [{ initiative: '__proto__', reason: 'moved' }],
  });
  assert.equal(rows.length, 1);
  assert.equal(rows[0].reason, 'moved', 'the reason for a __proto__ row must still be found');
  assert.equal({}.polluted, undefined);
  assert.equal(Object.prototype.polluted, undefined);
});

// AC 2.1 wants both numbers shown: the raw finish is the schedule, the commit is
// the promise, and Decision 9 exists because the two are not the same claim.
test('a row shows the raw finish as well as the buffered commit', () => {
  const html = orderTableHTML({
    initiatives: [{
      name: 'Both numbers', proposedRank: 1, startWeek: 1,
      rawFinishWeek: 14, bufferWeeks: 3, commitWeek: 17, verdict: 'no-date',
    }],
  });
  assert.match(html, /w14/, 'the raw finish week must appear');
  assert.match(html, /w17/, 'so must the buffered commit');
});

// FR-021: the inputs that produced a position have to be reportable, not just the
// binding constraint. Decision 2 chose a formula with named terms for this reason.
test('a row can be traced back to the terms that ranked it', () => {
  const html = orderTableHTML({
    initiatives: [{
      name: 'Traceable', proposedRank: 1, startWeek: 0, rawFinishWeek: 6, commitWeek: 8,
      verdict: 'on-time', targetWeek: 10,
      rankingTerms: { weight: 36, constraintWeeks: 6, slackWeeks: 4, index: 3.2, rule: 'tardiness-cost' },
      assumptions: ['broke a cycle at A -> B'],
      unestimatedPods: ['Delta'],
      provisional: true,
    }],
  });
  assert.match(html, /36/, 'the delay weight');
  assert.match(html, /3\.2/, 'the index that ranked it');
  assert.match(html, /broke a cycle/, 'its assumptions');
  assert.match(html, /Delta/, 'the pods with no estimate behind a provisional verdict');
});

// AC 4.3 is about the rendered order, so assert the rendered order. Sorting the
// input and then checking it is sorted proves nothing about the markup.
test('podQueueHTML renders the queue in start order, checked in the markup', () => {
  const sched = {
    podWeeks: [{
      pod: 'Delta', tracks: 1,
      slices: [
        { initiative: 'Third', startWeek: 9, finishWeek: 12, remainingWeeks: 3, waitWeeks: 4 },
        { initiative: 'First', startWeek: 0, finishWeek: 4, remainingWeeks: 4, waitWeeks: 0 },
        { initiative: 'Second', startWeek: 4, finishWeek: 9, remainingWeeks: 5, waitWeeks: 1 },
      ],
    }],
  };
  const html = podQueueHTML(sched, 'Delta');
  const order = ['First', 'Second', 'Third'].map((n) => html.indexOf(n));
  assert.ok(order.every((i) => i > -1), 'every slice is rendered');
  assert.deepEqual(order.slice().sort((a, b) => a - b), order,
    'rendered rows must climb by start week');
});

test('podQueueHTML breaks a start-week tie the way the server does', () => {
  const sched = {
    podWeeks: [{
      pod: 'Delta', tracks: 2,
      slices: [
        { initiative: 'Zulu', startWeek: 3, finishWeek: 6, remainingWeeks: 3, waitWeeks: 0 },
        { initiative: 'Alpha', startWeek: 3, finishWeek: 5, remainingWeeks: 2, waitWeeks: 0 },
      ],
    }],
  };
  const html = podQueueHTML(sched, 'Delta');
  assert.ok(html.indexOf('Alpha') < html.indexOf('Zulu'),
    'ties break on initiative name, matching podSchedules in schedule.go');
});

// A pod selector that only responds to a mouse is unusable by keyboard and opaque
// to assistive tech; an anchor with no href is not focusable at all.
test('the pod selector is a real button', () => {
  const html = podHeatmapHTML(sched, 26);
  assert.match(html, /<button[^>]*class="ord-podlink"[^>]*type="button"|<button[^>]*type="button"[^>]*class="ord-podlink"/,
    'pods must be reachable by keyboard');
  assert.ok(!/<a class="ord-podlink"/.test(html), 'no href-less anchors as controls');
});

test('esc closes the injection hole an initiative name could open', () => {
  const nasty = '<img src=x onerror=alert(1)>';
  const html = orderTableHTML({
    initiatives: [{ name: nasty, proposedRank: 1, startWeek: 0, commitWeek: 3, verdict: 'no-date' }],
  });
  assert.ok(!html.includes('<img'), 'an initiative name is user input from a spreadsheet');
  assert.match(html, /&lt;img/);
});

test('the view survives a schedule with nothing in it', () => {
  const html = orderViewHTML({}, {});
  assert.ok(!html.includes('undefined'));
  assert.match(html, /No initiatives/);
});
