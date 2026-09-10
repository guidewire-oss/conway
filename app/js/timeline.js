import { icon } from './icons.js';
// timeline.js — Stories 8-9's views (spec 001 §13.3-§13.5): the portfolio
// timeline, the pod lens, and the pod sheet.
//
// Pure functions from the Schedule the server returns to HTML strings, exactly
// like order.js and baseline.js — planui.js owns fetching and the DOM, and
// node --test covers this whole surface against the committed Go fixture.
//
// specs/034-readable-scrollable-timelines.md:88: every row shares a scrollable
// time canvas; percentages retain schedule precision without squeezing long spans.
//
// FR-044: colour never carries meaning alone. The buffer tail is hatched AND
// labelled, the target is a diamond glyph, today is an arrow, zero slack is a
// ⚠ beside the number.

import { esc, weekToDate, weekDateHTML, LEAD_ROLES } from './order.js';
import { fuzzyMatch, initiativeMatch } from './filter.js';
import { term } from './terms.js';

const unplaced = (si) => ['beyond-horizon', 'unschedulable'].includes(si.verdict) && !(si.slices || []).length;
const assignedPods = (si, inputs = []) => {
  const input = inputs.find((it) => it.name === si.name);
  return [...new Set([...(si.slices || []).map((sl) => sl.pod),
    ...Object.entries(input?.work || {}).filter(([, work]) => work.inPath).map(([pod]) => pod)])];
};
export const matchesTimelineTeam = (si, query, inputs = []) => !query || assignedPods(si, inputs).some((pod) => fuzzyMatch(query, pod));
function unscheduledReason(si) {
  const state = si.verdict === 'beyond-horizon' ? 'Not scheduled within this period' : 'Not scheduled';
  const assumptions = (si.assumptions || []).filter(Boolean).map(warningText).join('; ');
  const cause = si.bindingConstraint === 'lead' ? 'hard lead-capacity limit' : (si.bindingConstraint || 'the scheduler did not return a placement');
  return `${state}: ${cause}. No dates were calculated because this work is held.${si.bindingConstraint === 'lead' ? ' Input start and finish dates are not required. Use advisory lead limits in Scheduling assumptions to schedule against team capacity.' : ''}${assumptions ? ` Assumptions: ${assumptions}` : ''}`;
}
function warningText(text) {
  return String(text).replace(/\b(pm|eng|architect|pgm)\b(?=\s+(?:lead|ownership|capacity|limit))/gi,
    (role) => LEAD_ROLES.find(([key]) => key === role.toLowerCase())?.[1] || role);
}
function assumptionsHTML(si) {
  const assumptions = [...new Set((si?.assumptions || []).filter(Boolean))];
  return assumptions.length ? `<div class="tl-assumptions"><b>Forecast assumptions</b><ul>${assumptions.map((item) => `<li>${esc(warningText(item))}</li>`).join('')}</ul></div>` : '';
}
function unscheduledTeamHTML(items) {
  if (!items.length) return '';
  return `<div class="tl-unplaced-list"><b>Assigned work without a placement</b>${items.map((si) =>
    `<p><button class="btn btn-secondary btn-sm" type="button" data-select-init="${esc(si.name)}">${esc(si.name)}</button> ${esc(unscheduledReason(si))}</p>`).join('')}</div>`;
}

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

// specs/019-scheduling-audit-and-gantt-integrity.md:166: the same axis travels
// with each team and its export. Compact labels retain both ends of the scale.
function timeAxisHTML(span, periodStart) {
  const scale = axisScale(span), ticks = axisTicks(span);
  if (ticks.at(-1)?.week !== span) {
    // Leave a full tick interval before the endpoint, avoiding crowded dates.
    if (ticks.length > 1) ticks.pop();
    ticks.push({ week: span, label: `w${span}` });
  }
  const compactIndices = new Set(Array.from({ length: Math.min(5, ticks.length) }, (_, index) =>
    Math.round(index * (ticks.length - 1) / Math.min(4, ticks.length - 1))));
  return `<div class="tl-axis">${ticks.map((t, index) => {
    const title = tickTitle(t.week, periodStart);
    const compact = compactIndices.has(index);
    return `<span class="tl-tick${compact ? ' tl-tick-compact' : ''}" style="${pct(scale(t.week))}"${title ? ` title="${esc(title)}"` : ''}>${t.label}${title ? `<small class="tl-tick-date">${weekToDate(t.week, periodStart).slice(5)}</small>` : ''}</span>`;
  }).join('')}</div>`;
}

function timelinePlotHTML(axis, body, span, label) {
  const timeWidth = Math.max(1, span) * (span <= 16 ? 48 : 24);
  return `<div class="tl-plot" style="--tl-time-width:${timeWidth}px">
    <div class="tl-scroll overflow-x-auto" tabindex="0" role="region" aria-label="${esc(label)}; scroll horizontally for later weeks">
      <div class="tl-canvas"><div class="tl-axis-row"><div class="tl-axis-label" aria-hidden="true"></div>${axis}</div><div class="tl-body">${body}</div></div>
    </div>
    <div class="tl-label-resizer" role="separator" tabindex="0" aria-orientation="vertical" aria-label="Resize timeline labels" aria-valuemin="96" aria-valuemax="640" aria-valuenow="160" title="Drag to resize names; arrow keys adjust; Home resets"></div>
  </div>`;
}

function contextHTML(sched, opts, horizon, span) {
  const grid = axisTicks(span).map((t) => `<div class="tl-grid" style="${pct(axisScale(span)(t.week))}"></div>`).join('');
  const today = opts.todayWeek == null ? '' : todayLineHTML(opts.todayWeek, span);
  const bands = (opts.calendars || []).map((win) => bandHTML(win, { ...sched, periodStart: sched.periodStart || opts.periodStart }, span)).join('');
  return `<div class="tl-overlay">${grid}${bands}${today}${periodEndHTML(horizon, span)}</div>`;
}

function calendarContextHTML(sched, opts, span) {
  const context = { ...sched, periodStart: sched.periodStart || opts.periodStart };
  const windows = (opts.calendars || []).filter((win) => bandHTML(win, context, span));
  if (!windows.length) return '';
  return `<div class="tl-calendar-context" aria-label="Calendar context">${windows.map((win) => {
    const label = win.kind === 'change-freeze' ? 'Change freeze'
      : win.kind === 'site-nonworking' ? `${win.scope || 'Site'} non-working` : win.scope || win.kind || 'Calendar window';
    return `<span>${esc(label)}: ${esc(win.fromDate)}–${esc(win.toDate)}</span>`;
  }).join('')}</div>`;
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

function barHTML({ left, width, cls = '', label, title, initiative, pod, startWeek, lane, laneOrigin, estimate, lanes, loss }) {
  // tl-trunc on every bar (FR-039): the CSS clips overflow with ellipsis, and
  // the full text survives in the title. data-initiative/pod/startWeek carry
  // the drag contract (spec 008): a released drag pins that slice's start.
  // data-estimate carries the slice's effort weeks for the right-edge resize;
  // data-loss the pod's effective loss percent, so the resize converts
  // duration to effort at the rate the engine will re-apply (spec 014).
  const drag = initiative ? ` role="button" tabindex="0" aria-label="Select ${esc(initiative)} on ${esc(pod)} for precise editing" data-initiative="${esc(initiative)}" data-pod="${esc(pod)}" data-start-week="${startWeek}"${estimate !== undefined ? ` data-estimate="${estimate}"` : ''}${lanes !== undefined ? ` data-lanes="${lanes}"` : ''}${loss !== undefined ? ` data-loss="${loss}"` : ''}${lane !== undefined ? ` data-lane="${lane}"` : ''}${laneOrigin !== undefined ? ` data-lane-origin="${laneOrigin}"` : ''}` : '';
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

function checkpointHTML(name, week, span) {
  if (!Number.isFinite(week) || week < 0 || week > span) return '';
  return `<button type="button" class="btn btn-link p-0 tl-checkpoint" data-select-init="${esc(name)}" style="${pct(axisScale(span)(week))}" title="${esc(name)}: checkpoint w${week} (no track time)" aria-label="Select ${esc(name)}: checkpoint week ${week}, no track time">◆</button>`;
}

// sliceBar renders one pod slice's span. The handoff glyph marks a slice that
// waited on another pod (§13.3's "→ handoff"), so the dependency is visible
// even before the sub-row's text names it.
function sliceBar(sl, horizon) {
  const { left, width, overrun } = barGeom(sl.startWeek, sl.finishWeek, horizon);
  if (!width && sl.startWeek >= horizon) return `<span class="tl-outside">Outside this view: w${sl.startWeek}–w${sl.finishWeek}</span>`;
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
  // specs/018-scheduling-capacity-and-timeline-correctness.md:74: rejected
  // starts carry zero sentinels, not week-zero dates or an empty chart row.
  if (unplaced(si)) {
    return `<div class="tl-row tl-unplaced" data-init="${esc(si.name)}" data-expandable="0">
      <button type="button" class="btn btn-secondary py-0 ps-0 text-start border-0 tl-label tl-trunc" data-select-init="${esc(si.name)}" aria-pressed="${opts.selected === si.name}">${esc(si.name)}</button>
      <div class="tl-track"><p>${esc(unscheduledReason(si))}</p></div></div>`;
  }
  const horizon = opts.horizonWeeks || 26;
  const s = axisScale(horizon);
  const work = barGeom(si.startWeek, si.rawFinishWeek, horizon);
  const buf = barGeom(Math.max(si.rawFinishWeek, 0), si.commitWeek, horizon);
  const overrun = Math.max(0, si.commitWeek - horizon);

  // A bar with no in-period width renders nothing — the CSS minimum width
  // would push a zero-width marker outside the container. Work that starts
  // beyond the horizon is named by an edge marker instead, so the row still
  // says what happened to it.
  const bar = si.startWeek === si.rawFinishWeek && si.startWeek <= horizon ? checkpointHTML(si.name, si.startWeek, horizon) : work.width > 0 ? barHTML({
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
    <button type="button" class="btn btn-secondary py-0 ps-0 text-start border-0 tl-label tl-trunc" data-select-init="${esc(si.name)}" aria-pressed="${opts.selected === si.name}" ${(si.slices || []).length > 1 ? `aria-expanded="${!!opts.expand}"` : ''} title="Select ${esc(si.name)}">${expandMark}${esc(si.name)}</button>
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
  // Full labels and dates render in the separate calendar context below the
  // tracks. Decorative overlay text is hidden so it cannot cover a work bar.
  const dates = `${esc(win.fromDate)}–${esc(win.toDate)}`;
  return `<div class="tl-band tl-trunc ${win.kind === 'change-freeze' ? 'tl-band-freeze' : ''}"
    style="${pct(s(left))};width:${width.toFixed(2)}%">
    <span class="tl-band-label" aria-hidden="true">${dates} ${esc(label)}</span>
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
  const rows = (sched.initiatives || [])
    .slice()
    .sort((a, b) => a.proposedRank - b.proposedRank)
    .filter((si) => !opts.initiativeQuery || initiativeMatch(opts.initiativeQuery, si.name))
    .filter((si) => matchesTimelineTeam(si, podQ, opts.planInitiatives))
    .map((si) => timelineRowHTML(si, { ...opts, horizonWeeks: span, periodStart: sched.periodStart || opts.periodStart, expand: opts.expand === si.name }))
    .join('');
  const bands = (opts.calendars || []).length;
  const periodEnd = periodEndHTML(horizon, span);
  return `<div class="card p-3 panel-card tl-card">
    <div class="ord-head"><b>Timeline</b>
      <span class="hint">one row per initiative · the lighter tail is the ${term('buffer', 'buffer')} · ◆ is the ${term('target', 'target')}</span></div>
    <p class="hint mb-2">Scroll horizontally for later weeks. Drag the divider beside the labels to reveal longer names.</p>
    ${timelinePlotHTML(timeAxisHTML(span, sched.periodStart || opts.periodStart), rows + contextHTML(sched, opts, horizon, span), span, 'Initiative timeline')}
    ${calendarContextHTML(sched, opts, span)}
    <div class="hint">█ scheduled · ░ buffer · ◆ target · → waits on another pod · ↑ today${bands ? ' · ░freeze░ change freeze · ▒ non-working' : ''}${periodEnd ? ' · │ period end' : ''}</div>
  </div>`;
}

// Place the server's occupied intervals on physical tracks without reserving
// a split slice's future peak width before that phase actually starts.
function assignLanes(slices, cap = 0, pinnedLanes = null) {
  // specs/018-scheduling-capacity-and-timeline-correctness.md:75: phases
  // reserve their actual intervals. Assign chronological phases to free
  // physical lanes, retaining the prior phase's lanes whenever possible.
  const segments = slices.flatMap((sl, index) => {
    const phases = sl.displayPhases ?? (sl.phases?.length ? sl.phases : [{ fromWeek: sl.startWeek, toWeek: sl.finishWeek, lanes: sl.lanesUsed || 1 }]);
    return phases.map((phase) => ({ sl, phase, index }));
  }).sort((a, b) =>
    // Saved offsets reserve their future intervals before flexible work. An
    // earlier unpinned task may use another lane instead of displacing a pin.
    (Number.isInteger(pinnedLanes?.[b.sl.initiative]) ? 1 : 0) - (Number.isInteger(pinnedLanes?.[a.sl.initiative]) ? 1 : 0) ||
    a.phase.fromWeek - b.phase.fromWeek || a.index - b.index);
  const reserved = [], previous = new Map(), placement = [];
  const place = (sl, phase, maySplit = true) => {
    const width = Math.min(Math.max(1, phase.lanes || 1), cap || Infinity);
    const limit = cap || reserved.length + width;
    const off = pinnedLanes?.[sl.initiative];
    const pinned = Number.isInteger(off) ? Array.from({ length: width }, (_, i) => Math.max(0, Math.min(off, limit - width)) + i) : [];
    const candidates = [...new Set([...pinned, ...(previous.get(sl) || []), ...Array.from({ length: limit }, (_, i) => i)])];
    const free = candidates.filter((lane) => lane < limit && !(reserved[lane] || []).some((span) =>
      span.fromWeek < phase.toWeek && phase.fromWeek < span.toWeek));
    // specs/019-scheduling-audit-and-gantt-integrity.md:174: fixed future
    // reservations can require flexible work to change physical lanes.
    // Split only when no continuous lane assignment exists; dates and total
    // occupied lane-weeks still come from the original server interval.
    if (maySplit && !Number.isInteger(off) && free.length < width) {
      const boundaries = [...new Set([phase.fromWeek, phase.toWeek, ...reserved.flat().flatMap((p) => [p.fromWeek, p.toWeek])])]
        .filter((week) => week >= phase.fromWeek && week <= phase.toWeek).sort((a, b) => a - b);
      if (boundaries.length > 2) {
        for (let i = 1; i < boundaries.length; i++) place(sl, { ...phase, fromWeek: boundaries[i - 1], toWeek: boundaries[i], layoutSplit: true }, false);
        return;
      }
    }
    const chosen = free.slice(0, width);
    const collapsed = chosen.length < width;
    // Inconsistent server occupancy must not invent a physical track or hide
    // the initiative. A single labeled overlap keeps that exceptional case visible.
    if (collapsed) { chosen.length = 0; chosen.push(free[0] ?? 0); }
    chosen.forEach((lane, i) => {
      (reserved[lane] ||= []).push(phase);
      placement.push({ sl, phase, lane, lead: i === 0, collapsed });
    });
    previous.set(sl, chosen);
  };
  for (const { sl, phase } of segments) place(sl, phase);
  return { placement, lanes: reserved.length };
}

// specs/019-scheduling-audit-and-gantt-integrity.md:177: weekly occupancy is
// authoritative for flat slices whose elapsed duration includes calendar gaps.
// Require evidence for every week; older snapshots retain their interval fallback.
function displaySlices(ps) {
  const weeks = new Map((ps.weeks || []).map((week) => [week.week, week]));
  return (ps.slices || []).map((sl) => {
    if (sl.phases?.length || !Number.isInteger(sl.startWeek) || !Number.isInteger(sl.finishWeek)) return sl;
    const phases = [];
    for (let week = sl.startWeek; week < sl.finishWeek; week++) {
      const occupancy = weeks.get(week);
      if (!occupancy || !(Array.isArray(occupancy.initiatives) || (occupancy.initiatives == null && occupancy.busy === 0))) return sl;
      if (!(occupancy.initiatives || []).includes(sl.initiative)) continue;
      const previous = phases.at(-1);
      if (previous?.toWeek === week) previous.toWeek = week + 1;
      else phases.push({ fromWeek: week, toWeek: week + 1, lanes: sl.lanesUsed || 1, layoutSplit: true });
    }
    if (phases.length === 1 && phases[0].fromWeek === sl.startWeek && phases[0].toWeek === sl.finishWeek) return sl;
    return { ...sl, displayPhases: phases };
  });
}

function outsideWorkHTML(ps, opts) {
  const horizon = opts.horizonWeeks || 26, query = opts.initiativeQuery || '';
  const outside = displaySlices(ps).filter((sl) => (!query || initiativeMatch(query, sl.initiative)) &&
    (sl.startWeek >= horizon || (sl.displayPhases || sl.phases)?.some((phase) => phase.fromWeek >= horizon)));
  return outside.length ? `<div class="tl-outside-list"><b>Work outside this view</b>${outside.map((sl) =>
    `<p><button class="btn btn-secondary btn-sm" type="button" data-select-init="${esc(sl.initiative)}">${esc(sl.initiative)}</button> ${esc(ps.pod)}: w${sl.startWeek}–w${sl.finishWeek}. Widen the time span to see the remaining work.</p>`).join('')}</div>` : '';
}

// podLanesHTML is one pod's track lanes (§13.4): every slice in start order,
// labelled by initiative, and idle tracks shown — visible slack on
// non-constraint pods is the point, not noise.
export function podLanesHTML(ps, opts = {}) {
  const horizon = opts.horizonWeeks || 26;
  const q = opts.initiativeQuery || '';
  const ghost = !!opts.ghostOthers;
  const outsideHTML = opts.includeOutside === false ? '' : outsideWorkHTML(ps, opts);
  const checkpoints = (ps.slices || []).filter(sl => sl.startWeek === sl.finishWeek && sl.startWeek <= horizon && (!q || initiativeMatch(q, sl.initiative)))
    .map(sl => checkpointHTML(sl.initiative, sl.startWeek, horizon)).join('');
  const checkpointRow = checkpoints ? `<div class="tl-lane tl-checkpoints"><span class="hint">checkpoints</span><div class="tl-track">${checkpoints}</div></div>` : '';
  // A pod with work but no tracks is not a lane puzzle — it is the unknown/
  // zero-capacity case, and giving it a track lane would claim capacity that
  // does not exist. Named for what it is instead.
  if (!ps.tracks) {
    const bars = (ps.slices || []).map((sl) => {
      const { left, width } = barGeom(sl.startWeek, sl.finishWeek, horizon);
      const matched = !q || initiativeMatch(q, sl.initiative);
      if (!width || (!matched && !ghost)) return '';
      return barHTML({
        left, width, cls: `tl-nocap${matched ? '' : ' tl-ghost'}`, label: matched ? sl.initiative : '',
        title: `${sl.initiative}: w${sl.startWeek}–w${sl.finishWeek} — this pod has no tracks in the roster`,
      });
    }).join('');
    return `<div class="tl-lane"><span class="hint">no capacity</span><div class="tl-track">${bars || '<span class="hint">—</span>'}</div></div>${checkpointRow}${outsideHTML}`;
  }
  const { placement, lanes } = assignLanes(displaySlices(ps), ps.tracks || 0, opts.pinnedLanes || null);
  const laneOrigins = new Map();
  for (const p of placement) if (p.lead && !laneOrigins.has(p.sl)) laneOrigins.set(p.sl, p.lane);
  // Spec 008 S4: bars carry the initiative's ABSOLUTE effort weeks for the
  // right-edge resize (estimateEdits is pod -> effort). The schedule's slices
  // only know their post-division duration, so the plan's initiatives supply
  // the effort; the duration is the fallback when the plan is not at hand.
  // In-flight initiatives carry NO estimate: their remaining effort is
  // progress-adjusted and the absolute estimate cannot be derived from the
  // bar, so the resize gesture is withheld (falls through to a move).
  const effortOf = Object.create(null);
  for (const pi of opts.planInitiatives || []) effortOf[pi.name] = pi;
  const effortWeeks = (sl) => {
    const pi = effortOf[sl.initiative];
    if (pi?.inFlight) return undefined;
    const w = pi?.work?.[sl.pod]?.weeks;
    return (typeof w === 'number' && w > 0) ? w : sl.remainingWeeks;
  };
  const rows = [];
  for (let lane = 0; lane < lanes; lane++) {
    const inLane = placement.filter((p) => p.lane === lane);
    const bars = inLane.map(({ sl, phase, lead, collapsed, lane }) => {
      const pStart = phase.fromWeek;
      const pEnd = phase.toWeek;
      const { left, width, overrun } = barGeom(pStart, pEnd, horizon);
      if (!width) return '';
      const wTag = (sl.lanesUsed || 1) > 1 ? ` ×${sl.lanesUsed}` : '';
      // Continuation rows carry the label too (dimmed): a track with an
      // unlabelled bar reads as empty space. The lead row keeps the fuller
      // styling; continuations show name + duration.
      const dur = `${pEnd - pStart}w`;
      // Filter behavior (spec 010 Decision 1 as amended + ghost toggle from
      // the product owner's review): non-matching bars are hidden by default —
      // the isolated track across pods IS the picture. With "show other work"
      // on, they render as label-less ghosts at low opacity so the hidden
      // context (what else held the lanes during the gaps) is visible without
      // stealing focus. Ghosts are context, not editable plan — no drag
      // contract attached.
      const matched = !q || initiativeMatch(q, sl.initiative);
      if (!matched && !ghost) return '';
      if (!matched) {
        return `<div class="tl-bar tl-trunc tl-ghost" style="${pct(left)};width:${width.toFixed(2)}%" title="${esc(`${sl.initiative} (other work): w${sl.startWeek}–w${sl.finishWeek}`)}"></div>`;
      }
      return barHTML({
        left, width,
        cls: lead === false ? 'tl-cont' : '',
        label: `${sl.initiative} ${dur}${collapsed ? wTag : ''}`,
        initiative: sl.initiative, pod: sl.pod, startWeek: sl.startWeek, lane, laneOrigin: laneOrigins.get(sl),
        // A phase width is not the whole slice's duration. Keep move gestures
        // but route split-estimate changes through the precise inspector.
        estimate: sl.phases?.length > 1 || phase.layoutSplit ? undefined : effortWeeks(sl), lanes: sl.lanesUsed || 1,
        // Spec 014: the pod's effective loss rides the bar, so the resize
        // gesture converts duration to effort at the same rate the engine
        // will — a 30%-loss pod's drag is not a 10%-loss drag.
        loss: ps.lossPct,
        title: `${sl.initiative}: w${sl.startWeek}–w${sl.finishWeek} · start by w${sl.latestStartWeek}` +
          (sl.phases?.length || sl.displayPhases ? ` · this phase w${pStart}–w${pEnd}, ${phase.lanes} lanes · use precise controls to edit the total estimate` : '') +
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
  return rows.join('') + idle.join('') + checkpointRow + outsideHTML;
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
  const ghost = !!opts.ghostOthers;
  const rejected = (sched.initiatives || []).filter((si) => unplaced(si) && (!q || initiativeMatch(q, si.name)));
  const rejectedAt = (pod) => rejected.filter((si) => assignedPods(si, opts.planInitiatives).includes(pod));
  let pods = (sched.podWeeks || []).filter((ps) => !opts.podQuery || fuzzyMatch(opts.podQuery, ps.pod));
  if (q) {
    const key = (ps) => {
      const sl = (ps.slices || []).filter((s) => initiativeMatch(q, s.initiative));
      if (!sl.length) return rejectedAt(ps.pod).length ? [Infinity, Infinity] : null;
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
    // Spec 014 FR-005: an overridden pod's loss is legible where the pod is —
    // inheriting pods stay quiet (the plan header owns the global figure).
    const loss = ps.lossOverride && ps.lossPct ? ` · <b title="this pod's own capacity loss override">loss ${ps.lossPct}%</b> ${term('loss')}` : '';
    return `<div class="tl-pod" data-pod="${esc(ps.pod)}">
      <div class="ord-head"><b>${esc(ps.pod)}</b>
        <span class="hint">ρ ${rho.toFixed(2)} · ${ps.tracks} track${ps.tracks > 1 ? 's' : ''} · ${(ps.slices || []).length} slice${(ps.slices || []).length === 1 ? '' : 's'}${loss}</span>
        <button class="btn btn-secondary btn-sm" type="button" data-open-pod="${esc(ps.pod)}">View team sheet</button>
        <button type="button" class="btn btn-secondary btn-sm pod-export" data-export-pod="${esc(ps.pod)}" title="download this pod's timeline as a PNG">${icon('download')} Download PNG</button></div>
      ${timelinePlotHTML(timeAxisHTML(span, sched.periodStart || opts.periodStart), podLanesHTML(ps, { ...opts, horizonWeeks: span, includeOutside: false, pinnedLanes: (opts.pinnedLanes || {})[ps.pod] || null }) + contextHTML(sched, opts, horizon, span), span, ps.pod + ' timeline')}
      ${calendarContextHTML(sched, opts, span)}
      ${outsideWorkHTML(ps, { ...opts, horizonWeeks: span })}
      ${unscheduledTeamHTML(rejectedAt(ps.pod))}
    </div>`;
  }).join('');
  return `<div class="card p-3 panel-card tl-card">
    <div class="ord-head"><b>Timeline — by pod</b>
      <span class="hint">one lane per track · idle lanes are slack, shown on purpose · ${q ? 'waterfall: earliest matching start first' : 'hottest first'}</span></div>
    <p class="hint mb-2">Scroll each chart horizontally for later weeks. Drag the divider beside the labels to adjust their width.</p>
    ${blocks}
  </div>`;
}

// podSheetHTML is §13.5, the team's own sheet: every slice in start order with
// Start, Start by (FR-041), Slack (AC 9.3's ⚠ on zero), Waiting on, and Blocks
// — who is waiting on this pod, the thing pods most often cannot see.
export function podSheetHTML(ps, sched, opts = {}) {
  const slices = (ps.slices || []).slice().sort((a, b) => a.startWeek - b.startWeek);
  const rejected = (sched.initiatives || []).filter((si) => unplaced(si) && assignedPods(si, opts.planInitiatives).includes(ps.pod));
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
      ? '<b>no slack</b>'
      : `${sl.slackWeeks}w`;
    return `<tr>
      <td>${esc(sl.initiative)}${assumptionsHTML(byInit[sl.initiative])}</td>
      <td>${sl.finishWeek - sl.startWeek}w</td>
      <td>${weekDateHTML(sl.startWeek, sched.periodStart || opts.periodStart)}</td>
      <td>${weekDateHTML(sl.latestStartWeek, sched.periodStart || opts.periodStart)}</td>
      <td>${slackTxt}</td>
      <td>${waits.length ? waits.map(esc).join(', ') : '<span class="hint">—</span>'}</td>
      <td>${blocks.length ? blocks.map(esc).join(', ') : '<span class="hint">—</span>'}</td>
    </tr>`;
  }).join('') + rejected.map((si) => `<tr class="tl-unplaced-sheet"><td>${esc(si.name)}</td><td colspan="6">${esc(unscheduledReason(si))}</td></tr>`).join('');

  return `<div class="card p-3 panel-card ord-card" data-pod-sheet="${esc(ps.pod)}" tabindex="-1" role="region" aria-label="${esc(ps.pod)} team sheet">
    <div class="ord-head"><b>${esc(ps.pod)} — ${ps.tracks} track${ps.tracks > 1 ? 's' : ''}</b>
      <span class="hint">${slices.length} slice${slices.length === 1 ? '' : 's'} in start order${rejected.length ? ` · ${rejected.length} assigned without placement` : ''}${ps.lossPct ? ` · capacity loss ${ps.lossPct}%${ps.lossOverride ? '' : ' (plan default)'}` : ''}</span>
      <button type="button" class="btn btn-secondary btn-sm pod-export" data-export-sheet="${esc(ps.pod)}" title="download this sheet as a PNG">${icon('download')} Download PNG</button></div>
    <table class="table table-sm wip-table">
      <thead><tr><th>Initiative</th><th>Weeks</th><th>Start</th><th>Start by</th><th>Slack</th><th>Waiting on</th><th>Blocks</th></tr></thead>
      <tbody>${rows || '<tr><td colspan="7" class="hint">No scheduled work at this pod.</td></tr>'}</tbody>
    </table>
    <p class="hint">No slack: starting later moves the initiative's commit date. "Start by" is the last week that does not.</p>
  </div>`;
}

// timelineControlsHTML is the lens/zoom/filter row above the timeline. Kept as
// a pure string function so the tag balance is testable: when the `.seg` to
// `.btn-group` migration left `</span>` closers on `<div>` openers, the
// browser ignored the stray closers and the first .btn-group (a flex row)
// swallowed #tl-main — every button stretched viewport-tall and the chart
// squeezed into the leftover width. Every opener here must close.
// specs/017-planning-and-execution-usability.md:85: grouping never changes a filter's meaning.
export function timelineControlsHTML({ lens, spans, spanSel, filter, initiativeFilter, teamFilter, hideEmpty, ghost }) {
  const initiative = initiativeFilter ?? (lens === 'pod' ? filter : '') ?? '';
  const team = teamFilter ?? (lens === 'initiative' ? filter : '') ?? '';
  const lensBtn = (id, on, label) =>
    `<button type="button" class="btn btn-secondary ${on ? 'active' : ''}" id="${id}" aria-pressed="${on}">${label}</button>`;
  return `<div class="plan-views tl-controls d-flex flex-wrap gap-2 align-items-end">
    <div class="btn-group" role="group" aria-label="Timeline grouping">
      ${lensBtn('tl-by-initiative', lens === 'initiative', 'By initiative')}
      ${lensBtn('tl-by-pod', lens === 'pod', 'By team')}
    </div>
    <div class="btn-group" role="group" aria-label="Visible time span">
      ${spans.map((sp) => `<button type="button" class="btn btn-secondary ${sp.id === spanSel ? 'active' : ''}" data-tlspan="${sp.id}" aria-pressed="${sp.id === spanSel}">${sp.label}</button>`).join('')}
    </div>
    <button class="btn btn-secondary" type="button" id="tl-fullscreen" title="Open timeline full screen; Escape exits">${icon('expand')} Full screen</button>
    <div class="tl-filter d-flex flex-wrap gap-2 align-items-end" id="tl-filter-box">
      <label class="d-flex flex-column gap-1 mb-0 col-12 col-sm-auto">Initiative <input class="form-control" id="tl-initiative-filter" type="search" placeholder="Find an initiative" value="${esc(initiative)}"></label>
      <label class="d-flex flex-column gap-1 mb-0 col-12 col-sm-auto">Team <input class="form-control" id="tl-team-filter" type="search" placeholder="Find a team" value="${esc(team)}"></label>
    </div>
    <div class="d-flex flex-wrap gap-3 align-items-center w-100">
      <span class="hint" id="tl-filter-count" role="status"></span>
      ${lens === 'pod' ? `<label class="hint"><input class="form-check-input" type="checkbox" id="tl-hide-empty" ${hideEmpty ? 'checked' : ''}> Hide teams without matching work</label>
      <label class="hint"><input class="form-check-input" type="checkbox" id="tl-ghost" ${ghost ? 'checked' : ''}> Show other work</label>` : ''}
    </div></div>`;
}

// specs/017-planning-and-execution-usability.md:86: a persistent selection and
// ordinary form controls provide the same edit path as a pointer gesture.
export function timelineInspectorHTML(si, sched, opts = {}) {
  if (!si) return '<aside class="card tl-inspector panel-card"><h3>Initiative details</h3><p>Select an initiative to inspect its dates, dependencies and precise timeline controls.</p></aside>';
  const unplaced = ['beyond-horizon', 'unschedulable'].includes(si.verdict);
  const pi = (opts.planInitiatives || []).find((i) => i.name === si.name);
  const rows = (si.slices || []).map((sl) => {
    const team = (sched.podWeeks || []).find((p) => p.pod === sl.pod);
    const estimate = pi?.work?.[sl.pod]?.weeks;
    const packed = team ? assignLanes(displaySlices(team), team.tracks || 0, opts.pinnedLanes?.[sl.pod]) : null;
    const lane = (packed?.placement.find((p) => p.sl.initiative === si.name && p.lead)?.lane ?? opts.pinnedLanes?.[sl.pod]?.[si.name] ?? 0) + 1;
    return `<fieldset class="tl-edit-row" data-pod="${esc(sl.pod)}"><legend>${esc(sl.pod)}</legend>
      <p class="hint">${weekDateHTML(sl.startWeek, sched.periodStart)} to ${weekDateHTML(sl.finishWeek, sched.periodStart)} · ${sl.slackWeeks ?? 'unknown'}w slack${sl.dependsOn?.length ? ` · waits on ${sl.dependsOn.map(esc).join(', ')}` : ''}</p>
      <label>Start week <input class="form-control" name="startWeek" type="number" min="0" step="1" required value="${sl.startWeek ?? 0}"></label>
      <label>Estimate (weeks) <input class="form-control" name="estimateWeeks" type="number" min="0.1" step="any" ${pi?.inFlight || !Number.isFinite(estimate) ? 'disabled' : 'required'} value="${Number.isFinite(estimate) ? estimate : ''}"></label>
      <label>First lane <input class="form-control" name="lane" type="number" min="1" max="${Math.max(1, (team?.tracks || 1) - (sl.lanesUsed || 1) + 1)}" step="1" required value="${lane}"></label>
      ${pi?.inFlight ? '<p class="hint">In-flight effort cannot be resized from its remaining work.</p>' : ''}
    </fieldset>`;
  }).join('');
  return `<aside class="card tl-inspector panel-card" aria-label="Selected initiative">
    <h3>${esc(si.name)}</h3>
    <p>Buffered finish: ${unplaced ? 'unknown (not scheduled)' : weekDateHTML(si.commitWeek, sched.periodStart)}. Target: ${si.targetWeek == null ? 'not set' : weekDateHTML(si.targetWeek, sched.periodStart)}.</p>
    <p>${esc(si.bindingConstraint || 'No binding constraint reported')}${si.provisional ? ' · provisional estimate' : ''}</p>
    ${assumptionsHTML(si)}
    <p class="hint">Forecast from working inputs, staffing and calendar. Applying edits saves the working plan; the agreed baseline remains available.</p>
    ${rows ? `<form class="tl-precise-edit" data-init="${esc(si.name)}">${rows}<button class="btn btn-secondary" type="submit">Apply timeline edits</button></form>` : '<p>No scheduled slices to edit.</p>'}
  </aside>`;
}

export function timelineEditsFromRows(initiative, rows) {
  const edit = { name: initiative.name, pinnedStarts: { ...initiative.pinnedStarts }, pinnedLanes: { ...initiative.pinnedLanes }, estimateEdits: {} };
  for (const row of rows) {
    if (!Object.hasOwn(initiative.work || {}, row.pod)) throw new Error('Choose a team assigned to this initiative.');
    const start = Number(row.startWeek), lane = Number(row.lane);
    if (String(row.startWeek).trim() === '' || !Number.isInteger(start) || start < 0) throw new Error('Start week must be a whole number of zero or more.');
    if (String(row.lane).trim() === '' || !Number.isInteger(lane) || lane < 1) throw new Error('First lane must be a whole number of one or more.');
    edit.pinnedStarts[row.pod] = start;
    edit.pinnedLanes[row.pod] = lane - 1;
    if (row.estimateWeeks !== undefined && !initiative.inFlight) {
      const estimate = Number(row.estimateWeeks);
      if (!Number.isFinite(estimate) || estimate <= 0) throw new Error('Estimate must be greater than zero.');
      edit.estimateEdits[row.pod] = estimate;
    }
  }
  if (!Object.keys(edit.estimateEdits).length) delete edit.estimateEdits;
  return edit;
}
