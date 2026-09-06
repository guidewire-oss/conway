import test from 'node:test';
import assert from 'node:assert/strict';
import { fitSentence, capacitySectionHTML } from '../app/js/report.js';
import { timelineControlsHTML, timelineInspectorHTML, timelineEditsFromRows, portfolioTimelineHTML, podLensHTML } from '../app/js/timeline.js';
import { remediesPanelHTML, optionsExpanderHTML } from '../app/js/remedyui.js';
import { sortTable, prepareSortable } from '../app/js/sortable.js';
import { orderTableHTML } from '../app/js/order.js';

test('an early target miss is independent of horizon fit, using the server shape', () => {
  const sched = { horizonWeeks: 26, fit: { beyondHorizon: 0, podWeeksDemanded: 6, trackWeeksAvailable: 26 }, initiatives: [{ name: 'Atlas', targetWeek: 5, commitWeek: 6, verdict: 'late' }] };
  assert.equal(fitSentence(sched), 'The one initiative is forecast inside the period. 1 initiative misses its target.');
  sched.initiatives.push({ name: 'Beacon', commitWeek: 28, verdict: 'no-date' }, { name: 'Cascade', commitWeek: 0, verdict: 'beyond-horizon' });
  sched.fit.beyondHorizon = 1;
  assert.match(fitSentence(sched), /^2 of 3 initiatives will not finish inside the period\./);
});

test('missing evidence and provisional inputs cannot produce an unconditional all clear', () => {
  assert.match(fitSentence({ initiatives: [{ name: 'Atlas', verdict: 'on-time' }] }), /unknown/);
  assert.match(fitSentence({ horizonWeeks: 26, initiatives: [{ name: 'Atlas', verdict: 'on-time', commitWeek: 4, provisional: true }] }), /provisional/);
  assert.match(capacitySectionHTML({ podWeeks: [{ pod: 'Atlas', tracks: 2 }] }), /incomplete/);
  const unplaced = { horizonWeeks: 26, periodStart: '2026-01-05', initiatives: [{ name: 'Atlas', verdict: 'beyond-horizon', startWeek: 0, commitWeek: 0 }] };
  assert.match(orderTableHTML(unplaced), /not scheduled/);
  assert.doesNotMatch(orderTableHTML(unplaced), /<time datetime="2026-01-05"/);
  assert.match(timelineInspectorHTML(unplaced.initiatives[0], unplaced), /unknown \(not scheduled\)/);
});

const sched = { periodStart: '2026-01-05', horizonWeeks: 26, initiatives: [
  { name: 'Alpha', proposedRank: 1, startWeek: 0, rawFinishWeek: 3, commitWeek: 4, slices: [{ initiative: 'Alpha', pod: 'Atlas', startWeek: 0, finishWeek: 3, lanesUsed: 1 }] },
  { name: 'Beta', proposedRank: 2, startWeek: 4, rawFinishWeek: 6, commitWeek: 7, slices: [{ initiative: 'Beta', pod: 'Beacon', startWeek: 4, finishWeek: 6, lanesUsed: 1 }] },
] };
sched.podWeeks = sched.initiatives.map((i) => ({ pod: i.slices[0].pod, tracks: 2, slices: i.slices }));

test('both grouping lenses apply the same independent team and initiative filters', () => {
  for (const render of [portfolioTimelineHTML, podLensHTML]) {
    const match = render(sched, { initiativeQuery: 'Alpha', podQuery: 'Atlas', hideEmptyPods: true });
    assert.match(match, /Alpha/);
    assert.doesNotMatch(match, /Beta/);
    const none = render(sched, { initiativeQuery: 'Alpha', podQuery: 'Beacon', hideEmptyPods: true });
    assert.doesNotMatch(none, /data-init="Alpha"|data-initiative="Alpha"/);
  }
  for (const lens of ['initiative', 'pod']) {
    const controls = timelineControlsHTML({ lens, spans: [], initiativeFilter: 'Alpha', teamFilter: 'Atlas' });
    assert.match(controls, /id="tl-initiative-filter"[^>]+value="Alpha"/);
    assert.match(controls, /id="tl-team-filter"[^>]+value="Atlas"/);
  }
});

test('dates and selected initiative controls are visible and escaped', () => {
  const timeline = portfolioTimelineHTML(sched, { selected: 'Alpha' });
  assert.match(timeline, /class="tl-tick-date">01-05/);
  assert.match(timeline, /data-select-init="Alpha" aria-pressed="true"/);
  const pi = { name: 'Alpha', work: { Atlas: { weeks: 6 } } };
  const inspector = timelineInspectorHTML(sched.initiatives[0], sched, { planInitiatives: [pi] });
  assert.match(inspector, /2026-02-02/);
  assert.match(inspector, /name="estimateWeeks"[^>]+value="6"/);
  assert.match(inspector, /Apply timeline edits/);
  assert.match(timelineInspectorHTML({ ...sched.initiatives[0], name: '<script>' }, sched), /&lt;script&gt;/);
});

test('precise edits preserve other pins, convert visible lanes and refuse invalid numbers', () => {
  const pi = { name: 'Alpha', work: { Atlas: { weeks: 6 } }, pinnedStarts: { Beacon: 2 }, pinnedLanes: { Beacon: 1 } };
  const edit = timelineEditsFromRows(pi, [{ pod: 'Atlas', startWeek: '3', lane: '2', estimateWeeks: '8' }]);
  assert.deepEqual(edit, { name: 'Alpha', pinnedStarts: { Beacon: 2, Atlas: 3 }, pinnedLanes: { Beacon: 1, Atlas: 1 }, estimateEdits: { Atlas: 8 } });
  assert.deepEqual(pi.pinnedStarts, { Beacon: 2 }, 'preview input remains unchanged');
  for (const row of [{ pod: 'Atlas', startWeek: '', lane: 1 }, { pod: 'Atlas', startWeek: 1, lane: 0 }, { pod: 'Atlas', startWeek: 1, lane: 1, estimateWeeks: 'NaN' }, { pod: 'Unknown', startWeek: 1, lane: 1 }]) assert.throws(() => timelineEditsFromRows(pi, [row]));
  assert.equal(timelineEditsFromRows({ ...pi, inFlight: true }, [{ pod: 'Atlas', startWeek: 1, lane: 1, estimateWeeks: 8 }]).estimateEdits, undefined);
});

test('a remedy preview preserves its identity and horizon misses expose options', () => {
  const html = remediesPanelHTML([{ kind: 'raise-priority', target: 'Alpha' }, { kind: 'descope', target: 'Alpha', affectedInitiatives: [{ initiative: 'Beta', commitDeltaWeeks: 2 }] }]);
  assert.match(html, /data-remedy="1" data-kind="descope" data-target="Alpha"/);
  assert.match(html, /Beta \+2w/);
  assert.match(optionsExpanderHTML({ name: 'Alpha', verdict: 'beyond-horizon' }), /Review options/);
});

test('sorting retains table semantics and announces ascending and descending order', () => {
  const attrs = {};
  const th = { textContent: 'Weeks', classList: { toggle() {} }, setAttribute(k, v) { attrs[k] = v; }, hasAttribute(k) { return k in attrs; } };
  prepareSortable({ querySelectorAll() { return [th]; } });
  assert.equal(th.tabIndex, 0);
  assert.equal(attrs.scope, 'col');
  const rows = ['12w', '2w', '5w'].map((v) => ({ cells: [{ textContent: v }] }));
  const tbody = { rows, appendChild(row) { rows.splice(rows.indexOf(row), 1); rows.push(row); } };
  const table = { dataset: {}, tBodies: [tbody], tHead: { rows: [{ cells: [th] }] } };
  sortTable(table, 0);
  assert.deepEqual(rows.map((r) => r.cells[0].textContent), ['2w', '5w', '12w']);
  assert.equal(attrs['aria-sort'], 'ascending');
  sortTable(table, 0);
  assert.deepEqual(rows.map((r) => r.cells[0].textContent), ['12w', '5w', '2w']);
  assert.equal(attrs['aria-sort'], 'descending');
});
