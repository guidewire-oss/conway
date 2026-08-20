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
  }[si.verdict] || { symbol: '●', text: si.verdict || 'unknown', zone: 'idle' };
  // AC 2.5: a verdict resting on unestimated work is marked provisional wherever
  // it is shown, rather than being presented as though it were firm.
  return si.provisional ? { ...base, text: `${base.text}, provisional` } : base;
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

function orderRowHTML(row) {
  const { si, verdict } = row;
  const target = si.targetWeek === null || si.targetWeek === undefined
    ? '<span class="hint">—</span>' : weekLabel(si.targetWeek);
  // AC 2.1 shows both numbers: the raw finish is what the schedule says, the commit
  // is what may be promised, and Decision 9 exists because they are not one claim.
  const raw = si.rawFinishWeek === undefined ? '' :
    `<span class="hint" title="raw scheduled finish, before its buffer">${weekLabel(si.rawFinishWeek)} +${si.bufferWeeks || 0}w →</span> `;
  const main = `<tr class="ord-row">
    <td>${si.proposedRank}</td>
    <td>${esc(si.name)}</td>
    <td>${statedCell(si)}</td>
    <td>${weekLabel(si.startWeek)}</td>
    <td>${raw}<b>${weekLabel(si.commitWeek)}</b></td>
    <td>${target}</td>
    <td class="ord-${verdict.zone}"><b>${verdict.symbol} ${esc(verdict.text)}</b></td>
    <td>${esc(row.binds) || '<span class="hint">—</span>'}</td>
  </tr>`;
  const trace = rowTraceHTML(row);
  if (!trace) return main;
  return `${main}<tr class="ord-why"><td></td><td colspan="7">${trace}</td></tr>`;
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

export function orderTableHTML(sched) {
  const rows = orderRows(sched);
  if (!rows.length) return '<p class="hint">No initiatives to order yet.</p>';
  return `<table class="wip-table ord-table">
    <thead><tr>
      <th>#</th><th>Initiative</th><th>Stated</th><th>Start</th>
      <th>Commit</th><th>Target</th><th>Verdict</th><th>Binds</th>
    </tr></thead>
    <tbody>${rows.map(orderRowHTML).join('')}</tbody>
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
    Only a later date, less scope or an earlier start moves these.</p>`;
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

export function podHeatmapHTML(sched, horizonWeeks) {
  const pods = sched.podWeeks || [];
  if (!pods.length) return '<p class="hint">No pod load to show yet.</p>';
  const weeks = heatmapWeeks(sched, horizonWeeks);
  const drums = new Set(sched.drumPods || []);

  const head = Array.from({ length: weeks }, (_, w) => `<th class="ord-wk">${w % 5 === 0 ? w : ''}</th>`).join('');
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
    ${overrunNote(sched, weeks)}`;
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
  return `<div class="card ord-queue">
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

export function orderHeaderHTML(sched) {
  const obj = objectiveView(sched);
  const rules = (sched.rulesTried || []).length;
  let score;
  if (!obj.comparable) {
    score = '<span class="hint">no dates or priorities set yet, so there is no order to argue with</span>';
  } else if (obj.allOnTime) {
    score = '<b class="ord-green">every date in this plan holds</b> <span class="hint">— nothing is late under either order</span>';
  } else {
    score = `your stated order: <b>${obj.stated}</b> weighted weeks late · proposed: <b>${obj.proposed}</b>
      ${obj.delta !== 0 ? `· <b class="${obj.better ? 'ord-green' : 'ord-red'}">Δ ${obj.delta > 0 ? '+' : ''}${obj.delta}</b>` : ''}`;
  }
  return `<div class="ord-head">
    <b>Execution order</b>
    <span class="hint">rule: ${esc(sched.rule || '—')}${rules ? ` (best of ${rules})` : ''}</span>
    <span class="hint">${wipLimitNote(sched.wipLimit)}</span>
  </div>
  <div class="plan-summary">${score}</div>`;
}

export function orderViewHTML(sched, opts = {}) {
  return `<div class="card ord-card">
    ${orderHeaderHTML(sched)}
    ${orderTableHTML(sched)}
    ${infeasibleNote(sched)}
    ${noticesHTML(sched)}
  </div>
  <div class="card ord-card" style="margin-top:12px">
    <b>Pod load, week by week</b>
    <div class="ord-heatwrap">${podHeatmapHTML(sched, opts.horizonWeeks)}</div>
  </div>
  ${opts.pod ? podQueueHTML(sched, opts.pod) : ''}`;
}
