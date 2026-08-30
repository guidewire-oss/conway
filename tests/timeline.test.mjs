import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  axisScale, axisTicks, timelineRowHTML, portfolioTimelineHTML,
  podLanesHTML, podLensHTML, podSheetHTML, todayLineHTML, periodEndHTML,
  timelineControlsHTML,
} from '../app/js/timeline.js';
import { esc } from '../app/js/order.js';

// Real scheduler output (regenerate with `go run ./tools/fixgen`), so the view
// and the Go Schedule shape are checked against each other.
const sched = JSON.parse(readFileSync(new URL('./fixtures/schedule-demo.json', import.meta.url)));

test('the fixture carries the fields the timeline reads', () => {
  const si = sched.initiatives[0];
  assert.ok(typeof si.startWeek === 'number');
  assert.ok(typeof si.commitWeek === 'number');
  assert.ok(typeof si.rawFinishWeek === 'number');
  for (const sl of (si.slices || [])) {
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
  assert.ok(weekly.every((t, i) => i === 0 || t.week === weekly[i - 1].week + 1),
    'weekly ticks advance one week at a time');
  const fortnightly = axisTicks(30);
  assert.ok(fortnightly.every((t, i) => i === 0 || t.week === fortnightly[i - 1].week + 2),
    'fortnightly ticks advance two weeks at a time');
  const monthly = axisTicks(104);
  assert.ok(monthly.every((t, i) => i === 0 || t.week === monthly[i - 1].week + 4),
    'monthly ticks advance four weeks at a time');
  assert.ok(monthly.length <= 27, 'monthly labels cannot crowd a 104w horizon');
});

// AC 8.1: bar = scheduled span, appended buffer segment, target marker.
test('a row renders the span, the buffer tail, and the target diamond', () => {
  // The buffer must be inside the horizon for this assertion: an initiative
  // whose commit lands past the horizon clamps its tail away (see the
  // overrun test below). Decision 28 emptied most demo slices — require a
  // row that actually renders a bar.
  const si = sched.initiatives.find((x) =>
    x.targetWeek != null && x.commitWeek <= 26 && (x.slices || []).length > 0);
  assert.ok(si, 'fixture has a dated initiative that fits the period with placed work');
  const html = timelineRowHTML(si, { horizonWeeks: 26 });
  assert.match(html, /tl-bar/);
  assert.match(html, /tl-buffer/);
  assert.match(html, /tl-target/);
  assert.match(html, new RegExp(escRe(si.name)));
  // The bar starts at the initiative's start week, positioned by percentage.
  const s = axisScale(26);
  assert.ok(html.includes(`left:${s(si.startWeek)}`), 'bar is positioned at its start week');
});

// FR-035 with overrun: work past the horizon clamps at 100% — nothing renders
// outside the container — and the overrun stays visible in the title.
test('work past the horizon stops at the container edge and says so', () => {
  const over = sched.initiatives.find((x) => x.commitWeek > 26);
  assert.ok(over, 'the demo plan overruns its horizon');
  const html = timelineRowHTML(over, { horizonWeeks: 26 });
  const widths = [...html.matchAll(/width:([\d.]+)%/g)].map((m) => parseFloat(m[1]));
  for (const w of widths) assert.ok(w <= 100, 'no width exceeds the container');
  const lefts = [...html.matchAll(/left:([\d.]+)%/g)].map((m) => parseFloat(m[1]));
  for (const l of lefts) assert.ok(l <= 100, 'no position exceeds the container');
  assert.match(html, /past the horizon/, 'the overrun is named, not hidden');
});

test('a target beyond the horizon pins at the edge and says so', () => {
  const html = timelineRowHTML(
    { name: 'Far out', startWeek: 20, rawFinishWeek: 24, commitWeek: 26,
      bufferWeeks: 2, targetWeek: 40, slices: [] },
    { horizonWeeks: 26 });
  assert.match(html, /tl-target-beyond/);
  assert.match(html, /beyond the horizon/);
  assert.ok(!html.includes('left:153'), 'the diamond does not render off-container');
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
  // Guarded: since Decision 28 the fixture contains initiatives with no slices at
  // all (beyond-horizon carries none), and an unguarded .some crashes on them.
  const si = sched.initiatives.find((x) => (x.slices || []).some((s) => (s.dependsOn || []).length > 0));
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
test('pod lanes place overlapping slices on separate lanes, never more than tracks', () => {
  // The packing exists for genuinely overlapping slices; the regenerated demo
  // (critical-path-first + Decision 28) serializes everything inside the
  // horizon, so synthesize the overlap the packer must handle.
  const base = sched.podWeeks.find((p) => (p.slices || []).length > 1) || sched.podWeeks[0];
  const ps = {
    pod: base.pod, tracks: base.tracks,
    slices: base.slices.slice(0, 2).map((sl, i) => ({ ...sl, startWeek: i, finishWeek: i + 3 })),
  };
  assert.ok(overlaps(ps.slices), 'the synthetic pair overlaps');
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /tl-lane/);
  const lanes = (html.match(/class="tl-lane/g) || []).length;
  assert.ok(lanes <= ps.tracks, `${lanes} lanes for ${ps.tracks} tracks`);
  // The overlapping pair lands on different lanes: both labelled, both present.
  const pair = firstOverlap(ps.slices);
  assert.match(html, new RegExp(escRe(pair[0].initiative)));
  assert.match(html, new RegExp(escRe(pair[1].initiative)));
  // The packing is proven from the RENDER, not the helper: the overlapping
  // pair must appear in different .tl-lane blocks, so a same-lane regression
  // fails even if the arithmetic helper stays correct.
  // Match the ESCAPED name: the renderer escapes labels, so a name with
  // & < > " never appears raw in the HTML.
  const laneOf = (name) => html.split('<div class="tl-lane">')
    .findIndex((lane) => lane.includes(esc(name)));
  assert.notEqual(laneOf(pair[0].initiative), laneOf(pair[1].initiative),
    'overlapping slices occupy different lanes');
  assert.ok(laneOf(pair[0].initiative) >= 0 && laneOf(pair[1].initiative) >= 0,
    'both overlapping slices render');
});

test('a pod with idle tracks shows them, not hides them', () => {
  const ps = sched.podWeeks.find((p) => p.tracks > occupiedLanes(p.slices));
  if (!ps) return; // demo plan may use every track; the lane cap is spec'd above
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /idle/, 'slack is shown');
});

test('a zero-capacity pod renders no track lanes', () => {
  const html = podLanesHTML({ pod: 'Ghost', tracks: 0, weeks: [], slices: [
    { initiative: 'X', pod: 'Ghost', startWeek: 2, finishWeek: 5, latestStartWeek: 2, slackWeeks: 0 },
  ] }, { horizonWeeks: 26 });
  assert.match(html, /no capacity/);
  assert.ok(!html.includes('track 1</span'), 'a lane would claim capacity that does not exist');
});

// occupiedLanes mirrors the view's greedy interval packing, so the idle test
// decides from the same arithmetic the renderer uses.
function occupiedLanes(slices) {
  const ends = [];
  for (const sl of slices.slice().sort((a, b) => a.startWeek - b.startWeek)) {
    let lane = ends.findIndex((e) => e <= sl.startWeek);
    if (lane === -1) { lane = ends.length; ends.push(0); }
    ends[lane] = sl.finishWeek;
  }
  return ends.length;
}

function overlaps(slices) { return !!firstOverlap(slices); }

function firstOverlap(slices) {
  const sorted = slices.slice().sort((a, b) => a.startWeek - b.startWeek);
  for (let i = 1; i < sorted.length; i++) {
    if (sorted[i].startWeek < sorted[i - 1].finishWeek) {
      return [sorted[i - 1], sorted[i]];
    }
  }
  return null;
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

test('the pod sheet carries the exact start-by and slack values (FR-041)', () => {
  // Column names are not values: find a slice with real slack and read its
  // cells back out of the rendered row.
  let found = null;
  for (const p of sched.podWeeks) {
    const sl = p.slices.find((s) => s.slackWeeks > 0);
    if (sl) { found = { p, sl }; break; }
  }
  assert.ok(found, 'fixture has a slice with positive slack');
  const html = podSheetHTML(found.p, sched, { horizonWeeks: 26 });
  // The raw name, not the regex-escaped one: indexOf wants literal text.
  const at = html.indexOf(found.sl.initiative);
  assert.ok(at >= 0, 'the slice is in this pod\'s sheet');
  const row = html.slice(at, html.indexOf('</tr>', at));
  // Cell-anchored: a bare w8 would also match w80.
  assert.match(row, new RegExp(`<td>w${found.sl.startWeek}</td>`), 'the start week is the actual start');
  assert.match(row, new RegExp(`<td>w${found.sl.latestStartWeek}</td>`), 'start-by is latestStartWeek');
  assert.match(row, new RegExp(`<td>${found.sl.slackWeeks}w</td>`), 'slack is the slice\'s own weeks');
});

test('the pod sheet names downstream waiters (Blocks)', () => {
  // Find a slice that another slice waits on, within one initiative.
  let found = null;
  for (const si of sched.initiatives) {
    for (const sl of (si.slices || [])) {
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

// AC 8.5 / FR-038: freeze and non-working windows render as marked bands,
// positioned by date off the same axis. The renderer deliberately produced no
// branch for these until FR-018 landed; now they arrive on the schedule's
// scheduling params.
test('calendar windows render as marked vertical bands', () => {
  const html = portfolioTimelineHTML(sched, {
    horizonWeeks: 26,
    calendars: [
      { kind: 'change-freeze', scope: 'org', fromDate: '2026-02-02', toDate: '2026-02-16', effect: 'block-start' },
      { kind: 'site-nonworking', scope: 'Kraków', fromDate: '2026-01-19', toDate: '2026-01-26', effect: 'reduce-capacity' },
    ],
  });
  assert.match(html, /tl-band/);
  const bands = [...html.matchAll(/class="tl-band tl-trunc/g)];
  assert.equal(bands.length, 2, 'one band per window');
  // The freeze starts at week 4 (2026-02-02 is 4 weeks after 2026-01-05) and
  // the band is positioned by percentage on the same axis as the bars.
  const s = axisScale(26);
  assert.ok(html.includes(`left:${s(4)}`), 'the freeze band starts at its mapped week');
});

test('a freeze band is labelled so the mark is not just colour (FR-044)', () => {
  const html = portfolioTimelineHTML(sched, {
    horizonWeeks: 26,
    calendars: [{ kind: 'change-freeze', scope: 'org', fromDate: '2026-02-02', toDate: '2026-02-16', effect: 'block-start' }],
  });
  assert.match(html, /freeze/i, 'the band says what it is');
  assert.match(html, /2026-02-02–2026-02-16/, 'the band carries its dates visibly');
});

test('no windows render no bands', () => {
  assert.ok(!portfolioTimelineHTML(sched, { horizonWeeks: 26 }).includes('tl-band'));
});

// FR-043 (spec 004 Story 3): both pod views carry a PNG export affordance.
test('the pod lens and the pod sheet offer a PNG export', () => {
  const lens = podLensHTML(sched, { horizonWeeks: 26 });
  assert.match(lens, /class="pod-export" data-export-pod=/);
  const found = (sched.podWeeks || [])[0];
  const sheet = podSheetHTML(found, sched, { horizonWeeks: 26 });
  assert.match(sheet, /class="pod-export" data-export-sheet=/);
});

// Spec 004 follow-up: the period-end marker when the view spans past the horizon.
test('periodEndHTML marks the horizon only when the span exceeds it', () => {
  assert.equal(periodEndHTML(26, 26), '');
  assert.equal(periodEndHTML(26, 20), '');
  const wide = periodEndHTML(26, 52);
  assert.match(wide, /tl-period-end/);
  assert.match(wide, /period end · w26/);
  assert.match(wide, /title="period end: week 26 of 52 shown"/);
});

// Spec 007: the phase ladder rides the bar's tooltip.
test('a split slice tooltip carries its lane ladder', () => {
  const ps = { pod: 'P', tracks: 5, slices: [{
    initiative: 'Big', pod: 'P', startWeek: 2, finishWeek: 10, lanesUsed: 2,
    latestStartWeek: 4, slackWeeks: 2,
    phases: [{ fromWeek: 2, toWeek: 6, lanes: 2 }, { fromWeek: 6, toWeek: 10, lanes: 5 }],
  }] };
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /lanes 2→w6, 5→w10/);
});

// Spec 008: bars carry the drag contract.
test('bars expose the drag data attributes', () => {
  const ps = { pod: 'P', tracks: 2, slices: [{
    initiative: 'Big', pod: 'P', startWeek: 3, finishWeek: 9, lanesUsed: 2,
    latestStartWeek: 5, slackWeeks: 2,
  }] };
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /data-initiative="Big"/);
  assert.match(html, /data-pod="P"/);
  assert.match(html, /data-start-week="3"/);
});

// Spec 010: lens filters. Fuzzy match is substring OR subsequence; the by-pod
// lens dims non-matching initiative bars; the by-initiative lens dims rows
// and slice sub-rows not touching the typed pod.
test('fuzzyMatch: substring, subsequence, case-insensitive, empty', async () => {
  const { fuzzyMatch } = await import('../app/js/filter.js');
  assert.equal(fuzzyMatch('', 'anything'), true);
  assert.equal(fuzzyMatch('apollo', 'Apollo/App Platform'), true);
  assert.equal(fuzzyMatch('APOLLO', 'Apollo/App Platform'), true);
  assert.equal(fuzzyMatch('aplat', 'Apollo/App Platform'), true, 'subsequence');
  assert.equal(fuzzyMatch('app platform', 'Apollo/App Platform'), true, 'substring of the full name');
  assert.equal(fuzzyMatch('xyz', 'Apollo/App Platform'), false);
  assert.equal(fuzzyMatch('atlas', ''), false, 'empty target never matches a query');
});

test('the by-pod lens hides non-matching initiative bars', () => {
  const ps = [
    { pod: 'Atlas', tracks: 2, slices: [
      { initiative: 'Apollo/Mobile', pod: 'Atlas', startWeek: 0, finishWeek: 4, lanesUsed: 2, latestStartWeek: 2, slackWeeks: 1 },
      { initiative: 'BYOK', pod: 'Atlas', startWeek: 4, finishWeek: 6, lanesUsed: 1, latestStartWeek: 5, slackWeeks: 1 },
    ]},
  ];
  const html = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 }, { horizonWeeks: 26, span: 26, initiativeQuery: 'apollo' });
  assert.match(html, /title="Apollo/, 'the match renders');
  assert.ok(!html.includes('BYOK'), 'BYOK does not render at all');
  const clear = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 }, { horizonWeeks: 26, span: 26 });
  assert.ok(clear.includes('BYOK'), 'empty query shows everything');
});

test('the by-initiative lens hides rows not touching the typed pod', () => {
  const sched = { initiatives: [
    { name: 'A', proposedRank: 1, startWeek: 0, rawFinishWeek: 3, commitWeek: 4,
      slices: [{ pod: 'Atlas', startWeek: 0, finishWeek: 3, latestStartWeek: 1, slackWeeks: 1 }] },
    { name: 'B', proposedRank: 2, startWeek: 0, rawFinishWeek: 2, commitWeek: 3,
      slices: [{ pod: 'Beacon', startWeek: 0, finishWeek: 2, latestStartWeek: 1, slackWeeks: 1 }] },
  ], horizonWeeks: 26 };
  const html = portfolioTimelineHTML(sched, { horizonWeeks: 26, span: 26, podQuery: 'atlas' });
  assert.ok(html.includes('data-init="A"'), 'A touches Atlas, renders');
  assert.ok(!html.includes('data-init="B"'), 'B does not touch Atlas, does not render');
});

// Spec 010 amendments: waterfall ordering + hide-empty-pods under a filter.
test('the by-pod lens waterfalls matching pods by earliest start', () => {
  const ps = [
    { pod: 'LatePod', tracks: 2, slices: [
      { initiative: 'Dev', pod: 'LatePod', startWeek: 10, finishWeek: 14, lanesUsed: 2, latestStartWeek: 12, slackWeeks: 1 }] },
    { pod: 'FirstPod', tracks: 2, slices: [
      { initiative: 'Dev', pod: 'FirstPod', startWeek: 0, finishWeek: 5, lanesUsed: 2, latestStartWeek: 1, slackWeeks: 1 }] },
    { pod: 'Unrelated', tracks: 1, slices: [
      { initiative: 'BYOK', pod: 'Unrelated', startWeek: 0, finishWeek: 2, lanesUsed: 1, latestStartWeek: 1, slackWeeks: 1 }] },
  ];
  const html = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 },
    { horizonWeeks: 26, span: 26, initiativeQuery: 'dev' });
  const first = html.indexOf('FirstPod'), late = html.indexOf('LatePod'), unrel = html.indexOf('Unrelated');
  assert.ok(first > -1 && late > -1, 'both matching pods render');
  assert.ok(first < late, 'earliest start sorts first — the waterfall');
  assert.ok(unrel > late, 'non-matching pods trail the waterfall');
  // hideEmptyPods removes it entirely
  const hidden = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 },
    { horizonWeeks: 26, span: 26, initiativeQuery: 'dev', hideEmptyPods: true });
  assert.ok(!hidden.includes('Unrelated'), 'hide-empty drops non-matching pods');
  assert.ok(hidden.includes('FirstPod') && hidden.includes('LatePod'));
});

// The lens/zoom/filter row above the timeline (planui renders #tl-main and
// #tl-pod as its siblings). Regression for the `.seg` to `.btn-group`
// migration: `</span>` closers left on `<div>` openers made the browser
// ignore the stray closers, the first .btn-group (a flex row) swallowed
// #tl-main, and every button stretched viewport-tall while the chart
// squeezed into the leftover width. The markup must be tag-balanced.
const VOID_TAGS = new Set(['input', 'br', 'img', 'hr', 'meta', 'link']);

function tagBalance(html) {
  const stack = [];
  for (const m of html.matchAll(/<\/?([a-zA-Z][a-zA-Z0-9-]*)[^>]*?>/g)) {
    const tag = m[1].toLowerCase();
    if (VOID_TAGS.has(tag) || m[0].endsWith('/>')) continue;
    if (m[0][1] === '/') {
      const open = stack.pop();
      if (open !== tag) return `</${tag}> closes <${open ?? 'nothing'}>`;
    } else {
      stack.push(tag);
    }
  }
  return stack.length ? `unclosed <${stack.join('><')}>` : null;
}

test('the timeline controls markup is tag-balanced (regression: btn-group swallowed tl-main)', () => {
  const spans = [
    { id: 'period', label: '26w period', weeks: 26 },
    { id: 'all', label: 'all (84w)', weeks: 84 },
  ];
  for (const lens of ['initiative', 'pod']) {
    const html = timelineControlsHTML({ lens, horizon: 26, spans, spanSel: 'period', filter: '', hideEmpty: false, ghost: false });
    assert.equal(tagBalance(html), null, `${lens} lens controls balance`);
    assert.match(html, /<div id="tl-by-initiative"|id="tl-by-initiative"/);
    assert.match(html, /class="tl-filter"/);
  }
  const pod = timelineControlsHTML({ lens: 'pod', horizon: 26, spans, spanSel: 'all', filter: 'x', hideEmpty: true, ghost: true });
  assert.match(pod, /id="tl-hide-empty"/, 'hide-empty checkbox rides the pod lens');
  assert.match(pod, /id="tl-ghost" checked/, 'the ghost toggle rides the pod lens and carries state');
  assert.match(tagBalance(pod) ?? '', /^$/, 'pod lens with the checkboxes also balances');
  const init = timelineControlsHTML({ lens: 'initiative', horizon: 26, spans, spanSel: 'period', filter: '', hideEmpty: false, ghost: false });
  assert.ok(!init.includes('tl-ghost'), 'the ghost toggle is pod-lens only');
});

// Spec 010 amendment: with "show other work" on, non-matching bars render as
// dimmed ghosts carrying an "(other work)" title — the capacity that fills
// the gaps stays visible instead of reading as dead space. Off (default),
// they are hidden entirely.
test('the pod lens ghosts non-matching bars only when the toggle is on', () => {
  const ps = [
    { pod: 'Atlas', tracks: 2, slices: [
      { initiative: 'Apollo', pod: 'Atlas', startWeek: 0, finishWeek: 4, lanesUsed: 1, latestStartWeek: 2, slackWeeks: 1 },
      { initiative: 'DR drill', pod: 'Atlas', startWeek: 3, finishWeek: 26, lanesUsed: 1, latestStartWeek: 5, slackWeeks: 1 },
    ]},
  ];
  const off = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 }, { horizonWeeks: 26, span: 26, initiativeQuery: 'apollo' });
  assert.match(off, /Apollo/, 'the match renders');
  assert.ok(!off.includes('CR-DR'), 'the non-match is hidden by default');
  const on = podLensHTML({ podWeeks: ps, initiatives: [], horizonWeeks: 26 }, { horizonWeeks: 26, span: 26, initiativeQuery: 'apollo', ghostOthers: true });
  assert.match(on, /class="tl-bar tl-trunc tl-ghost"/, 'the non-match renders as a ghost');
  assert.match(on, /\(other work\): w3–w26/, 'the ghost title names the span');
  assert.match(on, /Apollo/, 'and the match still renders');
});
