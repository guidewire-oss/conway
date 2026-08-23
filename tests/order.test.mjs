import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  esc, zoneOf, weekLabel, verdictView, statedCell, objectiveView, wipLimitNote,
  orderRows, orderTableHTML, infeasibleNote, heatmapWeeks, overrunNote,
  podHeatmapHTML, podQueueHTML, noticesHTML, orderViewHTML, rowTraceHTML,
  schedulingFormHTML, schedulingFromForm, pctToFraction,
  wipModelsTableHTML, WIP_MODELS,
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

// allOnTime drives a header that makes an absolute claim, so every way of not
// being all-on-time has to defeat it.
test('allOnTime requires dated rows that all actually held', () => {
  const dated = (over) => ({ statedOrderObjectiveScore: 0, objectiveScore: 0, ...over });

  assert.equal(objectiveView(dated({ initiatives: [{ statedRank: 1 }] })).allOnTime, false,
    'priorities without dates: there is no date to hold');

  assert.equal(objectiveView(dated({
    initiatives: [{ targetWeek: 8, verdict: 'unschedulable' }],
  })).allOnTime, false, 'unschedulable is not on time, and it carries no weeksLate');

  assert.equal(objectiveView({
    statedOrderObjectiveScore: 12, objectiveScore: 0,
    initiatives: [{ targetWeek: 8, verdict: 'on-time' }],
  }).allOnTime, false, 'the stated order was late, so not every date holds under either order');

  assert.equal(objectiveView(dated({
    initiatives: [{ targetWeek: 8, verdict: 'on-time' }, { statedRank: 2 }],
  })).allOnTime, true, 'every dated row on time, both orders costing nothing');
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

test('orderViewHTML includes the form when given assumptions, and omits it otherwise', () => {
  assert.match(orderViewHTML(sched, { horizonWeeks: 26, scheduling: {} }), /ord-sched/);
  assert.ok(!orderViewHTML(sched, { horizonWeeks: 26 }).includes('ord-sched'),
    'undefined scheduling means the caller does not want the form');
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

// The trace escapes once. Escaping twice shows the reader "R&amp;D".
test('a pod name with markup characters is escaped exactly once', () => {
  const html = rowTraceHTML({
    si: { name: 'x', unestimatedPods: ['R&D', '<Ops>'], rankingTerms: { weight: 4, index: 1 } },
    reason: '',
  });
  assert.match(html, /R&amp;D/);
  assert.ok(!html.includes('&amp;amp;'), 'double-escaped: the entity itself is being displayed');
  assert.match(html, /&lt;Ops&gt;/);
  assert.ok(!html.includes('<Ops>'), 'and it must still be escaped');
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

// Go compares strings byte-wise, so "Zulu" sorts before "alpha" ('Z' is 0x5A,
// 'a' is 0x61). localeCompare would put them the other way round, which is the
// opposite of the server's order the comment claims to mirror.
// JavaScript's `<` compares UTF-16 code units, Go's compares UTF-8 bytes, and they
// disagree wherever a surrogate pair meets a BMP character above U+E000. U+F900
// encodes as EF A4 80 and an emoji as F0 9F 98 80, so Go puts U+F900 first; in
// UTF-16 the emoji is D83D DE00, which sorts before F900. Opposite answers.
test('the tie-break compares UTF-8 bytes, so astral names land where the server put them', () => {
  const html = podQueueHTML({
    podWeeks: [{
      pod: 'Delta', tracks: 2,
      slices: [
        { initiative: 'A\u{1F600} emoji', startWeek: 2, finishWeek: 4, remainingWeeks: 2, waitWeeks: 0 },
        { initiative: 'A\u{F900} compat', startWeek: 2, finishWeek: 5, remainingWeeks: 3, waitWeeks: 0 },
      ],
    }],
  }, 'Delta');
  assert.ok(html.indexOf('compat') < html.indexOf('emoji'),
    'U+F900 precedes an astral character in UTF-8, which is what Go compares');
});

test('the tie-break compares bytes, the way Go does, not by locale', () => {
  const html = podQueueHTML({
    podWeeks: [{
      pod: 'Delta', tracks: 2,
      slices: [
        { initiative: 'alpha release', startWeek: 3, finishWeek: 5, remainingWeeks: 2, waitWeeks: 0 },
        { initiative: 'Zulu release', startWeek: 3, finishWeek: 6, remainingWeeks: 3, waitWeeks: 0 },
      ],
    }],
  }, 'Delta');
  assert.ok(html.indexOf('Zulu release') < html.indexOf('alpha release'),
    'uppercase sorts first byte-wise, matching sort.SliceStable in schedule.go');
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

// --- scheduling assumptions form ------------------------------------------
// A form reader keyed the way the DOM would answer, so the collection logic is
// testable without a browser.
const reader = (vals) => (id) => (id in vals ? vals[id] : '');

test('the form shows the current assumptions as percentages a person types', () => {
  const html = schedulingFormHTML({
    periodStart: '2026-01-05', bufferPct: 0.25, kitGate: 0.8,
    maxConcurrentInitiatives: 4, maxInitiativesPerPod: 2, maxStartsPerQuarter: 3,
  }, { value: 4, derived: false });
  assert.match(html, /value="2026-01-05"/);
  assert.match(html, /value="25"/, 'a 0.25 fraction is shown as 25%');
  assert.match(html, /value="80"/);
  assert.match(html, /value="4"/);
});

// The one thing a planner cannot recover from on their own: no period start means
// no dates anywhere, and nothing else in the view explains why.
test('the form opens itself and says so when the plan has no period start', () => {
  const html = schedulingFormHTML({}, { value: 2, derived: true, fromPod: 'Delta' });
  assert.match(html, /<details class="ord-sched" open>/);
  assert.match(html, /no period start/);
  assert.match(html, /derived: 2 from Delta/, 'the blank WIP field says what blank does');
});

// The panel forces itself open for anything the planner must decide, and there are
// two such things: the period start and the WIP model. Both settled, both quiet.
test('the form does not open itself once nothing is outstanding', () => {
  const html = schedulingFormHTML({ periodStart: '2026-01-05', wipModel: 'strict' },
    { value: 2, derived: true, fromPod: 'Delta', model: 'strict' });
  assert.ok(!html.includes('ord-sched" open'));
  assert.ok(!html.includes('no period start'));
  assert.ok(!html.includes('No work-in-progress model chosen'));
});

test('the form still opens for a missing period start even with a model chosen', () => {
  const html = schedulingFormHTML({ wipModel: 'strict' }, { value: 2, model: 'strict' });
  assert.match(html, /<details class="ord-sched" open>/);
  assert.match(html, /no period start/);
});

test('the form offers no knob that does nothing', () => {
  const html = schedulingFormHTML({}, {});
  for (const dead of ['targetUtilization', 'leadCapacity', 'allowTransfers', 'transferRampWeeks', 'lookaheadK']) {
    assert.ok(!html.includes(dead), `${dead} has no implementation behind it yet`);
  }
});

// schedulingFormHTML is exported and takes whatever the stored policy blob holds,
// so its values are not guaranteed to be the numbers §7 describes.
test('a hostile value cannot break out of the value attribute', () => {
  const html = schedulingFormHTML({
    periodStart: '" onfocus=alert(1) x="',
    maxConcurrentInitiatives: '"><img src=x onerror=alert(1)>',
    maxInitiativesPerPod: '"><script>bad()</script>',
  }, {});
  assert.ok(!html.includes('<img'), 'markup injected through a value attribute');
  assert.ok(!html.includes('<script'), 'script injected through a value attribute');
  // The quotes are what would end the attribute early, so the property to assert is
  // that they arrive escaped. The rest of the payload is then inert text.
  assert.match(html, /value="&quot; onfocus=alert\(1\) x=&quot;"/,
    'the quotes must be escaped, so nothing escapes the attribute');
  assert.ok(!/value="[^"]*"[a-z]/i.test(html.replace(/&quot;/g, '')),
    'no attribute begins immediately after a value attribute closes');
});

// These are controls, not submitters. Inside a <form> a bare button defaults to
// type="submit" and would navigate.
test('the form controls are explicitly type=button', () => {
  const html = schedulingFormHTML({ periodStart: '2026-01-05' }, {});
  const buttons = html.match(/<button[^>]*>/g) || [];
  // save, cancel, and the calendar windows' add button (FR-018).
  assert.equal(buttons.length, 3);
  for (const b of buttons) {
    assert.match(b, /type="button"/, `missing type: ${b}`);
  }
});

test('a blank field is omitted so the server default applies', () => {
  const body = schedulingFromForm(reader({ 'sched-period-start': '2026-01-05' }));
  assert.deepEqual(body, { periodStart: '2026-01-05' });
  assert.ok(!('bufferPct' in body), 'absent bufferPct is how the 25% default is chosen');
  assert.ok(!('maxConcurrentInitiatives' in body), 'absent WIP is how the drum derivation is chosen');
});

// Decision 20 left an explicit 0 meaning "commit on the raw finish". Collapsing it
// into blank would silently remove that choice.
test('an explicit zero buffer is sent, unlike a blank one', () => {
  const zero = schedulingFromForm(reader({ 'sched-buffer': '0' }));
  assert.equal(zero.bufferPct, 0);
  assert.ok('bufferPct' in zero);

  const blank = schedulingFromForm(reader({ 'sched-buffer': '' }));
  assert.ok(!('bufferPct' in blank));
});

test('percentages become the 0..1 fractions §7 stores', () => {
  const body = schedulingFromForm(reader({ 'sched-buffer': '25', 'sched-kit': '80' }));
  assert.equal(body.bufferPct, 0.25);
  assert.equal(body.kitGate, 0.8);
  assert.equal(pctToFraction('150'), 1, 'clamped, not stored as 1.5');
  assert.equal(pctToFraction('-5'), 0);
  assert.equal(pctToFraction('abc'), null, 'unreadable is absent, never zero');
});

test('a zero or blank limit is omitted, because §7 spells no-limit as absent', () => {
  for (const v of ['', '0']) {
    const body = schedulingFromForm(reader({ 'sched-wip': v, 'sched-pod-wip': v, 'sched-quarter': v }));
    assert.ok(!('maxConcurrentInitiatives' in body), `wip "${v}" should be absent`);
    assert.ok(!('maxInitiativesPerPod' in body), `per-pod "${v}" should be absent`);
    assert.ok(!('maxStartsPerQuarter' in body), `quarter "${v}" should be absent`);
  }
});

test('limits are rounded to whole initiatives', () => {
  const body = schedulingFromForm(reader({ 'sched-wip': '3.6' }));
  assert.equal(body.maxConcurrentInitiatives, 4, 'you cannot run 3.6 initiatives');
});

test('the form round-trips through its own reader', () => {
  const saved = {
    periodStart: '2026-02-02', bufferPct: 0.3, kitGate: 0.75,
    maxConcurrentInitiatives: 5, maxInitiativesPerPod: 2, maxStartsPerQuarter: 4,
  };
  const html = schedulingFormHTML(saved, { value: 5, derived: false });
  // Read the values back out of the rendered markup, the way the DOM would.
  const valueOf = (id) => {
    const m = html.match(new RegExp(`id="${id}"[^>]*value="([^"]*)"`));
    return m ? m[1] : '';
  };
  assert.deepEqual(schedulingFromForm(valueOf), saved, 'what it renders is what it sends');
});

// --- the WIP model choice (§11 D22 amended) --------------------------------
const modelsFixture = {
  wipLimit: { value: 2, derived: true, fromPod: 'Delta', model: 'unchosen' },
  wipModels: [
    { model: 'strict', limit: 2, lastCommitWeek: 96, datesMissed: 5, late: 2, infeasible: 3, podsIdleAllPeriod: 3, objective: 4673 },
    { model: 'drum-gated', limit: 2, lastCommitWeek: 55, datesMissed: 5, late: 2, infeasible: 3, podsIdleAllPeriod: 1, objective: 1466 },
    { model: 'off', limit: 0, lastCommitWeek: 43, datesMissed: 5, late: 2, infeasible: 3, podsIdleAllPeriod: 1, objective: 1155 },
  ],
};

test('the comparison shows every model with its cost for this plan', () => {
  const html = wipModelsTableHTML(modelsFixture);
  for (const o of modelsFixture.wipModels) {
    assert.ok(html.includes(o.model), `${o.model} missing`);
    assert.ok(html.includes(String(o.objective)), `objective for ${o.model} missing`);
    assert.ok(html.includes(`w${o.lastCommitWeek}`), `end week for ${o.model} missing`);
  }
  assert.match(html, /none/, 'off has no limit, shown as none rather than 0');
});

// Unchosen still produced the order on screen. Marking nothing would leave the
// reader unable to tell which row made the table in front of them.
test('the comparison marks strict as the one being read when nothing is chosen', () => {
  const html = wipModelsTableHTML(modelsFixture); // fixture model is "unchosen"
  const strictRow = html.split('<tr').find((r) => r.includes('strict'));
  assert.match(strictRow, /reading as this/);
  assert.ok(!html.includes('in force'), 'nothing is in force until someone chooses');
});

test('the comparison marks which model is in force', () => {
  const inForce = { ...modelsFixture, wipLimit: { ...modelsFixture.wipLimit, model: 'drum-gated' } };
  const html = wipModelsTableHTML(inForce);
  const row = html.split('<tr').find((r) => r.includes('drum-gated'));
  assert.match(row, /in force/);
  assert.ok(!html.includes('reading as this'), 'a real choice is in force, not merely being read');
  assert.ok(!html.split('<tr').find((r) => r.includes('>off<'))?.includes('in force'));
});

// The table must not read as "pick the smallest number": on most plans the same
// dates are missed either way, and the objective is biased by construction.
test('the comparison says what the numbers do not mean', () => {
  const html = wipModelsTableHTML(modelsFixture);
  // The response carries counts, not which dates, so the claim is about the number.
  assert.match(html, /same number of dates/);
  assert.ok(!/same 5 dates are missed/.test(html),
    'identical counts are not identical dates, and the response cannot tell them apart');
  assert.match(html, /charges nothing for multitasking/);
  for (const m of WIP_MODELS) assert.ok(html.includes(m.blurb), `${m.id} blurb missing`);
});

test('the comparison is nothing at all when the server sent none', () => {
  assert.equal(wipModelsTableHTML({}), '');
  assert.equal(wipModelsTableHTML({ wipModels: [] }), '');
});

test('the form opens itself and explains when no model is chosen', () => {
  const html = schedulingFormHTML({ periodStart: '2026-01-05' }, modelsFixture.wipLimit, modelsFixture);
  assert.match(html, /<details class="ord-sched" open>/);
  assert.match(html, /No work-in-progress model chosen/);
  assert.match(html, /computed\s+as <b>strict<\/b> so nothing moves under you/);
  assert.match(html, /<option value=""\s*selected>— not chosen —<\/option>/);
});

// A stored model this build does not implement is scheduled as unchosen by the
// server, so the form must not go quiet as though a choice had been made.
test('an unsupported stored model still counts as unchosen', () => {
  const html = schedulingFormHTML({ periodStart: '2026-01-05', wipModel: 'wishful' },
    { value: 2, derived: true, fromPod: 'Delta', model: 'unchosen' }, modelsFixture);
  assert.match(html, /<details class="ord-sched" open>/);
  assert.match(html, /No work-in-progress model chosen/);
  assert.match(html, /<option value=""\s*selected>/, 'the selector falls back to not-chosen');
});

test('the header says no org limit under off, rather than a limit of zero', () => {
  const off = wipLimitNote({ value: 0, derived: false, model: 'off' });
  assert.match(off, /no org WIP limit/);
  assert.ok(!/WIP limit 0/.test(off), 'a limit of 0 reads as a setting nobody chose');
  assert.match(off, /off/);
  // The other models still report their number the way they always did.
  assert.match(wipLimitNote({ value: 2, derived: true, fromPod: 'Delta', model: 'strict' }), /derived from Delta/);
});

test('the form marks the chosen model as selected and stops nagging', () => {
  const html = schedulingFormHTML(
    { periodStart: '2026-01-05', wipModel: 'drum-gated' },
    { value: 2, derived: true, fromPod: 'Delta', model: 'drum-gated' }, modelsFixture);
  assert.match(html, /<option value="drum-gated" selected>/);
  assert.ok(!html.includes('No work-in-progress model chosen'));
  assert.ok(!html.includes('ord-sched" open'), 'a configured plan does not force the panel open');
});

test('the model is sent only when it is one the server implements', () => {
  const readerOf = (v) => (id) => (id === 'sched-wip-model' ? v : '');
  assert.equal(schedulingFromForm(readerOf('drum-gated')).wipModel, 'drum-gated');
  assert.ok(!('wipModel' in schedulingFromForm(readerOf(''))), 'unchosen stays unchosen');
  assert.ok(!('wipModel' in schedulingFromForm(readerOf('wishful'))), 'no inventing models');
});

// The [options ▾] expander (§13.2, AC 5.1): a missed date's row carries it,
// an on-time one does not, and it never appears without the trace row that
// holds it — the wiring in planui.js depends on that shape.
test('a missed date gets an options expander on its trace row', () => {
  // Start from a clean board: the raw fixture already carries misses, so
  // mutating it alone would not demarcate anything.
  const missed = structuredClone(sched);
  missed.initiatives.forEach((si) => { si.verdict = 'on-time'; });
  assert.ok(!orderTableHTML(missed).includes('ord-options'), 'clean board has no expanders');
  missed.initiatives[1].verdict = 'late';
  const html = orderTableHTML(missed);
  const expanders = html.match(/ord-options/g) || [];
  assert.equal(expanders.length, 1, 'exactly the missed date gets an expander');
  assert.match(html, /<tr class="ord-why"><td><\/td><td colspan="7">[^]*?ord-options/);
});

test('a plan with no misses renders no expanders', () => {
  const fine = structuredClone(sched);
  fine.initiatives.forEach((si) => { si.verdict = 'on-time'; });
  assert.ok(!orderTableHTML(fine).includes('ord-options'));
});

// FR-018's editor: the assumptions form lists the saved calendar windows with
// their fields filled, offers an add button, and reads them back on save.
test('the assumptions form lists saved calendar windows', () => {
  const html = schedulingFormHTML({
    periodStart: '2026-01-05',
    calendars: [
      { kind: 'change-freeze', scope: 'org', fromDate: '2026-02-02', toDate: '2026-02-16', effect: 'block-start' },
    ],
  }, { value: 2, derived: true, fromPod: 'Delta', model: 'strict' }, modelsFixture);
  assert.match(html, /calendar windows/i);
  assert.match(html, /value="change-freeze" selected/);
  assert.match(html, /value="block-start" selected/);
  assert.match(html, /2026-02-02/);
  // One row per saved window plus the add control.
  assert.equal((html.match(/class="cal-win"/g) || []).length, 1);
  assert.match(html, /add a window/i);
});

test('a plan with no windows shows the add control and an explanation, not a blank', () => {
  const html = schedulingFormHTML({ periodStart: '2026-01-05' },
    { value: 2, derived: true, fromPod: 'Delta', model: 'strict' }, modelsFixture);
  assert.match(html, /add a window/i);
  assert.match(html, /freeze|holiday|window/i, 'what a window is for is stated');
  assert.ok(!html.includes('undefined'));
});

test('the form reads windows back with only complete rows kept', () => {
  // schedulingFromForm takes a reader; the window rows are numbered, so the
  // reader answers by suffix. A row missing its dates is dropped rather than
  // sent — the server would refuse it anyway, and the planner should not have
  // to re-type the good rows.
  const rows = [
    { kind: 'change-freeze', scope: 'org', from: '2026-02-02', to: '2026-02-16', effect: 'block-start' },
    { kind: 'event', scope: '', from: '', to: '', effect: '' }, // half-filled
  ];
  const read = (id) => {
    const m = id.match(/^cal-(kind|scope|from|to|effect)-(\d+)$/);
    if (!m) return '';
    const row = rows[Number(m[2])];
    return row ? (row[m[1]] ?? '') : '';
  };
  const out = schedulingFromForm(read);
  assert.ok(Array.isArray(out.calendars));
  assert.equal(out.calendars.length, 1);
  assert.equal(out.calendars[0].fromDate, '2026-02-02');
  assert.equal(out.calendars[0].effect, 'block-start');
}
);
