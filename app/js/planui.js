import { mountPlanningAssistant } from './planning-assistant.js';
// Plan pillar UI: a manager uploads a teams roster + an initiatives matrix, and
// sees the directed cross-pod dependency network with per-pod utilization (ρ)
// and the constraint pods. Levers/what-if come in a later phase.
import { authFetch, authToken } from './auth.js';
import { readRoute, writeRoute } from './navigation.js';
import { icon } from './icons.js';
import { openModal, closeModal, containFocus } from './modal.js';
import { mountExecution } from './executionui.js';
import { mountReadyQueue } from './readyqueueui.js';
import { openImport } from './importui.js';
import { openLinkedSheets } from './linksheets.js';
import {
  heatColor, layoutColumns, bezierEdgePath, appendArrowMarker,
  enablePanZoom, enableNodeDrag, makeSpotlight,
} from './netgraph.js';
import { esc, compareScheduleCosts, orderViewHTML, schedulingFromForm, initiativeEditDialogHTML, initiativeEditFromBody, wipModelsTableHTML } from './order.js';
import { exportBlockPNG } from './exportpng.js';
import { attachDrag } from './drag.js';
import { openDocs } from './docs.js';
import { initiativeMatch } from './filter.js';
import { term } from './terms.js';
import { baselineChipHTML, baselinesDrawerHTML, saveErrorMessage, latestOnly, activeBaseline, compareTableHTML } from './baseline.js';
import { remediesPanelHTML, remediesErrorMessage } from './remedyui.js';
import { portfolioTimelineHTML, podLensHTML, podSheetHTML, timelineControlsHTML, timelineInspectorHTML, timelineEditsFromRows, matchesTimelineTeam } from './timeline.js';
import { healthReportHTML, remediesSectionHTML } from './report.js';

let root, current = null, disposeAssistant = null;
let pendingPlanDestination = '';
const planDestinations = { setup: 'plan setup', order: 'Plan commitments', timeline: 'Timeline', ready: 'Next work', execution: 'Review execution', assistant: 'Planning assistant', 'linked-sheets': 'Linked Google Sheets' };
function pendingDestinationHTML() {
  return pendingPlanDestination ? `<p data-pending-destination role="status">${current ? 'Complete this plan’s inputs' : 'Choose a plan'} to open ${esc(planDestinations[pendingPlanDestination])}. <button class="btn btn-secondary" type="button" data-cancel-destination>Cancel</button></p>` : '';
}
function wirePendingDestination() {
  root.querySelector('[data-cancel-destination]')?.addEventListener('click', () => {
    pendingPlanDestination = '';
    root.querySelector('[data-pending-destination]')?.remove();
  });
}
export async function openPlanDestination(destination) {
  if (!Object.hasOwn(planDestinations, destination)) return false;
  pendingPlanDestination = destination;
  document.querySelector('.tab[data-view="plan"]')?.click();
  return resumePlanDestination();
}
async function resumePlanDestination() {
  const destination = pendingPlanDestination;
  if (!destination || !current) return false;
  if (current.isDraft) {
    root.querySelector('[data-pending-destination]')?.remove();
    root.querySelector('.plan-head')?.insertAdjacentHTML('afterend', pendingDestinationHTML());
    wirePendingDestination();
    planNotice('Save or discard the upload preview before opening this feature.');
    return false;
  }
  if (destination === 'linked-sheets') {
    pendingPlanDestination = '';
    root.querySelector('[data-pending-destination]')?.remove();
    return showLinkedSheets();
  }
  if (destination === 'setup') {
    pendingPlanDestination = '';
    const setup = root.querySelector('.plan-setup');
    if (setup) { setup.open = true; setup.querySelector('input,button,select')?.focus(); }
    root.querySelector('[data-pending-destination]')?.remove();
    return !!setup;
  }
  setView(destination);
  if (!document.getElementById('plan-dash')) return false;
  pendingPlanDestination = '';
  root.querySelector('[data-pending-destination]')?.remove();
  return true;
}
// specs/017-planning-and-execution-usability.md:86: successful timeline edits
// retain undo history; failed saves and failed undo never consume an entry.
let dragUndo = null;
let dragHistory = [];
let timelineMutationPending = false;
let planLoadTicket = 0;
// The initiatives file the planner picked but has not previewed yet (review):
// selecting a file no longer throws the plan into draft preview — the Preview
// button does, after validating the roster and strict-mode warnings.
let pendingInitiativesFile = null;
// One comparison request at a time, keyed to what it is for. A bare boolean
// stranded the table: if the plan or the order moved while a request was out, the
// new dialog's request was skipped as "already pending" and the stale response was
// then discarded, leaving the container empty with nothing left to fill it.
let wipModelsPendingFor = null;

export function initPlanUI() {
  // Docs deep links (spec 012 FR-003): any .usage-link opens the in-app
  // manual at its anchor. Delegated — warnings re-render constantly.
  document.addEventListener('click', (ev) => {
    const link = ev.target.closest?.('.usage-link');
    if (!link) return;
    openDocs(link.dataset.anchor);
  });
  // The assumptions dialog's ESC + focus management: ONE handler for the app's
  // lifetime (renderOrder re-renders the dialog constantly; per-render
  // listeners would stack). ESC closes wherever focus sits; on close, focus
  // returns to the ⚙ that opened it — the same contract modal.js gives the
  // shared modals.
  document.addEventListener('keydown', (ev) => {
    // ⌘Z / Ctrl+Z undoes the last timeline drag (spec 008 S4). Not inside
    // fields — there the browser's own text undo must win — and not while the
    // health report is up: an edit fired from behind a modal would recompute
    // the schedule the open card is summarizing (spec 013 AC 3.2).
    if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'z' || ev.key === 'Z')) {
      const t = ev.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;
      if (document.querySelector('.report-overlay')) return;
      ev.preventDefault();
      undoDrag();
      return;
    }
    if (ev.key !== 'Escape') return;
    // The health report's overlay closes first, alone: a report open over a
    // fullscreen timeline is two ESC-exits stacked, and one keypress must not
    // unwind both.
    const report = document.querySelector('.report-overlay');
    if (report) {
      report.remove();
      document.getElementById('view-report')?.focus();
      return;
    }
    if (document.querySelector('.bl-drawer-overlay')) { closeBaselinesDrawer(); return; }
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
    if (d && !d.hidden) { d.hidden = true; if(current) current.assumptionsDismissed=true; document.getElementById('sched-open')?.focus(); }
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
  wireBaselineDelegation();
  root = document.getElementById('plan-root');
  if (!root) return;
  document.querySelector('.tab[data-view="plan"]')?.addEventListener('click', () => {
    if (!current) renderList();
  });
  document.querySelectorAll('.tab[data-view]').forEach(button => button.addEventListener('click', () => {
    if (button.dataset.view !== 'plan') {
      pendingPlanDestination = '';
      root.querySelector('[data-pending-destination]')?.remove();
    }
  }));
}

const fmtDate = (ts) => ts ? new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '—';

function planNotice(message, error = false) {
  if (current) current.saveNotice = {message, error};
  const el = document.getElementById('plan-save-status');
  if (el) { el.textContent = message; el.classList.toggle('plan-warn', error); el.setAttribute('role', error ? 'alert' : 'status'); }
}
async function req(path, opts = {}) {
  const method = opts.method || 'GET';
  const write = method !== 'GET' && !/\/(schedule(?:\/remedies)?|simulate|compare(?:-to\/[^/]+)?|preview|assistant)$/.test(path);
  const planId = current?.id;
  if (write) planNotice('Saving…');
  try {
    const response = await authFetch(path, opts);
    if (write && current?.id === planId) {
      const why = response.ok ? '' : (await response.clone().text()).slice(0, 240);
      if (current?.id === planId) planNotice(response.ok ? 'Changes saved.' : `Could not save: ${why || response.status}`, !response.ok);
    }
    return response;
  } catch {
    if (write && current?.id === planId) planNotice('Could not save: the server could not be reached. Your pending input is still here.', true);
    return null;
  }
}
function rememberPlanRoute(replace = false) {
  if (!current) return;
  writeRoute({view:'plan', plan:current.id, planView:view(), lens:current.tlLens || 'initiative', initiative:current.tlInitiativeFilter, team:current.tlTeamFilter, selected:current.selectedInitiative}, replace);
  try { localStorage.setItem(`conway-plan-view-${current.id}`, view()); } catch { /* optional preference */ }
}
export async function restorePlanLocation(route = readRoute(location.href)) {
  if (!root) return;
  if (!route.plan) { await renderList(); return; }
  await openPlan(route.plan, route);
}


// dragNote paints the drag outcome line above the timeline (success is
// silence; a refusal names the conflict). Module level so undoDrag — which
// lives outside renderTimeline's closure — can report too.
function dragNote(msg) {
  const el = document.getElementById('tl-drag-note');
  if (!el) return;
  el.textContent = msg || '';
  el.hidden = !msg;
  el.classList.toggle('plan-warn', !!msg);
}

// snapshotDragUndo records the pre-drag scheduling params of the initiative
// being dragged (spec 008 S4). The full pin maps are copied because the PATCH
// replaces them wholesale — restoring a merge would leak the drag's pin. The
// caller assigns the result to dragUndo only once the PATCH succeeded, so a
// refused drag cannot steal the undo slot.
function snapshotDragUndo(it, pod) {
  return {
    planId: current.id,
    name: it.name,
    pod,
    pinnedStarts: { ...(it.pinnedStarts || {}) },
    pinnedLanes: { ...(it.pinnedLanes || {}) },
    estimateEdits: Object.fromEntries(Object.entries(it.work || {})
      .filter(([key, work]) => (!pod || key === pod) && Number.isFinite(work.weeks) && work.weeks > 0)
      .map(([key, work]) => [key, work.weeks])),
  };
}

// Undo restores full pin maps and the prior estimates for all affected teams.
async function undoDrag() {
  const u = dragHistory.at(-1);
  if (timelineMutationPending || !u || !current || current.id !== u.planId || current.isDraft) return;
  const it = (current.initiatives || []).find((i) => i.name === u.name);
  if (!it) return;
  const edit = { name: u.name, pinnedStarts: u.pinnedStarts, pinnedLanes: u.pinnedLanes };
  if (Object.keys(u.estimateEdits || {}).length) edit.estimateEdits = u.estimateEdits;
  timelineMutationPending = true;
  const atEpoch = orderEpoch; // captured before the PATCH (cubic P1)
  try {
  const r = await req('/api/plan/' + u.planId + '/initiatives', {
    method: 'PATCH', body: JSON.stringify({ initiatives: [edit] }),
  });
  if (!current || current.id !== u.planId || orderEpoch !== atEpoch) return;
  if (!r || !r.ok) {
    const why = r ? await r.text() : 'the request did not reach the server';
    if (current?.id === u.planId && orderEpoch === atEpoch) dragNote(why.slice(0, 200));
    return;
  }
  const d = await r.json();
  if (!current || current.id !== u.planId || orderEpoch !== atEpoch) return;
  if (!Array.isArray(d.initiatives)) throw new Error('The saved inputs could not be read. Reload the plan before editing again.');
  current.initiatives = d.initiatives;
  dragHistory.pop();
  dragUndo = dragHistory.at(-1) || null;
  staleOrder();
  await renderCurrentPlanView();
  } catch (error) {
    if (current?.id === u.planId) dragNote(error.message || 'Undo could not be saved. Try again.');
  } finally {
    timelineMutationPending = false;
  }
}

async function renderList() {
  disposeAssistant?.(); disposeAssistant = null;
  const ticket = ++planLoadTicket;
  current = null;
  writeRoute({view:'plan', plan:null, planView:null, selected:null, initiative:null, team:null, lens:null});
  root.innerHTML = '<p class="hint">Loading plans…</p>';
  const r = await req('/api/plan');
  if (ticket !== planLoadTicket) return;
  if (!r || !r.ok) { root.innerHTML = '<p class="hint">Could not load plans (need the manager role).</p>'; return; }
  const plans = await r.json();
  if (ticket !== planLoadTicket) return;
  root.innerHTML = `
    <div class="plan-head"><h2>Your plans</h2><button id="plan-new" class="btn btn-primary">+ New plan</button><button class="btn btn-secondary" id="plan-demo">Load demo plan</button></div>
    ${pendingDestinationHTML()}
    <p class="hint">Sample files to try the upload path: <a href="/api/sample/teams.csv" download>teams.csv</a> · <a href="/api/sample/initiatives.xlsx" download>initiatives.xlsx</a> (same data as the demo).</p>
    <table class="table table-sm wip-table">
      <thead><tr><th>Name</th><th>Pods</th><th>Initiatives</th><th>Health</th><th>Updated</th><th></th></tr></thead>
      <tbody>${(plans || []).map((p) => `<tr>
        <td><button type="button" class="btn btn-secondary plan-open" data-id="${esc(p.id)}">${esc(p.name)}</button></td>
        <td>${p.teamCount || 0}</td><td>${p.initiativeCount || 0}</td>
        <td><span class="badge bg-body-secondary text-body tag">${p.estimateModel === 'effort' ? 'effort' : 'wall-clock'}</span> ${p.periodStart ? '<span class="badge bg-success-subtle text-success-emphasis tag">dates set</span>' : '<span class="hint">no dates</span>'} ${p.baselineCount ? `<span class="badge bg-body-secondary text-body tag">${p.baselineCount} baseline${p.baselineCount > 1 ? 's' : ''}</span>` : ''}</td>
        <td>${fmtDate(p.updatedAt)}</td>
        <td><button class="btn btn-secondary plan-del" data-id="${p.id}">delete</button></td></tr>`).join('')
      || '<tr><td colspan="6" class="hint">No plans yet — create one, then upload your teams and initiatives or link Google Sheets.</td></tr>'}
      </tbody></table>`;
  root.querySelector('#plan-new').addEventListener('click', createPlan);
  wirePendingDestination();
  root.querySelector('#plan-demo').addEventListener('click', async () => {
    const r = await req('/api/plan/demo', { method: 'POST' });
    if (!r || !r.ok) { alert('Could not create demo plan'); return; }
    openPlan((await r.json()).id);
  });
  root.querySelectorAll('.plan-open').forEach((a) => a.addEventListener('click', () => openPlan(a.dataset.id)));
  root.querySelectorAll('.plan-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm('Delete this plan?')) return;
    const res = await req('/api/plan/' + b.dataset.id, { method: 'DELETE' });
    if (res?.ok) renderList(); else b.after(Object.assign(document.createElement('span'), {textContent:' Could not delete the plan.'}));
  }));
}

async function createPlan() {
  const name = prompt('Plan name', 'New plan');
  if (name === null) return;
  const r = await req('/api/plan', { method: 'POST', body: JSON.stringify({ name }) });
  if (!r || !r.ok) { alert('Could not create plan'); return; }
  openPlan((await r.json()).id);
}

async function openPlan(id, route = null) {
  const ticket = ++planLoadTicket;
  const prior = current?.id === id ? current : null;
  staleOrder(); // any order request still in flight belongs to the plan being left
  root.innerHTML = '<p class="hint">Loading plan…</p>';
  const r = await req('/api/plan/' + id);
  if (ticket !== planLoadTicket) return;
  if (!r || !r.ok) { root.innerHTML = '<p class="hint">Could not load plan.</p>'; return; }
  const loaded = await r.json();
  if (ticket !== planLoadTicket) return;
  current = loaded;
  let remembered; try { remembered = localStorage.getItem(`conway-plan-view-${id}`); } catch { /* optional */ }
  current.view = route?.planView || prior?.view || remembered || 'order';
  current.tlLens = route?.lens || prior?.tlLens || 'initiative';
  current.tlInitiativeFilter = route?.initiative ?? prior?.tlInitiativeFilter ?? '';
  current.tlTeamFilter = route?.team ?? prior?.tlTeamFilter ?? '';
  current.selectedInitiative = route?.selected ?? prior?.selectedInitiative ?? '';
  current.saveNotice = prior?.saveNotice;
  current.tlHideEmpty = prior?.tlHideEmpty || false; // lens filter state is per-plan (spec 010 FR-004)
  await loadBaselines(); // the header chip needs these before the first paint
  if (ticket !== planLoadTicket) return;
  if (!route) rememberPlanRoute();
  renderPlan();
  if (ticket === planLoadTicket) await resumePlanDestination();
}

function uploadField(kind, label, count) {
  return `<div class="plan-up d-flex flex-wrap gap-2 align-items-end mw-100">
    <label class="form-label mb-0 mw-100 plan-upbtn">${label}<input class="form-control" type="file" accept=".csv,.xlsx" data-kind="${kind}"></label>
    <span class="hint">${count ? `${count} loaded` : 'none yet'}</span>
  </div>`;
}

function renderPlan() {
  disposeAssistant?.(); disposeAssistant = null;
  const p = current;
  const nTeams = (p.teams || []).length, nInit = (p.initiatives || []).length;
  const unknown = p.unknownTeams || [];
  root.innerHTML = `
    <div class="plan-head">
      <nav class="plan-crumbs" aria-label="You are here"><button type="button" class="btn btn-link p-0 plan-back">Plans</button><span class="hint">›</span><b>${esc(p.name)}</b></nav>
      <h2>${esc(p.name)}</h2>
      <span class="hint">${esc(p.scheduling?.periodStart || 'Period start not set')} · ${p.horizonWeeks} weeks</span>
      <button class="btn btn-secondary" type="button" id="plan-scenario" ${p.isDraft ? 'disabled' : ''}>${icon('copy')}Create scenario copy</button>
      <button class="btn btn-secondary" type="button" id="plan-linked-sheets" ${p.isDraft ? 'disabled' : ''}>Linked Google Sheets</button>
      <span id="plan-save-status" role="status" aria-live="polite" class="hint ${p.saveNotice?.error ? 'plan-warn' : ''}">${esc(p.saveNotice?.message || 'Working plan · saved. Edits autosave; baselines change only when you save an agreement.')}</span>
    </div>
    ${pendingDestinationHTML()}
    <details class="plan-setup"${(nTeams === 0 || nInit === 0) ? ' open' : ''}>
      <summary>Plan setup <span class="hint">${nTeams} pods · ${nInit} initiatives · ${(Math.round((p.capacityLoss || 0) * 100))}% capacity loss</span></summary>
      <p class="hint">Use the inputs below, or choose Linked Google Sheets above to maintain this plan from shared sheet ranges. For a new plan, link and apply the team roster before its initiatives.</p>
      <div class="row-actions flex-wrap align-items-center">
        <label class="d-flex align-items-center gap-2 mb-0">Period length <input class="form-control form-control-sm mw-100" style="width:5rem" id="plan-horizon" type="number" min="1" max="104" value="${p.horizonWeeks}"> weeks</label>
        <label class="d-flex align-items-center gap-2 mb-0">Capacity loss <input class="form-control form-control-sm mw-100" style="width:5rem" id="plan-loss" type="number" min="0" max="90" value="${Math.round((p.capacityLoss || 0) * 100)}">%</label>
        <button class="btn btn-secondary" id="plan-save">${icon('save')}Save settings</button>
      </div>
      <div class="plan-uploads">
        <div class="plan-step"><span class="plan-step-num">1</span>
          <div class="plan-step-body">
            <div class="plan-up" id="plan-roster-pick"></div>
            <div id="plan-sites"></div>
          </div>
        </div>
        <div class="plan-step"><span class="plan-step-num">2</span>
          <div class="plan-step-body">
            <div class="plan-up-init">
            ${uploadField('initiatives', `${icon('upload')}Initiatives (XLSX/CSV)`, nInit)}
            <button type="button" id="plan-init-preview" class="btn btn-secondary secondary" disabled
              title="render the network and order from this sheet, without saving">Preview</button>
            <span class="hint" id="plan-init-file"></span>
            <span class="plan-warn" id="plan-preview-warn" hidden></span>
          </div>
            <label class="hint plan-up" title="Drops dependency cells that don't match roster pods">
              <input class="form-check-input" type="checkbox" id="plan-strict-deps" ${current.strictDeps ? 'checked' : ''}> strict: match dependencies to roster
            </label>
            <p class="hint">strict drops dependency cells that don't match a roster pod name (case/whitespace-insensitive) — free text like "Requirements unknown" won't become a fake node in the network.</p>
            ${nTeams === 0 ? '<p class="plan-warn">Attach a roster first — dependency cells match pod names from the roster, so initiatives uploaded before one cannot resolve their deps.</p>' : ''}
          </div>
        </div>
      </div>
      <p class="hint">Then set the period start and assumptions in Plan commitments and read the proposed order.</p>
      <p class="hint">Need samples? <a href="/api/sample/teams.csv" download>teams.csv</a> · <a href="/api/sample/initiatives.xlsx" download>initiatives.xlsx</a></p>
    </details>
    ${current.isDraft ? `<p class="plan-warn">Previewing an unsaved initiatives upload — nothing is saved yet.
      <button id="plan-draft-save" class="btn btn-primary">Save initiatives</button>
      <button class="btn btn-secondary" id="plan-draft-discard">Discard</button></p>` : ''}
    ${unknown.length ? `<p class="plan-warn">${icon('warning')} ${unknown.length} pod(s) referenced by initiatives but missing from the roster: ${unknown.map(esc).join(', ')} — <button type="button" id="unknown-fix" class="btn btn-secondary warn-act">switch roster</button> or fix the sheet. <button type="button" class="btn btn-link p-0 usage-link" data-anchor="warnings">learn more</button></p>` : ''}
    ${nTeams > 0 && nInit > 0 ? `<div class="plan-views"><div class="btn-group" role="group" aria-label="Plan workspace">
      <button class="btn-secondary btn ${view() === 'order' ? 'active' : ''}" id="view-order" aria-pressed="${view() === 'order'}">Plan commitments</button><button class="btn-secondary btn ${view() === 'network' ? 'active' : ''}" id="plan-view-network" aria-pressed="${view() === 'network'}">Dependencies</button><button class="btn-secondary btn ${view() === 'timeline' ? 'active' : ''}" id="view-timeline" aria-pressed="${view() === 'timeline'}">Timeline</button><button class="btn-secondary btn ${view() === 'ready' ? 'active' : ''}" id="view-ready" aria-pressed="${view() === 'ready'}">Next work</button><button class="btn-secondary btn ${view() === 'execution' ? 'active' : ''}" id="view-execution" aria-pressed="${view() === 'execution'}">Review execution</button><button class="btn btn-secondary ${view() === 'assistant' ? 'active' : ''}" id="view-assistant" aria-pressed="${view() === 'assistant'}">Planning assistant</button><button class="btn-secondary btn" id="view-report" title="one printable card: verdicts, capacity, conflicts, remedies (spec 013)">${icon('report')}Report</button>
    </div>${baselineChipHTML(current.baselines)}</div>` : ''}
    ${nTeams === 0 ? `
      <div class="card p-3 panel-card plan-start">
        <h3>A plan is a roster + the initiatives you intend to run, sequenced by capacity.</h3>
        <p class="hint">Four steps: attach a roster (team composition, pinned as of today) → upload the initiatives matrix →
          review the proposed order and its verdicts → save the agreed order as a baseline. Nothing here writes to Jira.</p>
        <div class="plan-start-row">
          <button type="button" id="plan-start-demo" class="btn btn-primary">Start from the demo plan</button>
          <span class="hint">— or attach your own roster and upload your initiatives below, exactly as they are today.</span>
        </div>
      </div>` : ''
      }
    ${nTeams > 0 && nInit === 0 ? '<p class="hint">Roster loaded. Now upload the initiatives matrix.</p>' : ''}
    ${nTeams > 0 && nInit > 0 ? '<div id="plan-dash"></div>' : ''}`;

  root.querySelector('.plan-back').addEventListener('click', renderList);
  wirePendingDestination();
  // The empty-state's demo button (IA #5): the fastest honest path to seeing
  // what a plan does — same handler as the list's "Load demo plan". Wired here,
  // not in renderOrder: the empty state never renders the Order view.
  document.getElementById('plan-start-demo')?.addEventListener('click', async () => {
    const r = await req('/api/plan/demo', { method: 'POST' });
    if (!r || !r.ok) { alert('Could not create the demo plan'); return; }
    openPlan((await r.json()).id);
  });
  root.querySelector('#plan-save').addEventListener('click', savePlanParams);
  document.getElementById('plan-scenario')?.addEventListener('click', createScenario);
  document.getElementById('plan-linked-sheets')?.addEventListener('click', showLinkedSheets);
  root.querySelectorAll('#plan-horizon,#plan-loss').forEach(el=>el.addEventListener('input',()=>planNotice('Unsaved settings — choose Save settings to apply.')));
  // The missing-pod warning's fix (spec 009 AC 3.2): open setup at the roster.
  document.getElementById('unknown-fix')?.addEventListener('click', () => {
    const d = document.querySelector('.plan-setup');
    if (d) d.open = true;
    document.getElementById('plan-roster-pick')?.querySelector('select, button')?.focus();
  });
  pendingInitiativesFile = null; // a re-render clears the un-previewed pick
  root.querySelectorAll('input[type=file]').forEach((inp) => inp.addEventListener('change', () => {
    if (!inp.files[0]) return;
    if (inp.dataset.kind === 'initiatives') {
      // Review: selecting a file no longer drops the plan into draft preview —
      // the Preview button runs the validations (roster attached, strict
      // warning) and only then renders the graph.
      pendingInitiativesFile = inp.files[0];
      const btn = document.getElementById('plan-init-preview');
      if (btn) btn.disabled = false;
      const hint = document.getElementById('plan-init-file');
      if (hint) hint.textContent = inp.files[0].name + ' selected — click Preview';
      return;
    }
    uploadFile(inp.dataset.kind, inp.files[0]);
  }));
  // Preview (review): validates before rendering the graph — a roster must be
  // attached (step 1), and strict-off gets an explicit warning because free
  // text dependency cells would become phantom network nodes.
  document.getElementById('plan-init-preview')?.addEventListener('click', () => {
    if (!pendingInitiativesFile) return;
    const warnEl = document.getElementById('plan-preview-warn');
    const fail = (msg) => { if (warnEl) { warnEl.textContent = msg; warnEl.hidden = false; } };
    if (!(current.teams || []).length) {
      if (warnEl) { warnEl.textContent = 'Attach a roster first (step 1) — dependencies cannot resolve without pod names.'; warnEl.hidden = false; }
      return;
    }
    if (!current.strictDeps && !confirm('Strict matching is off: free-text dependency cells will become phantom nodes in the network. Render the preview anyway?')) {
      return;
    }
    if (warnEl) warnEl.hidden = true;
    previewInitiativesFile(pendingInitiativesFile);
  });
  document.getElementById('plan-draft-save')?.addEventListener('click', saveDraftInitiatives);
  document.getElementById('plan-draft-discard')?.addEventListener('click', () => openPlan(current.id));
  document.getElementById('plan-strict-deps')?.addEventListener('change', (e) => { current.strictDeps = e.target.checked; });
  document.getElementById('plan-view-network')?.addEventListener('click', () => setView('network'));
  document.getElementById('view-order')?.addEventListener('click', () => setView('order'));
  document.getElementById('view-timeline')?.addEventListener('click', () => setView('timeline'));
  document.getElementById('view-report')?.addEventListener('click', openHealthReport);
  document.getElementById('view-ready')?.addEventListener('click', () => setView('ready'));
  document.getElementById('view-execution')?.addEventListener('click', () => setView('execution'));
  document.getElementById('view-assistant')?.addEventListener('click', () => setView('assistant'));
  // The chip summarises a panel that only exists in the Order view, so it has to be
  // able to get there — otherwise it is a status message with no way through. The
  // scroll happens in renderOrder once the panel actually exists: with a stale
  // cached order the async path returns early and there is nothing to scroll to yet.
  // The baseline chip is wired by delegation (wireBaselineDelegation).
  renderRosterPicker(nTeams);
  renderPlanSites(nTeams);
  if (nTeams > 0 && nInit > 0) {
    current.levers = current.levers || [];
    current.netMode = current.netMode || 'after';
    renderCurrentPlanView();
  }
}

const view = () => ['network', 'timeline', 'ready', 'execution', 'assistant'].includes(current && current.view) ? current.view : 'order';

// specs/027-evidence-linked-planning-assistant.md:264: completed operations honor
// the current destination and preserve assistant question and evidence selections.
async function renderCurrentPlanView() {
 if(!current)return;
 switch(view()) {
  case 'order': return renderOrder();
  case 'timeline': return renderTimeline();
  case 'ready': return renderReadyQueue();
  case 'execution': return renderExecution();
  case 'assistant': {
   const refresh=document.querySelector('#plan-dash [data-assistant-refresh]');
   if(refresh){refresh.click();return;}
   return renderAssistant();
  }
  default: return renderDash();
 }
}

// specs/023-linked-google-sheets.md:241 — a late apply must not replace a
// different plan or an unsaved local draft when its source dialog finishes.
async function showLinkedSheets() {
  if (!current || current.isDraft) return false;
  const planID = current.id;
  const opened = await openLinkedSheets(planID, async () => {
    if (current?.id !== planID) return;
    const horizon = document.getElementById('plan-horizon');
    const loss = document.getElementById('plan-loss');
    const unsavedSettings = (horizon && Number(horizon.value) !== current.horizonWeeks)
      || (loss && Number(loss.value) !== Math.round((current.capacityLoss || 0) * 100));
    if (current.isDraft || pendingInitiativesFile || unsavedSettings) {
      planNotice('Linked sheet applied on the server. Finish or discard your local draft, then reopen the plan to load it.');
      return;
    }
    await openPlan(planID);
  });
  if (opened) window.dispatchEvent(new CustomEvent('conway:feature-opened', { detail: { action: 'linked-sheets' } }));
  return opened;
}

function proposalModal(title, content) {
  let ov = document.getElementById('plan-proposal-overlay');
  if (!ov) { ov = document.createElement('div'); ov.id = 'plan-proposal-overlay'; ov.className = 'modal-overlay'; document.body.appendChild(ov); }
  ov.proposalToken = Symbol('proposal');
  ov.innerHTML = `<div class="modal-box"><div class="modal-head"><h2 id="plan-proposal-title">${esc(title)}</h2><button type="button" class="btn btn-secondary proposal-close">${icon('close')}Close</button></div>${content}</div>`;
  ov.setAttribute('aria-labelledby', 'plan-proposal-title');
  ov.querySelector('.proposal-close').addEventListener('click',()=>closeModal(ov));
  openModal(ov);
  return ov;
}

async function createScenario() {
  if (!current || current.isDraft) return;
  const planId = current.id;
  const ov = proposalModal('Create scenario copy', `<p>A scenario starts with this working plan’s saved inputs. It has its own changes and no inherited agreement.</p><form id="scenario-form"><label>Scenario name <input class="form-control" name="name" required maxlength="100" value="${esc(current.name.slice(0,80))} — scenario"></label><p class="proposal-status" role="status" aria-live="polite"></p><button type="submit" class="btn btn-primary">Create and open scenario</button></form>`);
  const token = ov.proposalToken;
  ov.querySelector('form').addEventListener('submit',async ev=>{
    ev.preventDefault(); const form=ev.currentTarget, button=form.querySelector('button'), status=form.querySelector('.proposal-status');
    button.disabled=true; status.textContent='Creating scenario…';
    const r = await req(`/api/plan/${encodeURIComponent(planId)}/scenario`,{method:'POST',body:JSON.stringify({name:form.elements.name.value.trim()})});
    if (!r?.ok) { status.textContent=r ? (await r.text()).slice(0,240) : 'Could not reach the server. Try again.'; status.setAttribute('role','alert'); button.disabled=false; return; }
    const result=await r.json();
    if (ov.proposalToken !== token || ov.hidden || current?.id !== planId) return;
    closeModal(ov);
    dragUndo=null; dragHistory=[]; await openPlan(result.id);
  });
}

async function previewRemedy(remedy) {
  if (!current || !remedy || current.isDraft || current.levers?.length) return;
  const planId=current.id, epoch=orderEpoch;
  const ov=proposalModal('Preview proposed change','<p class="proposal-status" role="status">Calculating consequences…</p>');
  const token = ov.proposalToken;
  const live = () => current?.id === planId && orderEpoch === epoch && ov.proposalToken === token && !ov.hidden;
  const r=await req(`/api/plan/${encodeURIComponent(planId)}/schedule/remedies/preview`,{method:'POST',body:JSON.stringify({remedy})});
  if (!r?.ok) {
    const message = r ? (await r.text()).slice(0,240) : 'Could not load preview. Close and try again.';
    if (live()) ov.querySelector('.proposal-status').textContent=message;
    return;
  }
  const result=await r.json();
  if(!live()) return;
  const status=ov.querySelector('.proposal-status');
  status.outerHTML=`<p>${esc(remedy.note || remedy.kind)} · target ${esc(remedy.target)}</p><p class="hint">Review every affected commitment. Applying saves the working inputs; your agreed baseline stays unchanged.</p>${compareTableHTML({baseline:{name:'Current working plan'},to:{name:'Proposed change'},comparison:result.comparison})}<p class="proposal-status" role="status" aria-live="polite">Preview only. Nothing has been applied.</p><button type="button" class="btn btn-primary" id="remedy-apply">Apply to working plan</button>`;
  ov.querySelector('#remedy-apply').addEventListener('click',async ev=>{
    const button=ev.currentTarget, note=ov.querySelector('.proposal-status');
    if(!live()) { note.textContent='The working plan changed. Close and preview this remedy again.'; return; }
    button.disabled=true; note.textContent='Applying change…';
    const applied=await req(`/api/plan/${encodeURIComponent(planId)}/schedule/remedies/apply`,{method:'POST',body:JSON.stringify({remedy:result.remedy,fingerprint:result.fingerprint})});
    if(!applied?.ok) { note.textContent=applied ? (await applied.text()).slice(0,240) : 'Could not save. Your preview is still here; try again.'; note.setAttribute('role','alert'); button.disabled=false; return; }
    await applied.json(); if (ov.proposalToken === token && !ov.hidden) closeModal(ov);
    if(current?.id === planId) { dragUndo=null; dragHistory=[]; await openPlan(planId); }
  });
}

function openTeamReady(team) {
  current.tlTeamFilter = team;
  setView('ready');
}

function renderReadyQueue() {
  const host = document.getElementById('plan-dash'), plan = current;
  if (!host || !plan) return;
  if (plan.isDraft) { host.innerHTML = '<p class="plan-warn">Save or discard the upload preview before assessing release decisions against saved inputs.</p>'; return; }
  mountReadyQueue(host, {plan, request:req,
    onContext:(team, week) => {
      if (current?.id !== plan.id) return;
      current.tlTeamFilter = team;
      writeRoute({team, readyWeek:week});
    },
    onInspect:(team, initiative) => {
      current.tlLens = 'pod'; current.tlTeamFilter = team;
      current.tlInitiativeFilter = initiative; current.selectedInitiative = initiative;
      setView('timeline');
    },
    onReview:team => { current.tlTeamFilter = team; setView('execution'); }
  });
  window.dispatchEvent(new CustomEvent('conway:feature-opened', {detail:{action:'ready'}}));
}

function renderAssistant() {
 const host=document.getElementById('plan-dash'),plan=current;
 if(!host||!plan)return;
 if(plan.isDraft){host.innerHTML='<p class="alert alert-warning">Save or discard the upload preview before asking about saved planning inputs.</p>';return;}
 disposeAssistant?.();
 disposeAssistant = mountPlanningAssistant(host,{plan,request:req,getIdentity:authToken,live:()=>current===plan&&view()==='assistant'&&!root.hidden});
 window.dispatchEvent(new CustomEvent('conway:feature-opened',{detail:{action:'assistant'}}));
}

function renderExecution() {
  const host=document.getElementById('plan-dash'), plan=current;
  if(!host || !plan) return;
  if(plan.isDraft) { host.innerHTML='<p class="plan-warn">Save or discard the upload preview before reviewing execution against saved inputs.</p>'; return; }
  mountExecution(host,{plan,request:req,onImport:openImport,onAgreement:()=>setView('order'),
    onSnapshot:id=>writeRoute({executionSnapshot:id}),
    onTeam:team=>{
      if(current?.id !== plan.id) return;
      current.tlTeamFilter = team;
      writeRoute({team});
    },
    onBindingsSaved:result=>{
      if(current?.id !== plan.id) return;
      if(Array.isArray(result.initiatives)) current.initiatives=result.initiatives;
      dragUndo=null; dragHistory=[]; staleOrder();
      loadBaselines().then(()=>{ if(current?.id === plan.id) { const chip=document.getElementById('bl-chip'); if(chip) chip.outerHTML=baselineChipHTML(current.baselines); } });
    }});
  window.dispatchEvent(new CustomEvent('conway:feature-opened', { detail: { action: 'execution' } }));
}

// Spec 012 FR-004: one-time callouts. Dismissal persists per session.
function wireCallouts(host) {
  host.querySelectorAll('.callout-dismiss').forEach((b) =>
    b.addEventListener('click', () => {
      sessionStorage.setItem(`conway-callout-${b.dataset.dismiss}`, '1');
      host.querySelector(`[data-callout="${b.dataset.dismiss}"]`)?.remove();
    }));
}

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
  const body = schedulingFromForm((id) => document.getElementById(id)?.value, current.scheduling);
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
  dragUndo = null; dragHistory = []; // saved assumptions supersede any drag snapshot (spec 008 S4, FR-006)
  current.scheduling = saved.scheduling || body;
  current.calDraft = null; // the draft is saved now; the form reads the policy
  // The carried assumptions are saved too: leaving them would make the next
  // render restore the pre-save snapshot over the fresh policy.
  current.assumptionDraft = null;
  staleOrder(); // the assumptions moved, so the order has to be recomputed
  renderCurrentPlanView();
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
  refreshBaselinesDrawer(); // an open drawer must not show a stale list (AC 2.2)
}

// Baselines drawer (spec 015): ONE home for saving, history, activation and
// comparison. It slides over the Order view — which stays visible, because
// activation and comparison are decisions made about the order — and is
// appended to the document body, so Order re-renders cannot strand it (AC 2.2).
// The chip opens it in one click from any view (FR-001).
function baselinesDrawerOpen() {
  return !!document.querySelector('.bl-drawer-overlay');
}

function openBaselinesDrawer() {
  if (!current) return;
  if (baselinesDrawerOpen()) { refreshBaselinesDrawer(); return; }
  const overlay = document.createElement('div');
  overlay.className = 'bl-drawer-overlay';
  overlay.innerHTML = baselinesDrawerHTML(current.baselines, current.baselineCompare, { draft: current.isDraft });
  document.body.appendChild(overlay);
  overlay.querySelector('.bl-drawer-close')?.addEventListener('click', closeBaselinesDrawer);
  containFocus(overlay,closeBaselinesDrawer);
  // Backdrop click closes; clicks inside the drawer do not.
  overlay.addEventListener('click', (ev) => { if (ev.target === overlay) closeBaselinesDrawer(); });
  overlay.querySelector('#bl-drawer-name')?.focus();
}

function closeBaselinesDrawer() {
  document.querySelector('.bl-drawer-overlay')?.remove();
  document.getElementById('bl-chip')?.focus();
}

// refreshBaselinesDrawer re-renders the open drawer's content from the current
// state — the drawer is outside the re-rendered dash, so it must refresh
// itself whenever baselines move (save, activate, delete).
function refreshBaselinesDrawer() {
  const overlay = document.querySelector('.bl-drawer-overlay');
  if (!overlay) return;
  overlay.innerHTML = baselinesDrawerHTML(current.baselines, current.baselineCompare, { draft: current.isDraft });
  overlay.querySelector('.bl-drawer-close')?.addEventListener('click', closeBaselinesDrawer);
  overlay.addEventListener('click', (ev) => { if (ev.target === overlay) closeBaselinesDrawer(); });
}

// saveBaselinesDrawerSave persists the drawer's named snapshot (FR-003): a
// draft preview is refused, a name is required, and a 405 names its dominant
// cause (a checkout updated the page while the server binary is old).
async function saveBaselinesDrawer() {
  const name = document.getElementById('bl-drawer-name');
  const errEl = document.querySelector('.bl-drawer-overlay .bl-drawer-error');
  const fail = (msg) => { if (errEl) { errEl.textContent = msg; errEl.hidden = false; } };
  if (current.isDraft) {
    fail('Save the uploaded initiatives first — a baseline freezes what is stored, not the preview.');
    return;
  }
  const value = (name?.value || '').trim();
  if (!value) { name?.focus(); fail('Give the baseline a name — it is how this period\u2019s agreed order is referred to later.'); return; }
  const saveBtn = document.getElementById('bl-save');
  if (saveBtn) { saveBtn.disabled = true; saveBtn.textContent = 'Saving…'; }
  const forPlan = current.id;
  const r = await req('/api/plan/' + forPlan + '/baseline', {
    method: 'POST', body: JSON.stringify({ name: value, ...orderRequestBody() }),
  });
  if (!current || current.id !== forPlan) return;
  if (!r || !r.ok) {
    const why = r ? await r.text() : 'the request did not reach the server';
    fail(saveErrorMessage(r ? r.status : 0, why, 'save'));
    if (saveBtn) { saveBtn.disabled = false; saveBtn.textContent = 'Save current order'; }
    return;
  }
  await loadBaselines();
  current.baselineCompare = null;
  renderPlan(); // the chip changes too
  refreshBaselinesDrawer();
}

// deleteBaseline is a two-step in-place confirmation (spec 015 Decision 2):
// the first click arms the row's button, the second deletes. Any re-render
// disarms it. Deleting the active baseline leaves the plan with none — an
// honest state the chip reports (Q1 default).
async function deleteBaseline(btn) {
  if (btn.dataset.confirm !== '1') {
    btn.dataset.confirm = '1';
    btn.textContent = 'Confirm delete?';
    btn.classList.add('bl-delete-armed');
    return;
  }
  const forPlan = current.id;
  const r = await req('/api/plan/' + forPlan + '/baseline/' + btn.dataset.id, { method: 'DELETE' });
  if (!current || current.id !== forPlan) return;
  if (!r || !r.ok) { await baselineError(r, 'delete'); return; }
  await loadBaselines();
  current.baselineCompare = null;
  renderPlan();
  refreshBaselinesDrawer();
}

// Baseline controls are wired by DELEGATION (lesson 020): the Order view
// re-renders constantly, and per-render addEventListener wiring is how saving
// went dead for three weeks — a refactor replaced the wiring call and nothing
// failed loudly. One document-level handler survives every re-render.
function wireBaselineDelegation() {
  document.addEventListener('click', (ev) => {
    const t = ev.target;
    if (!(t instanceof Element)) return;
    if (t.closest('#bl-save')) { saveBaselinesDrawer(); return; }
    const del = t.closest('.bl-delete');
    if (del) { deleteBaseline(del); return; }
    if (t.closest('#bl-chip')) { openBaselinesDrawer(); return; }
    const act = t.closest('.bl-activate');
    if (act) { activateBaseline(act.dataset.id); return; }
    const cmp = t.closest('.bl-compare');
    if (cmp) { onCompareClick(cmp); return; }
  });
  document.addEventListener('change', (ev) => {
    const sel = ev.target instanceof Element ? ev.target.closest('.bl-vs-sel') : null;
    if (sel) onVsChange(sel);
  });
}

// onCompareClick: comparing the already-compared baseline dismisses the card —
// a second click that does nothing reads as broken (button audit, 2026-08-23).
function onCompareClick(btn) {
  if (current.baselineCompare && current.baselineCompare.baseline?.id === btn.dataset.id) {
    current.baselineCompare = null;
    renderCurrentPlanView();
    return;
  }
  compareBaseline(btn.dataset.id);
}

// onVsChange: pairwise baseline compare (spec 005). Both schedules are stored,
// so this never touches the live plan and needs no orderEpoch guard. It does
// need the compare gate: choose a second pair while the first request is out
// and the slower reply would otherwise land last and overwrite the newer card.
async function onVsChange(sel) {
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
  renderCurrentPlanView();
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
  renderCurrentPlanView();
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
    btn.setAttribute('aria-expanded', 'false');
    return;
  }
  document.querySelectorAll('tr.ord-remedies').forEach((el) => el.remove());
  document.querySelectorAll('.ord-options').forEach((b) => { b.textContent = 'options ▾'; b.setAttribute('aria-expanded', 'false'); });
  btn.textContent = 'options ▴';
  btn.setAttribute('aria-expanded', 'true');
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
    btn.setAttribute('aria-expanded', 'false');
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
    live.setAttribute('aria-expanded', 'true');
  }
  if (live) live.disabled = false;
  if (!r || !r.ok) {
    body.innerHTML = `<span class="plan-warn">${remediesErrorMessage(r ? r.status : 0, r ? await r.text() : '')}</span>`;
    return;
  }
  const out = await r.json();
  body.innerHTML = remediesPanelHTML(out.remedies, out.warnings);
  body.querySelectorAll('.rem-preview').forEach(b => b.addEventListener('click', () => previewRemedy(out.remedies[Number(b.dataset.remedy)])));
  if (current.isDraft || current.levers?.length) body.querySelectorAll('.rem-preview').forEach(b=>{b.disabled=true;b.title='Save the upload or clear temporary network levers before previewing a persisted remedy.';});
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
  rememberPlanRoute();
  renderPlan();
}

// ASSUMPTION_FIELDS are the scheduling form's inputs. carryLiveAssumptions
// snapshots them so an add/del re-render cannot drop a planner's edits —
// including a cleared field, which is an edit, not a reversion.
const ASSUMPTION_FIELDS = ['sched-period-start', 'sched-wip-model', 'sched-wip', 'sched-buffer',
  'sched-kit', 'sched-pod-wip', 'sched-quarter', 'sched-estimate-model', 'sched-split-tax',
  'sched-chunking', 'sched-split-min', 'sched-stagger',
  'sched-lead-mode', 'sched-lead-pm', 'sched-lead-eng', 'sched-lead-architect', 'sched-lead-pgm'];

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
  if (!host || !current || view() !== 'timeline') return;
  // Spec 012 FR-004: first-visit callout, dismissed once per session.
  const callout = (key, text) => {
    const k = `conway-callout-${key}`;
    if (sessionStorage.getItem(k)) return '';
    return `<p class="plan-warn callout" data-callout="${key}">${text}
      <button type="button" class="btn btn-secondary callout-dismiss" data-dismiss="${key}">got it</button></p>`;
  };
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
    if (view() !== 'timeline') return;
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
  host.innerHTML = `
    ${timelineControlsHTML({ lens, horizon, spans, spanSel, initiativeFilter: current.tlInitiativeFilter, teamFilter: current.tlTeamFilter, hideEmpty: current.tlHideEmpty, ghost: current.tlGhost })}
    <p class="hint">Edits autosave to the working plan. <button class="btn btn-secondary" type="button" id="tl-undo" ${!dragHistory.length || current.isDraft ? 'disabled' : ''}>${icon('undo')} Undo last edit${dragHistory.length ? ` (${dragHistory.length} available)` : ''}</button></p>
    <p id="tl-drag-note" role="status" hidden></p>
    <div id="tl-main"></div>
    <div id="tl-inspector"></div>
    <div id="tl-pod"></div>`;
  host.insertAdjacentHTML('afterbegin', callout('timeline', 'Select an initiative for its dates and precise editing controls. Filter by initiative and team; both filters persist when grouping changes. Changes autosave to the working plan and can be undone.'));
  document.getElementById('tl-undo')?.addEventListener('click', undoDrag);
  host.querySelectorAll('[data-tlspan]').forEach((b) =>
    b.addEventListener('click', () => { current.tlSpan = b.dataset.tlspan; renderTimeline(); }));
  wireCallouts(host);
  // Fullscreen (spec 008): lane-accurate dragging needs the real estate. ESC
  // exits — the stable document-level keydown lives in initPlanUI so the
  // re-render never stacks handlers.
  // Lens filters (spec 010): view state, debounced re-render, live counts.
  // The re-render replaces the input node, so focus and caret are restored
  // after it — otherwise every keystroke kicks the planner out of the box.
  for (const [id, field] of [['tl-initiative-filter', 'tlInitiativeFilter'], ['tl-team-filter', 'tlTeamFilter']]) {
    const input = document.getElementById(id);
    input?.addEventListener('input', () => {
      current[field] = input.value;
      rememberPlanRoute(true);
      clearTimeout(renderTimeline._filterT);
      renderTimeline._filterT = setTimeout(() => {
        // The timer can outlive its lens: a switch or plan change replaced
        // this input. Only act if it is still the live filter (cubic P2).
        if (!current || document.getElementById(id) !== input) return;
        const caret = input.selectionStart;
        renderTimeline().then(() => {
          const live = document.getElementById(id);
          if (live && current) {
            live.focus();
            live.setSelectionRange(caret, caret);
          }
        });
      }, 120);
    });
  }
  document.getElementById('tl-hide-empty')?.addEventListener('change', (ev) => {
    current.tlHideEmpty = ev.target.checked;
    renderTimeline();
  });
  document.getElementById('tl-ghost')?.addEventListener('change', (ev) => {
    current.tlGhost = ev.target.checked;
    renderTimeline();
  });
  document.getElementById('tl-fullscreen')?.addEventListener('click', () => {
    host.classList.toggle('tl-fullscreen');
    const btn = document.getElementById('tl-fullscreen');
    if (btn) btn.textContent = host.classList.contains('tl-fullscreen') ? 'Exit full screen (Escape)' : 'Full screen';
  });

  // pinnedLanesByPod inverts the stored per-initiative PinnedLanes into the
  // per-pod map assignLanes consumes.
  const pinnedLanesByPod = () => {
    const out = Object.create(null);
    for (const it of (current.initiatives || [])) {
      for (const [pod, off] of Object.entries(it.pinnedLanes || {})) {
        (out[pod] ||= Object.create(null))[it.name] = off;
      }
    }
    return out;
  };
  const applyTimelineEdit = async (it, edit, pod) => {
    if (timelineMutationPending || current.isDraft) return false;
    const forPlan = current.id, atEpoch = orderEpoch;
    const undo = snapshotDragUndo(it, pod);
    timelineMutationPending = true;
    try {
      const r = await req('/api/plan/' + forPlan + '/initiatives', {
        method: 'PATCH', body: JSON.stringify({ initiatives: [edit] }),
      });
      if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return false;
      if (!r?.ok) {
        const why = r ? (await r.text()).slice(0, 200) : 'The request did not reach the server. Your inputs remain available to retry.';
        if (current?.id === forPlan && orderEpoch === atEpoch) dragNote(why);
        return false;
      }
      const d = await r.json();
      if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return false;
      if (!Array.isArray(d.initiatives)) throw new Error('The saved inputs could not be read. Reload the plan before editing again.');
      current.initiatives = d.initiatives;
      dragHistory.push(undo);
      dragUndo = undo;
      staleOrder();
      await renderCurrentPlanView();
      return true;
    } catch (error) {
      if (current?.id === forPlan) dragNote(error.message || 'The edit could not be saved. Your inputs remain available to retry.');
      return false;
    } finally {
      timelineMutationPending = false;
    }
  };
  const paintInspector = () => {
    const holder = document.getElementById('tl-inspector');
    if (!holder) return;
    const si = (sched.initiatives || []).find((i) => i.name === current.selectedInitiative);
    holder.innerHTML = timelineInspectorHTML(si, sched, { planInitiatives: current.initiatives, pinnedLanes: pinnedLanesByPod() });
    const form = holder.querySelector('.tl-precise-edit');
    if (current.isDraft) {
      form?.querySelectorAll('input,button').forEach((el) => { el.disabled = true; });
      if (form) form.insertAdjacentHTML('beforebegin', '<p class="hint">Save the imported inputs before editing the timeline.</p>');
    }
    form?.addEventListener('submit', async (ev) => {
      ev.preventDefault();
      if (timelineMutationPending || current.isDraft) return;
      const it = (current.initiatives || []).find((i) => i.name === form.dataset.init);
      if (!it || !form.reportValidity()) return;
      try {
        const rows = [...form.querySelectorAll('.tl-edit-row')].map((row) => {
          const estimate = row.querySelector('[name="estimateWeeks"]');
          return { pod: row.dataset.pod, startWeek: row.querySelector('[name="startWeek"]').value, lane: row.querySelector('[name="lane"]').value, ...(estimate.disabled ? {} : { estimateWeeks: estimate.value }) };
        });
        const edit = timelineEditsFromRows(it, rows);
        const submit = form.querySelector('[type="submit"]');
        submit.disabled = true;
        try { await applyTimelineEdit(it, edit); } finally { submit.disabled = false; }
      } catch (error) { dragNote(error.message); }
    });
  };
  // dragNote is module level (used by the drag callbacks and undoDrag).
  const paint = () => {
    const main = document.getElementById('tl-main');
    if (!main) return;
    // Spec 001 Q17: an unchosen WIP model schedules as strict while the choice
    // is demanded — the Order view's set-up card demands it, and so must the
    // timeline, or a fresh plan renders two bars and reads as a broken filter.
    document.getElementById('tl-wip-banner')?.remove();
    if (sched.wipLimit?.model === 'unchosen') {
      main.insertAdjacentHTML('beforebegin', `<p class="plan-warn callout" id="tl-wip-banner">The WIP model hasn't been chosen for this plan — the scheduler is holding all but ${sched.wipLimit.value} concurrent initiatives, so most bars are missing. <button type="button" class="btn btn-link p-0 usage-link" id="tl-choose-wip">choose it now</button></p>`);
      document.getElementById('tl-choose-wip')?.addEventListener('click', () => {
        current.setupFocus = true;
        setView('order');
      });
      // and one click deeper: the assumptions dialog owns the WIP model choice
      document.getElementById('tl-choose-wip')?.addEventListener('click', () => {
        setTimeout(() => document.getElementById('sched-open')?.click(), 400);
      }, { once: true });
    }
    main.innerHTML = lens === 'pod'
      ? podLensHTML(sched, {
        horizonWeeks: horizon, span: spanWeeks, pinnedLanes: pinnedLanesByPod(), initiativeQuery: current.tlInitiativeFilter || '', podQuery: current.tlTeamFilter || '', hideEmptyPods: current.tlHideEmpty,
        todayWeek, calendars: (current.scheduling || {}).calendars || [],
        // Spec 010 amendment: non-matching bars render as dimmed ghosts when
        // "show other work" is on — the capacity filling the gaps (e.g., what
        // holds a pod while the filtered initiative waits) stays visible.
        ghostOthers: current.tlGhost,
        // Spec 008 S4: the resize gesture needs each slice's ABSOLUTE effort
        // weeks (estimateEdits is pod -> effort), which the schedule's slices
        // do not carry — only the plan's initiatives do.
        planInitiatives: current.initiatives || [],
      })
      : portfolioTimelineHTML(sched, {
        podQuery: current.tlTeamFilter || '', initiativeQuery: current.tlInitiativeFilter || '', selected: current.selectedInitiative,
        planInitiatives: current.initiatives || [],
        horizonWeeks: horizon, span: spanWeeks, todayWeek, expand: current.tlExpand,
        // AC 8.5: the bands come off the saved policy, not the schedule — the
        // schedule itself only carries the windows' effects, not their dates.
        calendars: (current.scheduling || {}).calendars || [],
      });
    // AC 8.4: expanding a row shows its pod slices. One open row at a time, so
    // the lens stays readable — the wireframe is one expanded initiative.
    main.querySelectorAll('[data-select-init], .tl-bar[data-initiative]').forEach((el) => {
      el.setAttribute('aria-pressed', String((el.dataset.selectInit || el.dataset.initiative) === current.selectedInitiative));
      const select = (ev) => {
        ev.stopPropagation();
        const name = el.dataset.selectInit || el.dataset.initiative;
        current.selectedInitiative = name;
        if (el.dataset.selectInit) current.tlExpand = current.tlExpand === name ? null : name;
        rememberPlanRoute();
        paint();
        const focus = [...main.querySelectorAll('[data-select-init], .tl-bar[data-initiative]')].find((item) => (item.dataset.selectInit || item.dataset.initiative) === name);
        focus?.focus({ preventScroll: true });
      };
      el.addEventListener('click', select);
      if (el.matches('.tl-bar')) el.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); select(ev); }
      });
    });
    paintInspector();
    // The pod lens's pod blocks open §13.5's sheet (AC 9.1 -> AC 9.5);
    // clicking the open pod again closes it, so the grid is never stuck.
    main.querySelectorAll('.tl-pod[data-pod]').forEach((el) =>
      el.addEventListener('click', (ev) => {
        if (ev.target.closest('button, .tl-bar')) return;
        if (current.tlPod === el.dataset.pod) { current.tlPod = null; const h = document.getElementById('tl-pod'); if (h) h.innerHTML = ''; return; }
        current.tlPod = el.dataset.pod;
        paintPodSheet(el.dataset.pod);
      }));
    main.querySelectorAll('[data-open-pod]').forEach((button) => button.addEventListener('click', (ev) => {
      ev.stopPropagation();
      current.tlPod = button.dataset.openPod;
      paintPodSheet(current.tlPod);
      const sheet = document.querySelector('[data-pod-sheet]');
      sheet?.focus();
    }));
    // Filter match count (spec 010 FR-005).
    const countEl = document.getElementById('tl-filter-count');
    if (countEl) {
      const iq = current.tlInitiativeFilter || '', tq = current.tlTeamFilter || '';
      const n = (sched.initiatives || []).filter((si) => (!iq || initiativeMatch(iq, si.name)) && matchesTimelineTeam(si, tq, current.initiatives)).length;
      countEl.textContent = `${n} of ${(sched.initiatives || []).length} initiatives match`;
    }
    // Spec 008: drag-to-edit. A released drag pins the slice's start and the
    // engine recomputes; the re-render repaints every view from one schedule.
    // Spec 008 S4 (Decision 4): the right-edge resize PATCHes the pod's
    // estimate (the engine re-divides by lanes), and the left edge moves the
    // start while shrinking the estimate so the finish anchors.
    main.dataset.horizon = String(spanWeeks);
    attachDrag(main, {
      readOnly: current.isDraft, // matchMedia('.pointer: coarse)') gate lives inside attachDrag
      onPreview: dragNote,

      horizon: spanWeeks,
      // Decision 4 math: the plan's own capacity loss, not the 10% default.
      lossFactor: 1 - (Number.isFinite(current.capacityLoss) ? current.capacityLoss : 0.1),
      onPin: async (initiative, pod, { startWeek, laneDelta, effort }, origin) => {
        const it = (current.initiatives || []).find((i) => i.name === initiative);
        if (!it) return;
        const edit = { name: initiative };
        if (startWeek !== null && startWeek !== undefined) {
          edit.pinnedStarts = { ...(it.pinnedStarts || {}), [pod]: startWeek };
        }
        if (laneDelta) {
          // The new pod-relative offset: current packed lane + delta, floored
          // at 0. The server refuses drops that overlap other work (409).
          const curLane = Number(origin?.lane ?? 0);
          const offset = Math.max(0, curLane + laneDelta);
          edit.pinnedLanes = { ...(it.pinnedLanes || {}), [pod]: offset };
        }
        // A left-edge drag (Q2): the estimate shrinks so the finish anchors.
        if (effort !== undefined && Number.isFinite(effort)) {
          edit.estimateEdits = { [pod]: Math.max(1, effort) };
        }
        await applyTimelineEdit(it, edit, pod);
      },
      // Spec 008 S4: right-edge resize PATCHes the pod's effort weeks. The
      // server accepts estimateEdits and re-divides by lanes.
      onResize: async (initiative, pod, newEffort) => {
        if (!Number.isFinite(newEffort)) return; // a mid-render gesture, not an edit
        const it = (current.initiatives || []).find((i) => i.name === initiative);
        if (!it) return;
        await applyTimelineEdit(it, { name: initiative, estimateEdits: { [pod]: Math.max(1, newEffort) } }, pod);
      },
    });
    // FR-043 (spec 004 Story 3): each pod block exports itself as a PNG. The
    // click must not also open the sheet, so it stops here.
    main.querySelectorAll('.pod-export[data-export-pod]').forEach((b) =>
      b.addEventListener('click', (ev) => {
        ev.stopPropagation();
        const pod = b.dataset.exportPod;
        exportBlockPNG(b.closest('.tl-pod'), `conway-${pod.replace(/\W+/g, '-').toLowerCase()}-timeline.png`).then((ok) => {
          if (!ok) dragNote('The timeline image could not be downloaded. Try again.');
        });
      }));
  };
  // The pod toggle (open/close) and the lens-switch redraw share ONE renderer —
  // duplicating the markup without the wiring left the redrawn sheet's PNG
  // button inert.
  const paintPodSheet = (pod) => {
    const holder = document.getElementById('tl-pod');
    if (!holder) return;
    const ps = (sched.podWeeks || []).find((p) => p.pod === pod);
    holder.innerHTML = ps ? podSheetHTML(ps, sched, { horizonWeeks: horizon, span: spanWeeks, planInitiatives: current.initiatives || [] }) : '';
    if (ps) {
      const next = document.createElement('button');
      next.type = 'button'; next.className = 'btn btn-secondary'; next.textContent = `Next work for ${pod}`;
      next.addEventListener('click', () => openTeamReady(pod));
      holder.prepend(next);
    }
    holder.querySelectorAll('.pod-export[data-export-sheet]').forEach((b) =>
      b.addEventListener('click', () => {
        exportBlockPNG(b.closest('[data-pod-sheet]'), `conway-${pod.replace(/\W+/g, '-').toLowerCase()}-sheet.png`).then((ok) => {
          if (!ok) dragNote('The team sheet image could not be downloaded. Try again.');
        });
      }));
  };
  paint();
  // A pod sheet open from before a lens switch stays open: render it directly
  // rather than through the toggle, which would read the selection as a
  // second click and clear it.
  if (current.tlPod) paintPodSheet(current.tlPod);

  // Independent filters keep their meaning across both grouping choices.
  document.getElementById('tl-by-initiative')?.addEventListener('click', () => { current.tlLens = 'initiative'; rememberPlanRoute(); renderTimeline(); });
  document.getElementById('tl-by-pod')?.addEventListener('click', () => { current.tlLens = 'pod'; rememberPlanRoute(); renderTimeline(); });
}

// openHealthReport renders the spec-013 health card from the CACHED schedule
// (never recomputing one — NFR-001; a plan without a schedule is routed to the
// Order view, which computes and caches it), then fills the remedies section
// from the per-click remedies endpoint. The overlay is a modal dialog with
// focus management (Decision 2): focus moves in on open, Tab is trapped, and
// view-report regains it on close — so no control beneath the card can take
// keyboard input while it is up (AC 3.2). Mouse users were already safe.
async function openHealthReport() {
  if (!current) return;
  if (!current.schedule) { setView('order'); return; }
  const dash = document.getElementById('plan-dash');
  if (!dash) return;
  const forPlan = current.id;
  const atEpoch = orderEpoch;
  const overlay = document.createElement('div');
  overlay.className = 'report-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', 'Plan health report');
  overlay.innerHTML = `
    <div class="report-actions no-print">
      <button class="btn btn-secondary" type="button" id="report-print">${icon('report')}Print</button>
      <button class="btn btn-secondary" type="button" id="report-close">Close</button>
    </div>
    ${healthReportHTML(current.schedule, {
      planName: current.name, baselines: current.baselines,
      generatedAt: new Date().toLocaleString(),
    })}`;
  dash.appendChild(overlay);
  const close = () => { overlay.remove(); document.getElementById('view-report')?.focus(); };
  const printBtn = overlay.querySelector('#report-print');
  const closeBtn = overlay.querySelector('#report-close');
  closeBtn?.addEventListener('click', close);
  printBtn?.addEventListener('click', () => window.print());
  containFocus(overlay,close);
  // The remedies link-back (AC 1.5): a row's "full options" opens the Order
  // view's priced-options panel for that initiative — the summary stays a
  // summary, the decision happens where the costs are priced.
  overlay.querySelector('.report-card')?.addEventListener('click', (ev) => {
    const btn = ev.target.closest?.('.report-remedy-link');
    if (!btn) return;
    const target = btn.dataset.target;
    close();
    setView('order');
    showRemediesFor(target);
  });
  printBtn?.focus();
  // Remedies are per-click and stateless server-side (same contract as the
  // Order view's panels): fill the section when the answer lands, name the
  // failure if it does not (NFR-003). A stale answer is discarded unread.
  try {
    const r = await req('/api/plan/' + forPlan + '/schedule/remedies', {
      method: 'POST', body: JSON.stringify(orderRequestBody()),
    });
    if (!current || current.id !== forPlan || orderEpoch !== atEpoch || !overlay.isConnected) return;
    const slot = overlay.querySelector('#report-remedies');
    if (!slot) return;
    if (!r || !r.ok) {
      const why = r ? await r.text() : 'the request did not reach the server';
      slot.innerHTML = remediesSectionHTML({ error: why.slice(0, 200) });
      return;
    }
    let data = null;
    try { data = await r.json(); } catch { data = { error: 'the server sent something this cannot read' }; }
    slot.innerHTML = remediesSectionHTML(data);
  } catch { /* a network raise leaves the card's own note standing */ }
}

// showRemediesFor opens the Order view's remedy panel for one initiative — the
// link-back target of the health report's top-remedies rows (spec 013 AC 1.5).
// setView('order') paints asynchronously, so the [options ▾] expander is looked
// up with a short retry, then clicked: toggleRemedies prices and mounts the
// panel exactly as a manual expand would.
function showRemediesFor(name) {
  const tryOpen = (attempt) => {
    const btn = document.querySelector(`.ord-options[data-init="${CSS.escape(name)}"]`);
    if (!btn) {
      if (attempt < 10) setTimeout(() => tryOpen(attempt + 1), 200);
      return;
    }
    btn.click();
  };
  tryOpen(0);
}

// renderOrder computes the execution order and paints §13.2's table plus the
// per-pod load grid. Stateless on the server side: nothing is saved by looking.
async function renderOrder() {
  const host = document.getElementById('plan-dash');
  if (!host || !current || view() !== 'order') return;
  // Spec 012 FR-004: first-visit callout, dismissed once per session.
  const callout = (key, text) => {
    const k = `conway-callout-${key}`;
    if (sessionStorage.getItem(k)) return '';
    return `<p class="plan-warn callout" data-callout="${key}">${text}
      <button type="button" class="btn btn-secondary callout-dismiss" data-dismiss="${key}">got it</button></p>`;
  };
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
    if (!current || current.id !== forPlan || orderEpoch !== atEpoch || view() !== 'order' || host !== document.getElementById('plan-dash')) return; // an answer to a stale question
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
  host.innerHTML = callout('order', 'Your stated priority order is the working plan. Preview optimized order to compare a suggestion; pin and edit to refine it. Saving an agreement freezes the accepted inputs.')
    + orderViewHTML(current.schedule, {
    noPin: current.isDraft, // nothing is saved to pin against on a draft
    engineRanks: current.schedule.engineRanks, // spec 006: the suggestion column
    storedInitiatives: current.initiatives, // spec 009: the sheet's own dates, pre-scheduler
    horizonWeeks: current.horizonWeeks,
    pod: current.orderPod,
    scheduling: schedForForm,
  });
  applyLiveAssumptions(); // edits carried across an add/del re-render
  wireCallouts(host);
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
  if(dialog && current.assumptionsDismissed) dialog.hidden=true;
  if(dialog) containFocus(dialog,()=>{ dialog.hidden=true; current.assumptionsDismissed=true; document.getElementById('sched-open')?.focus(); });
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
    if(opening) current.assumptionsDismissed=false;
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
    renderCurrentPlanView();
  });
  host.querySelectorAll('.cal-del').forEach((b) => b.addEventListener('click', () => {
    carryLiveAssumptions();
    const i = Number(b.closest('.cal-win')?.dataset.row);
    const live = snapshotCalRows() ?? schedForForm.calendars ?? [];
    current.calDraft = live.filter((_, j) => j !== i);
    renderCurrentPlanView();
  }));
  document.getElementById('sched-cancel')?.addEventListener('click', () => {
    current.assumptionsDismissed=true;
    current.calDraft = null; // cancel discards window edits, not just hides them
    current.assumptionDraft = null;
    renderCurrentPlanView().then(() => {
      // The re-render rebuilds the dialog; urgency (missing period/model) would
      // auto-open it again, and a Cancel that re-opens is not a Cancel.
      const d = document.getElementById('sched-dialog');
      if (d) d.hidden = true;
      document.getElementById('sched-open')?.focus();
    });
  });
  if (current.orderPod && !current.isDraft) {
    const queue = host.querySelector('.ord-queue');
    if (queue) {
      const next = document.createElement('button');
      next.type = 'button'; next.className = 'btn btn-secondary'; next.textContent = `Next work for ${current.orderPod}`;
      next.addEventListener('click', () => openTeamReady(current.orderPod));
      queue.prepend(next);
    }
  }
  host.querySelectorAll('.ord-podlink').forEach((a) => a.addEventListener('click', () => {
    // Clicking the open pod again closes it, so the grid is never stuck behind a panel.
    current.orderPod = current.orderPod === a.dataset.pod ? null : a.dataset.pod;
    renderCurrentPlanView();
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
      planNotice('The priority pin did not save. Try again.', true);
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
    dragUndo = null; dragHistory = [];
    current.schedule = null; // the order must answer the new pin, not the old one
    await renderCurrentPlanView();
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
    const rerender = renderCurrentPlanView;
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
    dragUndo = null; dragHistory = [];
    current.schedule = null;
    await rerender();
  };
  host.querySelectorAll('.setup-apply').forEach((b) =>
    b.addEventListener('click', () => {
      if (b.dataset.setup === 'wip') applySetup({ wipModel: 'strict' });
      if (b.dataset.setup === 'estimate') applySetup({ estimateModel: 'effort' });
    }));
  // A deliberate wall-clock keep is recorded like an explicit choice (cubic
  // P2): the card stops asking without forcing effort.
  host.querySelector('.setup-keep')?.addEventListener('click', () => applySetup({ estimateAck: true }));
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
      .reduce((m, r) => (!m || compareScheduleCosts(r, m) < 0 ? r : m), null);
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
        <span class="hint">Weighted unstarted work: yours ${esc(String(current.schedule.unscheduledWeight ?? 'unknown'))} → proposed ${esc(String(best?.unscheduledWeight ?? 'unknown'))}. Weighted lateness: yours ${esc(String(current.schedule.objectiveScore))} → proposed ${esc(String(best?.objective ?? 'unknown'))}. Lower unstarted work takes priority; lateness breaks ties.</span>
        ${moves ? `<ul class="hint">${moves}</ul>` : '<p class="hint">no moves — your order already matches the best rule found</p>'}
        <div class="sched-row" style="gap:8px">
          <button type="button" class="btn btn-primary" id="ord-accept">Accept the engine's order</button>
          <button class="btn btn-secondary" type="button" id="ord-reject">Keep my order</button>
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
    const atEpoch = orderEpoch;
    const body = { ...((current.scheduling || {})), acceptedOrdering: ordering };
    if (ordering === 'engine') body.acceptedOrderingAt = Math.floor(Date.now() / 1000);
    else { delete body.acceptedOrderingAt; body.acceptedOrdering = 'stated'; }
    const r = await req('/api/plan/' + forPlan + '/scheduling', {
      method: 'PATCH', body: JSON.stringify(body),
    });
    if (!current || current.id !== forPlan || orderEpoch !== atEpoch) return;
    if (!r || !r.ok) return; // req displays the error beside the header action
    // Write the cache only when this response is still the newest word: the
    // reader may have saved assumptions (or accepted again) while it was away.
    current.scheduling = { ...(current.scheduling || {}), ...body };
    dragUndo = null; dragHistory = [];
    current.schedule = null;
    await renderCurrentPlanView();
    if (ordering === 'engine' && current?.id === forPlan && orderEpoch === atEpoch && view() === 'order') {
      // Q1: ask to baseline AFTER the re-render — the drawer opens pre-filled
      // with a dated name, one click away from freezing the accepted order.
      if (!current.isDraft) {
        openBaselinesDrawer();
        const nameInput = document.getElementById('bl-drawer-name');
        if (nameInput && !nameInput.value) {
          nameInput.value = 'engine order ' + new Date().toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
        }
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
    containFocus(dlg,closeInitEditor);
    current.initEditorName = it.name;
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
      dragUndo = null; dragHistory = []; // a dialog save supersedes any drag snapshot (spec 008 S4)
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
      closeInitEditor();
      await renderCurrentPlanView();
    });
  }));
  if (current.setupFocus) {
    current.setupFocus = false; // one-shot: clicking the timeline banner, not every render
    const card = document.getElementById('setup-card');
    if (card) {
      card.scrollIntoView({ behavior: 'smooth', block: 'center' });
      card.classList.add('setup-focus');
      setTimeout(() => card.classList.remove('setup-focus'), 2400);
    } else {
      // the card was dismissed earlier: the WIP model lives in ⚙ Assumptions
      document.getElementById('sched-open')?.click();
    }
  }
  const closeInitEditor = () => {
    document.getElementById('init-edit-dialog')?.remove();
    [...host.querySelectorAll('.ord-edit')].find(b=>b.dataset.edit === current?.initEditorName)?.focus();
  };
  document.querySelector('.ord-queue')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

// previewInitiativesFile parses an uploaded sheet server-side and shows the
// resulting network/constraints WITHOUT saving — the sheet may still be a
// work in progress. "Save initiatives" (saveDraftInitiatives) persists it;
// closing the plan or picking a different file without saving discards it.
// specs/019-scheduling-audit-and-gantt-integrity.md:150: responses belong to
// the requesting plan and input revision, and only its newest request may apply.
let previewTicket = 0;
async function previewInitiativesFile(file) {
  if (!current) return;
  const forPlan = current.id, atEpoch = orderEpoch, ticket = ++previewTicket;
  const ownsResponse = () => current?.id === forPlan && orderEpoch === atEpoch && ticket === previewTicket;
  const fd = new FormData();
  fd.append('file', file);
  fd.append('strict', current.strictDeps ? '1' : '0');
  document.getElementById('plan-uploading')?.remove();
  root.querySelector('.plan-uploads').insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">reading…</span>');
  const r = await req('/api/plan/' + forPlan + '/initiatives/preview', { method: 'POST', body: fd });
  if (!ownsResponse()) return;
  document.getElementById('plan-uploading')?.remove();
  if (!r || !r.ok) {
    const why = r ? await r.text() : 'network';
    if (ownsResponse()) alert('Could not read file: ' + why);
    return;
  }
  const draft = await r.json();
  if (!ownsResponse()) return;
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
    if (current?.id !== forPlan || previewTicket !== ticket || current.draftFile !== file) return;
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
// renderPlanSites is the compact sites notice (spec 003 review): the big
// inline table read as a second form inside Plan setup. The roster carries its
// timezones now (the optional Timezone column), so this row only REPORTS the
// sync state — "N of M sites have a timezone" — and links to the small modal
// that fixes the missing ones.
const TZ_CHOICES = ['Europe/Dublin', 'Europe/London', 'Europe/Warsaw', 'Europe/Berlin',
  'Europe/Paris', 'Europe/Madrid', 'America/New_York', 'America/Chicago',
  'America/Denver', 'America/Los_Angeles', 'America/Toronto', 'America/Sao_Paulo',
  'Asia/Kolkata', 'Asia/Singapore', 'Asia/Tokyo', 'Asia/Jerusalem',
  'Australia/Sydney', 'Pacific/Auckland'];

async function renderPlanSites(nTeams) {
  const host = document.getElementById('plan-sites');
  if (!host || nTeams === 0) return;
  const r = await req('/api/plan/' + current.id + '/sites');
  if (!current || !r || !r.ok) return;
  let sites = [];
  try { sites = (await r.json()).sites || []; } catch { return; }
  const withTz = sites.filter((st) => st.timezone);
  const missing = sites.length - withTz.length;
  host.innerHTML = `
    <div class="plan-sites plan-note">
      <span class="hint">🌐 Sites: ${withTz.length} of ${sites.length} have a timezone.
      ${missing ? 'Complete missing timezone information for site analysis.' : 'Site working-hour information is recorded.'} The finite scheduler currently adds no timezone handoff delay.</span>
      ${missing ? '<button type="button" id="sites-fix" class="btn btn-link p-0 usage-link">set the missing timezones</button>' : ''}
      <span class="hint">Timezones can also ride the roster itself — a Timezone column on the teams sheet.</span>
    </div>`;
  document.getElementById('sites-fix')?.addEventListener('click', () => openSitesModal(sites.filter((st) => !st.timezone)));
}

// openSitesModal fixes the unconfigured sites in one small dialog — the roster
// seeded them by name; the manager only picks zones for the ones that matter.
function openSitesModal(missing) {
  if (!missing.length || document.querySelector('.sites-modal-overlay')) return;
  const overlay = document.createElement('div');
  overlay.className = 'sites-modal-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', 'Set site timezones');
  const options = (tz) => ['<option value="">pick a timezone…</option>']
    .concat(TZ_CHOICES.map((z) => `<option value="${z}" ${z === tz ? 'selected' : ''}>${z}</option>`)).join('');
  overlay.innerHTML = `
    <div class="card p-3 sites-modal panel-card">
      <h3>Set the missing timezones</h3>
      <p class="hint">These sites need timezone information. The roster's Timezone column fills it on the next upload. The finite scheduler currently adds no timezone handoff delay.</p>
      ${missing.map((st) => `<div class="sites-row" data-site="${esc(st.name)}">
        <b>${esc(st.name)}</b>
        <select class="form-select site-tz">${options(st.timezone)}</select>
      </div>`).join('')}
      <p class="plan-warn" id="sites-error" hidden></p>
      <div class="sites-actions">
        <button class="btn btn-secondary" type="button" id="sites-cancel">Cancel</button>
        <button type="button" id="sites-saveall" class="btn btn-primary">Save</button>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  const close = () => overlay.remove();
  overlay.querySelector('#sites-cancel').addEventListener('click', close);
  overlay.addEventListener('click', (ev) => { if (ev.target === overlay) close(); });
  overlay.addEventListener('keydown', (ev) => { if (ev.key === 'Escape') close(); });
  overlay.querySelector('.site-tz').focus();
  overlay.querySelector('#sites-saveall').addEventListener('click', async () => {
    const errEl = overlay.querySelector('#sites-error');
    const saveBtn = overlay.querySelector('#sites-saveall');
    saveBtn.disabled = true;
    for (const row of overlay.querySelectorAll('.sites-row')) {
      const tz = row.querySelector('.site-tz').value;
      if (!tz) continue; // leaving it unset is a legitimate choice
      const res = await req('/api/plan/' + current.id + '/sites', {
        method: 'PATCH',
        body: JSON.stringify({ name: row.dataset.site, timezone: tz, workStartHour: 9, workEndHour: 17 }),
      });
      if (!res || !res.ok) {
        errEl.textContent = (res ? await res.text() : 'the request did not reach the server').slice(0, 200);
        errEl.hidden = false;
        saveBtn.disabled = false;
        return;
      }
    }
    close();
    renderPlan(); // the notice recounts and the game/report pick up the new overlap
  });
}

async function renderRosterPicker(nTeams) {
  const box = document.getElementById('plan-roster-pick');
  if (!box) return;
  const rr = await req('/api/rosters');
  const rosters = (rr && rr.ok) ? (await rr.json()) || [] : [];
  const bindUpload = () => box.querySelectorAll('input[type=file]').forEach((inp) => inp.addEventListener('change', () => {
    if (inp.files[0]) uploadFile(inp.dataset.kind, inp.files[0]);
  }));
  if (!rosters.length) {
    box.innerHTML = '<span class="hint">No saved rosters yet — create one in Measure ▸ Rosters (recommended), or upload a roster file directly:</span>'
      + uploadField('teams', `${icon('upload')}Teams roster (CSV/XLSX)`, nTeams);
    bindUpload();
    return;
  }
  // Applies on selection (review): the dropdown choice IS the intent; no
  // extra confirm button. Guard: once initiatives exist, a roster switch may
  // orphan dependency cells — confirm only then; cancel restores the select.
  const prevRoster = current.rosterId || '';
  box.innerHTML = `<label class="hint">roster (applies on selection)
      <select class="form-select" id="plan-roster-sel">
        <option value="" ${!current.rosterId ? 'selected' : ''}>none selected</option>
        ${rosters.map((r) => `<option value="${r.id}" ${r.id === current.rosterId ? 'selected' : ''}>${esc(r.name)} (${r.podCount} pods)</option>`).join('')}
      </select>
    </label>
    <span class="hint">${nTeams ? `${nTeams} pods loaded` : 'none selected'}</span>`;
  box.querySelector('#plan-roster-sel').addEventListener('change', async (ev) => {
    const rosterId = ev.target.value;
    if (!rosterId) { ev.target.value = prevRoster; return; } // "none selected" is not a roster
    if ((current.initiatives || []).length && rosterId !== prevRoster) {
      if (!confirm('Switching the roster replaces this plan\u2019s pods — initiatives referencing missing pods will land with warnings. Apply?')) {
        ev.target.value = prevRoster; // revert the select
        return;
      }
    }
    box.insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">applying…</span>');
    const r = await req('/api/plan/' + current.id + '/roster', { method: 'POST', body: JSON.stringify({ rosterId }) });
    if (!r || !r.ok) { alert('Could not attach roster: ' + (r ? await r.text() : 'network')); document.getElementById('plan-uploading')?.remove(); return; }
    openPlan(current.id);
  });
}

async function savePlanParams() {
  const horizon = +document.getElementById('plan-horizon').value || 26;
  const loss = (+document.getElementById('plan-loss').value || 0) / 100;
  const forPlan = current.id;
  const res = await req('/api/plan/' + forPlan, { method: 'PATCH', body: JSON.stringify({ horizonWeeks: horizon, capacityLoss: loss }) });
  if (!res?.ok || current?.id !== forPlan) return;
  dragUndo = null; dragHistory = []; // saved params supersede any drag snapshot (spec 008 S4, FR-006)
  openPlan(current.id);
}

async function uploadFile(kind, file) {
  const fd = new FormData();
  fd.append('file', file);
  if (kind === 'initiatives') fd.append('strict', current.strictDeps ? '1' : '0');
  root.querySelector('.plan-uploads').insertAdjacentHTML('beforeend', '<span class="hint" id="plan-uploading">uploading…</span>');
  const r = await req('/api/plan/' + current.id + '/' + kind, { method: 'POST', body: fd });
  if (!r || !r.ok) { document.getElementById('plan-uploading')?.remove(); const b = document.getElementById('plan-draft-save'); if (b) {b.disabled=false;b.textContent='Save initiatives';} return; }
  dragUndo = null; dragHistory = []; // an upload replaces the initiatives wholesale (spec 008 S4, FR-006)
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

// renderDash ensures we have a simulation result (current inputs = no levers), then paints.
async function renderDash() {
  if (!current || view() !== 'network') return;
  if (!current.sim) { await runSim(); return; }
  paintDash();
}

// specs/019-scheduling-audit-and-gantt-integrity.md:150: discard stale what-if results.
let simulationTicket = 0;
async function runSim() {
  if (!current) return;
  const forPlan = current.id, atEpoch = orderEpoch, ticket = ++simulationTicket;
  const ownsResponse = () => current?.id === forPlan && orderEpoch === atEpoch && ticket === simulationTicket;
  const body = { levers: current.levers || [] };
  // draft preview mode: simulate against the unsaved sheet, not the stale saved one
  if (current.isDraft) body.initiatives = current.initiatives;
  const r = await req('/api/plan/' + forPlan + '/simulate', { method: 'POST', body: JSON.stringify(body) });
  if (!ownsResponse()) return;
  if (!r || !r.ok) {
    const host = document.getElementById('plan-dash');
    if (host && view() === 'network') host.innerHTML = '<p class="hint">Could not run simulation.</p>';
    return;
  }
  const sim = await r.json();
  if (!ownsResponse()) return;
  current.sim = sim;
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
  if (!current || view() !== 'network') return;
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
        tracks <input class="form-control" id="pe-tracks" type="number" min="0" max="50" value="${tm.tracks || ''}" placeholder="${l.tracks} (auto)" style="width:84px">
        <label><input class="form-check-input" id="pe-pairs" type="checkbox" ${tm.pairs ? 'checked' : ''}> pairs</label>
        <span class="hint">${tm.devs || 0} devs</span>
        <button class="btn btn-secondary pod-save" data-pod="${esc(l.team)}">save</button>
        <button class="btn btn-secondary pod-cancel">cancel</button></td></tr>`;
    }
    return `<tr>
      <td>${esc(l.team)}</td>
      <td><b style="color:${rhoColor(l.rho)}">${rhoTxt(l.rho)}</b></td>
      <td>${hasLevers ? `<b style="color:${rhoColor(a.rho)}">${rhoTxt(a.rho)}</b>` : '<span class="hint">—</span>'}</td>
      <td>${Math.round(l.demandWeeks)} / ${Math.round(l.capacityWeeks)}</td>
      <td>${l.tracks}${hasLevers && a.tracks !== l.tracks ? ` → ${a.tracks}` : ''} <button type="button" class="btn btn-secondary pod-edit" data-pod="${esc(l.team)}">${icon('edit')}Edit capacity</button></td></tr>`;
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
    <div class="card p-3 plan-net panel-card">
      <div class="plan-net-head"><b>Dependency network</b>
        <span>
          <div class="btn-group" role="group" aria-label="Network scenario"><button class="btn-secondary btn ${mode === 'before' ? 'active' : ''}" id="net-before" aria-pressed="${mode === 'before'}">Current inputs</button><button class="btn-secondary btn ${mode === 'after' ? 'active' : ''}" id="net-after" aria-pressed="${mode === 'after'}">with levers</button></div>
        </span></div>
      <div class="plan-net-wrap">
        <svg id="plan-svg"></svg>
        <aside id="plan-netpanel"><p class="hint">Click a pod to inspect it. Node size = demand weeks, ring color = queue heat (ρ), flow runs left→right, arrow points at the pod waiting.</p></aside>
      </div>
      <p class="hint">flow runs left→right · node size = demand · ring = ρ (heat) · showing <b>${mode === 'after' ? 'with levers' : 'baseline'}</b></p>
    </div>
    <div class="card p-3 plan-constraints panel-card" style="margin-top:12px">
      <b>Constraints <span class="hint">(hottest first)</span></b>
      <table class="table table-sm wip-table"><thead><tr><th>Pod</th><th>ρ now</th><th>ρ after</th><th>demand/cap</th><th>tracks</th></tr></thead>
        <tbody>${constraintRows}</tbody></table>
      <p class="hint">ρ: red ≥1 · amber ≥.85 · green &lt;.85 — utilization is the signal; lead time is directional.</p>
    </div>
    <div class="card p-3 plan-levers panel-card">
      <b>Levers — what-if</b>
      <div class="plan-summary">${summary}</div>
      <div class="lever-chips">${(current.levers || []).map((lv, i) => `<span class="chip">${esc(leverLabel(lv))} <a class="chip-x" data-lev="${i}">✕</a></span>`).join('') || '<span class="hint">no levers yet</span>'}</div>
      <div class="lever-add">
        <select class="form-select w-auto mw-100" id="lev-type" aria-label="Lever type">
          <option value="addCapacity">Add capacity</option>
          <option value="unpair">Un-pair a pod</option>
          <option value="descope">Descope an initiative</option>
          <option value="defer">Defer an initiative</option>
          <option value="reduceWip">Reduce WIP (focus)</option>
          <option value="reassign">Reassign a pod's work</option>
          <option value="dropPod">Drop a pod from an initiative</option>
        </select>
        <span id="lev-target" class="d-inline-flex flex-wrap align-items-center gap-2 mw-100"></span>
        <button id="lev-add" class="btn btn-primary">Add lever</button>
      </div>
    </div>
    <div class="card p-3 panel-card" style="margin-top:12px">
      <b>Initiatives</b>
      <table class="table table-sm wip-table"><thead><tr><th>Initiative</th><th>Pods in path</th><th>Lead time (directional)</th><th>Bottleneck</th></tr></thead>
        <tbody>${initRows}</tbody></table>
    </div>`;

  drawNetwork(p, loads);
  document.getElementById('net-before').addEventListener('click', () => { current.netMode = 'before'; paintDash(); });
  document.getElementById('net-after').addEventListener('click', () => { current.netMode = 'after'; paintDash(); });
  document.querySelectorAll('.lever-chips .chip-x').forEach((a) => a.addEventListener('click', () => {
    current.levers.splice(+a.dataset.lev, 1); dragUndo=null; dragHistory=[]; staleOrder(); runSim();
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
  const id=current.id;
  const r=await req('/api/plan/' + id + '/teams', {
    method: 'PATCH',
    body: JSON.stringify({ name: pod, pairs, tracks: tv === '' ? 0 : (+tv) }),
  });
  if(!r?.ok || current?.id !== id) return;
  current.editPod = null;
  reloadPlan();
}

// reloadPlan re-fetches the plan (roster changed) but keeps the applied levers.
// The order is dropped rather than kept: pod capacity is the input the scheduler
// is most sensitive to, so a retained order would be wrong about nearly everything.
async function reloadPlan() {
  if (!current) return;
  const prior=current, id=current.id, ticket=++planLoadTicket;
  staleOrder();
  const r=await req('/api/plan/' + id);
  if(!r?.ok || ticket !== planLoadTicket || current?.id !== id) return;
  const loaded=await r.json();
  if(ticket !== planLoadTicket || current?.id !== id) return;
  current={...prior,...loaded,schedule:null,sim:null};
  dragUndo=null; dragHistory=[];
  await loadBaselines();
  if(ticket !== planLoadTicket || current?.id !== id) return;
  renderPlan();
}

function renderLeverTarget() {
  const t = document.getElementById('lev-type').value;
  const podOpts = PODS().map((n) => `<option>${esc(n)}</option>`).join('');
  const initOpts = INITS().map((n) => `<option>${esc(n)}</option>`).join('');
  const el = document.getElementById('lev-target');
  if (t === 'addCapacity') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-pod" aria-label="Team">${podOpts}</select> +<input class="form-control" id="lev-n" aria-label="Number of tracks" type="number" min="1" max="10" value="2" style="width:48px"> tracks`;
  else if (t === 'unpair') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-pod" aria-label="Team">${podOpts}</select>`;
  else if (t === 'descope') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-init" aria-label="Initiative">${initOpts}</select> −<input class="form-control" id="lev-n" aria-label="Scope reduction (%)" type="number" min="5" max="90" value="40" style="width:48px">%`;
  else if (t === 'defer') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-init" aria-label="Initiative">${initOpts}</select>`;
  else if (t === 'reduceWip') el.innerHTML = `−<input class="form-control" id="lev-n" aria-label="Multitasking reduction (%)" type="number" min="5" max="40" value="15" style="width:48px">% multitasking`;
  else if (t === 'reassign') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-pod" aria-label="Team">${podOpts}</select> → <select class="form-select w-auto mw-100" id="lev-topod" aria-label="Target team">${podOpts}</select>`;
  else if (t === 'dropPod') el.innerHTML = `<select class="form-select w-auto mw-100" id="lev-pod" aria-label="Team">${podOpts}</select> from <select class="form-select w-auto mw-100" id="lev-init" aria-label="Initiative">${initOpts}</select>`;
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
  dragUndo=null; dragHistory=[];
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
  if (n.rho >= 1e8) flags.push('<span class="badge bg-danger-subtle text-danger-emphasis flag red">demand with zero capacity</span>');
  else if (n.rho >= 1) flags.push('<span class="badge bg-danger-subtle text-danger-emphasis flag red">over capacity (ρ≥1)</span>');
  else if (n.rho >= 0.85) flags.push('<span class="badge bg-warning-subtle text-warning-emphasis flag amber">queue hot (ρ≥0.85)</span>');
  const initRows = inits.map((i, idx) => {
    const w = i.work[n.name];
    const weeks = (w?.estimated && w?.weeks > 0)
      ? `<span class="insp-weeks">${Math.round(w.weeks)}w</span>`
      : (w?.weeks > 0 ? `<span class="insp-weeks">${Math.round(w.weeks)}w</span>` : '<span class="insp-weeks hint">no estimate</span>');
    return `<li><span class="insp-num">${idx + 1}</span><span class="insp-name">${esc(i.name)}</span>${weeks}</li>`;
  }).join('');
  document.getElementById('plan-netpanel').innerHTML = `
    <h2>${esc(n.name)}</h2>
    <div>${flags.join(' ') || '<span class="badge bg-success-subtle text-success-emphasis flag">healthy</span>'}</div>
    <dl>
      <dt>Demand / capacity</dt><dd>${Math.round(l.demandWeeks ?? n.weeks ?? 0)}w / ${Math.round(l.capacityWeeks ?? 0)}w · tracks ${l.tracks ?? '—'}</dd>
      <dt>Utilization</dt><dd>ρ ${rhoTxt(n.rho)}</dd>
      <dt>Coupling</dt><dd>depends on ${inAll.length} · ${outAll.length} depend on it</dd>
      <dt>Initiatives (${inits.length})</dt>
      <dd>${initRows ? `<ol class="insp-list">${initRows}</ol>` : '—'}</dd>
    </dl>`;
}
