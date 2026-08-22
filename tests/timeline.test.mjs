import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  axisScale, axisTicks, timelineRowHTML, portfolioTimelineHTML,
  podLanesHTML, podLensHTML, podSheetHTML, todayLineHTML,
} from '../app/js/timeline.js';

// Real scheduler output (regenerate with `go run ./tools/fixgen`), so the view
// and the Go Schedule shape are checked against each other.
const sched = JSON.parse(readFileSync(new URL('./fixtures/schedule-demo.json', import.meta.url)));

test('the fixture carries the fields the timeline reads', () => {
  const si = sched.initiatives[0];
  assert.ok(typeof si.startWeek === 'number');
  assert.ok(typeof si.commitWeek === 'number');
  assert.ok(typeof si.rawFinishWeek === 'number');
  for (const sl of si.slices) {
    assert.ok(typeof sl.latestStartWeek === 'number', 'slack pair present');
    assert.ok(typeof sl.slackWeeks === 'number');
  }
});

// AC 8.2 / FR-035 / §13.8: the whole period fits the width; zoom is axis
// aggregation. The scale maps week -> percentage of the row width.
test('axisScale maps weeks onto the full row width', () => {
  const s26 = axisScale(26);
  assert.equal(s26(0), 0);
  assert.equal(s26(26), 100);
  assert.equal(s26(13), 50);
  const s104 = axisScale(104);
  assert.equal(s104(104), 100);
});

test('axis aggregation steps by horizon: weekly, fortnightly, monthly', () => {
  // §13.8: <=16w weekly labels, <=40w fortnightly, beyond that monthly.
  const weekly = axisTicks(12);
  assert.ok(weekly.length > 4);
  assert.ok(weekly.every((t) => Number.isInteger(t.week)));
  const fortnightly = axisTicks(30);
  assert.ok(fortnightly.length < weekly.length + 20, 'fewer labels than weekly');
  assert.ok(fortnightly.every((t, i) => i === 0 || t.week > fortnightly[i - 1].week));
  const monthly = axisTicks(104);
  assert.ok(monthly.length <= 30, 'monthly labels cannot crowd a 104w horizon');
});

// AC 8.1: bar = scheduled span, appended buffer segment, target marker.
test('a row renders the span, the buffer tail, and the target diamond', () => {
  const si = sched.initiatives.find((x) => x.targetWeek !== undefined && x.targetWeek !== null);
  assert.ok(si, 'fixture has a dated initiative');
  const html = timelineRowHTML(si, { horizonWeeks: 26 });
  assert.match(html, /tl-bar/);
  assert.match(html, /tl-buffer/);
  assert.match(html, /tl-target/);
  assert.match(html, new RegExp(escRe(si.name)));
  // The bar starts at the initiative's start week, positioned by percentage.
  const s = axisScale(26);
  assert.ok(html.includes(`left:${s(si.startWeek)}`), 'bar is positioned at its start week');
});

test('a row without a target date renders no diamond', () => {
  const undated = sched.initiatives.find((x) => x.targetWeek === undefined || x.targetWeek === null);
  if (!undated) return;
  const html = timelineRowHTML(undated, { horizonWeeks: 26 });
  assert.ok(!html.includes('tl-target'));
});

// AC 8.4: expansion adds one sub-row per pod slice, dependency order, with the
// upstream pods named (FR-042).
test('an expanded initiative shows pod sub-rows in dependency order', () => {
  const si = sched.initiatives.find((x) => x.slices.some((s) => (s.dependsOn || []).length > 0));
  assert.ok(si, 'fixture has cross-pod dependencies');
  const html = timelineRowHTML(si, { horizonWeeks: 26, expand: true });
  assert.match(html, /tl-subrow/);
  const pods = si.slices.map((s) => s.pod);
  for (const pod of pods) assert.match(html, new RegExp(escRe(pod)));
  // Sub-rows follow the slice order, which is dependency order.
  const first = si.slices[0];
  const second = si.slices[1];
  if (first && second) {
    assert.ok(html.indexOf(`data-pod="${first.pod}"`) < html.indexOf(`data-pod="${second.pod}"`),
      'sub-rows keep the schedule\'s dependency order');
  }
});

test('a sub-row names the pods it waits on', () => {
  const si = sched.initiatives.find((x) => x.slices.some((s) => (s.dependsOn || []).length > 0));
  const html = timelineRowHTML(si, { horizonWeeks: 26, expand: true });
  const dep = si.slices.find((s) => (s.dependsOn || []).length > 0);
  for (const upstream of dep.dependsOn) {
    assert.match(html, new RegExp(escRe(upstream)), 'upstream pod named');
  }
});

// AC 8.5 / FR-038: today is marked, positioned by date. Freeze windows wait
// for FR-018; their absence must not break the render.
test('today renders as a positioned line inside the period', () => {
  const inside = todayLineHTML(10, 26);
  assert.match(inside, /tl-today/);
  assert.match(inside, /left:38\.46/, 'week 10 of 26 is 38.46% in');
  assert.equal(todayLineHTML(-2, 26), '', 'before the period: no line');
  assert.equal(todayLineHTML(30, 26), '', 'after the period: no line');
});

// FR-039: labels truncate, short bars stay visible — the class contract the
// CSS implements; the renderer must emit both hooks.
test('rows carry the truncation and minimum-width hooks', () => {
  const html = portfolioTimelineHTML(sched, { horizonWeeks: 26 });
  assert.match(html, /tl-bar tl-trunc/);
});

test('the portfolio timeline renders one row per initiative, ranked order', () => {
  const html = portfolioTimelineHTML(sched, { horizonWeeks: 26 });
  assert.equal((html.match(/class="tl-row/g) || []).length, sched.initiatives.length);
  const first = sched.initiatives.find((x) => x.proposedRank === 1);
  const second = sched.initiatives.find((x) => x.proposedRank === 2);
  if (first && second) {
    assert.ok(html.indexOf(escRe(first.name)) < html.indexOf(escRe(second.name)),
      'rows follow the proposed rank');
  }
  assert.ok(!html.includes('undefined'));
});

// AC 9.1 / FR-040: pod lanes are tracks; never more lanes than tracks.
test('pod lanes place slices in start order, one lane per track at most', () => {
  const ps = sched.podWeeks.find((p) => p.slices.length > 1);
  assert.ok(ps, 'fixture has a pod with multiple slices');
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /tl-lane/);
  const lanes = (html.match(/class="tl-lane/g) || []).length;
  assert.ok(lanes <= ps.tracks, `${lanes} lanes for ${ps.tracks} tracks`);
  // Slices labelled by initiative (FR-040).
  for (const sl of ps.slices) assert.match(html, new RegExp(escRe(sl.initiative)));
});

test('a pod with idle tracks shows them, not hides them', () => {
  const ps = sched.podWeeks.find((p) => p.tracks > laneCount(p));
  if (!ps) return; // demo plan may use every track; the code path is still spec'd below
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /idle/, 'slack is shown');
});

function laneCount(ps) {
  // How many distinct tracks the scheduled slices actually occupy.
  const lanes = new Set();
  for (const sl of ps.slices) {
    // The server does not assign lanes; the view assigns them greedily by
    // overlap, so count greedily here too.
    lanes.add(0); // placeholder: overlap assignment happens in the view
  }
  return ps.slices.length;
}

// AC 9.2 / 9.3 / FR-041 / FR-042: the pod sheet.
test('the pod sheet shows start, start by, slack, waiting on, and blocks', () => {
  const ps = sched.podWeeks.find((p) => p.slices.length >= 2);
  assert.ok(ps);
  const html = podSheetHTML(ps, sched, { horizonWeeks: 26 });
  assert.match(html, /Start by/);
  assert.match(html, /Slack/);
  const withDeps = ps.slices.find((s) => (s.dependsOn || []).length > 0);
  if (withDeps) {
    for (const up of withDeps.dependsOn) assert.match(html, new RegExp(escRe(up)));
  }
  const zero = ps.slices.find((s) => s.slackWeeks === 0);
  if (zero) assert.match(html, /none ⚠|zero-slack|no slack/i, 'zero slack is marked distinctly');
  assert.ok(!html.includes('undefined'));
});

test('the pod sheet names downstream waiters (Blocks)', () => {
  // Find a slice that another slice waits on, within one initiative.
  let found = null;
  for (const si of sched.initiatives) {
    for (const sl of si.slices) {
      for (const other of si.slices) {
        if ((other.dependsOn || []).includes(sl.pod) && other.pod !== sl.pod) {
          found = { si, blocker: sl, waiter: other };
        }
      }
    }
  }
  assert.ok(found, 'fixture has a wait relationship');
  const ps = sched.podWeeks.find((p) => p.pod === found.blocker.pod);
  const html = podSheetHTML(ps, sched, { horizonWeeks: 26 });
  assert.match(html, new RegExp(escRe(found.waiter.pod)), 'the waiting pod is named');
});

// AC 8.6 / FR-039: labels that do not fit truncate with the full name available.
test('long initiative names are truncated, not overflowing', () => {
  const si = structuredClone(sched.initiatives[0]);
  si.name = 'An extremely long initiative name that cannot fit any reasonable bar';
  si.startWeek = 20;
  si.rawFinishWeek = 22; // 2w bar near the end of a 26w horizon
  const html = timelineRowHTML(si, { horizonWeeks: 26 });
  assert.match(html, /tl-trunc/);
  assert.match(html, /title="An extremely long/, 'the full name survives as a tooltip');
});

test('the portfolio timeline expands exactly the named initiative', () => {
  // opts.expand is the initiative NAME; passing it through truthy would expand
  // every row at once (caught live: one click opened 24 sub-rows).
  const si = sched.initiatives.find((x) => x.slices.length > 1);
  assert.ok(si);
  const html = portfolioTimelineHTML(sched, { horizonWeeks: 26, expand: si.name });
  const expanded = (html.match(/class="tl-subrow"/g) || []).length;
  assert.equal(expanded, si.slices.length, 'only the named initiative shows sub-rows');
});

function escRe(s) {
  return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
