import test from 'node:test';
import assert from 'node:assert/strict';
import { timelineRowHTML, portfolioTimelineHTML, podLensHTML, podLanesHTML, podSheetHTML } from '../app/js/timeline.js';

const rejected = { name: 'Alpha', verdict: 'beyond-horizon', bindingConstraint: 'wip-limit', startWeek: 0, rawFinishWeek: 0, commitWeek: 0, targetWeek: 5, slices: [] };
const inputs = [{ name: 'Alpha', work: { Atlas: { inPath: true, weeks: 4 }, Beacon: { inPath: false, weeks: 2 } } }];
const idle = { pod: 'Atlas', tracks: 2, slices: [], weeks: Array.from({ length: 26 }, () => ({ busy: 0 })) };
const schedule = { horizonWeeks: 26, initiatives: [rejected], podWeeks: [idle] };

test('a hard lead hold explains the policy without asking for input dates', () => {
  const row = timelineRowHTML({ ...rejected, bindingConstraint: 'lead' });
  assert.match(row, /hard lead-capacity limit/);
  assert.match(row, /Input start and finish dates are not required/);
  assert.match(row, /Use advisory lead limits in Scheduling assumptions/);
  assert.doesNotMatch(row, /Start and finish are unknown/);
});

test('rejected assignments remain visible under input-team filters without invented week-zero bars', () => {
  const row = timelineRowHTML(rejected);
  assert.match(row, /Not scheduled within this period: wip-limit/);
  assert.match(row, /No dates were calculated because this work is held/);
  assert.doesNotMatch(row, /class="tl-bar|class="tl-target|w0/);
  const filtered = portfolioTimelineHTML(schedule, { podQuery: 'Atlas', initiativeQuery: 'Alpha', planInitiatives: inputs });
  assert.match(filtered, /data-init="Alpha"/);
  assert.match(filtered, /wip-limit/);
  assert.doesNotMatch(portfolioTimelineHTML(schedule, { podQuery: 'Beacon', planInitiatives: inputs }), /data-init="Alpha"/);
});

test('team lens and exported sheet retain rejected assignments with explicit unknown dates', () => {
  const lens = podLensHTML(schedule, { initiativeQuery: 'Alpha', podQuery: 'Atlas', hideEmptyPods: true, planInitiatives: inputs });
  assert.match(lens, /data-pod="Atlas"/);
  assert.match(lens, /Assigned work without a placement/);
  assert.match(lens, /data-select-init="Alpha"/);
  assert.match(lens, /wip-limit/);
  const sheet = podSheetHTML(idle, schedule, { planInitiatives: inputs });
  assert.match(sheet, /Alpha/);
  assert.match(sheet, /1 assigned without placement/);
  assert.match(sheet, /No dates were calculated because this work is held/);
  assert.doesNotMatch(sheet, /No scheduled work at this pod|<td>w0/);
});

test('rejection reasons and input names are escaped in both timeline and team sheet', () => {
  const unsafe = { ...rejected, name: '<Alpha>', bindingConstraint: '<script>' };
  const unsafeInputs = [{ ...inputs[0], name: unsafe.name }];
  for (const html of [timelineRowHTML(unsafe), podSheetHTML(idle, { ...schedule, initiatives: [unsafe] }, { planInitiatives: unsafeInputs })]) {
    assert.match(html, /&lt;script&gt;/);
    assert.doesNotMatch(html, /<script>/);
  }
});

const split = { initiative: 'Beta', pod: 'Atlas', startWeek: 2, finishWeek: 10, lanesUsed: 2, remainingWeeks: 28, latestStartWeek: 2, slackWeeks: 0,
  phases: [{ fromWeek: 2, toWeek: 6, lanes: 2 }, { fromWeek: 6, toWeek: 10, lanes: 5 }] };

// Decode rendered geometry back to intervals. The checks count actual physical
// lane occupancy instead of mirroring the implementation's packing decisions.
function intervals(html, horizon = 26) {
  const result = [];
  html.split('<div class="tl-lane">').slice(1).forEach((row, lane) => {
    for (const m of row.matchAll(/class="tl-bar[^>]*style="left:([\d.]+)%;width:([\d.]+)%"[^>]*data-initiative="([^"]+)"/g)) {
      result.push({ lane, start: Math.round(Number(m[1]) * horizon / 100), finish: Math.round((Number(m[1]) + Number(m[2])) * horizon / 100), name: m[3] });
    }
  });
  return result;
}

test('split phases occupy only their actual weeks and active physical lanes', () => {
  const html = podLanesHTML({ pod: 'Atlas', tracks: 5, slices: [split] }, { horizonWeeks: 26 });
  const spans = intervals(html);
  assert.equal(spans.filter((s) => s.start === 2 && s.finish === 6).length, 2);
  assert.equal(spans.filter((s) => s.start === 6 && s.finish === 10).length, 5);
  assert.equal(new Set(spans.map((s) => s.lane)).size, 5);
  assert.doesNotMatch(html, /data-estimate=/, 'a phase width cannot act as a whole-estimate resize handle');
  assert.match(html, /use precise controls to edit the total estimate/);
});

test('expanding work shares lanes with work that finishes before the expansion', () => {
  const earlier = { initiative: 'Gamma', pod: 'Atlas', startWeek: 0, finishWeek: 6, lanesUsed: 3, remainingWeeks: 18 };
  const html = podLanesHTML({ pod: 'Atlas', tracks: 5, slices: [earlier, split] }, { horizonWeeks: 26 });
  const spans = intervals(html);
  for (let week = 0; week < 10; week++) {
    const active = spans.filter((s) => s.start <= week && week < s.finish);
    const expected = week < 2 ? 3 : 5;
    assert.equal(active.length, expected, `week ${week} preserves scheduled occupancy`);
    assert.equal(new Set(active.map((s) => s.lane)).size, expected, `week ${week} never overlaps two tasks on a physical lane`);
  }
  assert.equal((html.match(/class="tl-lane"/g) || []).length, 5);
});

test('empty teams show exactly their physical idle lanes, including zero capacity', () => {
  const two = podLanesHTML(idle);
  assert.equal((two.match(/class="tl-lane"/g) || []).length, 2);
  assert.equal((two.match(/track 1/g) || []).length, 1);
  assert.equal((two.match(/· idle ·/g) || []).length, 2);
  const zero = podLanesHTML({ ...idle, tracks: 0 });
  assert.doesNotMatch(zero, /track 1/);
  assert.match(zero, /no capacity/);
});

test('a later saved lane pin reserves its interval before flexible earlier work', () => {
  const earlier = { initiative: 'Earlier', pod: 'Atlas', startWeek: 0, finishWeek: 10, lanesUsed: 1 };
  const pinned = { initiative: 'Pinned', pod: 'Atlas', startWeek: 2, finishWeek: 5, lanesUsed: 1 };
  const spans = intervals(podLanesHTML({ pod: 'Atlas', tracks: 2, slices: [earlier, pinned] }, { pinnedLanes: { Pinned: 0 } }));
  assert.equal(spans.find((s) => s.name === 'Pinned').lane, 0);
  assert.equal(spans.find((s) => s.name === 'Earlier').lane, 1);
});

test('flexible work can cross complementary pinned reservations without overlapping', () => {
  const slices = [
    { initiative: 'Early', pod: 'Atlas', startWeek: 0, finishWeek: 2, lanesUsed: 1 },
    { initiative: 'Late', pod: 'Atlas', startWeek: 2, finishWeek: 4, lanesUsed: 1 },
    { initiative: 'Flexible', pod: 'Atlas', startWeek: 0, finishWeek: 4, lanesUsed: 1 },
  ];
  const spans = intervals(podLanesHTML({ pod: 'Atlas', tracks: 2, slices }, { pinnedLanes: { Early: 0, Late: 1 } }));
  for (let week = 0; week < 4; week++) {
    const active = spans.filter((s) => s.start <= week && s.finish > week);
    assert.equal(active.length, 2);
    assert.equal(new Set(active.map((s) => s.lane)).size, 2);
    const fixed = active.find((s) => s.name !== 'Flexible');
    assert.equal(fixed.lane, week < 2 ? 0 : 1);
  }
});

test('team cards carry an axis, calendar context and a keyboard sheet control', () => {
  const html = podLensHTML({ ...schedule, periodStart: '2026-09-01' }, {
    planInitiatives: inputs, span: 52, todayWeek: 4,
    calendars: [{ kind: 'change-freeze', scope: 'Atlas', fromDate: '2026-09-08', toDate: '2026-09-15', effect: 'block-start' }],
  });
  assert.match(html, /data-pod="Atlas"[\s\S]*class="tl-axis"/);
  assert.match(html, /tl-band-freeze/);
  assert.match(html, /tl-today/);
  assert.match(html, /tl-period-end/);
  assert.match(html, /<button[^>]*data-open-pod="Atlas"/);
});

test('future work outside the selected span gets a label instead of a zero-width bar', () => {
  const ps = { pod: 'Atlas', tracks: 2, slices: [{ initiative: 'Later', pod: 'Atlas', startWeek: 30, finishWeek: 35, lanesUsed: 1 }] };
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /Later/);
  assert.match(html, /outside this view/i);
  assert.doesNotMatch(html, /class="tl-bar/);
  assert.doesNotMatch(html, /width:0/);
});

test('zero-capacity rows honor the work filter and optional ghost context', () => {
  const ps = { pod: 'Atlas', tracks: 0, slices: [{ initiative: 'Other', pod: 'Atlas', startWeek: 0, finishWeek: 4, lanesUsed: 1 }] };
  const hidden = podLanesHTML(ps, { initiativeQuery: 'Selected' });
  assert.doesNotMatch(hidden, /Other/);
  assert.doesNotMatch(hidden, /class="tl-bar/);
  assert.match(podLanesHTML(ps, { initiativeQuery: 'Selected', ghostOthers: true }), /tl-ghost/);
});

test('flat calendar gaps render only authoritative occupied weeks', () => {
  const ps = { pod: 'Atlas', tracks: 1, slices: [{ initiative: 'Alpha', pod: 'Atlas', startWeek: 0, finishWeek: 4, remainingWeeks: 2, lanesUsed: 1 }],
    // The API omits initiatives on idle weeks rather than returning an empty array.
    weeks: [0,1,2,3].map((week) => week === 0 || week === 3 ? { week, busy: 1, initiatives: ['Alpha'] } : { week, busy: 0 }) };
  const spans = intervals(podLanesHTML(ps));
  for (let week = 0; week < 4; week++) {
    assert.equal(spans.filter((s) => s.start <= week && s.finish > week).length, ps.weeks[week].busy, `week ${week}`);
  }
});
