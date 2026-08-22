// timeline.js — Stories 8-9's views (spec 001 §13.3-§13.5): the portfolio
// timeline, the pod lens, and the pod sheet.
//
// Pure functions from the Schedule the server returns to HTML strings, exactly
// like order.js and baseline.js — planui.js owns fetching and the DOM, and
// node --test covers this whole surface against the committed Go fixture.
//
// FR-035: the whole horizon always fits the width. Every position is a
// percentage of the row; zoom is the axis aggregating its labels, never the
// bars widening past the container.
//
// FR-044: colour never carries meaning alone. The buffer tail is hatched AND
// labelled, the target is a diamond glyph, today is an arrow, zero slack is a
// ⚠ beside the number.

import { esc } from './order.js';

// axisScale maps a week onto the row width as a percentage. The row is the
// whole horizon — a 104-week plan renders 1 week at ~1%, which is exactly the
// regime FR-039's minimum-width floor exists for.
export function axisScale(horizon) {
  const h = Math.max(1, horizon || 26);
  return (week) => Math.round((week / h) * 10000) / 100;
}

const pct = (n) => `left:${n.toFixed(2)}%`;

// axisTicks picks the label density (§13.8): <=16w weekly, <=40w fortnightly,
// beyond that every 4 weeks. Bars keep week precision regardless — only the
// labels aggregate.
export function axisTicks(horizon) {
  const h = Math.max(1, Math.ceil(horizon || 26));
  let step = 1;
  if (h > 40) step = 4;
  else if (h > 16) step = 2;
  const out = [];
  for (let w = 0; w <= h; w += step) out.push({ week: w, label: `w${w}` });
  return out;
}

// todayLineHTML marks today (FR-038), positioned by week. Outside the period
// there is no line: a today that is not on the chart is not context.
export function todayLineHTML(week, horizon) {
  if (week < 0 || week > horizon) return '';
  const s = axisScale(horizon);
  return `<div class="tl-today" style="${pct(s(week))}" title="today (week ${week})">↑</div>`;
}

function barHTML({ left, width, cls = '', label, title }) {
  // tl-trunc on every bar (FR-039): the CSS clips overflow with ellipsis, and
  // the full text survives in the title.
  return `<div class="tl-bar tl-trunc ${cls}" style="${pct(left)};width:${width.toFixed(2)}%" title="${esc(title)}">${esc(label)}</div>`;
}

// sliceBar renders one pod slice's span. The handoff glyph marks a slice that
// waited on another pod (§13.3's "→ handoff"), so the dependency is visible
// even before the sub-row's text names it.
function sliceBar(sl, horizon) {
  const s = axisScale(horizon);
  const width = s(sl.finishWeek) - s(sl.startWeek);
  const deps = (sl.dependsOn || []).length ? '→ ' : '';
  return barHTML({
    left: s(sl.startWeek), width,
    label: `${deps}${sl.pod} ${sl.finishWeek - sl.startWeek}w`,
    title: `${sl.pod}: w${sl.startWeek}–w${sl.finishWeek}` +
      ((sl.dependsOn || []).length ? ` (after ${(sl.dependsOn).join(', ')})` : ''),
  });
}

// timelineRowHTML is one initiative: the bar (start → raw finish), the buffer
// tail appended after it, and the target diamond where a date exists (AC 8.1).
// With expand, one sub-row per pod slice in dependency order follows (AC 8.4),
// each naming the pods it waits on (FR-042).
export function timelineRowHTML(si, opts = {}) {
  const horizon = opts.horizonWeeks || 26;
  const s = axisScale(horizon);
  const left = s(si.startWeek);
  const workWidth = Math.max(0, s(si.rawFinishWeek) - left);
  const bufLeft = s(si.rawFinishWeek);
  const bufWidth = Math.max(0, s(si.commitWeek) - bufLeft);

  const bar = barHTML({
    left, width: workWidth, label: si.name,
    title: `${si.name}: w${si.startWeek}–w${si.rawFinishWeek}, buffer ${si.bufferWeeks}w, commit w${si.commitWeek}`,
  });
  const buffer = bufWidth > 0
    ? `<div class="tl-buffer tl-trunc" style="${pct(bufLeft)};width:${bufWidth.toFixed(2)}%" title="buffer: w${si.rawFinishWeek}–w${si.commitWeek} (protects the commit, not slack to spend)">+${si.bufferWeeks}w buffer</div>`
    : '';
  const target = (si.targetWeek !== undefined && si.targetWeek !== null)
    ? `<div class="tl-target" style="${pct(s(si.targetWeek))}" title="target w${si.targetWeek}">◆</div>`
    : '';

  let subrows = '';
  if (opts.expand) {
    subrows = (si.slices || []).map((sl) => {
      const waits = (sl.dependsOn || []).length
        ? `<span class="hint">← ${(sl.dependsOn).map(esc).join(', ')}</span>` : '';
      return `<div class="tl-subrow" data-pod="${esc(sl.pod)}">
        <span class="hint">└ ${esc(sl.pod)} ${sl.finishWeek - sl.startWeek}w</span>
        ${waits}
        ${sliceBar(sl, horizon)}
      </div>`;
    }).join('');
  }

  const expandMark = (si.slices || []).length > 1 ? '▸ ' : '';
  return `<div class="tl-row" data-init="${esc(si.name)}" data-expandable="${(si.slices || []).length > 1 ? 1 : 0}">
    <span class="tl-label tl-trunc" title="${esc(si.name)}">${expandMark}${esc(si.name)}</span>
    <div class="tl-track">${bar}${buffer}${target}${subrows}</div>
  </div>`;
}

// portfolioTimelineHTML is §13.3: the axis, one ranked row per initiative,
// today, and the legend. Freeze/non-working bands wait for FR-018's calendar
// windows; their absence renders nothing, which is why there is no branch for
// them here rather than a guess.
export function portfolioTimelineHTML(sched, opts = {}) {
  const horizon = opts.horizonWeeks || sched.horizonWeeks || 26;
  const s = axisScale(horizon);
  const ticks = axisTicks(horizon).map((t) =>
    `<span class="tl-tick" style="${pct(s(t.week))}">${t.label}</span>`).join('');
  const grid = axisTicks(horizon).map((t) =>
    `<div class="tl-grid" style="${pct(s(t.week))}"></div>`).join('');
  const rows = (sched.initiatives || [])
    .slice()
    .sort((a, b) => a.proposedRank - b.proposedRank)
    .map((si) => timelineRowHTML(si, { ...opts, horizonWeeks: horizon, expand: opts.expand === si.name }))
    .join('');
  const today = opts.todayWeek === undefined ? '' : todayLineHTML(opts.todayWeek, horizon);
  return `<div class="card tl-card">
    <div class="ord-head"><b>Timeline</b>
      <span class="hint">one row per initiative · the lighter tail is the buffer · ◆ is the promise</span></div>
    <div class="tl-axis tl-trunc">${ticks}</div>
    <div class="tl-body">${grid}${today}${rows}</div>
    <div class="hint">█ scheduled · ░ buffer · ◆ target · → waits on another pod · ↑ today</div>
  </div>`;
}

// assignLanes packs slices into track lanes greedily: earliest start first,
// each onto the first lane free at its start. The count can never exceed the
// pod's tracks when the schedule is feasible — which is the capacity
// constraint made visual (FR-040).
function assignLanes(slices) {
  const sorted = slices.slice().sort((a, b) => a.startWeek - b.startWeek);
  const laneEnds = [];
  const placement = sorted.map((sl) => {
    let lane = laneEnds.findIndex((end) => end <= sl.startWeek);
    if (lane === -1) {
      lane = laneEnds.length;
      laneEnds.push(0);
    }
    laneEnds[lane] = sl.finishWeek;
    return { sl, lane };
  });
  return { placement, lanes: laneEnds.length };
}

// podLanesHTML is one pod's track lanes (§13.4): every slice in start order,
// labelled by initiative, and idle tracks shown — visible slack on
// non-constraint pods is the point, not noise.
export function podLanesHTML(ps, opts = {}) {
  const horizon = opts.horizonWeeks || 26;
  const { placement, lanes } = assignLanes(ps.slices || []);
  const rows = [];
  for (let lane = 0; lane < Math.max(lanes, 1); lane++) {
    const inLane = placement.filter((p) => p.lane === lane);
    const bars = inLane.map(({ sl }) => {
      const s = axisScale(horizon);
      return barHTML({
        left: s(sl.startWeek), width: Math.max(0, s(sl.finishWeek) - s(sl.startWeek)),
        label: `${sl.initiative} ${sl.finishWeek - sl.startWeek}w`,
        title: `${sl.initiative}: w${sl.startWeek}–w${sl.finishWeek} · start by w${sl.latestStartWeek}` +
          (sl.slackWeeks === 0 ? ' · no slack' : ` · ${sl.slackWeeks}w slack`),
      });
    }).join('');
    rows.push(`<div class="tl-lane"><span class="hint">track ${lane + 1}</span><div class="tl-track">${bars}</div></div>`);
  }
  // Idle tracks are lanes the schedule never needed — shown, not hidden.
  const idle = [];
  for (let lane = lanes; lane < (ps.tracks || lanes); lane++) {
    idle.push(`<div class="tl-lane"><span class="hint">track ${lane + 1}</span><div class="tl-track"><span class="hint">· idle ·</span></div></div>`);
  }
  return rows.join('') + idle.join('');
}

// podRho is the mean weekly utilization from the schedule's own occupancy —
// the number the Constraints table sorts by, recomputed here because
// PodSchedule does not carry it.
function podRho(ps) {
  const weeks = (ps.weeks || []).filter((w) => w.busy > 0);
  if (!weeks.length || !ps.tracks) return 0;
  let sum = 0;
  for (const w of weeks) sum += w.busy / ps.tracks;
  return sum / weeks.length;
}

// podLensHTML is §13.4: pods hottest-first, one block of track lanes each.
export function podLensHTML(sched, opts = {}) {
  const pods = (sched.podWeeks || []).slice()
    .sort((a, b) => podRho(b) - podRho(a));
  const blocks = pods.map((ps) => {
    const rho = podRho(ps);
    return `<div class="tl-pod" data-pod="${esc(ps.pod)}">
      <div class="ord-head"><b>${esc(ps.pod)}</b>
        <span class="hint">ρ ${rho.toFixed(2)} · ${ps.tracks} track${ps.tracks > 1 ? 's' : ''} · ${(ps.slices || []).length} slice${(ps.slices || []).length === 1 ? '' : 's'}</span></div>
      ${podLanesHTML(ps, opts)}
    </div>`;
  }).join('');
  return `<div class="card tl-card">
    <div class="ord-head"><b>Timeline — by pod</b>
      <span class="hint">one lane per track · idle lanes are slack, shown on purpose · hottest first</span></div>
    ${blocks}
  </div>`;
}

// podSheetHTML is §13.5, the team's own sheet: every slice in start order with
// Start, Start by (FR-041), Slack (AC 9.3's ⚠ on zero), Waiting on, and Blocks
// — who is waiting on this pod, the thing pods most often cannot see.
export function podSheetHTML(ps, sched, opts = {}) {
  const slices = (ps.slices || []).slice().sort((a, b) => a.startWeek - b.startWeek);
  const byInit = {};
  for (const si of sched.initiatives || []) byInit[si.name] = si;

  const rows = slices.map((sl) => {
    const waits = (sl.dependsOn || []);
    // Blocks: pods whose slice in the same initiative names this one upstream.
    const blocks = [];
    const sib = (byInit[sl.initiative] || {}).slices || [];
    for (const other of sib) {
      if (other.pod !== sl.pod && (other.dependsOn || []).includes(sl.pod)) {
        blocks.push(other.pod);
      }
    }
    const slackTxt = sl.slackWeeks === 0
      ? '<b>none ⚠</b>'
      : `${sl.slackWeeks}w`;
    return `<tr>
      <td>${esc(sl.initiative)}</td>
      <td>${sl.finishWeek - sl.startWeek}w</td>
      <td>w${sl.startWeek}</td>
      <td>w${sl.latestStartWeek}</td>
      <td>${slackTxt}</td>
      <td>${waits.length ? waits.map(esc).join(', ') : '<span class="hint">—</span>'}</td>
      <td>${blocks.length ? blocks.map(esc).join(', ') : '<span class="hint">—</span>'}</td>
    </tr>`;
  }).join('');

  return `<div class="card ord-card" data-pod-sheet="${esc(ps.pod)}">
    <div class="ord-head"><b>${esc(ps.pod)} — ${ps.tracks} track${ps.tracks > 1 ? 's' : ''}</b>
      <span class="hint">${slices.length} slice${slices.length === 1 ? '' : 's'} in start order</span></div>
    <table class="wip-table">
      <thead><tr><th>Initiative</th><th>Weeks</th><th>Start</th><th>Start by</th><th>Slack</th><th>Waiting on</th><th>Blocks</th></tr></thead>
      <tbody>${rows || '<tr><td colspan="7" class="hint">No scheduled work at this pod.</td></tr>'}</tbody>
    </table>
    <p class="hint">⚠ no slack: starting later moves the initiative's commit date. "Start by" is the last week that does not.</p>
  </div>`;
}
