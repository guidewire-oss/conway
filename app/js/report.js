// Plan health report (spec 013): one printable card answering "is my plan OK,
// and what would fix it" — the verdict distribution, the initiatives that will
// not fit the period, the pods that cannot carry the load, the date-locked
// conflicts, and the engine's top remedies.
//
// Decision 1: this card is a SUMMARIZING LENS over the rendered schedule and
// the remedies response. It aggregates (counts, sorting, top-N) — it never
// computes a verdict, a rho, or a schedule of its own, so a number on the card
// cannot contradict the Order view by construction.

import { esc } from './order.js';
import { activeBaseline, fmtWhen } from './baseline.js';
import { remedyKindLabel } from './remedyui.js';

// The verdicts that mean "this initiative does not land where planned".
const PROBLEM_VERDICTS = ['late', 'beyond-horizon', 'structurally-infeasible', 'unschedulable'];
const VERDICT_LABELS = {
  'on-time': 'on time',
  'late': 'late',
  'beyond-horizon': 'past the horizon',
  'structurally-infeasible': 'structurally infeasible',
  'unschedulable': 'unschedulable',
  'no-date': 'no target date',
};

const byVerdict = (sched, v) => (sched.initiatives || []).filter((i) => i.verdict === v);

// fitSentence is the one line the meeting needs (AC 1.2): how many of the
// total initiatives will not finish inside the period. Anything that is not
// a known-good verdict counts as not landing — a future server's new bad
// verdict must never read as all-green (remedyui's upgrade-tolerance rule).
const GOOD_VERDICTS = ['on-time', 'no-date'];

export function fitSentence(sched) {
  const inits = sched.initiatives || [];
  const doomed = inits.filter((i) => !GOOD_VERDICTS.includes(i.verdict));
  if (!inits.length) return 'No initiatives are scheduled yet.';
  if (!doomed.length) {
    return inits.length === 1
      ? 'The one initiative commits inside the period.'
      : `All ${inits.length} initiatives commit inside the period.`;
  }
  return `${doomed.length} of ${inits.length} initiatives will not finish inside the period.`;
}

export function verdictSectionHTML(sched) {
  const inits = sched.initiatives || [];
  if (!inits.length) return `<h3>Verdicts</h3><ul class="report-list"><li>no schedule</li></ul>`;
  const verdicts = [...new Set(inits.map((i) => i.verdict))];
  const rows = [];
  for (const v of ['on-time', ...PROBLEM_VERDICTS, 'no-date']) {
    const list = verdicts.includes(v) ? byVerdict(sched, v) : [];
    if (!list.length) continue;
    const label = VERDICT_LABELS[v] || v;
    if (PROBLEM_VERDICTS.includes(v)) {
      const names = list.map((i) => {
        const late = i.weeksLate ? ` (+${i.weeksLate}w)` : '';
        return `${esc(i.name)}${late}`;
      }).join(', ');
      rows.push(`<li><b>${list.length}</b> ${esc(label)}: ${names}</li>`);
    } else {
      rows.push(`<li><b>${list.length}</b> ${esc(label)}</li>`);
    }
  }
  // A verdict this page does not know (a server newer than the page) renders
  // generically rather than vanishing — counts must sum to the total.
  for (const v of verdicts.filter((x) => !['on-time', ...PROBLEM_VERDICTS, 'no-date'].includes(x))) {
    const list = byVerdict(sched, v);
    const names = list.map((i) => esc(i.name)).join(', ');
    rows.push(`<li><b>${list.length}</b> ${esc(v)}: ${names}</li>`);
  }
  return `<h3>Verdicts</h3><ul class="report-list">${rows.join('')}</ul>`;
}

// capacitySectionHTML names the pods that cannot carry the load (AC 1.3):
// over-capacity (rho >= 1) and hot (>= 0.85), hottest first, drum pods marked.
export function capacitySectionHTML(sched) {
  const pods = (sched.podWeeks || []).map((p) => ({
    pod: p.pod, rho: typeof p.flatRho === 'number' ? p.flatRho : null,
    tracks: p.tracks, drum: (sched.drumPods || []).includes(p.pod),
  }));
  const over = pods.filter((p) => p.rho !== null && p.rho >= 1).sort((a, b) => b.rho - a.rho);
  const hot = pods.filter((p) => p.rho !== null && p.rho >= 0.85 && p.rho < 1).sort((a, b) => b.rho - a.rho);
  const line = (p) => `<li><b>${esc(p.pod)}</b> flat ρ ${p.rho.toFixed(2)} · ${p.tracks} track${p.tracks > 1 ? 's' : ''}${p.drum ? ' · <b>drum</b>' : ''}</li>`;
  if (!over.length && !hot.length) {
    return `<h3>Capacity</h3><p class="report-ok">Every pod is comfortably inside capacity.</p>`;
  }
  return `<h3>Capacity</h3><ul class="report-list">
    ${over.length ? `<li><b>Over capacity (ρ≥1):</b><ul>${over.map(line).join('')}</ul></li>` : ''}
    ${hot.length ? `<li><b>Queue hot (ρ≥0.85):</b><ul>${hot.map(line).join('')}</ul></li>` : ''}
  </ul>`;
}

// conflictsSectionHTML surfaces date-locked contention or states its absence
// (AC 1.4) — the data rides the schedule (spec 001 AC 3.3), never recomputed.
export function conflictsSectionHTML(sched) {
  const conflicts = sched.conflicts || [];
  if (!conflicts.length) return `<h3>Date conflicts</h3><p class="report-ok">No date-locked initiatives contend for the same pod.</p>`;
  return `<h3>Date conflicts</h3><ul class="report-list">${conflicts.map((c) =>
    `<li><b>${esc(c.a)}</b> + <b>${esc(c.b)}</b> on ${esc(c.pod)} — ${esc(c.note || '')}</li>`).join('')}</ul>`;
}

// remediesSectionHTML lists the top remedies by portfolio improvement (AC 1.5).
// A failing fetch degrades to a named note (NFR-003) — the card still renders.
// Each row carries a link-back to the Order view's priced-options panel for
// that initiative (the AC's "full picture"); the server's warnings render
// under the list, so a missing transfer-capacity remedy explains itself.
export function remediesSectionHTML(data) {
  if (!data) return `<p class="hint">computing remedies…</p>`;
  if (data.error) return `<p class="plan-warn">${esc(data.error)}</p>`;
  const remedies = [...(data.remedies || [])]
    .filter((r) => typeof r.objectiveDelta === 'number')
    .sort((a, b) => a.objectiveDelta - b.objectiveDelta)
    .slice(0, 3);
  const warnings = (data.warnings || []).map((w) => `<p class="hint">${esc(w)}</p>`).join('');
  if (!remedies.length) return `${warnings || ''}<p class="report-ok">No remedies — the engine sees nothing worth pulling.</p>`;
  return `<ul class="report-list">${remedies.map((r) => {
    const victims = (r.affectedInitiatives || []).length;
    return `<li><b>${esc(remedyKindLabel(r.kind))}</b> — ${esc(r.target)} → ${esc(remedyLabelOf(r.resultingVerdict))}, portfolio ${fmtDelta(r.objectiveDelta)}${victims ? `, moves ${victims} other initiative${victims > 1 ? 's' : ''}` : ''} <button type="button" class="report-remedy-link" data-target="${esc(r.target)}">full options</button></li>`;
  }).join('')}</ul>${warnings}`;
}

const remedyLabelOf = (v) => VERDICT_LABELS[v] || v || 'a better verdict';
// The Order view's own convention: the objective is a cost, so a lower number
// is an improvement, and zero is printed as-is (an unlock that helps nothing
// is a real row the server emits for every date-locked initiative).
const fmtDelta = (n) => (n === 0 ? '±0' : `${n < 0 ? '−' : '+'}${Math.abs(n)}`);

// healthReportHTML assembles the card (FR-002..FR-005, FR-008, FR-009, FR-011).
// remediesHTML is filled in by the caller once the remedies POST returns.
export function healthReportHTML(sched, opts = {}) {
  if (!sched || !sched.initiatives) {
    return `<div class="report-card"><h2>Plan health report</h2>
      <p class="hint">No schedule yet — open the Order view to compute one, then open this report.</p></div>`;
  }
  const active = activeBaseline(opts.baselines);
  const period = sched.periodStart
    ? ` · period starts ${esc(sched.periodStart)}, horizon ${sched.horizonWeeks}w`
    : ` · horizon ${sched.horizonWeeks}w`;
  return `<div class="report-card">
    <div class="report-head">
      <h2>${esc(opts.planName || 'Plan')} — health report</h2>
      <p class="hint">${fitSentence(sched)}${period}</p>
    </div>
    ${verdictSectionHTML(sched)}
    ${capacitySectionHTML(sched)}
    ${conflictsSectionHTML(sched)}
    <h3>Top remedies <span class="hint">(what the engine would change)</span></h3>
    <div id="report-remedies">${opts.remediesHTML || '<p class="hint">computing remedies…</p>'}</div>
    <div class="report-foot">
      <p>${active
        ? `Baseline: <b>${esc(active.name)}</b>, saved ${fmtWhen(active.createdAt)}.`
        : 'No baseline saved — save one to compare future re-plans against.'}</p>
      <p class="hint">Dispatch rule: ${esc(sched.rule || '?')} · portfolio objective ${esc(String(sched.objectiveScore ?? '?'))} · generated ${esc(opts.generatedAt || '')}</p>
    </div>
  </div>`;
}
