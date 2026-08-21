// Baselines in the Plan UI: the agreed order for a period, frozen (spec 001
// Story 7, §13.1's always-visible chip and §13.2's save control).
//
// Pure functions from server payloads to HTML strings, like order.js — planui.js
// owns the fetching and the DOM. That is what lets tests/baseline.test.mjs cover
// the whole surface under `node --test`.

import { esc, weekLabel } from './order.js';

// fmtWhen renders a unix timestamp the way the rest of Plan does: short and local,
// because "12 Jan" is what a planner recognises, not an ISO string.
export function fmtWhen(unix) {
  if (!unix) return '';
  return new Date(unix * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

// activeBaseline is the one actuals and variance are measured against (FR-031).
export function activeBaseline(baselines) {
  return (baselines || []).find((b) => b.active) || null;
}

// baselineChipHTML is §13.1's always-visible indicator. The dot turns amber when
// the plan's inputs have moved since the active baseline was saved (FR-030) —
// which is the whole reason it is always visible rather than tucked into a panel.
export function baselineChipHTML(baselines) {
  const active = activeBaseline(baselines);
  if (!active) {
    return `<span class="bl-chip hint" title="Nothing has been agreed for this period yet">
      baseline: <span class="bl-dot bl-none">○</span> none</span>`;
  }
  const diverged = !!active.diverged;
  const why = diverged
    ? "The plan's inputs have changed since this baseline was saved, so it no longer describes this plan"
    : 'This baseline matches the plan as it stands';
  return `<span class="bl-chip" title="${esc(why)}">baseline:
    <span class="bl-dot ${diverged ? 'bl-diverged' : 'bl-current'}">●</span>
    ${esc(active.name)}${diverged ? ' <span class="tag">diverged</span>' : ''}</span>`;
}

// baselineListHTML is the history. Every baseline stays readable, not just the
// active one — AC 7.3 makes that the point of having several.
export function baselineListHTML(baselines) {
  const list = baselines || [];
  if (!list.length) {
    return `<p class="hint">No baselines yet. Saving one freezes this order with the roster,
      initiatives and parameters that produced it, so it can be reproduced and compared against later.</p>`;
  }
  const rows = list.map((b) => `<tr${b.active ? ' class="bl-active"' : ''}>
    <td>${esc(b.name)}${b.active ? ' <span class="tag">active</span>' : ''}</td>
    <td>${esc(fmtWhen(b.createdAt))}</td>
    <td>${esc(b.createdBy || '—')}</td>
    <td>${b.diverged ? '<span class="ord-amber">inputs moved since</span>' : '<span class="hint">matches</span>'}</td>
    <td>
      ${b.active ? '' : `<button type="button" class="bl-activate" data-id="${esc(b.id)}">make active</button>`}
      <button type="button" class="bl-compare" data-id="${esc(b.id)}">compare</button>
    </td>
  </tr>`).join('');
  return `<table class="wip-table bl-table"><thead><tr>
      <th>Baseline</th><th>Saved</th><th>By</th><th>Against this plan</th><th></th>
    </tr></thead><tbody>${rows}</tbody></table>
    <p class="hint">Actuals and variance are measured against the active one. The others stay readable as history.</p>`;
}

// deltaCell renders a week movement. A sign is always shown for a non-zero value,
// because "3" beside "w17" is ambiguous about which way it went.
export function deltaCell(weeks) {
  if (!weeks) return '<span class="hint">—</span>';
  const cls = weeks > 0 ? 'ord-red' : 'ord-green'; // later is worse, earlier is better
  return `<span class="${cls}">${weeks > 0 ? '+' : '−'}${Math.abs(weeks)}w</span>`;
}

// compareTableHTML is AC 7.4: baseline start/commit against current, the delta in
// weeks, and additions and removals listed separately — neither has moved, so
// neither belongs in a column of movements.
export function compareTableHTML(result) {
  if (!result || !result.comparison) return '';
  const cmp = result.comparison;
  const rows = (cmp.initiatives || []).map((d) => `<tr>
    <td>${esc(d.name)}</td>
    <td>${weekLabel(d.baselineStartWeek)} → ${weekLabel(d.startWeek)}</td>
    <td>${deltaCell(d.startDeltaWeeks)}</td>
    <td>${weekLabel(d.baselineCommitWeek)} → ${weekLabel(d.commitWeek)}</td>
    <td>${deltaCell(d.commitDeltaWeeks)}</td>
    <td>${d.verdictChanged
    ? `<span class="hint">${esc(d.baselineVerdict)} →</span> <b>${esc(d.verdict)}</b>`
    : `<span class="hint">${esc(d.verdict)}</span>`}</td>
  </tr>`).join('');

  const name = result.baseline ? esc(result.baseline.name) : 'the baseline';
  const moved = cmp.moved || 0;
  const summary = moved
    ? `${moved} initiative${moved > 1 ? 's have' : ' has'} moved since <b>${name}</b>`
    : `Nothing has moved since <b>${name}</b>`;

  const listed = (label, names) => (names || []).length
    ? `<p class="hint">${label}: ${names.map((n) => esc(n)).join(' · ')}</p>` : '';

  return `<div class="card bl-compare-card">
    <div class="plan-summary">${summary}${result.diverged
    ? ' <span class="tag">inputs moved</span>' : ''}</div>
    ${rows ? `<table class="wip-table"><thead><tr>
      <th>Initiative</th><th>Start</th><th>Δ</th><th>Commit</th><th>Δ</th><th>Verdict</th>
    </tr></thead><tbody>${rows}</tbody></table>` : '<p class="hint">No initiatives in common with this baseline.</p>'}
    ${listed('Added since', cmp.added)}
    ${listed('Removed since', cmp.removed)}
  </div>`;
}

// baselinePanelHTML is the Order view's baseline section: §13.2's save control,
// the history, and whatever comparison is open.
export function baselinePanelHTML(baselines, compare) {
  return `<div class="card ord-card bl-panel">
    <div class="ord-head">
      <b>Baselines</b>
      <span class="hint">the agreed order for this period, frozen with the inputs that produced it</span>
    </div>
    ${baselineListHTML(baselines)}
    <div class="bl-save">
      <input id="bl-name" type="text" placeholder="name this order, e.g. v2 agreed 12 Jan" maxlength="80">
      <button type="button" id="bl-save" class="primary">Save as baseline</button>
    </div>
  </div>
  ${compareTableHTML(compare)}`;
}
