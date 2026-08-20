import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  esc, zoneOf, weekLabel, verdictView, statedCell, objectiveView, wipLimitNote,
  orderRows, orderTableHTML, infeasibleNote, heatmapWeeks, overrunNote,
  podHeatmapHTML, podQueueHTML, noticesHTML, orderViewHTML,
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
  const v = objectiveView({ statedOrderObjectiveScore: 14, objectiveScore: 3.5 });
  assert.equal(v.stated, 14);
  assert.equal(v.proposed, 3.5);
  assert.equal(v.delta, -10.5);
  assert.equal(v.better, true);
  assert.equal(v.comparable, true);
});

test('objectiveView does not claim a win when there is no order to compare', () => {
  const v = objectiveView({ statedOrderObjectiveScore: 0, objectiveScore: 0 });
  assert.equal(v.comparable, false, 'a plan with no dates or priorities has nothing to argue about');
  assert.equal(v.delta, 0);
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

test('orderTableHTML renders one row per initiative plus a reason line for each move', () => {
  const html = orderTableHTML(sched);
  const rows = html.match(/class="ord-row"/g) || [];
  assert.equal(rows.length, sched.initiatives.length);
  const whys = html.match(/class="ord-why"/g) || [];
  const moved = sched.initiatives.filter((si) => si.statedRank && si.statedRank !== si.proposedRank);
  assert.equal(whys.length, moved.length);
  for (const si of sched.initiatives) {
    assert.ok(html.includes(esc(si.name)), `${si.name} missing from the table`);
  }
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
