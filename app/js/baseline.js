// Baselines in the Plan UI: the agreed order for a period, frozen (spec 001
// Story 7, §13.1's always-visible chip and §13.2's save control).
//
// Pure functions from server payloads to HTML strings, like order.js — planui.js
// owns the fetching and the DOM. That is what lets tests/baseline.test.mjs cover
// the whole surface under `node --test`.

import { esc, weekLabel } from './order.js';
import { term } from './terms.js';

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
//
// It is a button, not a label. The panel it summarises lives in the Order view, so
// from the Network view an inert chip is a status with no way through to the thing
// it is talking about — which is how it shipped first, and it was useless.
export function baselineChipHTML(baselines) {
  const active = activeBaseline(baselines);
  if (!active) {
    return `<button type="button" class="bl-chip bl-empty" id="bl-chip"
      title="Nothing has been agreed for this period yet — open the Order view to save one">
      baseline: <span class="bl-dot bl-none">○</span> none <span class="hint">— save one ▸</span></button>`;
  }
  const diverged = !!active.diverged;
  const why = diverged
    ? "The plan's inputs have changed since this baseline was saved, so it no longer describes this plan"
    : 'This baseline matches the plan as it stands';
  return `<button type="button" class="bl-chip" id="bl-chip" title="${esc(why)} — open the baselines panel">
    agreed: <span class="bl-dot ${diverged ? 'bl-diverged' : 'bl-current'}">●</span>
    ${esc(active.name)}${diverged ? ' <span class="tag">inputs have moved</span>' : ' <span class="tag">matches</span>'} <span class="hint">▾</span></button>`;
}

// baselineListHTML is the history. Every baseline stays readable, not just the
// active one — AC 7.3 makes that the point of having several.
export function baselineListHTML(baselines) {
  const list = baselines || [];
  if (!list.length) {
    return `<p class="hint">No baselines yet. Saving one freezes this order with the roster,
      initiatives and parameters that produced it, so it can be reproduced and compared against later.</p>`;
  }
  // Pairwise compare (spec 005): offered only when there is another baseline
  // to point at — a select with no options is a control that does nothing.
  const vsSelect = (b) => {
    const others = list.filter((o) => o.id !== b.id);
    if (!others.length) return '';
    return `<select class="bl-vs-sel" data-from="${esc(b.id)}" title="compare this baseline against another saved one">
      <option value="">vs…</option>
      ${others.map((o) => `<option value="${esc(o.id)}">${esc(o.name)}</option>`).join('')}
    </select>`;
  };
  const rows = list.map((b) => `<tr${b.active ? ' class="bl-active"' : ''}>
    <td>${esc(b.name)}${b.active ? ' <span class="tag">active</span>' : ''}</td>
    <td>${esc(fmtWhen(b.createdAt))}</td>
    <td>${esc(b.createdBy || '—')}</td>
    <td>${b.diverged ? '<span class="ord-amber">inputs moved since</span>' : '<span class="hint">matches</span>'}</td>
    <td>
      ${b.active ? '' : `<button type="button" class="bl-activate" data-id="${esc(b.id)}">make active</button>`}
      <button type="button" class="bl-compare" data-id="${esc(b.id)}">compare</button>
      ${vsSelect(b)}
      <button type="button" class="bl-delete" data-id="${esc(b.id)}" title="delete this baseline">Delete</button>
    </td>
  </tr>`).join('');
  return `<table class="wip-table bl-table"><thead><tr>
      <th>Baseline${term('baseline')}</th><th>Saved</th><th>By</th><th>Against this plan</th><th></th>
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
  const toName = result.to ? esc(result.to.name) : '';
  const moved = cmp.moved || 0;
  // Pairwise results carry both ends (spec 005 AC 1.3): the reader has to know
  // which way the deltas read without reverse-engineering it from the rows.
  const summary = moved
    ? (toName
      ? `${moved} initiative${moved > 1 ? 's have' : ' has'} moved from <b>${name}</b> → <b>${toName}</b>`
      : `${moved} initiative${moved > 1 ? 's have' : ' has'} moved since <b>${name}</b>`)
    : (toName
      ? `Nothing moved between <b>${name}</b> and <b>${toName}</b>`
      : `Nothing has moved since <b>${name}</b>`);

  const listed = (label, names) => (names || []).length
    ? `<p class="hint">${label}: ${names.map((n) => esc(n)).join(' · ')}</p>` : '';

  return `<div class="panel-card bl-compare-card">
    <div class="plan-summary">${summary}${result.diverged
    ? ' <span class="tag">inputs moved</span>' : ''}</div>
    ${rows ? `<table class="wip-table"><thead><tr>
      <th>Initiative</th><th>Start</th><th>Δ</th><th>Commit</th><th>Δ</th><th>Verdict</th>
    </tr></thead><tbody>${rows}</tbody></table>` : '<p class="hint">No initiatives in common with this baseline.</p>'}
    ${listed('Added since', cmp.added)}
    ${listed('Removed since', cmp.removed)}
  </div>`;
}

// baselinesDrawerHTML is the slide-over that holds EVERYTHING baseline-related
// (spec 015): the save row, the history with per-baseline actions, and the
// pairwise comparison. It slides over the Order view, which stays visible —
// activation and comparison are decisions made about the order, so the order
// stays in sight. Draft=true blocks the save (FR-029): a baseline must freeze
// stored inputs, and the next list-read would call the freshly saved one
// diverged — a false alarm on the chip the reader just set.
export function baselinesDrawerHTML(baselines, compare, { draft = false } = {}) {
  const active = activeBaseline(baselines);
  const cta = active?.diverged
    ? '<p class="plan-warn bl-cta">The inputs have moved since this baseline was saved — save this changed plan as a new baseline to keep the old one comparable.</p>'
    : '';
  return `<aside class="bl-drawer" role="dialog" aria-modal="true" aria-label="Baselines">
    <div class="bl-drawer-head">
      <b>Baselines${term('baseline')}</b>
      <span class="hint">the agreed order for this period, frozen with the inputs that produced it</span>
      <button type="button" class="bl-drawer-close" title="close (ESC)">✕</button>
    </div>
    ${cta}
    <div class="bl-save">
      <input id="bl-drawer-name" type="text" placeholder="name this order, e.g. v2 agreed 12 Jan" maxlength="80" ${draft ? 'disabled' : ''} aria-label="baseline name">
      <button type="button" id="bl-save" class="primary" ${draft ? 'disabled' : ''}>Save current order</button>
      ${draft ? '<span class="plan-warn">Save the uploaded initiatives first — a baseline freezes what is stored, not the preview you are looking at.</span>' : ''}
    </div>
    ${baselineListHTML(baselines)}
    ${compareTableHTML(compare)}
  </aside>`;
}

// saveErrorMessage turns a failed baseline request into something a planner can
// act on. It lives here, beside the other pure functions, because the version that
// lived in planui.js pasted the server's response body straight into the page —
// untested, because that side of the line owns the DOM and not the wording.
//
// What that shipped as: a 405 whose body is the word "method", rendered next to
// the Save button, looking for all the world like a control labelled "Method".
// 405 on these routes has one dominant cause worth naming outright — app/js is
// served from disk with no-cache, so a checkout updates the page instantly while
// the compiled server keeps serving routes it was built with.
// Like every export here it returns HTML-safe output: the server's text is escaped
// at the point it is interpolated, so a caller cannot forget to.
export function saveErrorMessage(status, body, op) {
  const verb = op === 'compare' ? 'compare against that baseline' : 'save that';
  const detail = esc(String(body || '').trim());
  if (!status) return 'That did not reach the server — check it is still running.';
  if (status === 405) {
    // Keep the server's detail: it names the verb and path refused, which is what
    // separates "the restart did not take" from "the route is genuinely missing".
    // Dropping it once meant the next bug report arrived with no evidence in it.
    return 'The server binary is older than this page — rebuild and restart it, then reload this page. '
      + '(app/ is served from disk, so the page updated on checkout; routes are compiled in.)'
      + (detail ? ` The server said: ${detail}` : '');
  }
  if (status === 401 || status === 403) return 'You are not allowed to change this plan’s baselines.';
  if (status === 503) return 'The database is unavailable, so nothing can be saved right now.';
  if (status >= 500) return `The server could not ${verb}${detail ? ': ' + detail : ''}.`;
  // A 4xx body is written for a person — it is the useful part, so keep it.
  return detail ? `That was refused: ${detail}` : 'That was refused by the server.';
}

// latestOnly guards a view against a slower earlier request landing last.
//
// Each request claims a ticket before it goes out and checks it is still the
// current one before rendering. The plan-id check that guards these calls cannot
// do this job: choosing a second baseline pair while the first request is out
// leaves the plan unchanged, so the stale response passes that check and
// overwrites the newer card.
//
// This is the same idea as planui's orderEpoch, kept here as a pure factory
// because that is what makes it testable — planui.js is the fetch-and-DOM layer.
export function latestOnly() {
  let issued = 0;
  return {
    claim: () => ++issued,
    isCurrent: (ticket) => ticket === issued,
  };
}
