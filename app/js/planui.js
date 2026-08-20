// Plan pillar UI: a manager uploads a teams roster + an initiatives matrix, and
// sees the directed cross-pod dependency network with per-pod utilization (ρ)
// and the constraint pods. Levers/what-if come in a later phase.
import { authFetch } from './auth.js';
import {
  heatColor, layoutColumns, bezierEdgePath, appendArrowMarker,
  enablePanZoom, enableNodeDrag, makeSpotlight,
} from './netgraph.js';
import { esc, orderViewHTML } from './order.js';

let root, current = null;

export function initPlanUI() {
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
      <a class="plan-back">← all plans</a>
      <h2>${esc(p.name)}</h2>
      <label class="hint">horizon <input id="plan-horizon" type="number" min="1" max="104" value="${p.horizonWeeks}" style="width:56px">w</label>
      <label class="hint">capacity loss <input id="plan-loss" type="number" min="0" max="90" value="${Math.round((p.capacityLoss || 0) * 100)}" style="width:52px">%</label>
      <button id="plan-save">Save</button>
    </div>
    <div class="plan-uploads">
      <div class="plan-up" id="plan-roster-pick"></div>
      ${uploadField('initiatives', '⤓ Initiatives (XLSX/CSV)', nInit)}
      <label class="hint plan-up" title="Drop any dependency cell that doesn't match a roster pod name (case/whitespace-insensitive) — free text like &quot;Requirements unknown&quot; won't become a fake node in the network.">
        <input type="checkbox" id="plan-strict-deps" ${current.strictDeps ? 'checked' : ''}> strict: match dependencies to roster
      </label>
    </div>
    <p class="hint">Need samples? <a href="/api/sample/teams.csv" download>teams.csv</a> · <a href="/api/sample/initiatives.xlsx" download>initiatives.xlsx</a></p>
    ${current.isDraft ? `<p class="plan-warn">✎ Previewing an unsaved initiatives upload — nothing is saved yet.
      <button id="plan-draft-save" class="primary">Save initiatives</button>
      <button id="plan-draft-discard">Discard</button></p>` : ''}
    ${unknown.length ? `<p class="plan-warn">⚠ ${unknown.length} pod(s) referenced by initiatives but missing from the roster: ${unknown.map(esc).join(', ')} — fix the sheet or add them to the roster.</p>` : ''}
    ${nTeams > 0 && nInit > 0 ? `<div class="plan-views">
      <button class="seg ${view() === 'network' ? 'seg-on' : ''}" id="view-network">Network</button><button class="seg ${view() === 'order' ? 'seg-on' : ''}" id="view-order">Order</button>
    </div>` : ''}
    ${nTeams === 0 ? '<p class="hint">Step 1: pick a roster (team composition drifts over time — this pins it). Step 2: upload the initiatives matrix.</p>'
      : nInit === 0 ? '<p class="hint">Roster loaded. Now upload the initiatives matrix.</p>'
      : '<div id="plan-dash"></div>'}`;

  root.querySelector('.plan-back').addEventListener('click', renderList);
  root.querySelector('#plan-save').addEventListener('click', savePlanParams);
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
  renderRosterPicker(nTeams);
  if (nTeams > 0 && nInit > 0) {
    current.levers = current.levers || [];
    current.netMode = current.netMode || 'after';
    if (view() === 'order') renderOrder(); else renderDash();
  }
}

const view = () => (current && current.view) === 'order' ? 'order' : 'network';

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
  host.innerHTML = orderViewHTML(current.schedule, {
    horizonWeeks: current.horizonWeeks,
    pod: current.orderPod,
  });
  host.querySelectorAll('.ord-podlink').forEach((a) => a.addEventListener('click', () => {
    // Clicking the open pod again closes it, so the grid is never stuck behind a panel.
    current.orderPod = current.orderPod === a.dataset.pod ? null : a.dataset.pod;
    renderOrder();
  }));
  document.querySelector('.ord-queue')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
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
function showPlanPodPanel(n, p, loads, net) {
  const l = (loads || []).find((x) => x.team === n.name) || {};
  const inAll = (net.edges || []).filter((e) => e.to === n.name);
  const outAll = (net.edges || []).filter((e) => e.from === n.name);
  const inits = (p.initiatives || []).filter((i) => i.work?.[n.name]?.inPath);
  const flags = [];
  if (n.rho >= 1e8) flags.push('<span class="flag red">demand with zero capacity</span>');
  else if (n.rho >= 1) flags.push('<span class="flag red">over capacity (ρ≥1)</span>');
  else if (n.rho >= 0.85) flags.push('<span class="flag amber">queue hot (ρ≥0.85)</span>');
  document.getElementById('plan-netpanel').innerHTML = `
    <h2>${esc(n.name)}</h2>
    <div>${flags.join(' ') || '<span class="flag" style="color:var(--green)">healthy</span>'}</div>
    <dl>
      <dt>Demand / capacity</dt><dd>${Math.round(l.demandWeeks ?? n.weeks ?? 0)}w / ${Math.round(l.capacityWeeks ?? 0)}w · tracks ${l.tracks ?? '—'}</dd>
      <dt>Utilization</dt><dd>ρ ${rhoTxt(n.rho)}</dd>
      <dt>Coupling</dt><dd>depends on ${inAll.length} · ${outAll.length} depend on it</dd>
      <dt>Initiatives (${inits.length})</dt>
      ${inits.map((i) => `<dd>${esc(i.name)}</dd>`).join('') || '<dd>—</dd>'}
    </dl>`;
}
