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

import { esc, weekToDate } from './order.js';
import { fuzzyMatch } from './filter.js';
import { term } from './terms.js';

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

// tickTitle is the hover date for a week label: the calendar day the week
// begins, from the schedule's own period start. No period start (a plan
// without dates yet) means no title — an invented anchor would date every
// week wrongly, which is worse than no date at all.
export function tickTitle(week, periodStart) {
  const d = weekToDate(week, periodStart);
  return d ? `week of ${d}` : '';
}

// todayLineHTML marks today (FR-038), positioned by week. Outside the period
// there is no line: a today that is not on the chart is not context.
export function todayLineHTML(week, horizon) {
  if (week < 0 || week > horizon) return '';
  const s = axisScale(horizon);
  return `<div class="tl-today" style="${pct(s(week))}" title="today (week ${week})">↑</div>`;
}

// periodEndHTML is the horizon marker (spec 004 follow-up): when the view spans
// past the period, a labelled line at the horizon week says where the selected
// period ends and the overrun begins. Without it, a 52-week view of a 26-week
// period reads as one undifferentiated stretch and the planner cannot tell
// promised work from overrun.
export function periodEndHTML(horizon, span) {
  if (span <= horizon) return '';
  const s = axisScale(span);
  const w = Math.min(horizon, span);
  return `<div class="tl-period-end" style="${pct(s(w))}" title="period end: week ${w} of ${span} shown">
    <span class="tl-period-end-label">period end · w${w}</span></div>`;
}

function barHTML({ left, width, cls = '', label, title, initiative, pod, startWeek, lane }) {
  // tl-trunc on every bar (FR-039): the CSS clips overflow with ellipsis, and
  // the full text survives in the title. data-initiative/pod/startWeek carry
  // the drag contract (spec 008): a released drag pins that slice's start.
  const drag = initiative ? ` data-initiative="${esc(initiative)}" data-pod="${esc(pod)}" data-start-week="${startWeek}"${lane !== undefined ? ` data-lane="${lane}"` : ''}` : '';
  return `<div class="tl-bar tl-trunc ${cls}" style="${pct(left)};width:${width.toFixed(2)}%" title="${esc(title)}"${drag}>${esc(label)}</div>`;
}

// barGeom clamps a week span to the chart: work that runs past the horizon
// (common — most plans overrun) must stop at 100% rather than extend outside
// the timeline, and the clamped weeks ride along so the title can say so.
// FR-035's no-overflow rule is about the container, not about hiding overrun.
function barGeom(startWeek, endWeek, horizon) {
  const s = axisScale(horizon);
  const clampedEnd = Math.min(endWeek, horizon);
  const left = Math.min(s(startWeek), 100);
  const width = Math.max(0, s(clampedEnd) - left);
  const overrun = Math.max(0, endWeek - horizon);
  return { left, width, overrun };
}

// sliceBar renders one pod slice's span. The handoff glyph marks a slice that
// waited on another pod (§13.3's "→ handoff"), so the dependency is visible
// even before the sub-row's text names it.
function sliceBar(sl, horizon) {
  const { left, width, overrun } = barGeom(sl.startWeek, sl.finishWeek, horizon);
  const deps = (sl.dependsOn || []).length ? '→ ' : '';
  return barHTML({
    left, width,
    label: `${deps}${sl.pod} ${sl.finishWeek - sl.startWeek}w`,
    title: `${sl.pod}: w${sl.startWeek}–w${sl.finishWeek}` +
      ((sl.dependsOn || []).length ? ` (after ${(sl.dependsOn).join(', ')})` : '') +
      (overrun > 0 ? ` — ${overrun}w past the horizon` : ''),
  });
}

// timelineRowHTML is one initiative: the bar (start → raw finish), the buffer
// tail appended after it, and the target diamond where a date exists (AC 8.1).
// With expand, one sub-row per pod slice in dependency order follows (AC 8.4),
// each naming the pods it waits on (FR-042).
export function timelineRowHTML(si, opts = {}) {
  const horizon = opts.horizonWeeks || 26;
  const s = axisScale(horizon);
  const work = barGeom(si.startWeek, si.rawFinishWeek, horizon);
  const buf = barGeom(Math.max(si.rawFinishWeek, 0), si.commitWeek, horizon);
  const overrun = Math.max(0, si.commitWeek - horizon);

  // A bar with no in-period width renders nothing — the CSS minimum width
  // would push a zero-width marker outside the container. Work that starts
  // beyond the horizon is named by an edge marker instead, so the row still
  // says what happened to it.
  const bar = work.width > 0 ? barHTML({
    left: work.left, width: work.width, label: si.name,
    title: `${si.name}: w${si.startWeek}–w${si.rawFinishWeek}, buffer ${si.bufferWeeks}w, commit w${si.commitWeek}` +
      (overrun > 0 ? ` — ${overrun}w past the horizon` : ''),
  }) : (si.startWeek >= horizon
    ? `<div class="tl-beyond" title="${esc(si.name)}: starts w${si.startWeek}, past this horizon">›</div>`
    : '');
  const buffer = buf.width > 0
    ? `<div class="tl-buffer tl-trunc" style="${pct(buf.left)};width:${buf.width.toFixed(2)}%" title="buffer: w${si.rawFinishWeek}–w${si.commitWeek} (protects the commit, not slack to spend)">+${si.bufferWeeks}w buffer</div>`
    : '';
  const target = (si.targetWeek !== undefined && si.targetWeek !== null && si.targetWeek <= horizon)
    ? `<div class="tl-target" style="${pct(s(si.targetWeek))}" title="target w${si.targetWeek}">◆</div>`
    : (si.targetWeek !== undefined && si.targetWeek !== null)
      ? `<div class="tl-target tl-target-beyond" style="${pct(s(horizon))}" title="target w${si.targetWeek} — beyond the horizon">◆›</div>`
      : '';

  let subrows = '';
  if (opts.expand) {
    subrows = (si.slices || []).map((sl) => {
      const waits = (sl.dependsOn || []).length
        ? `<span class="hint">← ${(sl.dependsOn).map(esc).join(', ')}</span>` : '';
      if ((opts.podQuery || '') && !fuzzyMatch(opts.podQuery, sl.pod)) return '';
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
    <div class="tl-track${subrows ? ' tl-expanded' : ''}">${bar}${buffer}${target}${subrows}</div>
  </div>`;
}

// bandHTML renders one calendar window as a marked vertical band (AC 8.5,
// FR-038). The band carries its name in text, not colour alone (FR-044): a
// freeze says "freeze", a holiday names its site, and the tooltip holds the
// dates. Weeks are mapped off the period start exactly as the Go side maps
// them — toDate inclusive.
function bandHTML(win, sched, horizon) {
  const s = axisScale(horizon);
  const from = weekOfDate(sched.periodStart, win.fromDate);
  const toInclusive = weekOfDate(sched.periodStart, win.toDate);
  if (from === null || toInclusive === null) return '';
  const left = Math.max(0, from);
  const right = Math.min(horizon, toInclusive + 1); // exclusive end
  if (right <= left) return '';
  const width = s(right) - s(left);
  const label = win.kind === 'change-freeze' ? '░freeze░'
    : win.kind === 'site-nonworking' ? `▒ ${win.scope} non-working ▒`
    : `▒ ${win.scope} ▒`;
  // The dates render as visible text beside the label: bands sit in the
  // pointer-events:none overlay, so a title would never fire, and a data
  // attribute is inert — the dates must be readable for a narrow band too.
  const dates = `${esc(win.fromDate)}–${esc(win.toDate)}`;
  return `<div class="tl-band tl-trunc ${win.kind === 'change-freeze' ? 'tl-band-freeze' : ''}"
    style="${pct(s(left))};width:${width.toFixed(2)}%">
    <span class="tl-band-label">${dates} ${esc(label)}</span>
  </div>`;
}

function weekOfDate(periodStart, date) {
  if (!periodStart || !date) return null;
  const t0 = new Date(`${periodStart.trim()}T00:00:00Z`).getTime();
  const t1 = new Date(`${date.trim()}T00:00:00Z`).getTime();
  if (Number.isNaN(t0) || Number.isNaN(t1)) return null;
  return Math.floor((t1 - t0) / (7 * 86400000));
}

// portfolioTimelineHTML is §13.3: the axis, one ranked row per initiative,
// today, and the legend. Freeze/non-working bands wait for FR-018's calendar
// windows; their absence renders nothing, which is why there is no branch for
// them here rather than a guess.
//
// The grid and today line live in an overlay that starts after the label
// column, so their week percentages address the same width the bars do.
export function portfolioTimelineHTML(sched, opts = {}) {
  // Pod filter (spec 010): slices not touching the typed pod dim; rows with
  // no lit slice collapse to a slim dimmed row so the matches read in order.
  const podQ = opts.podQuery || '';
  const horizon = opts.horizonWeeks || sched.horizonWeeks || 26;
  const span = opts.span || horizon; // the drawn span can exceed the period
  const s = axisScale(span);
  const ticks = axisTicks(span).map((t) => {
    const title = tickTitle(t.week, sched.periodStart || opts.periodStart);
    return `<span class="tl-tick" style="${pct(s(t.week))}"${title ? ` data-tip="${esc(title)}"` : ''}>${t.label}</span>`;
  }).join('');
  const grid = axisTicks(span).map((t) =>
    `<div class="tl-grid" style="${pct(s(t.week))}"></div>`).join('');
  const rows = (sched.initiatives || [])
    .slice()
    .sort((a, b) => a.proposedRank - b.proposedRank)
    .filter((si) => !podQ || (si.slices || []).some((sl) => fuzzyMatch(podQ, sl.pod)))
    .map((si) => timelineRowHTML(si, { ...opts, horizonWeeks: span, expand: opts.expand === si.name }))
    .join('');
  const today = opts.todayWeek === undefined || opts.todayWeek === null
    ? '' : todayLineHTML(opts.todayWeek, span);
  const bands = (opts.calendars || [])
    .map((win) => bandHTML(win, sched, span))
    .join('');
  const periodEnd = periodEndHTML(horizon, span);
  return `<div class="card tl-card">
    <div class="ord-head"><b>Timeline</b>
      <span class="hint">one row per initiative · the lighter tail is the ${term('buffer', 'buffer')} · ◆ is the ${term('target', 'target')}</span></div>
    <div class="tl-axis">${ticks}</div>
    <div class="tl-body">${rows}<div class="tl-overlay">${grid}${bands}${today}${periodEnd}</div></div>
    <div class="hint">█ scheduled · ░ buffer · ◆ target · → waits on another pod · ↑ today${bands ? ' · ░freeze░ change freeze · ▒ non-working' : ''}${periodEnd ? ' · │ period end' : ''}</div>
  </div>`;
}

// assignLanes packs slices into track lanes greedily: earliest start first,
// each onto the first lane free at its start. The count can never exceed the
// pod's tracks when the schedule is feasible — which is the capacity
// constraint made visual (FR-040).
// assignLanes packs slices into track lanes greedily: earliest start first,
// each onto the first lane free at its start. A multi-lane slice (spec 006:
// lanesUsed > 1) occupies that many CONSECUTIVE lanes — it is one piece of
// work running across the pod, and drawing it on a single track made the
// other tracks look idle while the server had them busy.
// cap is the pod's track count from the roster (spec 006: pairing halves
// devs, non-pairing one track per dev). The Gantt shows exactly that many
// lanes — never more. Slices are serialized by the scheduler to fit, so a
// stack beyond `cap` would mean a rendering bug, not more capacity.
function assignLanes(slices, cap = 0, pinnedLanes = null) {
  // Width for layout: the slice's PEAK lanes (a growing split slice spans
  // its widest phase), so the track rows reflect the most it ever occupies.
  const width = (sl) => Math.max(1, sl.lanesUsed || 1,
    ...(sl.phases || []).map((p) => p.lanes || 0));
  // Pinned slices (spec 008 vertical drag) take their pod-relative lane
  // offset FIRST; the rest pack around them.
  const forced = new Map();
  const rest = [];
  for (const sl of slices) {
    const off = pinnedLanes?.[sl.initiative];
    if (off !== undefined && Number.isInteger(off)) {
      forced.set(sl, off);
    } else {
      rest.push(sl);
    }
  }
  const sorted = [...forced.keys(), ...rest.sort((a, b) => a.startWeek - b.startWeek)];
  const laneEnds = [];
  const placement = [];
  for (const sl of sorted) {
    const w = Math.min(width(sl), cap || width(sl));
    let lane = 0;
    if (forced.has(sl)) {
      lane = Math.max(0, Math.min(forced.get(sl), (cap || 999) - w));
      // Two saved pins on the same lanes collide here too: unless the
      // forced span is genuinely free at sl.startWeek, fall back to the
      // walk rather than overlapping bars (cubic P2).
      let blocked = false;
      for (let i = 0; i < w; i++) {
        if ((laneEnds[lane + i] || 0) > sl.startWeek) { blocked = true; break; }
      }
      if (blocked) {
        lane = -1; // signal the walk
      }
    }
    if (lane < 0 || !forced.has(sl)) {
      lane = 0;
      for (;;) {
        // find the first `w` consecutive lanes all free at sl.startWeek
        let ok = true;
        for (let i = 0; i < w; i++) {
          if ((laneEnds[lane + i] || 0) > sl.startWeek) { ok = false; break; }
        }
        if (ok) break;
        lane++;
      }
    }
    for (let i = 0; i < w; i++) {
      laneEnds[lane + i] = sl.finishWeek;
      placement.push({ sl, lane: lane + i, lead: i === 0 });
    }
  }
  // Never draw more lanes than the pod has tracks. When time-overlapping
  // multi-lane slices cannot each get their own consecutive span (they were
  // serialized server-side, so they can), the walk pushed some past the cap —
  // dropping them would hide real work (Apollo vanished this way). Overflow
  // slices collapse onto the first lane, one row tall, with their width badge
  // still carrying lanesUsed.
  const lanes = cap > 0 ? Math.min(laneEnds.length, cap) : laneEnds.length;
  const fixed = [];
  for (const p of placement) {
    if (p.lane < lanes) { fixed.push(p); continue; }
    if (p.lead !== false) {
      // re-place the whole slice on lane 0 row-wise (visual stacking); its
      // continuations (lead === false) are skipped — one row represents it
      fixed.push({ ...p, lane: 0, collapsed: true });
    }
  }
  return { placement: fixed, lanes };
}

// podLanesHTML is one pod's track lanes (§13.4): every slice in start order,
// labelled by initiative, and idle tracks shown — visible slack on
// non-constraint pods is the point, not noise.
export function podLanesHTML(ps, opts = {}) {
  const horizon = opts.horizonWeeks || 26;
  // A pod with work but no tracks is not a lane puzzle — it is the unknown/
  // zero-capacity case, and giving it a track lane would claim capacity that
  // does not exist. Named for what it is instead.
  if (!ps.tracks) {
    const bars = (ps.slices || []).map((sl) => {
      const { left, width } = barGeom(sl.startWeek, sl.finishWeek, horizon);
      return barHTML({
        left, width, cls: 'tl-nocap', label: `${sl.initiative}`,
        title: `${sl.initiative}: w${sl.startWeek}–w${sl.finishWeek} — this pod has no tracks in the roster`,
      });
    }).join('');
    return `<div class="tl-lane"><span class="hint">no capacity</span><div class="tl-track">${bars || '<span class="hint">—</span>'}</div></div>`;
  }
  const q = opts.initiativeQuery || '';
  const { placement, lanes } = assignLanes(ps.slices || [], ps.tracks || 0, opts.pinnedLanes || null);
  const rows = [];
  for (let lane = 0; lane < Math.max(lanes, 1); lane++) {
    const inLane = placement.filter((p) => p.lane === lane);
    const bars = inLane.map(({ sl, lead, collapsed, lane }) => {
      // Per-phase geometry: a split slice's bar on each track covers that
      // phase's own weeks at that lane's occupancy. `lane` is the
      // pod-absolute row; the slice-relative offset is lane − offs, where
      // offs is the slice's own first lane in the packed placement (cubic:
      // mixing them picks the wrong phase; the fallback is the whole span).
      const relLane = placement.filter((p) => p.sl === sl).every((p) => p.lane === placement.filter((q) => q.sl === sl)[0]?.lane)
        ? lane : lane;
      // The slice-relative offset of this row: pick the min lane this slice
      // occupies in this placement, and index the phase by lane − offs.
      let offs = lane;
      {
        const own = placement.filter((p) => p.sl === sl).map((p) => p.lane);
        if (own.length) offs = Math.min(...own);
      }
      const phase = (sl.phases || []).find((ph) => {
        let base = 0;
        for (const ph0 of sl.phases || []) {
          if (lane - offs >= base && lane - offs < base + ph0.lanes) return true;
          base += ph0.lanes;
        }
        return false;
      });
      const pStart = phase ? phase.fromWeek : sl.startWeek;
      const pEnd = phase ? phase.toWeek : sl.finishWeek;
      const { left, width, overrun } = barGeom(pStart, pEnd, horizon);
      const wTag = (sl.lanesUsed || 1) > 1 ? ` ×${sl.lanesUsed}` : '';
      // Continuation rows carry the label too (dimmed): a track with an
      // unlabelled bar reads as empty space. The lead row keeps the fuller
      // styling; continuations show name + duration.
      const dur = `${pEnd - pStart}w`;
      // Hide non-matching bars entirely (spec 010 Decision 1, amended on the
      // product owner's call): the isolated track across pods IS the picture
      // — the dimmed crowd read as noise, not context.
      if (q && !fuzzyMatch(q, sl.initiative)) return '';
      return barHTML({
        left, width,
        cls: lead === false ? 'tl-cont' : '',
        label: `${sl.initiative} ${dur}${collapsed ? wTag : ''}`,
        initiative: sl.initiative, pod: sl.pod, startWeek: sl.startWeek, lane,
        title: `${sl.initiative}: w${sl.startWeek}–w${sl.finishWeek} · start by w${sl.latestStartWeek}` +
          (sl.slackWeeks === 0 ? ' · no slack' : ` · ${sl.slackWeeks}w slack`) +
          (overrun > 0 ? ` · ${overrun}w past the horizon` : '') +
          // Split slices (spec 007): the phase ladder is the honest shape of
          // the work — "3 lanes to w12, then 5" is what the team actually ran.
          ((sl.phases || []).length > 1
            ? ` · lanes ${sl.phases.map((ph) => `${ph.lanes}→w${ph.toWeek}`).join(', ')}`
            : ''),
      });
    }).join('');
    rows.push(`<div class="tl-lane"><span class="hint">track ${lane + 1}</span><div class="tl-track">${bars}</div></div>`);
  }
  // Idle tracks are lanes the schedule never needed — shown, not hidden.
  const idle = [];
  for (let lane = lanes; lane < ps.tracks; lane++) {
    idle.push(`<div class="tl-lane"><span class="hint">track ${lane + 1}</span><div class="tl-track"><span class="hint">· idle ·</span></div></div>`);
  }
  return rows.join('') + idle.join('');
}

// podRho is the mean weekly utilization over the configured horizon — every
// week in the period, busy or idle, and nothing past the horizon. Averaging
// only the busy weeks would rank a bursty pod as hot as a genuinely saturated
// one; averaging the overrun weeks would describe a period the lens does not
// show. Both disagree with the Constraints table this lens is meant to echo.
function podRho(ps, horizon) {
  const weeks = (ps.weeks || []).slice(0, Math.max(1, horizon));
  if (!weeks.length || !ps.tracks) return 0;
  let sum = 0;
  for (const w of weeks) sum += w.busy;
  return sum / (weeks.length * ps.tracks);
}

// podLensHTML is §13.4: pods hottest-first, one block of track lanes each.
export function podLensHTML(sched, opts = {}) {
  const horizon = opts.horizonWeeks || sched.horizonWeeks || 26;
  const span = opts.span || horizon;
  // Waterfall ordering (spec 010 amendment): under a filter, pods sort by
  // the earliest matching slice's start then finish — the initiative's chain
  // reads top-to-bottom like a dependency waterfall, which is the point of
  // tracing it. Unfiltered keeps the hottest-first capacity view.
  const q = opts.initiativeQuery || '';
  let pods = (sched.podWeeks || []).slice();
  if (q) {
    const key = (ps) => {
      const sl = (ps.slices || []).filter((s) => fuzzyMatch(q, s.initiative));
      if (!sl.length) return null;
      const start = Math.min(...sl.map((s) => s.startWeek));
      const finish = Math.min(...sl.filter((s) => s.startWeek === start).map((s) => s.finishWeek));
      return [start, finish];
    };
    const keyed = pods.map((ps) => ({ ps, k: key(ps) }));
    // matching pods waterfall first (start, then finish, then name for
    // determinism); non-matching pods keep the rho order after them —
    // hidden entirely when hideEmpty is set (the checkbox).
    const match = keyed.filter((x) => x.k).sort((a, b) => a.k[0] - b.k[0] || a.k[1] - b.k[1] || a.ps.pod.localeCompare(b.ps.pod));
    const rest = keyed.filter((x) => !x.k).sort((a, b) => podRho(b.ps, horizon) - podRho(a.ps, horizon));
    if (opts.hideEmptyPods) {
      pods = match.map((x) => x.ps);
    } else {
      pods = [...match, ...rest].map((x) => x.ps);
    }
  } else {
    pods.sort((a, b) => podRho(b, horizon) - podRho(a, horizon));
  }
  const blocks = pods.map((ps) => {
    const rho = podRho(ps, horizon);
    return `<div class="tl-pod" data-pod="${esc(ps.pod)}">
      <div class="ord-head"><b>${esc(ps.pod)}</b>
        <span class="hint">ρ ${rho.toFixed(2)} · ${ps.tracks} track${ps.tracks > 1 ? 's' : ''} · ${(ps.slices || []).length} slice${(ps.slices || []).length === 1 ? '' : 's'}</span>
        <button type="button" class="pod-export" data-export-pod="${esc(ps.pod)}" title="download this pod's timeline as a PNG">⬇ PNG</button></div>
      ${podLanesHTML(ps, { ...opts, horizonWeeks: span, pinnedLanes: (opts.pinnedLanes || {})[ps.pod] || null })}
    </div>`;
  }).join('');
  const s = axisScale(span);
  const ticks = axisTicks(span).map((t) => {
    const title = tickTitle(t.week, sched.periodStart || opts.periodStart);
    return `<span class="tl-tick" style="${pct(s(t.week))}"${title ? ` data-tip="${esc(title)}"` : ''}>${t.label}</span>`;
  }).join('');
  return `<div class="card tl-card">
    <div class="ord-head"><b>Timeline — by pod</b>
      <span class="hint">one lane per track · idle lanes are slack, shown on purpose · hottest first</span></div>
    <div class="tl-axis">${ticks}</div>
    <div class="tl-body"><div class="tl-overlay">${periodEndHTML(horizon, span)}</div></div>
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
      <span class="hint">${slices.length} slice${slices.length === 1 ? '' : 's'} in start order</span>
      <button type="button" class="pod-export" data-export-sheet="${esc(ps.pod)}" title="download this sheet as a PNG">⬇ PNG</button></div>
    <table class="wip-table">
      <thead><tr><th>Initiative</th><th>Weeks</th><th>Start</th><th>Start by</th><th>Slack</th><th>Waiting on</th><th>Blocks</th></tr></thead>
      <tbody>${rows || '<tr><td colspan="7" class="hint">No scheduled work at this pod.</td></tr>'}</tbody>
    </table>
    <p class="hint">⚠ no slack: starting later moves the initiative's commit date. "Start by" is the last week that does not.</p>
  </div>`;
}
