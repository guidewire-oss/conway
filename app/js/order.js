// Execution-order view: the proposed order and its reconciliation (spec 001
// §13.2), plus the per-pod weekly load heatmap (AC 4.1) and a pod's queue in
// scheduled start order (AC 4.3).
//
// Everything here is a pure function from the Schedule the server returns to an
// HTML string. No fetching, no DOM reads, no module state — planui.js owns all of
// that. That is what lets tests/order.test.mjs exercise the whole view under
// `node --test` without a browser.
//
// FR-044: colour never carries meaning on its own. Every verdict and every heat
// cell also has a symbol or a number, so the view survives being read in
// greyscale or by someone who cannot distinguish red from green.

// The remedies expander lives in remedyui.js beside its siblings; the import
// cycle (remedyui imports esc from here) is safe because neither module calls
// the other's exports while evaluating — only inside functions invoked later.
import { optionsExpanderHTML } from './remedyui.js';
import { term } from './terms.js';

export const esc = (s) => String(s ?? '').replace(/[&<>"]/g,
  (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

export const weekLabel = (w) => `w${w}`;

// zoneOf matches the thresholds the app already uses everywhere else (AC 4.1,
// and rhoColor in planui.js): at or over capacity is red, 0.85 up is amber.
export function zoneOf(utilization) {
  if (!isFinite(utilization)) return 'red';
  if (utilization >= 1) return 'red';
  if (utilization >= 0.85) return 'amber';
  if (utilization > 0) return 'green';
  return 'idle';
}

// verdictView turns a scheduled initiative into what the row shows: a symbol, a
// short phrase and a zone. The phrase carries the number of weeks late, because
// "late" without "by how much" is the answer the planner already has.
export function verdictView(si) {
  const late = si.weeksLate || 0;
  const base = {
    'on-time': { symbol: '●', text: 'on time', zone: 'green' },
    'at-risk': { symbol: '▲', text: 'at risk', zone: 'amber' },
    late: { symbol: '▲', text: `late ${late}w`, zone: 'red' },
    'no-date': { symbol: '●', text: 'no date', zone: 'idle' },
    'structurally-infeasible': { symbol: '⚠', text: `cannot fit${late ? ` (${late}w over)` : ''}`, zone: 'red' },
    unschedulable: { symbol: '⚠', text: 'unschedulable', zone: 'red' },
    // Decision 28: it could not begin inside the period at all, so it has no
    // start, no commit and no lateness — "does not fit this period" is the whole
    // of what is known, and inventing a week number a decade out was the old
    // behaviour this replaced.
    'beyond-horizon': { symbol: '⚠', text: 'does not fit this period', zone: 'red' },
  }[si.verdict] || { symbol: '●', text: si.verdict || 'unknown', zone: 'idle' };
  // AC 2.5: a verdict resting on unestimated work is marked provisional wherever
  // it is shown, rather than being presented as though it were firm.
  return si.provisional ? { ...base, text: `${base.text}, provisional` } : base;
}

// verdictBadgeHTML is the Order table's verdict as a pill (the audit's
// Stripe-style status badge): tinted background, symbol + text. Colour is
// never the only carrier — the symbol and words stay (FR-044).
export function verdictBadgeHTML(si) {
  const v = verdictView(si);
  return `<span class="vbadge v-${v.zone}">${v.symbol} ${esc(v.text)}</span>`;
}

// suggestedCell is the engine's rank for this initiative when the working
// order is the planner's (spec 006 Q2: a column, not a second view). Shown
// only when it differs from the spine — a column of identical numbers is noise.
export function suggestedCell(si, engineRanks) {
  if (!engineRanks) return '';
  const sug = engineRanks[si.name];
  if (sug === undefined || sug === si.proposedRank) return '';
  const cls = sug < si.proposedRank ? 'ord-up' : 'ord-down';
  return ` <span class="ord-move ${cls}" title="the engine suggests rank ${sug} — your order stands until you accept the proposal">↳${sug}</span>`;
}

// statedCell renders "2 →1" — what the planner said, and what the engine
// proposes. Decision 3 makes this the centre of the table rather than a footnote:
// a reordering nobody explains reads as being ignored.
export function statedCell(si) {
  const lock = si.priorityLocked ? ' <span class="tag">locked</span>' : '';
  if (!si.statedRank) return `<span class="hint">—</span>${lock}`;
  if (si.statedRank === si.proposedRank) return `${si.statedRank}${lock}`;
  const dir = si.proposedRank < si.statedRank ? 'up' : 'down';
  return `${si.statedRank} <span class="ord-move ord-${dir}">→${si.proposedRank}</span>${lock}`;
}

// objectiveView is the "your order costs this much, mine costs that" line, which
// Decision 6 calls the most persuasive output the feature has.
export function objectiveView(sched) {
  const stated = sched.statedOrderObjectiveScore || 0;
  const proposed = sched.objectiveScore || 0;
  const delta = Math.round((proposed - stated) * 10) / 10;
  const inits = sched.initiatives || [];
  // Comparability comes from the inputs, not the scores. The objective is weighted
  // lateness, so a plan where every date holds scores 0 on both runs — reading that
  // as "no dates set" would tell the planner the opposite of what just happened.
  const isDated = (si) => si.targetWeek !== null && si.targetWeek !== undefined;
  const dated = inits.filter(isDated);
  const ranked = inits.some((si) => si.statedRank > 0);
  return {
    stated, proposed, delta,
    comparable: dated.length > 0 || ranked,
    // "Every date holds" is an absolute claim, so all three have to be true: there
    // are dates, every one of them came back on-time — weeksLate is 0 for an
    // unschedulable row too, so the verdict is what counts — and neither order
    // costs anything, since a stated order that was late is still a miss.
    allOnTime: dated.length > 0 && dated.every((si) => si.verdict === 'on-time') &&
      stated === 0 && proposed === 0,
    better: delta < 0,
  };
}

// wipLimitNote satisfies Decision 22's requirement that a derived limit says so
// and names the pod it came from: the number moves when the roster moves, which
// is correct and surprising enough that an unlabelled figure reads as a bug.
export function wipLimitNote(wip) {
  if (!wip) return '';
  // Under off there is no limit, so reporting "WIP limit 0" would describe a number
  // as though someone had chosen it. Say what is actually true instead.
  if (wip.model === 'off') {
    return 'no org WIP limit <span class="hint">(model: off)</span>';
  }
  return wip.derived
    ? `WIP limit ${wip.value} <span class="hint">(derived from ${esc(wip.fromPod)}'s tracks)</span>`
    : `WIP limit ${wip.value} <span class="hint">(set on this plan)</span>`;
}

// orderRows is the table's data, separated from its markup so the ordering and
// the per-row reasoning can be tested without parsing HTML.
export function orderRows(sched) {
  // Object.create(null), not {}: these keys are initiative names typed into a
  // spreadsheet, and a row called "__proto__" would otherwise reach the prototype.
  const reasons = Object.create(null);
  (sched.reconciliation || []).forEach((r) => { reasons[r.initiative] = r.reason; });
  return (sched.initiatives || [])
    .slice()
    .sort((a, b) => a.proposedRank - b.proposedRank)
    .map((si) => ({
      si,
      verdict: verdictView(si),
      reason: reasons[si.name] || '',
      binds: si.bindingConstraint || '',
    }));
}

function orderRowHTML(row, opts = {}) {
  const { si, verdict } = row;
  const target = si.targetWeek === null || si.targetWeek === undefined
    ? '<span class="hint">—</span>' : weekLabel(si.targetWeek);
  // AC 2.1 shows both numbers: the raw finish is what the schedule says, the commit
  // is what may be promised, and Decision 9 exists because they are not one claim.
  const raw = si.rawFinishWeek === undefined ? '' :
    `<span class="hint" title="raw scheduled finish, before its buffer">${weekLabel(si.rawFinishWeek)} +${si.bufferWeeks || 0}w →</span> `;
  // Spec 004 AC 1.1/1.2: pin/unpin rides the stated-rank cell (the deviation
  // lives there), as a toggle. Locked rows offer unpin; moved rows offer pin.
  // An UNRANKED initiative has no stated rank to pin — a lock without a rank
  // is a misleading "locked" tag that changes nothing — so it gets no control
  // until the ✎ editor gives it one. A draft has nothing saved to pin against
  // either, so no control is offered there.
  const canPin = !opts.noPin && (si.statedRank > 0 || si.priorityLocked);
  const pin = !canPin ? '' : (si.priorityLocked
    ? `<button type="button" class="ord-pin" data-pin="${esc(si.name)}" data-locked="1" title="release this priority back to the engine">unpin</button>`
    : `<button type="button" class="ord-pin" data-pin="${esc(si.name)}" data-locked="" title="lock this initiative to its stated rank">pin</button>`);
  // ✎ opens the sequencing-attribute editor (spec 004: the in-app half of the
  // sheet upload). Next to the name, where the row's identity lives.
  const edit = opts.noPin ? '' : `<button type="button" class="ord-edit" data-edit="${esc(si.name)}" title="edit priority, dates, tier, dependencies…">✎ edit</button>`;
  const main = `<tr class="ord-row">
    <td class="num">#${si.proposedRank}${suggestedCell(si, opts.engineRanks)}</td>
    <td>${esc(si.name)} ${edit}</td>
    <td>${statedCell(si)} ${pin}</td>
    <td class="num">${weekLabel(si.startWeek)}</td>
    <td class="num">${raw}<b>${weekLabel(si.commitWeek)}</b></td>
    <td class="num">${target}</td>
    <td>${verdictBadgeHTML(si)}</td>
    <td>${esc(row.binds) || '<span class="hint">—</span>'}</td>
  </tr>`;
  const trace = rowTraceHTML(row);
  // A missed date gets [options ▾] on its trace line (§13.2, AC 5.1), so the
  // trace row exists for a miss even when the rank never moved.
  const options = optionsExpanderHTML(si);
  if (!trace && !options) return main;
  return `${main}<tr class="ord-why"><td></td><td colspan="7">${trace}${options}</td></tr>`;
}

// rowTraceHTML is FR-021 at the row level: the reason the rank moved, the named
// terms that produced it, and any assumption the schedule rests on. Decision 2
// picked a formula with separable terms precisely so this could be shown; a
// binding constraint on its own does not explain a position.
export function rowTraceHTML(row) {
  const { si } = row;
  const parts = [];
  const t = si.rankingTerms;
  if (t) {
    parts.push(`weight ${t.weight}`);
    if (t.constraintWeeks) parts.push(`${t.constraintWeeks}w of drum time`);
    if (t.slackWeeks) parts.push(`${t.slackWeeks}w slack`);
    if (t.index !== undefined) parts.push(`index ${t.index}`);
  }
  if ((si.unestimatedPods || []).length) {
    // Raw here: parts is escaped once below, and escaping twice would print the
    // entity rather than the character ("R&amp;D" instead of "R&D").
    parts.push(`no estimate from ${si.unestimatedPods.join(', ')}`);
  }
  const terms = parts.length ? `<div class="hint">${parts.map(esc).join(' · ')}</div>` : '';
  const assume = (si.assumptions || []).length
    ? `<div class="hint">${si.assumptions.map((a) => esc(a)).join('; ')}</div>` : '';
  const reason = row.reason ? `<span class="hint">└ ${esc(row.reason)}</span>` : '';
  if (!reason && !terms && !assume) return '';
  // The reason stays visible; the arithmetic sits behind a disclosure, so the table
  // reads as a table until someone asks why.
  return `${reason}${terms || assume ? `<details class="ord-terms"><summary class="hint">why this rank</summary>${terms}${assume}</details>` : ''}`;
}

export function orderTableHTML(sched, opts = {}) {
  const rows = orderRows(sched);
  if (!rows.length) return '<p class="hint">No initiatives to order yet.</p>';
  return `<table class="wip-table ord-table">
    <thead><tr>
      <th>#</th><th>Initiative</th><th>Stated</th><th>Start</th>
      <th>${term('commit', 'Commit')}</th><th>${term('target', 'Target')}</th><th>${term('verdict', 'Verdict')}</th><th>${term('binds', 'Binds')}</th>
    </tr></thead>
    <tbody>${rows.map((r) => orderRowHTML(r, opts)).join('')}</tbody>
  </table>`;
}

// infeasibleNote lists the dates no ordering could meet, separately from the ones
// lost to contention (Decision 12) — they are different conversations, and only
// one of them has a remedy that involves reordering anything.
export function infeasibleNote(sched) {
  const stuck = (sched.initiatives || []).filter((si) => si.verdict === 'structurally-infeasible');
  if (!stuck.length) return '';
  const names = stuck.map((si) => `${esc(si.name)} <span class="hint">(needs ${weekLabel(si.commitWeek)}, wanted ${weekLabel(si.targetWeek)})</span>`);
  return `<p class="plan-warn">⚠ ${stuck.length} date${stuck.length > 1 ? 's' : ''} no ordering can meet: ${names.join(' · ')}.
    Only a later date, less scope or an earlier start moves these.
    <button type="button" class="usage-link" data-anchor="warnings">learn more</button></p>`;
}

// feverZone applies the Observe fever chart's thresholds (sim.js feverPoint):
// burn ratio <0.5 green, <1 amber, >=1 red. Same zones in both views, or the
// planner reads one number two ways — worse than no chart at all.
export function feverZone(ratio) {
  return ratio < 0.5 ? 'green' : ratio < 1 ? 'amber' : 'red';
}

// feverChartHTML is FR-024 (spec 004 Story 2): the plan-time fever chart. One
// dot per DATED initiative, x = chain progress at the target week, y = buffer
// consumed by then. Pure SVG (no d3) — it is a scatter, not a simulation, and
// the axes are fixed 0..1 so the zones are honest comparison territory.
// Meaning is never color-only (FR-044): position carries it, and the legend
// names the zones.
export function feverChartHTML(sched) {
  const dated = (sched.initiatives || []).filter((si) =>
    si.targetWeek !== null && si.targetWeek !== undefined);
  if (!dated.length) return '';
  const W = 320, H = 200, M = { t: 10, r: 10, b: 30, l: 38 };
  const x = (v) => M.l + v * (W - M.l - M.r);
  const y = (v) => H - M.b - Math.min(v, 1.5) / 1.5 * (H - M.b - M.t);
  const zone = (r, fill) =>
    `<path d="M${x(0)},${y(0)} L${x(1)},${y(Math.min(r, 1.5))} L${x(1)},${y(0)} Z" fill="${fill}" opacity="0.14"/>`;
  const dots = dated.map((si) => {
    const p = si.targetProgress || 0;
    const b = si.targetBurn || 0;
    const z = feverZone(si.burnRatio || 0);
    const label = `${si.name}: ${(p * 100).toFixed(0)}% of chain by its target, ${b.toFixed(1)}x buffer burned`;
    return `<circle cx="${x(p)}" cy="${y(b)}" r="4" class="fever-dot fever-${z}"><title>${esc(label)}</title></circle>`;
  }).join('');
  const pct = (v) => `${Math.round(v * 100)}%`;
  const yTicks = [0, 0.5, 1, 1.5].map((v) =>
    `<text x="${M.l - 6}" y="${y(v) + 3}" text-anchor="end" class="fever-tick">${pct(v)}</text>`).join('');
  const xTicks = [0, 0.5, 1].map((v) =>
    `<text x="${x(v)}" y="${H - M.b + 14}" text-anchor="middle" class="fever-tick">${pct(v)}</text>`).join('');
  return `<div class="fever-wrap">
  <svg viewBox="0 0 ${W} ${H}" role="img" aria-label="buffer fever chart: chain progress at each target date against buffer consumed">
    ${zone(0.5, 'var(--green)')}${zone(1, 'var(--amber)')}
    <rect x="${x(0)}" y="${y(1.5)}" width="${x(1) - x(0)}" height="${y(0) - y(1.5)}" fill="var(--red)" opacity="0.10"/>
    <line x1="${x(0)}" y1="${y(0)}" x2="${x(1)}" y2="${y(0)}" class="fever-axis"/>
    <line x1="${x(0)}" y1="${y(0)}" x2="${x(0)}" y2="${y(1.5)}" class="fever-axis"/>
    ${yTicks}${xTicks}${dots}
    <text x="${(x(0) + x(1)) / 2}" y="${H - 2}" text-anchor="middle" class="fever-label">chain done by the target date</text>
    <text x="10" y="${(y(0) + y(1.5)) / 2}" text-anchor="middle" class="fever-label"
      transform="rotate(-90 10 ${(y(0) + y(1.5)) / 2})">buffer consumed</text>
  </svg>
  <p class="hint">Fever chart (plan-time): each dot is a dated initiative read at its target date —
    how much of the chain should be done, against how much buffer the date eats.
    Zones match the Observe fever chart: below the green line is comfortable, amber is the
    buffer going fast, red has spent it. ${dated.length} dated of ${(sched.initiatives || []).length}.</p>
</div>`;
}

// heatmapWeeks bounds the grid to the period, not the whole schedule. AC 4.1 asks
// for every week of the period, and a plan that overruns can stretch to three
// times the horizon — rendering all of it would shrink the cells to nothing and
// break FR-035's no-horizontal-scroll rule for the part that matters.
export function heatmapWeeks(sched, horizonWeeks) {
  const horizon = Math.max(1, Math.ceil(horizonWeeks || sched.horizonWeeks || 26));
  let widest = 0;
  (sched.podWeeks || []).forEach((ps) => { widest = Math.max(widest, (ps.weeks || []).length); });
  return Math.min(horizon, widest || horizon);
}

// overrunNote is the honest counterpart to bounding the grid: work scheduled past
// the period is not drawn, so it has to be counted rather than quietly dropped.
export function overrunNote(sched, weeks) {
  const over = (sched.initiatives || []).filter((si) => si.commitWeek > weeks);
  if (!over.length) return '';
  return `<p class="hint">${over.length} initiative${over.length > 1 ? 's' : ''} commit after ${weekLabel(weeks)}
    and continue past this grid — the period is ${weeks} weeks long.</p>`;
}

// idleNoteHTML is AC 4.2's attribution sentence, per pod: where the scheduled
// mean weekly utilization diverges from the flat rho the Network view reports
// by more than 2 points, say both numbers AND what the idle time was spent on
// (calendar windows, waiting upstream, or held for a release slot). Within 2
// points the views agree and NFR-005 demands silence, not a per-pod essay.
export function idleNoteHTML(pods, horizonWeeks) {
  const gaps = (pods || []).filter((ps) =>
    ps.meanUtil !== undefined && ps.flatRho !== undefined &&
    Math.abs((ps.meanUtil || 0) - (ps.flatRho || 0)) > 0.02);
  if (!gaps.length) return '';
  const fmt = (n) => `${Math.round((n || 0) * 100)}%`;
  // The denominator is the PERIOD's track-weeks: a schedule that overruns
  // carries weeks the server never attributed idle over, so dividing by the
  // whole span understates every cause.
  const trackWeeks = (ps) => Math.max(1, (ps.tracks || 1) * (horizonWeeks || (ps.weeks || []).length));
  const line = (ps) => {
    const idle = ps.idle || {};
    const parts = [];
    if (idle.calendar) parts.push(`${fmt(idle.calendar / trackWeeks(ps))} calendar`);
    if (idle.upstream) parts.push(`${fmt(idle.upstream / trackWeeks(ps))} waiting upstream`);
    if (idle.heldForRelease) parts.push(`${fmt(idle.heldForRelease / trackWeeks(ps))} held for a release slot`);
    return `<li><b>${esc(ps.pod)}</b>: scheduled ${fmt(ps.meanUtil)} vs flat ρ ${fmt(ps.flatRho)}${parts.length ? ` — idle: ${parts.join(', ')}` : ''}</li>`;
  };
  return `<details class="ord-idle"><summary class="hint">${gaps.length} pod${gaps.length > 1 ? 's' : ''} where the schedule runs lighter than the flat ρ</summary>
    <ul class="hint">${gaps.map(line).join('')}</ul>
    <p class="hint">Attributions are what the schedule can see: calendar windows cut capacity, upstream
      waits are dependencies, and the rest is what the release gates (WIP limit, change-absorption
      cap, kit readiness) hold back.</p></details>`;
}

export function podHeatmapHTML(sched, horizonWeeks) {
  const pods = sched.podWeeks || [];
  if (!pods.length) return '<p class="hint">No pod load to show yet.</p>';
  const weeks = heatmapWeeks(sched, horizonWeeks);
  const drums = new Set(sched.drumPods || []);

  // The tick hover carries the calendar date (spec 004: dates on hover) —
  // the week number alone asks the planner to do date arithmetic in their head.
  const head = Array.from({ length: weeks }, (_, w) => {
    const d = weekToDate(w, sched.periodStart);
    return `<th class="ord-wk"${d ? ` data-tip="week of ${esc(d)}"` : ''}>${w % 5 === 0 ? w : ''}</th>`;
  }).join('');
  const rows = pods.map((ps) => {
    const cells = Array.from({ length: weeks }, (_, w) => {
      const wk = (ps.weeks || [])[w] || { busy: 0, tracks: ps.tracks, utilization: 0, initiatives: [] };
      const zone = zoneOf(wk.utilization);
      const who = (wk.initiatives || []).map((n) => esc(n)).join(', ');
      const title = `${esc(ps.pod)} ${weekLabel(w)}: ${wk.busy}/${ps.tracks} tracks${who ? ` — ${who}` : ' — idle'}`;
      // The busy count is in the cell, not only its colour (FR-044).
      return `<td class="ord-cell ord-${zone}" title="${title}">${wk.busy || ''}</td>`;
    }).join('');
    const drum = drums.has(ps.pod) ? ' <span class="tag">drum</span>' : '';
    return `<tr><th class="ord-pod"><button type="button" class="ord-podlink" data-pod="${esc(ps.pod)}">${esc(ps.pod)}</button>
      <span class="hint">${ps.tracks}t</span>${drum}</th>${cells}</tr>`;
  }).join('');

  return `<table class="ord-heat"><thead><tr><th class="ord-pod">Pod</th>${head}</tr></thead>
    <tbody>${rows}</tbody></table>
    <p class="hint">Each cell is one week: the number is tracks in use, red is at or over capacity,
      amber from 0.85. Click a pod for its queue.</p>
    ${overrunNote(sched, weeks)}
    ${idleNoteHTML(pods, weeks)}`;
}

const utf8 = new TextEncoder();

// byteOrder compares two strings the way Go's `<` operator does, so a tie broken
// here lands where the server put it.
//
// It cannot just use `<`. JavaScript compares UTF-16 code units and Go compares
// UTF-8 bytes, and the two disagree wherever a surrogate pair meets a BMP
// character above U+E000: an astral character is 0xD800-0xDBFF in UTF-16 (so it
// sorts low) but starts 0xF0 in UTF-8 (so it sorts high). Encoding first is the
// only way to get the same answer; a pod queue is a handful of slices, so the cost
// of encoding per comparison does not matter here.
export function byteOrder(a, b) {
  if (a === b) return 0;
  const x = utf8.encode(a);
  const y = utf8.encode(b);
  const shared = Math.min(x.length, y.length);
  for (let i = 0; i < shared; i++) {
    if (x[i] !== y[i]) return x[i] < y[i] ? -1 : 1;
  }
  if (x.length === y.length) return 0;
  return x.length < y.length ? -1 : 1;
}

// podQueueHTML is AC 4.3: a pod's slices in scheduled start order, each with the
// wait before it, so a pod lead can see what they are queued behind.
export function podQueueHTML(sched, pod) {
  const ps = (sched.podWeeks || []).find((x) => x.pod === pod);
  if (!ps) return '';
  // Mirror podSchedules in schedule.go: start week, then initiative name compared
  // byte-wise. Go's `<` on strings is a byte comparison, so "Zulu" precedes
  // "alpha"; localeCompare would reverse exactly that pair and disagree with the
  // server it claims to follow.
  const slices = (ps.slices || []).slice().sort((a, b) =>
    a.startWeek - b.startWeek || byteOrder(String(a.initiative), String(b.initiative)));
  if (!slices.length) return `<p class="hint">${esc(pod)} has no scheduled work in this plan.</p>`;
  const rows = slices.map((s) => `<tr>
    <td>${esc(s.initiative)}</td>
    <td>${weekLabel(s.startWeek)}</td>
    <td>${weekLabel(s.finishWeek)}</td>
    <td>${s.remainingWeeks}w</td>
    <td>${s.waitWeeks ? `${s.waitWeeks}w` : '<span class="hint">none</span>'}</td>
    <td>${esc(s.bindingConstraint) || '<span class="hint">—</span>'}</td>
  </tr>`).join('');
  return `<div class="panel-card ord-queue">
    <b>${esc(pod)} — its queue, in scheduled order</b>
    <table class="wip-table"><thead><tr>
      <th>Initiative</th><th>Start</th><th>Finish</th><th>Weeks</th><th>Waited</th><th>Waiting on</th>
    </tr></thead><tbody>${rows}</tbody></table>
    <p class="hint">Waited is the gap between being ready and starting. Finish weeks are exclusive:
      work starting w5 for 6 weeks finishes w11.</p>
  </div>`;
}

// noticesHTML surfaces the assumptions and warnings every schedule carries
// (FR-021). A schedule that quietly broke a dependency cycle or scheduled around
// a missing estimate has to say so where the order is read, not in a log.
export function noticesHTML(sched) {
  const warn = (sched.warnings || []).map((wmsg) => `<li>${esc(wmsg)}</li>`).join('');
  const assume = (sched.assumptions || []).map((a) => `<li>${esc(a)}</li>`).join('');
  if (!warn && !assume) return '';
  return `${warn ? `<div class="plan-warn"><b>Warnings</b><ul>${warn}</ul></div>` : ''}
    ${assume ? `<details class="ord-assume"><summary class="hint">${(sched.assumptions || []).length} assumption(s) applied</summary><ul>${assume}</ul></details>` : ''}`;
}

// verdictBannerHTML is the answer-first sentence (IA audit, Finding 2): what a
// VP needs before any machinery — how many dated initiatives miss under the
// best order found, and the worst case by name. The objective line beneath it
// remains for the planner who wants the arithmetic.
export function verdictBannerHTML(sched, opts = {}) {
  const inits = sched.initiatives || [];
  const dated = inits.filter((si) => si.targetWeek !== null && si.targetWeek !== undefined);
  if (!dated.length) {
    // Spec 009 AC 3.1: the empty verdict must teach the fix, not read as an
    // error. The fix differs by CAUSE: dates on the sheet with no period
    // start read as none (the scheduler cannot week-ify them) — tell the
    // planner to set the period start, not to add more dates (cubic P2).
    if (opts.sheetHasDates) {
      return `<div class="verdict-banner verdict-none">The sheet carries target dates, but the plan has no period start —
        dates cannot become weeks without one. Set it in ⚙ Assumptions and every verdict on this page comes alive.
        <button type="button" class="usage-link" data-anchor="planning-loop">learn more</button></div>`;
    }
    return `<div class="verdict-banner verdict-none">No target dates yet — dates are what the verdicts measure.
      Add one via ✎ on any row below (or upload a sheet with a Target Date column) and every date on this page comes alive.
      <button type="button" class="usage-link" data-anchor="order">learn more</button></div>`;
  }
  // Every non-on-time verdict counts — unschedulable rows have weeksLate 0
  // (the verdict carries the information), so filtering on verdict alone is
  // what keeps the "every dated initiative holds" claim honest.
  const missing = dated.filter((si) => si.verdict !== 'on-time');
  if (!missing.length) {
    return `<div class="verdict-banner verdict-ok">Every dated initiative holds its target under this order.</div>`;
  }
  // Reduce over the misses with ||0: an on-time first row has no weeksLate,
  // and `undefined > n` is false — the banner would name the clean row and
  // print "undefinedw over".
  const worst = missing.reduce((a, b) => ((b.weeksLate || 0) > (a.weeksLate || 0) ? b : a));
  const n = missing.length;
  const worstWhy = worst.verdict === 'structurally-infeasible' ? 'no ordering meets it'
    : worst.verdict === 'unschedulable' ? 'it cannot be scheduled as entered'
    : `${worst.weeksLate || 0}w over`;
  return `<div class="verdict-banner verdict-miss">
    <b>${n} of ${dated.length} dated initiatives miss their target</b> under the best order found.
    Worst: ${esc(worst.name)} — ${worstWhy}${term('verdict')}
  </div>`;
}

// comparisonBarsHTML renders yours-vs-proposed as two bars (IA audit: the
// arithmetic line stays, but the shape of the comparison should be seen, not
// computed). Numbers ride on the bars — length never carries meaning alone
// (WCAG 1.4.1).
export function comparisonBarsHTML(obj) {
  if (!obj.comparable) {
    return '<span class="hint">no dates or priorities set yet, so there is no order to argue with</span>';
  }
  if (obj.stated === 0 && obj.proposed === 0) {
    // "Every date holds" is only true when there ARE dates; a priority-only
    // plan scores zero because no date can be missed, not because all held.
    return '<span class="hint">neither order costs any weighted lateness</span>';
  }
  const max = Math.max(obj.stated, obj.proposed, 1);
  const pctOf = (v) => Math.max(2, Math.round((v / max) * 100)); // 2% floor: a bar must be visible
  const yours = pctOf(obj.stated), prop = pctOf(obj.proposed);
  return `<div class="ord-bars" title="weighted weeks late under each order — lower is better">
    <div class="ord-bar-row"><span class="ord-bar-lbl">yours</span>
      <span class="ord-bar-track"><span class="ord-bar-fill ord-yours" style="width:${yours}%"></span></span>
      <span class="ord-bar-val">${obj.stated}</span></div>
    <div class="ord-bar-row"><span class="ord-bar-lbl">proposed</span>
      <span class="ord-bar-track"><span class="ord-bar-fill ord-prop" style="width:${prop}%"></span></span>
      <span class="ord-bar-val">${obj.proposed}</span></div>
    <span class="hint">weighted weeks late${term('weighted-late')} — lower is better${obj.delta !== 0 ? ` · the proposed order ${obj.better ? 'saves' : 'costs'} <b>${Math.abs(obj.delta)}</b>` : ''}</span>
  </div>`;
}

// orderingBadge states which order is in force (spec 006 Decision 1). The
// planner's own order is the default and needs no apology; an accepted engine
// proposal is the planner's explicit choice and is labelled with the verb that
// made it true.
export function orderingBadge(sp = {}) {
  if (sp.acceptedOrdering === 'engine') {
    const when = sp.acceptedOrderingAt
      ? new Date(sp.acceptedOrderingAt * 1000).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
      : '';
    return `<span class="tag ord-enginetag" title="The engine's proposed order, accepted${when ? ` ${when}` : ''}. Your stated order still shows in the Stated column.">engine's order${when ? ` · accepted ${when}` : ''}</span>`;
  }
  return `<span class="tag ord-yourtag" title="Your stated order is the working plan. The engine's suggestion is one click away.">your order</span>`;
}

// optimizeDeltaHTML prices the engine's best run against the working order, so
// the Optimize button can carry its offer on its face (spec 006 AC 3.1).
export function optimizeOfferHTML(sched) {
  const best = (sched.rulesTried || [])
    .filter((r) => r.rule !== sched.rule)
    .reduce((m, r) => (r.objective < m.objective ? r : m), { rule: '', objective: Infinity });
  if (best.rule === '' || !Number.isFinite(best.objective)) return '';
  const cur = sched.objectiveScore || 0;
  const save = Math.round((cur - best.objective) * 10) / 10;
  if (save <= 0) return '';
  return `<span class="hint" title="The best dispatch rule scores ${best.objective} weighted lateness versus this order's ${cur}. A suggestion, not an answer — the sequencing problem has no single solution.">suggestion: ${esc(best.rule)} would cost ${save} less</span>`;
}

export function orderHeaderHTML(sched, opts = {}) {
  const obj = objectiveView(sched);
  const rules = (sched.rulesTried || []).length;
  const yours = (opts.scheduling || {}).acceptedOrdering !== 'engine';
  const sheetHasDates = (opts.storedInitiatives || []).some((i) => i && i.targetDate);
  return `${verdictBannerHTML(sched, { sheetHasDates })}
  <div class="ord-head">
    <b>Execution order</b> ${orderingBadge(opts.scheduling || {})}
    <span class="hint">rule: ${esc(sched.rule || '—')}${rules ? ` (best of ${rules})` : ''}${term('objective')}</span>
    <span class="hint">${wipLimitNote(sched.wipLimit)}</span>
    ${yours ? optimizeOfferHTML(sched) : ''}
    ${yours ? `${term('optimize')}<button type="button" class="primary" id="ord-optimize" title="Run every dispatch rule and present the best ordering beside yours, priced. Accepting it is always your call — this is an optimization, not the solution.">⚡ Optimize order</button>`
      : `<button type="button" id="ord-unoptimize" title="Return to your stated order. The engine's proposal stays available.">↩ back to your order</button>`}
    <button type="button" class="docs-link" data-docs="order" title="how the Order view works — every column and action">📖 docs</button>
    <button type="button" id="sched-open" title="period start, WIP model, buffers, freezes — set once">⚙ Assumptions</button>
    <button type="button" id="tl-open" title="open this order as a timeline (Story 8)">▦ Open timeline ▸</button>
    <span class="ord-bl-head">
      <input id="bl-name-head" type="text" placeholder="name this order…" maxlength="80"${opts.noPin ? ' disabled title="save the uploaded initiatives first — a baseline freezes what is stored"' : ''}>
      <button type="button" id="bl-save-head" class="primary" title="freeze this order with the inputs that produced it"${opts.noPin ? ' disabled' : ''}>✓ Save baseline</button>
    </span>
  </div>
  <div class="plan-summary">${comparisonBarsHTML(obj)}</div>`;
}

// initiativeEditDialogHTML is the in-app half of spec 001 §10 Q9 (spec 004:
// close the gap). The uploaded sheet is one entry point for sequencing
// attributes; this dialog is the other — every field the edit API accepts,
// prefilled from the STORED initiative (not the scheduled row, which carries
// only what the schedule reports), so a planner can fill what the sheet left
// blank (the the reference plan matrix has no priority column at all) or correct what it got
// wrong without re-uploading.
export function initiativeEditDialogHTML(it) {
  if (!it) return '';
  const v = (x) => (x === null || x === undefined ? '' : String(x));
  return `<div class="ord-sched" id="init-edit-dialog" role="dialog" aria-modal="true"
    aria-label="Edit initiative sequencing attributes" hidden>
    <div class="ord-sched-box">
      <b>Edit “${esc(it.name)}”</b>
      <p class="hint">Sequencing attributes — the same fields an uploaded sheet can carry. Dates are YYYY-MM-DD inside the period.</p>
      <div class="sched-grid">
        <label class="hint sched-f">priority (1 = highest, blank = unranked)
          <span class="sched-row"><input id="ie-priority" type="number" min="1" step="1" value="${v(it.statedPriority)}"
            placeholder="unranked"></span></label>
        <label class="hint sched-f">target date
          <span class="sched-row"><input id="ie-target" type="date" value="${esc(it.targetDate || '')}"></span></label>
        <label class="hint sched-f">tier (1 contractual .. 4 aspirational)
          <span class="sched-row"><input id="ie-tier" type="number" min="1" max="4" step="1" value="${v(it.tier)}"
            placeholder="unscored"></span></label>
        <label class="hint sched-f">cost of delay / week (1-10, ratios are what count)
          <span class="sched-row"><input id="ie-cod" type="number" min="0" max="10" step="0.5" value="${v(it.costOfDelayPerWeek)}"
            placeholder="1"></span></label>
        <label class="hint sched-f">earliest start
          <span class="sched-row"><input id="ie-earliest" type="date" value="${esc(it.earliestStart || '')}"></span></label>
        <label class="hint sched-f">full-kit readiness % (blank = 100)
          <span class="sched-row"><input id="ie-kit" type="number" min="0" max="100" step="1" value="${it.kitPct !== undefined && it.kitPct !== null ? Math.round(it.kitPct * 100) : ''}"
            placeholder="100"></span></label>
        <label class="hint sched-f">after initiatives (comma-separated, quotes for commas in names)
          <span class="sched-row"><input id="ie-after" value="${esc((it.afterInitiatives || []).map(quoteName).join(', '))}"
            placeholder="none"></span></label>
        <label class="hint sched-f">progress % (already done)
          <span class="sched-row"><input id="ie-progress" type="number" min="0" max="100" step="1" value="${it.progressPct ? Math.round(it.progressPct * 100) : ''}"
            placeholder="0"></span></label>
        <label class="hint sched-f">in flight (carryover already running)
          <span class="sched-row"><input id="ie-inflight" type="checkbox" ${it.inFlight ? 'checked' : ''}></span></label>
      </div>
      <div class="sched-row" style="gap:14px; margin-top:6px">
        <label class="hint"><input type="checkbox" id="ie-priority-locked" ${it.priorityLocked ? 'checked' : ''}> priority fixed (pin the stated rank)</label>
        <label class="hint"><input type="checkbox" id="ie-date-locked" ${it.dateLocked ? 'checked' : ''}> date fixed (a commitment, not an aspiration)</label>
      </div>
      <span id="ie-error" class="login-err"></span>
      <div class="sched-row" style="gap:8px; margin-top:8px">
        <button type="button" class="primary" id="ie-save">Save</button>
        <button type="button" id="ie-cancel">Cancel</button>
      </div>
    </div>
  </div>`;
}

// weekToDate turns a week number into YYYY-MM-DD from the period start —
// the timeline tick hover and the editor's date fields read it. Local time,
// not UTC, so the date is the day the period actually starts on.
export function weekToDate(week, periodStart) {
  if (!periodStart) return '';
  const t = new Date(String(periodStart).trim() + 'T00:00:00');
  if (Number.isNaN(t.getTime())) return '';
  t.setDate(t.getDate() + week * 7);
  return t.toLocaleDateString('en-CA'); // YYYY-MM-DD in local time
}

// quoteName applies the sheet convention to a single name going INTO the field:
// a name with a comma or a quote is wrapped in double quotes (internal quotes
// doubled), or saving the dialog would split it into pieces on the way back.
export function quoteName(n) {
  if (!/["\n,]/.test(n)) return n;
  return '"' + n.replace(/"/g, '""') + '"';
}

// splitInitiativeNames is the sheet convention (planning.go splitInitiativeList):
// comma-separated, but a name may be double-quoted to hold a comma of its own.
// The sheet parser and this dialog must agree, or a name edited here would not
// match the one the upload produced.
export function splitInitiativeNames(s) {
  const t = s.trim();
  if (!t || /^replace this with/i.test(t)) return [];
  const out = [];
  let cur = '';
  let inQuotes = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (c === '"') {
      if (inQuotes && t[i + 1] === '"') { cur += '"'; i++; continue; } // doubled quote
      inQuotes = !inQuotes;
      continue;
    }
    if (c === ',' && !inQuotes) {
      const name = cur.trim();
      if (name && !/^none$/i.test(name)) out.push(name);
      cur = '';
      continue;
    }
    cur += c;
  }
  const last = cur.trim();
  if (last && !/^none$/i.test(last)) out.push(last);
  return out;
}

// initiativeEditFromBody reads the dialog's live DOM back into an InitiativeEdit
// body (the PATCH shape). Explicit-clear protocol: JSON null means "not
// mentioned" to the server's pointers, so every field the dialog emptied sends
// a real clear — priority 0, tier 0, kit/progress 0 (where one existed), an
// empty after-list, and clearDate for a date that had one. Otherwise a planner
// who empties a field sees it come back on the next open and learns the dialog
// lies.
export function initiativeEditFromBody(read, name, had = {}) {
  const num = (id, max) => {
    const raw = String(read(id) ?? '').trim();
    if (raw === '') return null;
    const n = Number(raw);
    if (!Number.isFinite(n) || n < 0) return undefined; // invalid: caller shows the error
    if (max !== undefined && n > max) return undefined;
    return n;
  };
  const body = { name };
  const priority = num('ie-priority');
  if (priority === undefined) return { error: 'priority must be a number ≥ 1' };
  if (priority !== null) body.statedPriority = Math.max(1, Math.round(priority));
  else body.statedPriority = 0; // explicit clear: blank means unranked
  const tier = num('ie-tier', 4);
  if (tier === undefined) return { error: 'tier must be 1-4' };
  if (tier !== null && tier >= 1) body.tier = Math.round(tier);
  else body.tier = 0; // explicit clear: blank means unscored
  const cod = num('ie-cod', 10);
  if (cod === undefined) return { error: 'cost of delay must be 0-10' };
  if (cod !== null) body.costOfDelayPerWeek = cod;
  else if (had.costOfDelayPerWeek) body.costOfDelayPerWeek = 0; // cleared, if one existed
  const kit = num('ie-kit', 100);
  if (kit === undefined) return { error: 'kit readiness must be 0-100' };
  if (kit !== null) body.kitPct = kit / 100;
  else if (had.kitPct) body.kitPct = 0; // cleared, if one existed
  const progress = num('ie-progress', 100);
  if (progress === undefined) return { error: 'progress must be 0-100' };
  if (progress !== null) body.progressPct = progress / 100;
  else if (had.progressPct) body.progressPct = 0; // cleared, if one existed
  const target = String(read('ie-target') ?? '').trim();
  if (target) body.targetDate = target;
  else if (had.targetDate) body.clearDate = true; // explicit clear of an existing date
  body.earliestStart = String(read('ie-earliest') ?? '').trim() || null;
  body.priorityLocked = !!read('ie-priority-locked');
  body.dateLocked = !!read('ie-date-locked');
  body.inFlight = !!read('ie-inflight');
  body.afterInitiatives = splitInitiativeNames(String(read('ie-after') ?? ''));
  return { body };
}

// setupCardHTML is spec 009 FR-005: the three model choices a fresh plan
// hasn't made, as a checklist with the recommended defaults one click away.
// Once every model is chosen (or the card is acknowledged) it disappears.
export function setupCardHTML(sp = {}, dated = 0) {
  if (sp.setupAcknowledged) return '';
  const items = [];
  const wipChosen = WIP_MODELS.some((m) => m.id === sp.wipModel);
  if (!wipChosen) {
    items.push({ field: 'Work-in-progress model', why: 'which initiatives count against the org limit',
      rec: 'strict — protect the constraint absolutely, accept idle elsewhere (you can compare the three anytime)' });
  }
  // Wall-clock is the persisted default, so "never chose" and "deliberately
  // wall-clock" are the same wire value — estimateAck records the deliberate
  // keep so the card stops asking without forcing effort (cubic P2).
  if (sp.estimateModel !== 'effort' && !sp.estimateAck) {
    items.push({ field: 'Estimate model', why: 'how the sheet\u2019s estimate column is read',
      rec: 'effort — total work divided across each team\u2019s lanes', key: 'estimate' });
  }
  if (!items.length) return '';
  return `<div class="panel-card ord-setup-card" id="setup-card">
    <b>Set up this plan</b>
    <p class="hint">Two choices shape every number here. Recommended defaults below — one click each, change later in ⚙ Assumptions.</p>
    ${items.map((it, i) => `<div class="setup-item">
      <div><b>${esc(it.field)}</b> <span class="hint">— ${esc(it.why)}</span></div>
      <div class="hint">recommended: ${esc(it.rec)}</div>
      <button type="button" class="primary setup-apply" data-setup="${i === 0 && !wipChosen ? 'wip' : 'estimate'}">use recommended</button>
      ${it.key === 'estimate' ? '<button type="button" class="setup-keep hint">keep wall-clock</button>' : ''}
    </div>`).join('')}
    <button type="button" class="setup-dismiss hint">dismiss — keep the defaults silently</button>
  </div>`;
}

export function orderViewHTML(sched, opts = {}) {
  const dated = (sched.initiatives || []).filter((si) => si.targetWeek !== null && si.targetWeek !== undefined).length;
  return `${setupCardHTML(opts.scheduling || {}, dated)}
  <div class="panel-card ord-card">
    ${orderHeaderHTML(sched, opts)}
    ${opts.scheduling === undefined ? '' : schedulingDialogHTML(opts.scheduling, sched.wipLimit, sched)}
    ${orderTableHTML(sched, opts)}
    ${feverChartHTML(sched)}
    ${fitNote(sched.fit, opts.horizonWeeks || sched.horizonWeeks)}
    ${infeasibleNote(sched)}
    ${noticesHTML(sched)}
  </div>
  <div class="panel-card ord-card" style="margin-top:12px">
    <b>Pod load, week by week</b>${term('rho')}
    <div class="ord-heatwrap">${podHeatmapHTML(sched, opts.horizonWeeks)}</div>
  </div>
  ${opts.pod ? podQueueHTML(sched, opts.pod) : ''}`;
}

// --- Scheduling assumptions form (§7 SchedulingParams) --------------------
//
// These knobs live in the Order view rather than the plan header on purpose. The
// header holds plan-wide capacity facts that Network, the rho table and the
// simulator all consume; these only affect the order, and putting them in the
// header would imply that moving them changes the Network view's numbers.
//
// Only knobs that do something are offered. leadCapacity and the transfer
// settings are in §7 but nothing consumes them yet, and a control that silently
// does nothing is worse than an absent one. (targetUtilization joined the live
// set when the drum stagger landed — spec 004 Story 5.)

// The input and its suffix share a row inside the column layout, and the input is
// wide enough for its placeholder: these placeholders say what leaving the field
// blank does, and a truncated "deri…" defeats the point of saying it.
const numField = (id, label, value, placeholder, help, suffix, max) =>
  `<label class="hint sched-f">${label}
    <span class="sched-row">
      <input id="${id}" type="number" min="0" max="${max}" step="1" value="${esc(value)}"
        placeholder="${esc(placeholder)}">${suffix ? `<span class="hint">${suffix}</span>` : ''}
    </span>
    <span class="hint">${esc(help)}</span></label>`;

const pctField = (id, label, value, placeholder, help) =>
  numField(id, label, value, placeholder, help, '%', 100);

const intField = (id, label, value, placeholder, help) =>
  numField(id, label, value, placeholder, help, '', 99);

const asPct = (v) => (v === null || v === undefined || v === '') ? '' : String(Math.round(v * 100));
const asInt = (v) => (!v ? '' : String(v));

// calendarWindowsHTML is FR-018's editor: one row per saved window (kind,
// scope, dates, effect), an add button, and one sentence saying what windows
// are for. Rows are numbered so schedulingFromForm can read them back; a
// half-filled row is dropped at read time rather than refused by the server.
const CAL_KINDS = [
  ['change-freeze', 'change freeze'],
  ['site-nonworking', 'site non-working'],
  ['event', 'event'],
];
const CAL_EFFECTS = [
  ['block-start', 'block starts'],
  ['block-finish', 'block completions'],
  ['reduce-capacity', 'reduce capacity'],
];

function calendarWindowsHTML(windows) {
  const sel = (id, opts, val, label) => `<select class="cal-sel" id="${id}" aria-label="${esc(label)}">${opts.map(([v, l]) =>
    `<option value="${v}"${v === val ? ' selected' : ''}>${l}</option>`).join('')}</select>`;
  const row = (w, i) => `<div class="cal-win" data-row="${i}">
    ${sel(`cal-kind-${i}`, CAL_KINDS, w.kind || 'change-freeze', `window ${i + 1} kind`)}
    <input class="cal-in" placeholder="scope (org, a site, a pod)" id="cal-scope-${i}"
      aria-label="window ${i + 1} scope" value="${esc(w.scope || '')}">
    <input class="cal-in" type="date" id="cal-from-${i}" aria-label="window ${i + 1} from date" value="${esc(w.fromDate || '')}">
    <input class="cal-in" type="date" id="cal-to-${i}" aria-label="window ${i + 1} to date" value="${esc(w.toDate || '')}">
    ${sel(`cal-effect-${i}`, CAL_EFFECTS, w.effect || 'block-start', `window ${i + 1} effect`)}
    <button type="button" class="cal-del" aria-label="remove window ${i + 1}">✕</button>
  </div>`;
  const intro = windows.length
    ? '' : `<p class="hint">Calendar windows: a change freeze that blocks starts or completions, a site's non-working weeks, or an event that reduces a pod's capacity — drawn on the timeline and enforced by the order.</p>`;
  return `<div class="cal-wins">
    <b class="hint">calendar windows</b>
    ${intro}
    ${windows.map(row).join('')}
    <button type="button" id="cal-add">add a window</button>
  </div>`;
}

// schedulingFormHTML renders the assumptions behind the order, with the current
// values filled in. Placeholders carry what happens when a field is left blank,
// since "blank" is a real setting for every one of these and not an omission.
// The assumptions form is a settings dialog (IA audit, Finding 2): set-once
// configuration that used to live in the reading path. The header keeps a
// compact "assumptions" button; the dialog carries the full form.
export function schedulingDialogHTML(sp, wip, sched) {
  // Auto-open on the same conditions the old <details open> used: a missing
  // period start or an unchosen WIP model is a decision the plan cannot be
  // read without. Everything else opens on the ⚙ button.
  const urgent = !sp.periodStart || !WIP_MODELS.some((m) => m.id === sp.wipModel);
  return `<div class="ord-sched" id="sched-dialog" role="dialog" aria-modal="true"
    aria-label="Scheduling assumptions"${urgent ? '' : ' hidden'}>
    <div class="ord-sched-box">${schedulingFormHTML(sp, wip, sched)}</div>
  </div>`;
}

export function schedulingFormHTML(sp = {}, wip, sched) {
  const derived = wip && wip.derived
    ? `derived: ${wip.value} from ${wip.fromPod}` : 'derived from the drum';
  const missingPeriod = !sp.periodStart;
  const unchosenModel = !WIP_MODELS.some((m) => m.id === sp.wipModel);
  const options = [
    `<option value=""${unchosenModel ? ' selected' : ''}>— not chosen —</option>`,
    ...WIP_MODELS.map((m) => `<option value="${m.id}"${sp.wipModel === m.id ? ' selected' : ''}>${esc(m.label)}</option>`),
  ].join('');
  return `<div>
    ${missingPeriod ? `<p class="plan-warn">This plan has no period start, so target dates cannot be
      turned into weeks and every initiative reads as "no date". Set one to see the verdicts.</p>` : ''}
    ${unchosenModel ? `<p class="plan-warn">No work-in-progress model chosen. The order below is computed
      as <b>strict</b> so nothing moves under you, but the choice changes what the schedule means —
      compare the three below and pick one.</p>` : ''}
    <div class="sched-grid">
      <label class="hint sched-f">work-in-progress model${term('wip-model')}
        <span class="sched-row"><select id="sched-wip-model">${options}</select></span>
        <span class="hint">which initiatives count against the org limit</span></label>
      <label class="hint sched-f">period starts
        <span class="sched-row"><input id="sched-period-start" type="date" value="${esc(sp.periodStart || '')}"></span>
        <span class="hint">week 0 — target dates are measured from here</span></label>
      <label class="hint sched-f">estimate model${term('estimate-model')}
        <span class="sched-row"><select id="sched-estimate-model">
          <option value="wall-clock"${sp.estimateModel !== 'effort' ? ' selected' : ''}>wall-clock (one lane's duration)</option>
          <option value="effort"${sp.estimateModel === 'effort' ? ' selected' : ''}>effort (divided across lanes)</option>
        </select></span>
        <span class="hint">how the sheet's estimate column is read — existing plans stay on wall-clock</span></label>
      <label class="hint sched-f">splitting tax${term('splitTax')}
        <span class="sched-row"><input id="sched-split-tax" type="number" min="0" step="1" value="${asInt(sp.splitTaxWeeks)}"
          placeholder="off"></span>
        <span class="hint">weeks of overhead per lane-split; blank disables splitting</span></label>
      <label class="hint sched-f">lane chunking
        <span class="sched-row"><select id="sched-chunking">
          <option value="spread"${!sp.splitMinWeeks ? ' selected' : ''}>spread evenly (all lanes take a share)</option>
          <option value="chunk"${sp.splitMinWeeks ? ' selected' : ''}>chunk — no track carries more than…</option>
        </select></span>
        <span class="hint">how big work divides across a team's tracks: spread shares it evenly, chunk caps each track's load (45w chunks 20+20+5 at a 20-week cap)</span></label>
      <label class="hint sched-f">chunk size (weeks)
        <span class="sched-row"><input id="sched-split-min" type="number" min="1" step="1" value="${esc(String(asInt(sp.splitMinWeeks) || 20))}"
          ${sp.splitMinWeeks ? '' : 'disabled'}></span>
        <span class="hint">the per-track cap when chunking; work under this stays whole on one track</span></label>
      ${intField('sched-wip', 'org WIP limit', asInt(sp.maxConcurrentInitiatives), derived,
    'initiatives in flight at once; blank derives it from the drum pod')}
      ${pctField('sched-buffer', 'buffer', asPct(sp.bufferPct), '25', 'of each chain; blank means 25%, 0 commits on the raw finish')}
      ${pctField('sched-kit', 'full-kit gate', asPct(sp.kitGate), 'none', 'minimum readiness to start; blank means no gate')}
      ${intField('sched-pod-wip', 'per-pod limit', asInt(sp.maxInitiativesPerPod), 'uncapped',
    'initiatives at one pod at once')}
      ${intField('sched-quarter', 'starts per quarter', asInt(sp.maxStartsPerQuarter), 'uncapped',
    'how much change the org can absorb')}
      ${pctField('sched-stagger', 'drum target utilization', asPct(sp.targetUtilization), 'off',
    'hold releases so drum load stays under this; blank means no stagger')}
    </div>
    ${calendarWindowsHTML(sp.calendars || [])}
    <button type="button" id="sched-save" class="primary">Save assumptions</button>
    <button type="button" id="sched-cancel">Cancel</button>
    <!-- Always present, even when empty: the comparison is fetched only when this
         dialog opens (it costs three extra schedules server-side), and planui fills
         this container in place rather than re-rendering the view around it. -->
    <div id="wip-models">${sched ? wipModelsTableHTML(sched) : ''}</div>
  </div>`;
}

// The WIP models a planner chooses between (spec 001 §11 D22, amended). The wording
// matters more than usual here: this choice changes what the schedule *means*, not
// just a number in it, so each option states what it protects and what it costs.
export const WIP_MODELS = [
  { id: 'strict', label: 'strict', blurb: 'every initiative counts against the drum\u2019s tracks — protects the constraint absolutely, accepts idle elsewhere' },
  { id: 'drum-gated', label: 'drum-gated', blurb: 'only initiatives that use the drum count — work that never touches the constraint flows freely' },
  { id: 'off', label: 'off', blurb: 'no org limit; pod tracks, leads and dependencies are the only constraints' },
];

// wipModelsTableHTML compares the models on the planner's own plan. Static help
// text can describe a model; only this can say what choosing it costs here.
export function wipModelsTableHTML(sched) {
  const rows = sched.wipModels || [];
  if (!rows.length) return '';
  const chosen = (sched.wipLimit && sched.wipLimit.model) || 'unchosen';
  // Unchosen still produced the order on screen, computed as strict. Marking that
  // row "reading as this" rather than "in force" tells the reader which line made
  // the table they are looking at, without claiming a choice nobody made.
  const unchosen = chosen === 'unchosen';
  const marked = unchosen ? 'strict' : chosen;
  const tag = unchosen ? 'reading as this' : 'in force';

  const body = rows.map((o) => {
    const current = o.model === marked;
    return `<tr${current ? ' class="ord-inforce"' : ''}>
      <td>${current ? '<b>' : ''}${esc(o.model)}${current ? `</b> <span class="tag">${tag}</span>` : ''}</td>
      <td>${o.limit > 0 ? o.limit : '<span class="hint">none</span>'}</td>
      <td>${weekLabel(o.lastCommitWeek)}</td>
      <td>${o.datesMissed}${o.infeasible ? ` <span class="hint">(${o.infeasible} cannot fit)</span>` : ''}</td>
      <td>${o.podsIdleAllPeriod}</td>
      <td>${o.objective}</td>
    </tr>`;
  }).join('');

  // The same dates are missed under every model on most plans; the difference is
  // what they cost. Saying so stops the table reading as "pick the smallest number".
  const missed = new Set(rows.map((o) => o.datesMissed));
  const sameMisses = missed.size === 1 && rows.length > 1;

  return `<table class="wip-table ord-models"><thead><tr>
      <th>model${term('wip-model')}</th><th>limit</th><th>ends</th><th>dates missed</th><th>pods idle all period</th><th>cost${term('weighted-late')}</th>
    </tr></thead><tbody>${body}</tbody></table>
    <ul class="hint ord-models-why">
      ${WIP_MODELS.map((m) => `<li><b>${esc(m.label)}</b> — ${esc(m.blurb)}</li>`).join('')}
    </ul>
    ${sameMisses ? `<p class="hint">Every model here misses the same number of dates
      (${rows[0].datesMissed}) — though not necessarily the same ones. What changes is what the
      misses cost and how much of the org sits idle: a model buys cheaper misses and busier pods,
      not fewer misses.</p>` : ''}
    <p class="hint">Cost is weighted weeks late. It favours <b>off</b> by construction: the schedule
      makes waiting explicit but charges nothing for multitasking, so it cannot price what a WIP limit
      is for. That is why this is a choice and not a calculation.</p>`;
}

// pctToFraction turns a percentage a person typed into the 0..1 fraction §7 stores.
// Anything unreadable is treated as absent rather than as zero, because 0 is a
// meaningful setting here and must only come from someone typing it.
export function pctToFraction(raw) {
  const n = Number(String(raw).trim());
  if (!Number.isFinite(n)) return null;
  return Math.min(1, Math.max(0, n / 100));
}

// schedulingFromForm reads the form into the body PATCH /scheduling expects.
//
// The distinction that matters: a blank field is omitted from the body entirely,
// so the server's default applies, while a typed 0 is sent as 0. For bufferPct
// those mean different things — absent is 25% of the chain, an explicit 0 is
// "commit on the raw finish" — and collapsing them would take away a choice
// Decision 20 deliberately left open.
export function schedulingFromForm(read) {
  const raw = (id) => String(read(id) ?? '').trim();
  const out = {};

  const start = raw('sched-period-start');
  if (start) out.periodStart = start;

  // Absent means unchosen, which is a state the server reports rather than a
  // default it silently applies.
  const model = raw('sched-wip-model');
  if (WIP_MODELS.some((m) => m.id === model)) out.wipModel = model;

  for (const [id, key] of [
    ['sched-wip', 'maxConcurrentInitiatives'],
    ['sched-pod-wip', 'maxInitiativesPerPod'],
    ['sched-quarter', 'maxStartsPerQuarter'],
  ]) {
    const n = Number(raw(id));
    // 0 and blank both mean "no limit" for these, and §7 spells that as absent.
    if (raw(id) !== '' && Number.isFinite(n) && n > 0) out[key] = Math.round(n);
  }

  for (const [id, key] of [['sched-buffer', 'bufferPct'], ['sched-kit', 'kitGate']]) {
    if (raw(id) === '') continue;
    const f = pctToFraction(raw(id));
    if (f !== null) out[key] = f;
  }

  // The drum stagger's target (spec 004 Story 5): a fraction in (0,1); blank or
  // 100 means off. Anything else is ignored rather than clamped silently.
  {
    const f = pctToFraction(raw('sched-stagger'));
    if (f !== null && f > 0 && f < 1) out.targetUtilization = f;
  }

  // The estimate model (spec 006 Decision 2): wall-clock is the migration-safe
  // default, so only an explicit effort is sent.
  if (raw('sched-estimate-model') === 'effort') out.estimateModel = 'effort';

  // The splitting tax (spec 007): weeks per split; blank or 0 disables.
  {
    const n = Number(raw('sched-split-tax'));
    if (Number.isFinite(n) && n > 0) out.splitTaxWeeks = Math.round(n);
  }

  // The split threshold (spec 007 amendment): the per-track cap when the
  // lane-chunking mode is "chunk". "spread" sends nothing — the server
  // spreads evenly, the default.
  if (raw('sched-chunking') === 'chunk') {
    const n = Number(raw('sched-split-min'));
    if (Number.isFinite(n) && n > 0) out.splitMinWeeks = Math.round(n);
  }

  // Calendar windows: read every numbered row, keep only the complete ones.
  // A half-filled row is a planner mid-edit, not a constraint.
  out.calendars = [];
  for (let i = 0; ; i++) {
    const kind = raw(`cal-kind-${i}`);
    const scope = raw(`cal-scope-${i}`);
    const from = raw(`cal-from-${i}`);
    if (!kind && !scope && !from) break;
    const to = raw(`cal-to-${i}`);
    const effect = raw(`cal-effect-${i}`);
    if (!from || !to || !effect) continue;
    out.calendars.push({ kind: kind || 'event', scope, fromDate: from, toDate: to, effect });
  }
  if (!out.calendars.length) delete out.calendars;
  return out;
}

// fitNote is Decision 28's aggregate: the sentence that explains why work did not
// fit, in terms a planner can act on. A week number a decade out was the old
// answer to "why doesn't this fit"; this one names the load, or the constraint
// when capacity is not the reason.
//
// Silent when everything fits. A note that always shows is a note nobody reads.
export function fitNote(fit, horizonWeeks) {
  if (!fit || !fit.beyondHorizon) return '';
  const horizon = Math.max(1, Math.ceil(horizonWeeks || 26));
  const n = fit.beyondHorizon;
  const many = n > 1;
  // No tracks at all is its own answer, not a 0% load. The percentage branch below
  // would report "0% of capacity is used" and blame the release rules, sending a
  // planner to look at limits when the problem is that there is nobody to do the work.
  if (!(fit.trackWeeksAvailable > 0)) {
    return `<p class="plan-warn ord-fit">${n} initiative${many ? 's' : ''}
      ${many ? 'do' : 'does'} not fit this ${horizon}-week period: these pods have
      no capacity at all — every one is at zero tracks, so nothing can be scheduled.</p>`;
  }
  const load = Math.round((fit.podWeeksDemanded / fit.trackWeeksAvailable) * 100);
  // Over capacity, the load is the story. Under it, the pods had room and
  // something else refused the work — saying "125%" there would send the planner
  // to hire people when the lever is a limit they chose.
  const why = load >= 100
    ? `this plan asks for <b>${load}%</b> of what these pods can absorb in ${horizon} weeks
       (${Math.round(fit.podWeeksDemanded)} pod-weeks of work against
       ${Math.round(fit.trackWeeksAvailable)} available)`
    : (fit.heldBy || []).length
      ? `the pods had room — ${load}% of capacity is used — but
         <b>${esc(fit.heldBy[0].constraint)}</b> held ${fit.heldBy[0].count > 1 ? 'them' : 'it'} out`
      : `${load}% of capacity is used, and the release rules held ${many ? 'them' : 'it'} out`;
  return `<p class="plan-warn ord-fit">${n} initiative${many ? 's' : ''}
    ${many ? 'do' : 'does'} not fit this ${horizon}-week period: ${why}.</p>`;
}
