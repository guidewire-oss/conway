// remedyui.js — the Order view's priced-remedies expander (spec 001 §13.2,
// Story 5 / AC 5.1): pure functions returning HTML strings, exactly like
// baseline.js. The fetch and DOM side lives in planui.js.
//
// Upgrade tolerance is the design constraint here, in both directions:
// - app/js is served from disk with no-cache while routes are compiled into
//   the binary, so this page can be NEWER than its server: an unknown verdict
//   or kind from a future binary must render generically, never throw, and a
//   405 from a binary without the endpoint must explain itself.
// - and the page can be OLDER than its server: a remedy kind added later
//   (transfer-capacity, once Q1 lands) renders through the same generic path.

import { esc } from './order.js';

// The kinds this page has labels for. A kind outside this list is rendered
// with its raw name — a label map that threw on the unknown would make every
// server-side addition a page-side outage.
export const REMEDY_KINDS = [
  'raise-priority', 'descope', 'add-capacity', 'transfer-capacity',
  'relax-date', 'defer-other', 'unlock',
];

const KIND_LABELS = {
  'raise-priority': 'raise priority',
  'descope': 'descope',
  'add-capacity': 'add capacity',
  'transfer-capacity': 'transfer capacity',
  'relax-date': 'move the date',
  'defer-other': 'defer another initiative',
  'unlock': 'release the date lock',
};

export function remedyKindLabel(kind) {
  // Own-property only: a future kind named "constructor" or "__proto__" would
  // otherwise find the inherited property and render a function instead of the
  // raw fallback the upgrade contract promises.
  return Object.prototype.hasOwnProperty.call(KIND_LABELS, kind) ? KIND_LABELS[kind] : kind;
}

// verdictLabel maps the Go verdicts to a phrase; unknown verdicts pass through
// so a newer server's new verdict still reads as something.
function verdictLabel(v) {
  return ({
    'on-time': 'lands on time',
    'late': 'still late',
    'structurally-infeasible': 'still cannot fit',
    'no-date': 'no date',
  })[v] || v || '';
}

const signed = (n) => `${n < 0 ? '−' : '+'}${Math.abs(n)}`;

// remedyRowHTML is one priced option: what it does, what it lands, what it
// costs the portfolio, and who pays. Every field is optional except the kind
// and the verdict — the Go type says more, but a page that required the whole
// shape would break the day the server adds a field and an older page meets it.
export function remedyRowHTML(r) {
  const label = esc(remedyKindLabel(r.kind));
  const verdict = esc(verdictLabel(r.resultingVerdict));
  const stillLate = r.targetWeeksLate > 0
    ? ` <span class="hint">(${r.targetWeeksLate}w late)</span>` : '';
  const delta = typeof r.objectiveDelta === 'number'
    ? `<span class="${r.objectiveDelta <= 0 ? 'ord-green' : 'ord-red'}">${signed(r.objectiveDelta)}</span>` : '';
  const victims = (r.affectedInitiatives || []).map((v) =>
    `${esc(v.initiative)} ${signed(v.commitDeltaWeeks || v.startDeltaWeeks || 0)}w`).join(', ');
  const victimsLine = victims
    ? `<div class="hint">moves: ${victims}</div>`
    : '<div class="hint">nothing else moves</div>';
  // The target rides on the row even though the expander already names it:
  // a row lifted out of context (a future combined view) still says whose
  // date it rescues.
  const whose = r.target ? `<span class="hint">${esc(r.target)}:</span> ` : '';
  return `<div class="rem-row">
    ${whose}<b>${label}</b> ${r.note ? `<span class="hint">${esc(r.note)}</span>` : ''}
    → ${verdict}${stillLate}
    <span class="hint">objective ${delta}</span>
    ${victimsLine}
  </div>`;
}

// remediesPanelHTML is the expanded body under [options ▾]: the options the
// engine priced, cheapest first (the server ranks them), and the warnings —
// a deferred remedy kind is a gap the reader must see, not one to hide.
export function remediesPanelHTML(remedies, warnings) {
  const rows = (remedies || []).map(remedyRowHTML).join('');
  const list = rows || '<p class="hint">No options found — this date may need a later target or less scope.</p>';
  const warns = (warnings || []).map((w) => `<div class="hint">${esc(w)}</div>`).join('');
  return `<div class="rem-panel">
    ${rows ? '<div class="hint">cheapest first — the number is what it does to the whole portfolio</div>' : ''}
    ${list}
    ${warns}
  </div>`;
}

// optionsExpanderHTML is the [options ▾] control on a missed date's row
// (§13.2). Only a miss gets one: an on-time date has nothing to rescue, and
// no-date is not a miss. The name rides along as data so the wiring in
// planui.js never has to parse it back out of rendered markup.
export function optionsExpanderHTML(si) {
  if (si.verdict !== 'late' && si.verdict !== 'structurally-infeasible') return '';
  return ` <button type="button" class="ord-options" data-init="${esc(si.name)}">options ▾</button>`;
}

// remediesErrorMessage turns a failed remedies fetch into something a planner
// can act on. 405's one dominant cause is a server binary older than this
// page (app/ is served from disk; routes are compiled in) — the same seam the
// baselines panel hit once and rendered as a control labelled "Method".
export function remediesErrorMessage(status, body) {
  const detail = esc(String(body || '').trim());
  if (!status) return 'That did not reach the server — check it is still running.';
  if (status === 405) {
    return 'The server binary is older than this page — rebuild and restart it, then reload. '
      + (detail ? `The server said: ${detail}` : '');
  }
  if (status === 401 || status === 403) return 'You are not allowed to read this plan’s options.';
  if (status >= 500) return `The server could not price those options${detail ? ': ' + detail : ''}.`;
  return detail ? `That was refused: ${detail}` : 'That was refused by the server.';
}
