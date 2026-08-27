// Plan pillar UI: a manager uploads a teams roster + an initiatives matrix, and
// sees the directed cross-pod dependency network with per-pod utilization (ρ)
// and the constraint pods. Levers/what-if come in a later phase.
import { authFetch } from './auth.js';
import {
  heatColor, layoutColumns, bezierEdgePath, appendArrowMarker,
  enablePanZoom, enableNodeDrag, makeSpotlight,
} from './netgraph.js';
import { esc, orderViewHTML, schedulingFromForm, initiativeEditDialogHTML, initiativeEditFromBody, wipModelsTableHTML } from './order.js';
import { exportBlockPNG } from './exportpng.js';
import { attachDrag } from './drag.js';
import { fuzzyMatch } from './filter.js';
import { baselineChipHTML, baselinePanelHTML, saveErrorMessage, latestOnly } from './baseline.js';
import { remediesPanelHTML, remediesErrorMessage } from './remedyui.js';
import { portfolioTimelineHTML, podLensHTML, podSheetHTML } from './timeline.js';

let root, current = null;
// One comparison request at a time, keyed to what it is for. A bare boolean
// stranded the table: if the plan or the order moved while a request was out, the
// new dialog's request was skipped as "already pending" and the stale response was
// then discarded, leaving the container empty with nothing left to fill it.
let wipModelsPendingFor = null;

export function initPlanUI() {
  // The assumptions dialog's ESC + focus management: ONE handler for the app's
  // lifetime (renderOrder re-renders the dialog constantly; per-render
  // listeners would stack). ESC closes wherever focus sits; on close, focus
  // returns to the ⚙ that opened it — the same contract modal.js gives the
  // shared modals.
  document.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    // Fullscreen timeline exits first (spec 008): it is a view state, and
    // ESC is its advertised exit.
    const fs = document.getElementById('plan-dash');
    if (fs?.classList.contains('tl-fullscreen')) {
      fs.classList.remove('tl-fullscreen');
      const btn = document.getElementById('tl-fullscreen');
      if (btn) btn.textContent = '⛶ full screen';
      return;
    }
    const d = document.getElementById('sched-dialog');
    if (d && !d.hidden) { d.hidden = true; document.getElementById('sched-open')?.focus(); }
  });
  // Focus-in on open: the first field, so keyboard and screen-reader users
  // land inside the dialog that aria-modal just told them owns the page.
  // Delegated — the ⚙ button is re-rendered with every renderOrder.
  document.addEventListener('click', (ev) => {
    const open = ev.target.closest?.('#sched-open');
    if (!open) return;
    const d = document.getElementById('sched-dialog');
    if (d && !d.hidden) d.querySelector('input, select')?.focus();
  });
  root = document.getElementById('plan-root');
  if (!root) return;
  document.querySelector('.tab[data-view="plan"]')?.addEventListener('click', () => {
    if (!current) renderList();
  });
}

const fmtDate = (ts) => ts ? new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '—';

async function req(path, opts) { try { return await authFetch(path, opts); } catch { return null; } }

async function renderList() {
  current = null;
  root.innerHTML = '<p class="hint">Loading plans…</p>';
  const r = await req('/api/plan');
  if (!r || !r.ok) { root.innerHTML = '<p class="hint">Could not load plans (need the manager role).</p>'; return; }
  const plans = await r.json();
  root.innerHTML = `
    <div class="plan-head"><h2>Your plans</h2><button id="plan-new" class="primary">+ New plan</button><button id="plan-demo">Load demo plan</button></div>
    <p class="hint">Sample files to try the upload path: <a href="/api/sample/teams.csv" download>teams.csv</a> · <a href="/api/sample/initiatives.xlsx" download>initiatives.xlsx</a> (same data as the demo).</p>
    <table class="wip-table">
      <thead><tr><th>Name</th><th>Pods</th><th>Initiatives</th><th>Updated</th><th></th></tr></thead>
      <tbody>${(plans || []).map((p) => `<tr>
        <td><a class="plan-open" data-id="${p.id}">${esc(p.name)}</a></td>
        <td>${p.teamCount || 0}</td><td>${p.initiativeCount || 0}</td>
        <td>${fmtDate(p.updatedAt)}</td>
        <td><button class="plan-del" data-id="${p.id}">delete</button></td></tr>`).join('')
      || '<tr><td colspan="5" class="hint">No plans yet — create one to upload your teams &amp; initiatives.</td></tr>'}
      </tbody></table>`;
  root.querySelector('#plan-new').addEventListener('click', createPlan);
  root.querySelector('#plan-demo').addEventListener('click', async () => {
    const r = await req('/api/plan/demo', { method: 'POST' });
    if (!r || !r.ok) { alert('Could not create demo plan'); return; }
    openPlan((await r.json()).id);
  });
  root.querySelectorAll('.plan-open').forEach((a) => a.addEventListener('click', () => openPlan(a.dataset.id)));
  root.querySelectorAll('.plan-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm('Delete this plan?')) return;
    await req('/api/plan/' + b.dataset.id, { method: 'DELETE' });
    renderList();
  }));
}

async function createPlan() {
  const name = prompt('Plan name', 'New plan');
  if (name === null) return;
  const r = await req('/api/plan', { method: 'POST', body: JSON.stringify({ name }) });
  if (!r || !r.ok) { alert('Could not create plan'); return; }
  openPlan((await r.json()).id);
}

async function openPlan(id) {
  staleOrder(); // any order request still in flight belongs to the plan being left
  root.innerHTML = '<p class="hint">Loading plan…</p>';
  const r = await req('/api/plan/' + id);
  if (!r || !r.ok) { root.innerHTML = '<p class="hint">Could not load plan.</p>'; return; }
  current = await r.json();
  current.tlFilter = ''; // lens filters are per-plan view state (spec 010 FR-004)
  await loadBaselines(); // the header chip needs these before the first paint
  renderPlan();
}

function uploadField(kind, label, count) {
  return `<div class="plan-up">
    <label class="plan-upbtn">${label}<input type="file" accept=".csv,.xlsx" data-kind="${kind}" hidden></label>
    <span class="hint">${count ? `${count} loaded` : 'none yet'}</span>
  </div>`;
}

function renderPlan() {
  const p = current;
  const nTeams = (p.teams || []).length, nInit = (p.initiatives || []).length;
  const unknown = p.unknownTeams || [];
  root.innerHTML = `
    <div class="plan-head">
      <nav class="plan-crumbs" aria-label="You are here"><button type="button" class="plan-back">Plans</button><span class="hint">›</span><b>${esc(p.name)}</b></nav>
      <h2>${esc(p.name)}</h2>
      <label class="hint">horizon <input id="plan-horizon" type="number" min="1" max="104" value="${p.horizonWeeks}" style="width:56px">w</label>
      <label class="hint">capacity loss <input id="plan-loss" type="number" min="0" max="90" value="${Math.round((p.capacityLoss || 0) * 100)}" style="width:52px">%</label>
      <button id="plan-save">Save</button>
    </div>
    <details class="plan-setup"${(nTeams === 0 || nInit === 0) ? ' open' : ''}>
      <summary>Plan setup <span class="hint">${nTeams} pods · ${nInit} initiatives · ${(Math.round((p.capacityLoss || 0) * 100))}% capacity loss</span></summary>
      <div class="plan-uploads">
        <div class="plan-up" id="plan-roster-pick"></div>
        ${uploadField('initiatives', '⤓ Initiatives (XLSX/CSV)', nInit)}
        <label class="hint plan-up" title="Drop any dependency cell that doesn't match a roster pod name (case/whitespace-insensitive) — free text like &quot;Requirements unknown&quot; won't become a fake node in the network.">
          <input type="checkbox" id="plan-strict-deps" ${current.strictDeps ? 'checked' : ''}> strict: match dependencies to roster
        </label>
      </div>
      <p class="hint">Need samples? <a href="/api/sample/teams.csv" download>teams.csv</a> · <a href="/api/sample/initiatives.xlsx" download>initiatives.xlsx</a></p>
    </details>
    ${current.isDraft ? `<p class="plan-warn">✎ Previewing an unsaved initiatives upload — nothing is saved yet.
      <button id="plan-draft-save" class="primary">Save initiatives</button>
      <button id="plan-draft-discard">Discard</button></p>` : ''}
    ${unknown.length ? `<p class="plan-warn">⚠ ${unknown.length} pod(s) referenced by initiatives but missing from the roster: ${unknown.map(esc).join(', ')} — <button type="button" id="unknown-fix" class="warn-act">switch roster</button> or fix the sheet.</p>` : ''}
    ${nTeams > 0 && nInit > 0 ? `<div class="plan-views"><span class="seg">
      <button class="${view() === 'network' ? 'seg-on' : ''}" id="view-network">Network</button><button class="${view() === 'order' ? 'seg-on' : ''}" id="view-order">Order</button><button class="${view() === 'timeline' ? 'seg-on' : ''}" id="view-timeline">▦ Timeline</button>
    </span>${baselineChipHTML(current.baselines)}</div>` : ''}
    ${nTeams === 0 ? `
      <div class="card plan-start">
        <h3>A plan is a roster + the initiatives you intend to run, sequenced by capacity.</h3>
        <p class="hint">Four steps: attach a roster (team composition, pinned as of today) → upload the initiatives matrix →
          review the proposed order and its verdicts → save the agreed order as a baseline. Nothing here writes to Jira.</p>
        <div class="plan-start-row">
          <button type="button" id="plan-start-demo" class="primary">Start from the demo plan</button>
          <span class="hint">— or attach your own roster and upload your initiatives below, exactly as they are today.</span>
        </div>
      </div>` : ''
      }
    ${nTeams > 0 && nInit === 0 ? '<p class="hint">Roster loaded. Now upload the initiatives matrix.</p>' : ''}
    ${nTeams > 0 && nInit > 0 ? '<div id="plan-dash"></div>' : ''}`;

  root.querySelector('.plan-back').addEventListener('click', renderList);
  // The empty-state's demo button (IA #5): the fastest honest path to seeing
  // what a plan does — same handler as the list's "Load demo plan". Wired here,
  // not in renderOrder: the empty state never renders the Order view.
  document.getElementById('plan-start-demo')?.addEventListener('click', async () => {
    const r = await req('/api/plan/demo', { method: 'POST' });
    if (!r || !r.ok) { alert('Could not create the demo plan'); return; }
    openPlan((await r.json()).id);
  });
  root.querySelector('#plan-save').addEventListener('click', savePlanParams);
  // The missing-pod warning's fix (spec 009 AC 3.2): open setup at the roster.
  document.getElementById('unknown-fix')?.addEventListener('click', () => {
    const d = document.querySelector('.plan-setup');
    if (d) d.open = true;
    document.getElementById('plan-roster-pick')?.querySelector('select, button')?.focus();
  });
  root.querySelectorAll('input[type=file]').forEach((inp) => inp.addEventListener('change', () => {
    if (!inp.files[0]) return;
    if (inp.dataset.kind === 'initiatives') previewInitiativesFile(inp.files[0]);
    else uploadFile(inp.dataset.kind, inp.files[0]);
  }));
  document.getElementById('plan-draft-save')?.addEventListener('click', saveDraftInitiatives);
  document.getElementById('plan-draft-discard')?.addEventListener('click', () => openPlan(current.id));
  document.getElementById('plan-strict-deps')?.addEventListener('change', (e) => { current.strictDeps = e.target.checked; });
  document.getElementById('view-network')?.addEventListener('click', () => setView('network'));
  document.getElementById('view-order')?.addEventListener('click', () => setView('order'));
  document.getElementById('view-timeline')?.addEventListener('click', () => setView('timeline'));
  // The chip summarises a panel that only exists in the Order view, so it has to be
  // able to get there — otherwise it is a status message with no way through. The
  // scroll happens in renderOrder once the panel actually exists: with a stale
  // cached order the async path returns early and there is nothing to scroll to yet.
  document.getElementById('bl-chip')?.addEventListener('click', () => {
    if (view() !== 'order') { current.baselineFocus = true; setView('order'); return; }
    document.querySelector('.bl-panel')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    document.getElementById('bl-name')?.focus();
  });
  renderRosterPicker(nTeams);
  if (nTeams > 0 && nInit > 0) {
    current.levers = current.levers || [];
    current.netMode = current.netMode || 'after';
    if (view() === 'order') renderOrder(); else if (view() === 'timeline') renderTimeline(); else renderDash();
  }
}

const view = () => ['order', 'timeline'].includes(current && current.view) ? current.view : 'network';

// loadWipModels re-asks for the order with the per-model comparison attached, for
// the assumptions dialog. A failure is quiet on purpose: the dialog's job is editing
// the assumptions, and it stays usable without the comparison table.
async function loadWipModels() {
  const forPlan = current.id;
  const atEpoch = orderEpoch;
  const key = forPlan + '|' + atEpoch;
  if (wipModelsPendingFor === key) return; // this exact request is already out
  wipModelsPendingFor = key;
  let payload = null;
  try {
    const r = await req('/api/plan/' + forPlan + '/schedule', {
      method: 'POST', body: JSON.stringify({ ...orderRequestBody(), wipModels: true }),
    });
    if (!r || !r.ok) return;
    try { payload = await r.json(); } catch { return; }
  } finally {
    // Only if it is still ours: a newer request for a different plan or epoch owns
    // the gate now, and clearing it unconditionally would let a third request in.
    if (wipModelsPendingFor === key) wipModelsPendingFor = null;
  }
  if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return; // a stale answer
  current.schedule = payload;
  // Fill the table in place. Deliberately NOT renderOrder(): rebuilding the view
  // around an open dialog threw away three things the planner owns -- assumptions
  // they had typed but not saved, the view they had switched to, and the dialog
  // they had closed. A request nobody is waiting on any more must not reach in and
  // move the page.
  const slot = document.getElementById('wip-models');
  if (slot) slot.innerHTML = wipModelsTableHTML(payload);
}

// saveScheduling stores the plan-level assumptions and recomputes the order. This
// is the only way to give a plan a period start, without which target dates cannot
// become weeks and every initiative reads as "no date".
async function saveScheduling() {
  const btn = document.getElementById('sched-save');
  const body = schedulingFromForm((id) => document.getElementById(id)?.value);
  // The accepted-ordering marker is not a form field; carry it or every
  // assumptions save silently returns the plan to the stated order (cubic:
  // the marker must survive an unrelated save).
  if (current.scheduling?.acceptedOrdering === 'engine') {
    body.acceptedOrdering = 'engine';
    body.acceptedOrderingAt = current.scheduling.acceptedOrderingAt;
  }
  // The setup-card dismissal rides the same blob (spec 009): an unrelated
  // assumptions save must not resurrect the card.
  if (current.scheduling?.setupAcknowledged) body.setupAcknowledged = true;
  // Same guard as renderOrder, and it matters more here: this response is written
  // into current.scheduling, so a late answer would not just display the wrong
  // assumptions, it would be the ones the next save sends.
  const forPlan = current.id;
  const atEpoch = orderEpoch;
  if (btn) { btn.disabled = true; btn.textContent = 'Saving…'; } // this render's button
  const r = await req('/api/plan/' + forPlan + '/scheduling', {
    method: 'PATCH', body: JSON.stringify(body),
  });
  if (!current || current.id !== forPlan) return; // the reader moved on; not their error
  if (!r || !r.ok) {
    const why = r ? await r.text() : 'the request did not reach the server';
    // Re-query rather than reusing the captured button. A reload of the same plan
    // re-renders the form, which detaches the node this closure holds — writing the
    // failure onto it would put the message nowhere and the save would look fine.
    const live = document.getElementById('sched-save');
    if (live) { live.disabled = false; live.textContent = 'Save assumptions'; }
    document.getElementById('sched-error')?.remove();
    live?.insertAdjacentHTML('afterend',
      `<span class="plan-warn" id="sched-error">${esc(why)}</span>`);
    return;
  }
  const saved = await r.json();
  if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return; // a stale save
  current.scheduling = saved.scheduling || body;
  current.calDraft = null; // the draft is saved now; the form reads the policy
  // The carried assumptions are saved too: leaving them would make the next
  // render restore the pre-save snapshot over the fresh policy.
  current.assumptionDraft = null;
  staleOrder(); // the assumptions moved, so the order has to be recomputed
  renderOrder();
}

// loadBaselines refreshes the list, which also carries whether the plan's inputs
// have moved since each was saved (FR-030). Cheap: metadata only, no frozen blobs.
async function loadBaselines() {
  const forPlan = current.id;
  const r = await req('/api/plan/' + forPlan + '/baseline');
  if (!r || !r.ok) return;
  const body = await r.json();
  if (!current || current.id !== forPlan) return; // an answer to a stale question
  current.baselines = body.baselines || [];
}

function wireBaselineControls() {
  document.getElementById('bl-save')?.addEventListener('click', () => saveBaseline('bl-name'));
  document.getElementById('bl-save-head')?.addEventListener('click', () => saveBaseline('bl-name-head'));
  document.querySelectorAll('.bl-activate').forEach((b) =>
    b.addEventListener('click', () => activateBaseline(b.dataset.id)));
  document.querySelectorAll('.bl-compare').forEach((b) =>
    b.addEventListener('click', () => {
      // Toggle: comparing the already-compared baseline dismisses the card —
      // a second click that does nothing reads as broken (button audit, 2026-08-23).
      if (current.baselineCompare && current.baselineCompare.baseline?.id === b.dataset.id) {
        current.baselineCompare = null;
        renderOrder();
        return;
      }
      compareBaseline(b.dataset.id);
    }));
  // Pairwise baseline compare (spec 005): both schedules are stored, so this never
  // touches the live plan and needs no orderEpoch guard. It does need the compare
  // gate: choose a second pair while the first request is out and the slower reply
  // would otherwise land last and overwrite the newer card.
  document.querySelectorAll('.bl-vs-sel').forEach((sel) =>
    sel.addEventListener('change', async () => {
      const other = sel.value;
      sel.value = ''; // a one-shot trigger, not a persistent selection
      if (!other) return;
      const forPlan = current.id;
      const ticket = compareGate.claim();
      const r = await req('/api/plan/' + forPlan + '/baseline/' + sel.dataset.from + '/compare-to/' + other, {
        method: 'POST', body: '{}',
      });
      const mine = () => !!current && current.id === forPlan && compareGate.isCurrent(ticket);
      if (!mine()) return;
      if (!r || !r.ok) { await baselineError(r, 'compare', mine); return; }
      const res = await r.json();
      // Parsing suspended too, so the gate is checked again before the write:
      // passing it above only proved this was current a moment ago.
      if (!mine()) return;
      // The card reads result.baseline for the "from" end; the pairwise
      // endpoint returns `from`. Same object, the view's name for it.
      if (res && res.from && !res.baseline) res.baseline = res.from;
      current.baselineCompare = res;
      renderOrder();
    }));
}

// Both compare paths render into current.baselineCompare, so they share one gate:
// whichever request was issued last is the only one allowed to paint. The plan-id
// and orderEpoch checks cannot cover this — picking a second pair changes neither.
const compareGate = latestOnly();

// orderRequestBody is what /schedule was given, so a baseline or a comparison uses
// the same inputs as the order on screen rather than re-deriving them differently.
function orderRequestBody() {
  const body = {};
  if (current.isDraft) body.initiatives = current.initiatives;
  if ((current.levers || []).length) body.levers = current.levers;
  return body;
}

// baselineError renders a failed request. It takes the response rather than a
// string so that translating a status into readable copy is not something a call
// site can skip — echoing the body is how a 405 once reached the page as "method".
// op names the operation so a compare failure does not wear the word "save".
async function baselineError(r, op, stillWanted) {
  // Reading the body suspends, so a newer request can be issued in between. The
  // predicate is checked after the read, immediately before painting: an error
  // from a superseded request is as wrong to show as its result would be.
  const msg = saveErrorMessage(r ? r.status : 0, r ? await r.text() : '', op);
  if (stillWanted && !stillWanted()) return;
  baselineNote(msg);
}

// baselineNote paints one message beside the save control, replacing any previous
// one. Callers pass HTML-safe text: saveErrorMessage escapes what came from the
// server, and the local validation messages are literals.
function baselineNote(msg) {
  document.getElementById('bl-error')?.remove();
  document.querySelector('.bl-save')?.insertAdjacentHTML('beforeend',
    `<span class="plan-warn" id="bl-error">${msg}</span>`);
}

// saveBaseline freezes the order currently on screen, under a name. The body
// carries the same draft initiatives and levers the order was computed from, so a
// baseline records what the planner was actually looking at.
async function saveBaseline(from = 'bl-name') {
  // A draft previews an unsaved sheet; a baseline freezes what is STORED —
  // saving one from a draft would freeze the stale copy (cubic P1). Both
  // entries (header and panel) land here, so the gate covers both.
  if (current.isDraft) {
    baselineNote('Save the uploaded initiatives first — a baseline freezes what is stored, not the preview.');
    return;
  }
  // from is the input id that triggered the save (spec 009 FR-002: the Order
  // header carries its own entry; both land in the same handler and state).
  const input = document.getElementById(from);
  const name = (input?.value || '').trim();
  if (!name) {
    input?.focus();
    baselineNote('Give the baseline a name \u2014 it is how this period\u2019s agreed order is referred to later.');
    return;
  }
  const btn = document.getElementById(from === 'bl-name' ? 'bl-save' : 'bl-save-head');
  if (btn) { btn.disabled = true; btn.textContent = 'Saving\u2026'; }
  const forPlan = current.id;
  const r = await req('/api/plan/' + forPlan + '/baseline', {
    method: 'POST', body: JSON.stringify({ name, ...orderRequestBody() }),
  });
  if (!current || current.id !== forPlan) return;
  if (!r || !r.ok) {
    await baselineError(r);
    // Restore whichever button was pressed (header or panel entry).
    const live = document.getElementById(btn?.id || 'bl-save');
    if (live) { live.disabled = false; live.textContent = live.id === 'bl-save-head' ? '✓ Save baseline' : 'Save as baseline'; }
    return;
  }
  await loadBaselines();
  current.baselineCompare = null;
  renderPlan(); // the header chip changes too, so repaint the plan rather than the panel
}

async function activateBaseline(id) {
  const forPlan = current.id;
  const r = await req('/api/plan/' + forPlan + '/baseline/' + id, {
    method: 'PATCH', body: JSON.stringify({ active: true }),
  });
  if (!current || current.id !== forPlan) return;
  if (!r || !r.ok) { await baselineError(r); return; }
  await loadBaselines();
  renderPlan();
}

// compareBaseline measures the order on screen against a saved one (AC 7.4).
async function compareBaseline(id) {
  const forPlan = current.id;
  const atEpoch = orderEpoch; // a lever or upload can land while this request is out
  const ticket = compareGate.claim(); // and a newer compare can be asked for
  const r = await req('/api/plan/' + forPlan + '/baseline/' + id + '/compare', {
    method: 'POST', body: JSON.stringify(orderRequestBody()),
  });
  const mine = () => !!current && current.id === forPlan
    && orderEpoch === atEpoch && compareGate.isCurrent(ticket);
  if (!mine()) return; // answers the plan and the order it left, and is not superseded
  if (!r || !r.ok) { await baselineError(r, 'compare', mine); return; }
  const res = await r.json(); // parse first, then re-check: awaiting is a gap
  if (!mine()) return;
  current.baselineCompare = res;
  renderOrder();
}

// toggleRemedies expands or collapses one initiative's priced options
// (§13.2's [options ▾], AC 5.1). The panel is inserted under the row rather
// than cached on `current`: remedies are per-click, stateless server-side
// (FR-022), and a cached panel is one more thing to invalidate when the order
// moves — the orderEpoch check already refuses to render into a reordered
// table, and re-fetching on every expand is the honest version of "priced
// against what you are looking at".
async function toggleRemedies(btn) {
  const name = btn.dataset.init;
  const row = btn.closest('tr');
  const open = row?.nextElementSibling;
  if (open?.classList?.contains('ord-remedies')) {
    open.remove(); // collapse
    btn.textContent = 'options ▾';
    return;
  }
  document.querySelectorAll('tr.ord-remedies').forEach((el) => el.remove());
  document.querySelectorAll('.ord-options').forEach((b) => { b.textContent = 'options ▾'; });
  btn.textContent = 'options ▴';
  btn.disabled = true;
  const forPlan = current.id;
  const atEpoch = orderEpoch; // the order can move while the price is being computed
  const body = document.createElement('td');
  body.colSpan = 8;
  const holder = document.createElement('tr');
  holder.className = 'ord-remedies';
  holder.appendChild(body);
  row.after(holder);
  body.innerHTML = '<span class="hint">pricing options…</span>';

  const r = await req('/api/plan/' + forPlan + '/schedule/remedies', {
    method: 'POST', body: JSON.stringify({ ...orderRequestBody(), targets: [name] }),
  });
  if (!current || current.id !== forPlan || orderEpoch !== atEpoch) {
    holder.remove(); // the order moved: the row this belonged to is gone
    return;
  }
  // A redraw without an input change (opening a pod queue) does not bump the
  // epoch but still replaces the table — the holder can be detached while the
  // plan and epoch checks pass. Moving the existing holder under the live
  // expander re-parents it (no new elements, nothing to reassign), so the
  // priced answer reaches the reader who asked for it.
  const live = document.querySelector(`.ord-options[data-init="${CSS.escape(name)}"]`);
  if (!holder.isConnected) {
    if (!live) return; // the redraw removed the miss: nothing to attach to
    live.closest('tr').after(holder);
    live.textContent = 'options ▴';
  }
  if (live) live.disabled = false;
  if (!r || !r.ok) {
    body.innerHTML = `<span class="plan-warn">${remediesErrorMessage(r ? r.status : 0, r ? await r.text() : '')}</span>`;
    return;
  }
  const out = await r.json();
  body.innerHTML = remediesPanelHTML(out.remedies, out.warnings);
}

// staleOrder drops the cached execution order. Anything that changes the inputs —
// levers, the roster, the sheet — has to call it, or the Order view keeps showing
// an order computed from a plan that no longer exists, which is worse than a spinner.
//
// It also bumps orderEpoch, which is what makes a slow response harmless: a request
// issued before the inputs moved can still land afterwards, and without the epoch it
// would write an order for a plan nobody is looking at any more.
let orderEpoch = 0;

function staleOrder() {
  orderEpoch += 1;
  if (current) current.schedule = null;
}

function setView(v) {
  if (!current || view() === v) return;
  current.view = v;
  renderPlan();
}

// ASSUMPTION_FIELDS are the scheduling form's inputs. carryLiveAssumptions
// snapshots them so an add/del re-render cannot drop a planner's edits —
// including a cleared field, which is an edit, not a reversion.
const ASSUMPTION_FIELDS = ['sched-period-start', 'sched-wip-model', 'sched-wip', 'sched-buffer',
  'sched-kit', 'sched-pod-wip', 'sched-quarter', 'sched-estimate-model', 'sched-split-tax',
  'sched-chunking', 'sched-split-min', 'sched-stagger'];

// Hoisted function declarations, not consts: renderOrder calls
// applyLiveAssumptions mid-body, and a const there would still be in its
// temporal dead zone on the first render.
function carryLiveAssumptions() {
  if (!current) return;
  const live = {};
  for (const id of ASSUMPTION_FIELDS) {
    const el = document.getElementById(id);
    if (el) live[id] = el.value;
  }
  current.assumptionDraft = live;
}

function applyLiveAssumptions() {
  if (!current || !current.assumptionDraft) return;
  for (const [id, v] of Object.entries(current.assumptionDraft)) {
    const el = document.getElementById(id);
    if (el) el.value = v;
  }
  // The chunk-size input's disabled state depends on the mode select, which
  // the loop above may have just restored (cubic P2) — re-derive it.
  const mode = document.getElementById('sched-chunking');
  const num = document.getElementById('sched-split-min');
  if (mode && num) num.disabled = mode.value !== 'chunk';
}

// renderTimeline paints Stories 8-9's views (§13.3-§13.5). It shares the order
// cache: the timeline IS the same schedule seen as spans, so a second fetch
// would be a second answer to one question. Same epoch discipline as
// renderOrder — an in-flight order can land after the view switched.
async function renderTimeline() {
  const host = document.getElementById('plan-dash');
  if (!host) return;
  if (!current.schedule) {
    host.innerHTML = '<p class="hint">Working out the order…</p>';
    const forPlan = current.id;
    const atEpoch = orderEpoch;
    const body = {};
    if (current.isDraft) body.initiatives = current.initiatives;
    if ((current.levers || []).length) body.levers = current.levers;
    const r = await req('/api/plan/' + current.id + '/schedule', {
      method: 'POST', body: JSON.stringify(body),
    });
    let payload = null;
    if (r && r.ok) { try { payload = await r.json(); } catch { /* handled below */ } }
    if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return;
    if (!payload) {
      host.innerHTML = '<p class="plan-warn">Could not compute the schedule the timeline draws.</p>';
      return;
    }
    current.schedule = payload;
  }
  const sched = current.schedule;
  const horizon = Math.max(1, Math.ceil(current.horizonWeeks || sched.horizonWeeks || 26));
  // AC 8.5: today, positioned by date — but only when it is genuinely inside
  // the period. A today clamped to the last week would claim the period is
  // further along than it is, and a missing period start means no position.
  let todayWeek;
  if (sched.periodStart) {
    // Date-only, so the period's final day still counts as inside: comparing
    // wall-clock against period-start midnight would suppress the marker for
    // the whole of that last day.
    const now = new Date();
    const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
    const start = new Date(`${sched.periodStart.trim()}T00:00:00Z`).getTime();
    const end = start + horizon * 7 * 86400000;
    if (today >= start && today <= end) {
      todayWeek = Math.floor((today - start) / (7 * 86400000));
    }
  }

  const lens = current.tlLens || 'initiative';
  // The view span: the period by default; wider spans exist because plans
  // overrun and the horizon cut made everything past it invisible (the user
  // could not scroll right). 'all' fits the widest commit week.
  const widest = (sched.initiatives || []).reduce((m, si) => Math.max(m, si.commitWeek || 0), horizon);
  const spans = [
    { id: 'period', label: `${horizon}w period`, weeks: horizon },
    { id: 'double', label: `${horizon * 2}w`, weeks: horizon * 2 },
    { id: 'all', label: `all (${widest}w)`, weeks: widest },
  ].filter((sp) => sp.weeks > horizon || sp.id === 'period');
  const spanSel = current.tlSpan || 'period';
  const spanWeeks = (spans.find((sp) => sp.id === spanSel) || spans[0]).weeks;
  const lensBtn = (id, on, label) =>
    `<button class="${on ? 'seg-on' : ''}" id="${id}">${label}</button>`;
  host.innerHTML = `
    <div class="plan-views"><span class="seg">
      ${lensBtn('tl-by-initiative', lens === 'initiative', '◉ by initiative')}
      ${lensBtn('tl-by-pod', lens === 'pod', '○ by pod')}
    </span>
    <span class="seg" title="how much of the schedule to draw — the period end is marked when the view is wider">
      ${spans.map((sp) => `<button class="${sp.id === spanSel ? 'seg-on' : ''}" data-tlspan="${sp.id}">${sp.label}</button>`).join('')}
    </span>
    <span class="seg">
      <button id="tl-fullscreen" title="fullscreen the timeline for lane-accurate dragging (ESC to exit)">⛶ full screen</button>
    </span>
    <span class="tl-filter" id="tl-filter-box">
      <input id="tl-filter" type="search" placeholder="${lens === 'pod' ? 'filter by initiative…' : 'filter by pod…'}"
        value="${esc(current.tlFilter || '')}" aria-label="${lens === 'pod' ? 'filter initiatives' : 'filter pods'}">
      <span class="hint" id="tl-filter-count"></span>
    </span></div>
    <p class="hint" id="tl-drag-note" hidden></p>
    <div id="tl-main"></div>
    <div id="tl-pod"></div>`;
  host.querySelectorAll('[data-tlspan]').forEach((b) =>
    b.addEventListener('click', () => { current.tlSpan = b.dataset.tlspan; renderTimeline(); }));
  // Fullscreen (spec 008): lane-accurate dragging needs the real estate. ESC
  // exits — the stable document-level keydown lives in initPlanUI so the
  // re-render never stacks handlers.
  // Lens filters (spec 010): view state, debounced re-render, live counts.
  {
    const input = document.getElementById('tl-filter');
    input?.addEventListener('input', () => {
      clearTimeout(renderTimeline._filterT);
      renderTimeline._filterT = setTimeout(() => {
        current.tlFilter = input.value;
        renderTimeline();
      }, 120);
    });
  }
  document.getElementById('tl-fullscreen')?.addEventListener('click', () => {
    host.classList.toggle('tl-fullscreen');
    const btn = document.getElementById('tl-fullscreen');
    if (btn) btn.textContent = host.classList.contains('tl-fullscreen') ? '⛶ exit full screen (esc)' : '⛶ full screen';
  });

  // pinnedLanesByPod inverts the stored per-initiative PinnedLanes into the
  // per-pod map assignLanes consumes.
  const pinnedLanesByPod = () => {
    const out = {};
    for (const it of (current.initiatives || [])) {
      for (const [pod, off] of Object.entries(it.pinnedLanes || {})) {
        (out[pod] ||= {})[it.name] = off;
      }
    }
    return out;
  };
  // dragNote paints the drag outcome line above the timeline (success is
  // silence; an overlap refusal names the conflict).
  function dragNote(msg) {
    const el = document.getElementById('tl-drag-note');
    if (!el) return;
    el.textContent = msg || '';
    el.hidden = !msg;
    el.classList.toggle('plan-warn', !!msg);
  }
  const paint = () => {
    const main = document.getElementById('tl-main');
    if (!main) return;
    main.innerHTML = lens === 'pod'
      ? podLensHTML(sched, { horizonWeeks: horizon, span: spanWeeks, pinnedLanes: pinnedLanesByPod(), initiativeQuery: current.tlFilter || '' })
      : portfolioTimelineHTML(sched, {
        podQuery: current.tlFilter || '',
        horizonWeeks: horizon, span: spanWeeks, todayWeek, expand: current.tlExpand,
        // AC 8.5: the bands come off the saved policy, not the schedule — the
        // schedule itself only carries the windows' effects, not their dates.
        calendars: (current.scheduling || {}).calendars || [],
      });
    // AC 8.4: expanding a row shows its pod slices. One open row at a time, so
    // the lens stays readable — the wireframe is one expanded initiative.
    main.querySelectorAll('.tl-row[data-expandable="1"] .tl-label').forEach((el) =>
      el.addEventListener('click', () => {
        const name = el.closest('.tl-row').dataset.init;
        current.tlExpand = current.tlExpand === name ? null : name;
        paint();
      }));
    // The pod lens's pod blocks open §13.5's sheet (AC 9.1 -> AC 9.5);
    // clicking the open pod again closes it, so the grid is never stuck.
    main.querySelectorAll('.tl-pod[data-pod]').forEach((el) =>
      el.addEventListener('click', () => {
        if (current.tlPod === el.dataset.pod) { current.tlPod = null; const h = document.getElementById('tl-pod'); if (h) h.innerHTML = ''; return; }
        current.tlPod = el.dataset.pod;
        paintPodSheet(el.dataset.pod);
      }));
    // Filter match count (spec 010 FR-005).
    const countEl = document.getElementById('tl-filter-count');
    if (countEl) {
      const q = current.tlFilter || '';
      if (!q) countEl.textContent = '';
      else if (lens === 'pod') {
        const n = new Set((sched.initiatives || []).filter((si) => fuzzyMatch(q, si.name)).map((si) => si.name)).size;
        countEl.textContent = `${n} of ${(sched.initiatives || []).length} initiatives`;
      } else {
        const pods = new Set((sched.podWeeks || []).map((ps) => ps.pod));
        const n = [...pods].filter((pd) => fuzzyMatch(q, pd)).length;
        countEl.textContent = `${n} of ${pods.size} pods`;
      }
    }
    // Spec 008: drag-to-edit. A released drag pins the slice's start and the
    // engine recomputes; the re-render repaints every view from one schedule.
    main.dataset.horizon = String(spanWeeks);
    attachDrag(main, {
      readOnly: current.isDraft, // matchMedia('.pointer: coarse)') gate lives inside attachDrag

      horizon: spanWeeks,
      onPin: async (initiative, pod, { startWeek, laneDelta }) => {
        const forPlan = current.id;
        const it = (current.initiatives || []).find((i) => i.name === initiative);
        if (!it) return;
        const edit = { name: initiative };
        if (startWeek !== null && startWeek !== undefined) {
          edit.pinnedStarts = { ...(it.pinnedStarts || {}), [pod]: startWeek };
        }
        if (laneDelta) {
          // The new pod-relative offset: current packed lane + delta, floored
          // at 0. The server refuses drops that overlap other work (409).
          const bar = [...document.querySelectorAll(`.tl-bar[data-initiative="${CSS.escape(initiative)}"][data-pod="${CSS.escape(pod)}"]`)][0];
          const curLane = Number(bar?.dataset.lane || 0);
          const offset = Math.max(0, curLane + laneDelta);
          edit.pinnedLanes = { ...(it.pinnedLanes || {}), [pod]: offset };
        }
        const atEpoch = orderEpoch; // captured before the PATCH (cubic P1)
        const r = await req('/api/plan/' + forPlan + '/initiatives', {
          method: 'PATCH',
          body: JSON.stringify({ initiatives: [edit] }),
        });
        if (!current || current.id !== forPlan) return;
        if (!r || !r.ok) {
          // The overlap refusal (spec 008 Decision 3) surfaces as the
          // timeline's own note — the chart is the context for the error.
          const why = r ? await r.text() : 'the request did not reach the server';
          if (current && current.id === forPlan) dragNote(why.slice(0, 200));
          return;
        }
        if (orderEpoch !== atEpoch) return; // a recompute landed while the PATCH was away
        dragNote('');
        try {
          const d = await r.json();
          if (Array.isArray(d.initiatives)) current.initiatives = d.initiatives;
        } catch { /* cache stays; the server is authoritative */ }
        current.schedule = null;
        // Re-render the CURRENT view, not necessarily Timeline (cubic P2).
        if (view() === 'order') await renderOrder(); else if (view() === 'timeline') await renderTimeline(); else await renderDash();
      },
    });
    // FR-043 (spec 004 Story 3): each pod block exports itself as a PNG. The
    // click must not also open the sheet, so it stops here.
    main.querySelectorAll('.pod-export[data-export-pod]').forEach((b) =>
      b.addEventListener('click', (ev) => {
        ev.stopPropagation();
        const pod = b.dataset.exportPod;
        exportBlockPNG(b.closest('.tl-pod'), `conway-${pod.replace(/\W+/g, '-').toLowerCase()}-timeline.png`);
      }));
  };
  // The pod toggle (open/close) and the lens-switch redraw share ONE renderer —
  // duplicating the markup without the wiring left the redrawn sheet's PNG
  // button inert.
  const paintPodSheet = (pod) => {
    const holder = document.getElementById('tl-pod');
    if (!holder) return;
    const ps = (sched.podWeeks || []).find((p) => p.pod === pod);
    holder.innerHTML = ps ? podSheetHTML(ps, sched, { horizonWeeks: horizon, span: spanWeeks }) : '';
    holder.querySelectorAll('.pod-export[data-export-sheet]').forEach((b) =>
      b.addEventListener('click', () => {
        exportBlockPNG(b.closest('[data-pod-sheet]'), `conway-${pod.replace(/\W+/g, '-').toLowerCase()}-sheet.png`);
      }));
  };
  paint();
  // A pod sheet open from before a lens switch stays open: render it directly
  // rather than through the toggle, which would read the selection as a
  // second click and clear it.
  if (current.tlPod) paintPodSheet(current.tlPod);

  // Lens switches clear the filter (spec 010 AC 2.2 as amended): the query
  // means a different thing in each lens, and carrying 'devspace' into the
  // pod filter matches nothing — surprising beats persistent here.
  document.getElementById('tl-by-initiative')?.addEventListener('click', () => { current.tlLens = 'initiative'; current.tlFilter = ''; renderTimeline(); });
  document.getElementById('tl-by-pod')?.addEventListener('click', () => { current.tlLens = 'pod'; current.tlFilter = ''; renderTimeline(); });
}

// renderOrder computes the execution order and paints §13.2's table plus the
// per-pod load grid. Stateless on the server side: nothing is saved by looking.
async function renderOrder() {
  const host = document.getElementById('plan-dash');
  if (!host) return;
  if (!current.schedule) {
    host.innerHTML = '<p class="hint">Working out the order…</p>';
    // What this request is for. Checked again on arrival, because between issuing it
    // and it landing the reader may have switched plans, applied a lever or loaded a
    // draft — and answering the wrong question confidently is the worst outcome here.
    const forPlan = current.id;
    const atEpoch = orderEpoch;
    const body = {};
    // Draft preview mode: order the unsaved sheet, not the stale saved one — the
    // same rule simulate already follows.
    if (current.isDraft) body.initiatives = current.initiatives;
    // Send the levers too. The Network view already shows "with levers", and an
    // Order view that quietly ignored them would describe a different plan.
    if ((current.levers || []).length) body.levers = current.levers;
    const r = await req('/api/plan/' + current.id + '/schedule', {
      method: 'POST', body: JSON.stringify(body),
    });
    // Drain the response before the staleness check, so the body is read exactly
    // once whichever way this goes.
    let payload = null;
    let why = 'the request did not reach the server';
    if (r && r.ok) {
      try { payload = await r.json(); } catch { why = 'the server sent something this cannot read'; }
    } else if (r) {
      why = await r.text();
    }
    if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return; // an answer to a stale question
    if (!payload) {
      host.innerHTML = `<p class="plan-warn">Could not compute the execution order: ${esc(why)}</p>`;
      return;
    }
    current.schedule = payload;
  }
  // The form's calendar rows come from the draft when one exists (an added or
  // deleted row is a planner mid-edit, not a policy change), else the saved
  // policy. Always an object, never undefined: passing undefined is how the
  // view is told to omit the form entirely, and on a real plan it must be offered.
  const schedOpts = current.scheduling || {};
  const schedForForm = current.calDraft
    ? { ...schedOpts, calendars: current.calDraft }
    : schedOpts;
  host.innerHTML = orderViewHTML(current.schedule, {
    noPin: current.isDraft, // nothing is saved to pin against on a draft
    engineRanks: current.schedule.engineRanks, // spec 006: the suggestion column
    horizonWeeks: current.horizonWeeks,
    pod: current.orderPod,
    scheduling: schedForForm,
  }) + baselinePanelHTML(current.baselines, current.baselineCompare, current.isDraft);
  applyLiveAssumptions(); // edits carried across an add/del re-render
  wireBaselineControls();
  host.querySelectorAll('.ord-options').forEach((b) =>
    b.addEventListener('click', () => toggleRemedies(b)));
  // AC 8.1: the timeline opens from the order view in one action.
  document.getElementById('tl-open')?.addEventListener('click', () => setView('timeline'));
  document.getElementById('sched-save')?.addEventListener('click', saveScheduling);
  // Assumptions live behind the ⚙ button (IA #2): set-once config out of the
  // reading path. The dialog itself auto-opens when something's outstanding.
  // The dialog auto-opens when a decision is outstanding (no period start, or an
  // unchosen WIP model). That is the case where the comparison matters most, and
  // the click handler below never fires for it -- so the table would have sat empty
  // under a heading inviting the planner to compare three models.
  const dialog = document.getElementById('sched-dialog');
  if (dialog && !dialog.hidden) {
    // aria-modal with focus left outside is a dialog a screen reader announces and
    // a keyboard user cannot reach. The click path focuses through the delegated
    // handler; the auto-open path had nothing. Skipped when focus is already
    // inside, so a re-render does not yank the caret out of a field being typed in.
    if (!dialog.contains(document.activeElement)) {
      dialog.querySelector('input, select')?.focus();
    }
    if (current.schedule && !current.schedule.wipModels) loadWipModels();
  }
  // Lane-chunking mode gates the chunk-size input (spec 007 amendment):
  // a disabled number is the honest picture of "spread" — no cap in force.
  document.getElementById('sched-chunking')?.addEventListener('change', () => {
    const n = document.getElementById('sched-split-min');
    if (n) n.disabled = document.getElementById('sched-chunking').value !== 'chunk';
  });
  document.getElementById('sched-open')?.addEventListener('click', async () => {
    const d = document.getElementById('sched-dialog');
    if (!d) return;
    const opening = d.hidden;
    d.hidden = !d.hidden;
    // The WIP-model comparison inside this dialog costs one extra full schedule per
    // model server-side (spec 001 §11 D22 as amended). It is fetched when the dialog
    // is actually opened, rather than on every /schedule for the benefit of a table
    // nobody has looked at.
    if (opening && current.schedule && !current.schedule.wipModels) await loadWipModels();
  });
  // (ESC handling and focus live in the stable document-level handler wired
  // once in initPlanUI — renderOrder must not stack a listener per render.)
  // FR-018's editor: adding appends an empty row; removing drops one. Both
  // snapshot the LIVE form first — the planner may have edited dates in rows
  // that exist only in the DOM, and rebuilding from the stale draft would
  // silently revert them.
  const snapshotCalRows = () => {
    const rows = [...host.querySelectorAll('.cal-win')].map((el, i) => ({
      kind: document.getElementById(`cal-kind-${i}`)?.value || 'change-freeze',
      scope: document.getElementById(`cal-scope-${i}`)?.value || '',
      fromDate: document.getElementById(`cal-from-${i}`)?.value || '',
      toDate: document.getElementById(`cal-to-${i}`)?.value || '',
      effect: document.getElementById(`cal-effect-${i}`)?.value || 'block-start',
    }));
    return rows.length ? rows : null;
  };
  document.getElementById('cal-add')?.addEventListener('click', () => {
    carryLiveAssumptions();
    const live = snapshotCalRows() ?? schedForForm.calendars ?? [];
    current.calDraft = [...live,
      { kind: 'change-freeze', scope: 'org', fromDate: '', toDate: '', effect: 'block-start' }];
    renderOrder();
  });
  host.querySelectorAll('.cal-del').forEach((b) => b.addEventListener('click', () => {
    carryLiveAssumptions();
    const i = Number(b.closest('.cal-win')?.dataset.row);
    const live = snapshotCalRows() ?? schedForForm.calendars ?? [];
    current.calDraft = live.filter((_, j) => j !== i);
    renderOrder();
  }));
  document.getElementById('sched-cancel')?.addEventListener('click', () => {
    current.calDraft = null; // cancel discards window edits, not just hides them
    current.assumptionDraft = null;
    renderOrder().then(() => {
      // The re-render rebuilds the dialog; urgency (missing period/model) would
      // auto-open it again, and a Cancel that re-opens is not a Cancel.
      const d = document.getElementById('sched-dialog');
      if (d) d.hidden = true;
    });
  });
  host.querySelectorAll('.ord-podlink').forEach((a) => a.addEventListener('click', () => {
    // Clicking the open pod again closes it, so the grid is never stuck behind a panel.
    current.orderPod = current.orderPod === a.dataset.pod ? null : a.dataset.pod;
    renderOrder();
  }));
  // Spec 004 AC 1.1/1.2: pin/unpin persists priorityLocked through the edit API
  // and recomputes. A draft has nothing saved to pin against, so the control is
  // rendered only on saved plans (orderRowHTML decides per row).
  host.querySelectorAll('.ord-pin').forEach((b) => b.addEventListener('click', async () => {
    const name = b.dataset.pin;
    const lock = b.dataset.locked !== '1'; // toggle
    b.disabled = true;
    // Capture the world as the request saw it: the reader may switch plans (or
    // trigger another recompute) while the PATCH is in flight, and writing a
    // stale answer into the new plan is worse than dropping it.
    const forPlan = current.id;
    const atEpoch = orderEpoch;
    const r = await req('/api/plan/' + forPlan + '/initiatives', {
      method: 'PATCH',
      body: JSON.stringify({ initiatives: [{ name, priorityLocked: lock }] }),
    });
    if (!current || current.id !== forPlan) return; // the reader moved on
    if (!r || !r.ok) {
      b.disabled = false;
      b.title = 'the pin did not save — try again';
      return;
    }
    // The PATCH response is the full post-edit initiative list: use it as the
    // cache, so a later ✎ save cannot silently re-send the stale lock state
    // and undo the pin.
    try {
      const d = await r.json();
      if (Array.isArray(d.initiatives)) current.initiatives = d.initiatives;
    } catch { /* cache stays; the server is still authoritative */ }
    if (orderEpoch !== atEpoch) return; // a recompute already superseded this
    current.schedule = null; // the order must answer the new pin, not the old one
    await renderOrder();
  }));
  // Spec 009 FR-005: the setup card's one-click recommendations.
  const applySetup = async (patch) => {
    const forPlan = current.id;
    // Bump the epoch FIRST: a loadWipModels() or schedule request already in
    // flight belongs to the pre-setup plan, and a late answer must not paint
    // it back over the recomputed order (cubic P2).
    staleOrder();
    // Capture AFTER the bump (cubic P2): if a newer operation owns the view
    // when this settles, its re-render wins and this one stays silent.
    const atEpoch = orderEpoch;
    const body = { ...((current.scheduling || {})), ...patch };
    const r = await req('/api/plan/' + forPlan + '/scheduling', {
      method: 'PATCH', body: JSON.stringify(body),
    });
    if (!current || current.id !== forPlan) return;
    if (orderEpoch !== atEpoch) return; // superseded by a newer operation
    const rerender = () => {
      // The CURRENT view, not always Order (cubic P2): rendering Order into
      // an active Network or Timeline destroys that view.
      if (view() === 'order') return renderOrder();
      if (view() === 'timeline') return renderTimeline();
      return renderDash();
    };
    if (!r || !r.ok) {
      // The epoch bump already invalidated the cached schedule's guards, so
      // leaving it silently stale is a dead cache under a live table (cubic
      // P2, second round). Re-render from the server's truth: the un-applied
      // setup stays, the schedule is recomputed fresh.
      current.schedule = null;
      await rerender();
      return;
    }
    current.scheduling = { ...(current.scheduling || {}), ...patch };
    current.schedule = null;
    await rerender();
  };
  host.querySelectorAll('.setup-apply').forEach((b) =>
    b.addEventListener('click', () => {
      if (b.dataset.setup === 'wip') applySetup({ wipModel: 'strict' });
      if (b.dataset.setup === 'estimate') applySetup({ estimateModel: 'effort' });
    }));
  host.querySelector('.setup-dismiss')?.addEventListener('click', () => {
    applySetup({ setupAcknowledged: true });
  });

  // Spec 006 Decision 1: accepting the engine's order (or returning to the
  // planner's) flips the working schedule via the scheduling params, and an
  // accept ends by OFFERING a baseline (Q1: explicit, asked, never assumed).
  // AC 3.1: Optimize presents the proposal first — both scores, the winning
  // rule, and the per-initiative moves — and accept/reject act on it. The
  // panel is view state, not a request: EngineRanks and rulesTried already
  // came with the schedule.
  const optimizePanel = document.getElementById('ord-optimize-panel');
  document.getElementById('ord-optimize')?.addEventListener('click', () => {
    const p = document.getElementById('ord-optimize-panel');
    if (p) { p.hidden = !p.hidden; return; }
    const best = (current.schedule.rulesTried || [])
      .filter((r) => r.rule !== current.schedule.rule)
      .reduce((m, r) => (r.objective < (m?.objective ?? Infinity) ? r : m), null);
    const moves = (current.schedule.initiatives || [])
      .map((si) => {
        const sug = (current.schedule.engineRanks || {})[si.name];
        return sug !== undefined && sug !== si.proposedRank
          ? `<li>#${si.proposedRank} <b>${esc(si.name)}</b> → #${sug}</li>` : '';
      })
      .filter(Boolean).join('');
    host.querySelector('.ord-card')?.insertAdjacentHTML('afterbegin', `
      <div class="ord-optimize-panel" id="ord-optimize-panel">
        <b>⚡ The engine suggests: ${esc(best ? best.rule : '—')}</b>
        <span class="hint">this order costs ${esc(String(best ? best.objective : '—'))} weighted lateness versus ${esc(String(current.schedule.objectiveScore))} for yours — an optimization, not a solution</span>
        ${moves ? `<ul class="hint">${moves}</ul>` : '<p class="hint">no moves — your order already matches the best rule found</p>'}
        <div class="sched-row" style="gap:8px">
          <button type="button" class="primary" id="ord-accept">Accept the engine's order</button>
          <button type="button" id="ord-reject">Keep my order</button>
        </div>
      </div>`);
    document.getElementById('ord-accept')?.addEventListener('click', () => setAcceptedOrdering('engine'));
    document.getElementById('ord-reject')?.addEventListener('click', () => {
      document.getElementById('ord-optimize-panel')?.remove();
    });
  });
  if (optimizePanel) { /* re-render keeps it closed; opening is one click */ }
  const setAcceptedOrdering = async (ordering) => {
    const forPlan = current.id;
    // Accepting an order invalidates everything in flight about the old one:
    // bump the epoch FIRST so a schedule or comparison that lands late is
    // refused by the guards it already carries (cubic: the unchanged epoch
    // let stale responses re-render the superseded order).
    staleOrder();
    const body = { ...((current.scheduling || {})), acceptedOrdering: ordering };
    if (ordering === 'engine') body.acceptedOrderingAt = Math.floor(Date.now() / 1000);
    else { delete body.acceptedOrderingAt; body.acceptedOrdering = 'stated'; }
    const r = await req('/api/plan/' + forPlan + '/scheduling', {
      method: 'PATCH', body: JSON.stringify(body),
    });
    if (!current || current.id !== forPlan) return;
    if (!r || !r.ok) return; // the assumptions dialog shows save errors; this is a header action
    // Write the cache only when this response is still the newest word: the
    // reader may have saved assumptions (or accepted again) while it was away.
    current.scheduling = { ...(current.scheduling || {}), ...body };
    current.schedule = null;
    await renderOrder();
    if (ordering === 'engine') {
      // Q1: ask to baseline AFTER the re-render, or the fresh DOM drops the
      // hint. One click, pre-filled, dismissible.
      const bl = document.getElementById('bl-name');
      if (bl) {
        bl.value = bl.value || ('engine order ' + new Date().toLocaleDateString(undefined, { day: 'numeric', month: 'short' }));
        document.querySelector('.bl-panel')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
        document.getElementById('bl-optimize-hint')?.remove();
        bl.insertAdjacentHTML('afterend', '<span class="hint" id="bl-optimize-hint">save this accepted order as a baseline?</span>');
      }
    }
  };
  document.getElementById('ord-unoptimize')?.addEventListener('click', () => setAcceptedOrdering('stated'));

  // ✎ sequencing-attribute editor (spec 004): built from the STORED initiative,
  // not the scheduled row — tier/CoD/kit/progress live only on the stored one.
  // Like the assumptions dialog, the element is re-rendered with every
  // renderOrder, so the wiring is per-render and never stacks.
  host.querySelectorAll('.ord-edit').forEach((b) => b.addEventListener('click', () => {
    closeInitEditor();
    const it = (current.initiatives || []).find((i) => i.name === b.dataset.edit);
    if (!it) return;
    host.insertAdjacentHTML('beforeend', initiativeEditDialogHTML(it));
    const dlg = document.getElementById('init-edit-dialog');
    if (!dlg) return;
    dlg.hidden = false;
    document.getElementById('ie-priority')?.focus();
    document.getElementById('ie-cancel')?.addEventListener('click', closeInitEditor);
    document.getElementById('ie-save')?.addEventListener('click', async () => {
      const errEl = document.getElementById('ie-error');
      // Checkboxes read .checked; everything else .value.
      const read = (id) => {
        const el = document.getElementById(id);
        if (!el) return '';
        return el.type === 'checkbox' ? el.checked : el.value;
      };
      // "had" carries what the STORED initiative had, so an emptied field can
      // send an explicit clear instead of a "not mentioned" null.
      const parsed = initiativeEditFromBody(read, it.name, {
        targetDate: it.targetDate, costOfDelayPerWeek: it.costOfDelayPerWeek,
        kitPct: it.kitPct, progressPct: it.progressPct,
      });
      if (parsed.error || !parsed.body) {
        if (errEl) errEl.textContent = parsed.error || 'those values do not parse';
        return;
      }
      const save = document.getElementById('ie-save');
      if (save) { save.disabled = true; save.textContent = 'Saving…'; }
      const forPlan = current.id;
      const atEpoch = orderEpoch;
      const r = await req('/api/plan/' + forPlan + '/initiatives', {
        method: 'PATCH', body: JSON.stringify({ initiatives: [parsed.body] }),
      });
      if (!current || current.id !== forPlan) return; // the reader moved on
      if (!r || !r.ok) {
        if (save) { save.disabled = false; save.textContent = 'Save'; }
        const why = r ? await r.text() : 'the request did not reach the server';
        if (errEl) errEl.textContent = why.slice(0, 200);
        return;
      }
      // The PATCH response carries the full post-edit list: it IS the refreshed
      // cache, so the next open shows the saved values and the next save cannot
      // resend stale ones. No second GET whose failure would strand the cache.
      try {
        const d = await r.json();
        if (Array.isArray(d.initiatives)) current.initiatives = d.initiatives;
      } catch { /* cache stays; the server is still authoritative */ }
      if (orderEpoch !== atEpoch) return; // a recompute already superseded this
      current.schedule = null;
      await renderOrder();
    });
  }));
  const closeInitEditor = () => document.getElementById('init-edit-dialog')?.remove();
  document.querySelector('.ord-queue')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  if (current.baselineFocus) {
    current.baselineFocus = false; // one-shot: clicking the chip, not every render
    document.querySelector('.bl-panel')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    document.getElementById('bl-name')?.focus();
  }
}

// previewInitiativesFile parses an uploaded sheet server-side and shows the
// resulting network/constraints WITHOUT saving — the sheet may still be a
// work in progress. "Save initiatives" (saveDraftInitiatives) persists it;
// closing the plan or picking a different file without saving discards it.
async function previewInitiativesFile(file) {
  const fd = new FormData();
  fd.append('file', file);
  fd.append('strict', current.strictDeps ? '1' : '0');
  root.querySelector('.plan-uploads').insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">reading…</span>');
  const r = await req('/api/plan/' + current.id + '/initiatives/preview', { method: 'POST', body: fd });
  document.getElementById('plan-uploading')?.remove();
  if (!r || !r.ok) { alert('Could not read file: ' + (r ? await r.text() : 'network')); return; }
  const draft = await r.json();
  current.initiatives = draft.initiatives;
  current.network = draft.network;
  current.unknownTeams = draft.unknownTeams;
  current.sim = draft.sim;
  staleOrder(); // the sheet changed, so the order did too
  current.levers = [];
  current.netMode = 'after';
  current.isDraft = true;
  current.draftFile = file;
  renderPlan();
  // A picked file whose only feedback is a banner far above the scroll reads as
  // "the button did nothing". Bring the banner into view once the re-rendered
  // view has settled (renderOrder is async; the timeout covers its schedule
  // fetch without coupling to its internals).
  setTimeout(() => {
    document.getElementById('plan-draft-save')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }, 600);
}

async function saveDraftInitiatives() {
  if (!current.draftFile) return;
  const btn = document.getElementById('plan-draft-save');
  if (btn) { btn.disabled = true; btn.textContent = 'Saving…'; }
  await uploadFile('initiatives', current.draftFile); // real save endpoint; re-fetches + clears draft state on success
}

// renderRosterPicker sources the plan's team structure from a saved roster
// (preferred — pinned at attach time, matches the same roster the pods came
// from) with a raw CSV/XLSX upload as the fallback when no roster exists yet.
async function renderRosterPicker(nTeams) {
  const box = document.getElementById('plan-roster-pick');
  if (!box) return;
  const rr = await req('/api/rosters');
  const rosters = (rr && rr.ok) ? (await rr.json()) || [] : [];
  const bindUpload = () => box.querySelectorAll('input[type=file]').forEach((inp) => inp.addEventListener('change', () => {
    if (inp.files[0]) uploadFile(inp.dataset.kind, inp.files[0]);
  }));
  if (!rosters.length) {
    box.innerHTML = '<span class="hint">No saved rosters yet — create one in Observe ▸ 👥 Rosters (recommended), or upload a roster file directly:</span>'
      + uploadField('teams', '⤓ Teams roster (CSV/XLSX)', nTeams);
    bindUpload();
    return;
  }
  box.innerHTML = `<label class="hint">roster
      <select id="plan-roster-sel">${rosters.map((r) => `<option value="${r.id}" ${r.id === current.rosterId ? 'selected' : ''}>${esc(r.name)} (${r.podCount} pods)</option>`).join('')}</select>
    </label>
    <button id="plan-roster-use">${nTeams ? 'switch roster' : 'use roster'}</button>
    <span class="hint">${nTeams ? `${nTeams} pods loaded` : 'none yet'}</span>`;
  box.querySelector('#plan-roster-use').addEventListener('click', async () => {
    const rosterId = document.getElementById('plan-roster-sel').value;
    box.insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">applying…</span>');
    const r = await req('/api/plan/' + current.id + '/roster', { method: 'POST', body: JSON.stringify({ rosterId }) });
    if (!r || !r.ok) { alert('Could not attach roster: ' + (r ? await r.text() : 'network')); document.getElementById('plan-uploading')?.remove(); return; }
    openPlan(current.id);
  });
}

async function savePlanParams() {
  const horizon = +document.getElementById('plan-horizon').value || 26;
  const loss = (+document.getElementById('plan-loss').value || 0) / 100;
  await req('/api/plan/' + current.id, { method: 'PATCH', body: JSON.stringify({ horizonWeeks: horizon, capacityLoss: loss }) });
  openPlan(current.id);
}

async function uploadFile(kind, file) {
  const fd = new FormData();
  fd.append('file', file);
  if (kind === 'initiatives') fd.append('strict', current.strictDeps ? '1' : '0');
  root.querySelector('.plan-uploads').insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">uploading…</span>');
  const r = await req('/api/plan/' + current.id + '/' + kind, { method: 'POST', body: fd });
  if (!r || !r.ok) { alert('Upload failed: ' + (r ? await r.text() : 'network')); document.getElementById('plan-uploading')?.remove(); return; }
  openPlan(current.id); // re-fetch assembled view
}

// ρ → color (matches the app's red/amber/green semantics)
function rhoColor(rho) {
  if (!isFinite(rho)) return 'var(--red)';
  if (rho >= 1) return 'var(--red)';
  if (rho >= 0.85) return 'var(--amber)';
  return 'var(--green)';
}
// server sends InfiniteRho (1e9, JSON-safe stand-in for +Inf) for demand with zero capacity
const rhoTxt = (rho) => rho >= 1e8 ? '∞' : rho.toFixed(2);
// lead time is directional: past the horizon it "won't fit", but still show the
// raw estimate in parens, e.g. ">26w (207w) — won't fit".
const fmtLead = (weeks, horizon) => {
  const r = Math.round(weeks);
  return r > horizon ? `&gt;${horizon}w (${r}w) — won't fit` : `${r}w`;
};

// renderDash ensures we have a simulation result (baseline = no levers), then paints.
async function renderDash() {
  if (!current.sim) { await runSim(); return; }
  paintDash();
}

async function runSim() {
  const body = { levers: current.levers || [] };
  // draft preview mode: simulate against the unsaved sheet, not the stale saved one
  if (current.isDraft) body.initiatives = current.initiatives;
  const r = await req('/api/plan/' + current.id + '/simulate', { method: 'POST', body: JSON.stringify(body) });
  if (!r || !r.ok) { document.getElementById('plan-dash').innerHTML = '<p class="hint">Could not run simulation.</p>'; return; }
  current.sim = await r.json();
  paintDash();
}

const PODS = () => (current.teams || []).map((t) => t.name).sort();
const INITS = () => (current.initiatives || []).map((i) => i.name);
const leverLabel = (lv) => ({
  addCapacity: `+${lv.n} track(s) → ${lv.pod}`,
  unpair: `un-pair ${lv.pod}`,
  descope: `descope ${lv.initiative} −${Math.round(lv.n * 100)}%`,
  defer: `defer ${lv.initiative}`,
  reduceWip: `reduce WIP −${Math.round(lv.n * 100)}%`,
  reassign: `reassign ${lv.pod} → ${lv.toPod}`,
  dropPod: `drop ${lv.pod} from ${lv.initiative}`,
}[lv.type] || lv.type);

function delta(before, after, lowerIsBetter = true) {
  const better = lowerIsBetter ? after < before : after > before;
  const worse = lowerIsBetter ? after > before : after < before;
  const col = better ? 'var(--green)' : worse ? 'var(--red)' : 'var(--muted)';
  return `<span style="color:${col}">${before} → ${after}</span>`;
}

function paintDash() {
  const p = current, sim = current.sim;
  const horizon = current.horizonWeeks || 26;
  const leadDelta = (b, a) => {
    const col = a < b ? 'var(--green)' : a > b ? 'var(--red)' : 'var(--muted)';
    return `<span style="color:${col}">${fmtLead(b, horizon)} → ${fmtLead(a, horizon)}</span>`;
  };
  const mode = current.netMode === 'before' ? 'before' : 'after';
  const loads = (sim[mode].loads || []);
  const byB = {}; sim.before.loads.forEach((l) => byB[l.team] = l);
  const byA = {}; sim.after.loads.forEach((l) => byA[l.team] = l);
  const hasLevers = (current.levers && current.levers.length > 0);
  const fitDelta = (b, a) => {
    const col = a.fitting > b.fitting ? 'var(--green)' : a.fitting < b.fitting ? 'var(--red)' : 'var(--muted)';
    return `<span style="color:${col}">${b.fitting}/${b.total} → ${a.fitting}/${a.total}</span>`;
  };

  const teamByName = {}; (current.teams || []).forEach((t) => teamByName[t.name] = t);
  const constraintRows = sim.before.loads.map((l) => {
    const a = byA[l.team] || l;
    const tm = teamByName[l.team] || {};
    if (current.editPod === l.team) {
      return `<tr><td>${esc(l.team)}</td><td colspan="4">
        tracks <input id="pe-tracks" type="number" min="0" max="50" value="${tm.tracks || ''}" placeholder="${l.tracks} (auto)" style="width:84px">
        <label><input id="pe-pairs" type="checkbox" ${tm.pairs ? 'checked' : ''}> pairs</label>
        <span class="hint">${tm.devs || 0} devs</span>
        <button class="pod-save" data-pod="${esc(l.team)}">save</button>
        <button class="pod-cancel">cancel</button></td></tr>`;
    }
    return `<tr>
      <td>${esc(l.team)}</td>
      <td><b style="color:${rhoColor(l.rho)}">${rhoTxt(l.rho)}</b></td>
      <td>${hasLevers ? `<b style="color:${rhoColor(a.rho)}">${rhoTxt(a.rho)}</b>` : '<span class="hint">—</span>'}</td>
      <td>${Math.round(l.demandWeeks)} / ${Math.round(l.capacityWeeks)}</td>
      <td>${l.tracks}${hasLevers && a.tracks !== l.tracks ? ` → ${a.tracks}` : ''} <a class="pod-edit" data-pod="${esc(l.team)}" title="edit capacity">✎</a></td></tr>`;
  }).join('');

  const initB = {}; sim.before.initiatives.forEach((i) => initB[i.name] = i);
  const initA = {}; sim.after.initiatives.forEach((i) => initA[i.name] = i);
  const initRows = sim.before.initiatives.map((i) => {
    const a = initA[i.name];
    const pods = Object.keys((p.initiatives.find((x) => x.name === i.name) || {}).work || {});
    return `<tr><td>${esc(i.name)}</td><td>${esc(pods.join(', '))}</td>
      <td>${fmtLead(i.leadWeeks, horizon)}${hasLevers && a && a.leadWeeks !== i.leadWeeks ? ` → <b>${fmtLead(a.leadWeeks, horizon)}</b>` : ''}</td>
      <td>${esc(i.bottleneck || '—')}</td></tr>`;
  }).join('');

  const summary = `Constraints ${delta(sim.before.constraints, sim.after.constraints)} ·
    Fitting ${fitDelta(sim.before, sim.after)} ·
    Median lead ${leadDelta(sim.before.medianLeadWeeks, sim.after.medianLeadWeeks)}`;

  document.getElementById('plan-dash').innerHTML = `
    <div class="plan-net card">
      <div class="plan-net-head"><b>Dependency network</b>
        <span>
          <button class="seg ${mode === 'before' ? 'seg-on' : ''}" id="net-before">baseline</button><button class="seg ${mode === 'after' ? 'seg-on' : ''}" id="net-after">with levers</button>
        </span></div>
      <div class="plan-net-wrap">
        <svg id="plan-svg"></svg>
        <aside id="plan-netpanel"><p class="hint">Click a pod to inspect it. Node size = demand weeks, ring color = queue heat (ρ), flow runs left→right, arrow points at the pod waiting.</p></aside>
      </div>
      <p class="hint">flow runs left→right · node size = demand · ring = ρ (heat) · showing <b>${mode === 'after' ? 'with levers' : 'baseline'}</b></p>
    </div>
    <div class="plan-constraints card" style="margin-top:12px">
      <b>Constraints <span class="hint">(hottest first)</span></b>
      <table class="wip-table"><thead><tr><th>Pod</th><th>ρ now</th><th>ρ after</th><th>demand/cap</th><th>tracks</th></tr></thead>
        <tbody>${constraintRows}</tbody></table>
      <p class="hint">ρ: red ≥1 · amber ≥.85 · green &lt;.85 — utilization is the signal; lead time is directional.</p>
    </div>
    <div class="plan-levers card">
      <b>Levers — what-if</b>
      <div class="plan-summary">${summary}</div>
      <div class="lever-chips">${(current.levers || []).map((lv, i) => `<span class="chip">${esc(leverLabel(lv))} <a class="chip-x" data-lev="${i}">✕</a></span>`).join('') || '<span class="hint">no levers yet</span>'}</div>
      <div class="lever-add">
        <select id="lev-type">
          <option value="addCapacity">Add capacity</option>
          <option value="unpair">Un-pair a pod</option>
          <option value="descope">Descope an initiative</option>
          <option value="defer">Defer an initiative</option>
          <option value="reduceWip">Reduce WIP (focus)</option>
          <option value="reassign">Reassign a pod's work</option>
          <option value="dropPod">Drop a pod from an initiative</option>
        </select>
        <span id="lev-target"></span>
        <button id="lev-add" class="primary">Add lever</button>
      </div>
    </div>
    <div class="card" style="margin-top:12px">
      <b>Initiatives</b>
      <table class="wip-table"><thead><tr><th>Initiative</th><th>Pods in path</th><th>Lead time (directional)</th><th>Bottleneck</th></tr></thead>
        <tbody>${initRows}</tbody></table>
    </div>`;

  drawNetwork(p, loads);
  document.getElementById('net-before').addEventListener('click', () => { current.netMode = 'before'; paintDash(); });
  document.getElementById('net-after').addEventListener('click', () => { current.netMode = 'after'; paintDash(); });
  document.querySelectorAll('.lever-chips .chip-x').forEach((a) => a.addEventListener('click', () => {
    current.levers.splice(+a.dataset.lev, 1); staleOrder(); runSim();
  }));
  const typeSel = document.getElementById('lev-type');
  typeSel.addEventListener('change', renderLeverTarget);
  renderLeverTarget();
  document.getElementById('lev-add').addEventListener('click', addLever);
  // per-pod capacity editing
  document.querySelectorAll('.pod-edit').forEach((a) => a.addEventListener('click', () => { current.editPod = a.dataset.pod; paintDash(); }));
  document.querySelector('.pod-cancel')?.addEventListener('click', () => { current.editPod = null; paintDash(); });
  document.querySelector('.pod-save')?.addEventListener('click', (e) => savePod(e.target.dataset.pod));
}

async function savePod(pod) {
  const tv = document.getElementById('pe-tracks').value;
  const pairs = document.getElementById('pe-pairs').checked;
  await req('/api/plan/' + current.id + '/teams', {
    method: 'PATCH',
    body: JSON.stringify({ name: pod, pairs, tracks: tv === '' ? 0 : (+tv) }),
  });
  current.editPod = null;
  reloadPlan();
}

// reloadPlan re-fetches the plan (roster changed) but keeps the applied levers.
// The order is dropped rather than kept: pod capacity is the input the scheduler
// is most sensitive to, so a retained order would be wrong about nearly everything.
async function reloadPlan() {
  staleOrder(); // the roster moved, so an order still in flight is already wrong
  const levers = current.levers || [];
  const was = view();
  const pod = current.orderPod;
  const r = await req('/api/plan/' + current.id);
  if (!r || !r.ok) return;
  current = await r.json();
  current.tlFilter = ''; // lens filters are per-plan view state (spec 010 FR-004)
  await loadBaselines(); // replaced wholesale above, so re-fetch rather than show none
  current.levers = levers;
  // Keep the reader where they were. Replacing `current` wholesale is what drops
  // the stale order, which is wanted; bouncing them back to Network is not.
  current.view = was;
  current.orderPod = pod;
  renderPlan();
}

function renderLeverTarget() {
  const t = document.getElementById('lev-type').value;
  const podOpts = PODS().map((n) => `<option>${esc(n)}</option>`).join('');
  const initOpts = INITS().map((n) => `<option>${esc(n)}</option>`).join('');
  const el = document.getElementById('lev-target');
  if (t === 'addCapacity') el.innerHTML = `<select id="lev-pod">${podOpts}</select> +<input id="lev-n" type="number" min="1" max="10" value="2" style="width:48px"> tracks`;
  else if (t === 'unpair') el.innerHTML = `<select id="lev-pod">${podOpts}</select>`;
  else if (t === 'descope') el.innerHTML = `<select id="lev-init">${initOpts}</select> −<input id="lev-n" type="number" min="5" max="90" value="40" style="width:48px">%`;
  else if (t === 'defer') el.innerHTML = `<select id="lev-init">${initOpts}</select>`;
  else if (t === 'reduceWip') el.innerHTML = `−<input id="lev-n" type="number" min="5" max="40" value="15" style="width:48px">% multitasking`;
  else if (t === 'reassign') el.innerHTML = `<select id="lev-pod">${podOpts}</select> → <select id="lev-topod">${podOpts}</select>`;
  else if (t === 'dropPod') el.innerHTML = `<select id="lev-pod">${podOpts}</select> from <select id="lev-init">${initOpts}</select>`;
}

function addLever() {
  const t = document.getElementById('lev-type').value;
  const pod = document.getElementById('lev-pod')?.value;
  const init = document.getElementById('lev-init')?.value;
  const n = +(document.getElementById('lev-n')?.value || 0);
  let lv;
  if (t === 'addCapacity') lv = { type: t, pod, n };
  else if (t === 'unpair') lv = { type: t, pod };
  else if (t === 'descope') lv = { type: t, initiative: init, n: n / 100 };
  else if (t === 'defer') lv = { type: t, initiative: init };
  else if (t === 'reduceWip') lv = { type: t, n: n / 100 };
  else if (t === 'reassign') lv = { type: t, pod, toPod: document.getElementById('lev-topod')?.value };
  else if (t === 'dropPod') lv = { type: t, pod, initiative: init };
  current.levers = current.levers || [];
  current.levers.push(lv);
  staleOrder();
  runSim();
}

// drawNetwork renders the same left-to-right layered format as Observe's Org
// Network (columns by dependency depth, ring color = ρ heat, pan/zoom,
// click-to-spotlight) via the shared netgraph.js primitives, so a manager
// reads both diagrams the same way.
function drawNetwork(p, loads) {
  const svg = d3.select('#plan-svg');
  if (svg.empty() || typeof d3 === 'undefined') return;
  svg.selectAll('*').remove();
  const rect = svg.node().getBoundingClientRect();
  const width = rect.width > 50 ? rect.width : 900;
  const height = rect.height > 50 ? rect.height : 520;
  svg.attr('viewBox', `0 0 ${width} ${height}`);

  const net = p.network || { nodes: [], edges: [] };
  const byPod = {}; (loads || []).forEach((l) => { byPod[l.team] = l; });
  const nodes = net.nodes.map((n) => ({ name: n.team, weeks: byPod[n.team]?.demandWeeks ?? n.weeks, rho: byPod[n.team]?.rho ?? 0 }));
  if (!nodes.length) {
    svg.append('text').attr('x', width / 2).attr('y', height / 2).attr('text-anchor', 'middle')
      .attr('fill', 'var(--muted)').attr('font-size', 14).text('No in-path work yet.');
    return;
  }
  const names = nodes.map((n) => n.name);
  const idset = new Set(names);
  const edges = net.edges.filter((e) => idset.has(e.from) && idset.has(e.to));

  const pos = layoutColumns(names, edges, width, height);
  appendArrowMarker(svg, 'plan-arrow', 'var(--muted)');
  const g = svg.append('g');
  enablePanZoom(svg, g);
  svg.on('click', () => spotlight(null));

  const maxW = Math.max(1, d3.max(nodes, (n) => n.weeks) || 1);
  const radius = (n) => 8 + 16 * Math.sqrt((n.weeks || 0) / maxW);
  const nodeByName = new Map(nodes.map((n) => [n.name, n]));

  const edgePath = bezierEdgePath(pos, (name) => radius(nodeByName.get(name)), edges);
  const linkSel = g.append('g').selectAll('path').data(edges).join('path')
    .attr('d', edgePath).attr('fill', 'none')
    .attr('stroke', 'var(--muted)').attr('stroke-opacity', 0.5)
    .attr('stroke-width', (e) => Math.min(4, 1 + e.count))
    .attr('marker-end', 'url(#plan-arrow)');

  const nodeSel = g.append('g').selectAll('g').data(nodes).join('g')
    .attr('transform', (n) => `translate(${pos.get(n.name).x},${pos.get(n.name).y})`);
  const drag = enableNodeDrag(nodeSel, pos, () => linkSel.attr('d', edgePath));
  drag.onClick((ev, n) => {
    ev?.stopPropagation(); spotlight(n.name); showPlanPodPanel(n, p, loads, net);
  });

  nodeSel.append('circle').attr('r', radius)
    .attr('fill', 'var(--bg)')
    .attr('stroke', (n) => heatColor(n.rho))
    .attr('stroke-width', 2.5);
  nodeSel.append('text').text((n) => n.name)
    .attr('dy', (n) => -radius(n) - 8).attr('text-anchor', 'middle')
    .attr('fill', 'var(--text)').attr('font-size', 11);
  nodeSel.append('text').text((n) => Math.round(n.weeks) || 0)
    .attr('dy', 3.5).attr('text-anchor', 'middle').attr('pointer-events', 'none')
    .attr('fill', 'var(--text)').attr('font-size', 10).attr('font-weight', 700);

  const spotlight = makeSpotlight(nodeSel, linkSel, edges);
}

// showPlanPodPanel is Plan's equivalent of Observe's node inspector — same
// "click a node, see its detail card" interaction, with Plan-relevant fields
// (demand/capacity/tracks, initiatives it's on the path for) instead of Jira
// activity.
//
// The initiatives list is a numbered, row-separated list — not consecutive
// <dd>s — because sheet-typed names are long and wrap; without numbering and
// rules, a wrapped name reads as a new paragraph (the "blob" the maintainer
// flagged). Each row also carries this pod's weeks on that initiative, which
// the blob was hiding.
function showPlanPodPanel(n, p, loads, net) {
  const l = (loads || []).find((x) => x.team === n.name) || {};
  const inAll = (net.edges || []).filter((e) => e.to === n.name);
  const outAll = (net.edges || []).filter((e) => e.from === n.name);
  const inits = (p.initiatives || []).filter((i) => i.work?.[n.name]?.inPath);
  const flags = [];
  if (n.rho >= 1e8) flags.push('<span class="flag red">demand with zero capacity</span>');
  else if (n.rho >= 1) flags.push('<span class="flag red">over capacity (ρ≥1)</span>');
  else if (n.rho >= 0.85) flags.push('<span class="flag amber">queue hot (ρ≥0.85)</span>');
  const initRows = inits.map((i, idx) => {
    const w = i.work[n.name];
    const weeks = (w?.estimated && w?.weeks > 0)
      ? `<span class="insp-weeks">${Math.round(w.weeks)}w</span>`
      : (w?.weeks > 0 ? `<span class="insp-weeks">${Math.round(w.weeks)}w</span>` : '<span class="insp-weeks hint">no estimate</span>');
    return `<li><span class="insp-num">${idx + 1}</span><span class="insp-name">${esc(i.name)}</span>${weeks}</li>`;
  }).join('');
  document.getElementById('plan-netpanel').innerHTML = `
    <h2>${esc(n.name)}</h2>
    <div>${flags.join(' ') || '<span class="flag" style="color:var(--green)">healthy</span>'}</div>
    <dl>
      <dt>Demand / capacity</dt><dd>${Math.round(l.demandWeeks ?? n.weeks ?? 0)}w / ${Math.round(l.capacityWeeks ?? 0)}w · tracks ${l.tracks ?? '—'}</dd>
      <dt>Utilization</dt><dd>ρ ${rhoTxt(n.rho)}</dd>
      <dt>Coupling</dt><dd>depends on ${inAll.length} · ${outAll.length} depend on it</dd>
      <dt>Initiatives (${inits.length})</dt>
      <dd>${initRows ? `<ol class="insp-list">${initRows}</ol>` : '—'}</dd>
    </dl>`;
}
