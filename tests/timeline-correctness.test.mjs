import test from 'node:test';
import assert from 'node:assert/strict';
import { timelineRowHTML, portfolioTimelineHTML, podLensHTML, podLanesHTML, podSheetHTML } from '../app/js/timeline.js';

const rejected = { name: 'Alpha', verdict: 'beyond-horizon', bindingConstraint: 'wip-limit', startWeek: 0, rawFinishWeek: 0, commitWeek: 0, targetWeek: 5, slices: [] };
const inputs = [{ name: 'Alpha', work: { Atlas: { inPath: true, weeks: 4 }, Beacon: { inPath: false, weeks: 2 } } }];
const idle = { pod: 'Atlas', tracks: 2, slices: [], weeks: Array.from({ length: 26 }, () => ({ busy: 0 })) };
const schedule = { horizonWeeks: 26, initiatives: [rejected], podWeeks: [idle] };

test('rejected assignments remain visible under input-team filters without invented week-zero bars', () => {
  const row = timelineRowHTML(rejected);
  assert.match(row, /Not scheduled within this period: wip-limit/);
  assert.match(row, /Start and finish are unknown/);
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
  assert.match(sheet, /Start and finish are unknown/);
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
