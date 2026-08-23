import { openModal, closeModal } from './modal.js';
// "Import from Jira" — builds a dated org snapshot. Auth is OAuth-via-Okta when
// the server is configured for it (a "Connect Jira" button → SSO → no token),
// otherwise it falls back to a pasted API token. Either way: pick projects +
// a roster (mandatory — team structure drifts over time, so the snapshot is
// pinned to the roster as it stood then) and import.
import { authFetch } from './auth.js';
import { getJiraBaseUrl } from './data.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
async function req(p, o) { try { return await authFetch(p, o); } catch { return null; } }

let projects = []; // loaded project list {key,name}
let rosters = [];  // saved rosters {id,name}
let selectedKeySet = new Set(); // persists across filter re-renders, unlike the DOM

export async function openImport() {
  let ov = document.getElementById('import-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'import-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
    // no click-outside-to-close: an accidental click while filling in the
    // form (name, roster, project picks) would silently discard it — the ✕
    // button below is the only deliberate way out.
  }
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>📥 Import from Jira</h2><button id="imp-close">✕</button></div>
      <p class="hint">Fetch live activity for the projects you pick and build a dated snapshot.
        Team structure comes from a roster.</p>
      <div id="imp-auth"></div>
      <div id="imp-err" class="login-err"></div>
      <div id="imp-step2" hidden>
        <div class="games-create" style="flex-wrap:wrap">
          <input id="imp-name" placeholder="Snapshot name (e.g. Q3 2026)" style="min-width:220px">
          <label class="hint">roster <select id="imp-plan"></select></label>
        </div>
        <p class="hint" id="imp-struct-note"></p>
        <div class="games-create" style="flex-wrap:wrap">
          <label class="hint">count WIP as
            <select id="imp-wipmode">
              <option value="leaf">every in-progress story/task (recommended)</option>
              <option value="epic_or_parentless">in-progress epics as one unit each</option>
            </select>
          </label>
        </div>
        <p class="hint" id="imp-wipmode-note"></p>
        <div class="games-create">
          <input id="imp-filter" placeholder="filter projects (e.g. ABC)" style="min-width:200px">
          <button id="imp-all">select all shown</button>
          <button id="imp-none">clear</button>
          <span id="imp-count" class="hint"></span>
        </div>
        <div id="imp-projects" class="imp-projects"></div>
        <div class="row-actions">
          <button id="imp-go" class="primary">Import snapshot ▶</button>
          <span id="imp-status" class="hint"></span>
        </div>
      </div>
    </div>`;
  openModal(ov);
  ov.querySelector('#imp-close').addEventListener('click', () => closeModal(ov));
  ov.querySelector('#imp-go').addEventListener('click', () => runImport(ov));
  renderAuth(ov);
}

const err = (ov, m) => { ov.querySelector('#imp-err').textContent = m || ''; };

async function jiraStatus() {
  const r = await req('/api/jira/status');
  return (r && r.ok) ? r.json() : { configured: false, connected: false };
}

// Render the auth section by mode: connected (OAuth), connect prompt, or token.
async function renderAuth(ov) {
  const box = ov.querySelector('#imp-auth');
  const st = await jiraStatus();
  if (st.connected) {
    box.innerHTML = `<p class="hint">✅ Connected to <b>${esc(st.site || 'Jira')}</b> via SSO.
      <a id="imp-switch">switch account</a></p>`;
    box.querySelector('#imp-switch').addEventListener('click', () => startOAuth(ov));
    loadProjects(ov, false);
    return;
  }
  if (st.configured) {
    box.innerHTML = `<div class="games-create">
        <button id="imp-connect" class="primary">🔗 Connect Jira (SSO)</button>
        <span class="hint">Sign in through your org's single sign-on — no token needed.</span>
      </div>`;
    box.querySelector('#imp-connect').addEventListener('click', () => startOAuth(ov));
    return;
  }
  // fallback: API token
  const jiraBase = await getJiraBaseUrl();
  box.innerHTML = `<div class="games-create" style="flex-wrap:wrap">
      <input id="imp-url" placeholder="https://yourorg.atlassian.net" value="${esc(jiraBase)}" style="min-width:260px">
      <input id="imp-email" placeholder="email" autocomplete="username" style="min-width:200px">
      <input id="imp-token" type="password" placeholder="API token" autocomplete="off" style="min-width:200px">
      <button id="imp-load" class="primary">Load projects ▶</button>
    </div>
    <p class="hint">Create a token at id.atlassian.com → Security → API tokens. Used for this import only — never stored.</p>`;
  box.querySelector('#imp-load').addEventListener('click', () => loadProjects(ov, true));
}

async function startOAuth(ov) {
  err(ov, '');
  const r = await req('/api/jira/oauth/start');
  if (!r || !r.ok) { err(ov, (r && (await r.text()).trim()) || 'Could not start sign-in.'); return; }
  const { url } = await r.json();
  location.assign(url); // browser → Atlassian → Okta → back to /?import=1
}

function creds(ov) {
  return {
    baseUrl: ov.querySelector('#imp-url')?.value.trim() || '',
    email: ov.querySelector('#imp-email')?.value.trim() || '',
    token: ov.querySelector('#imp-token')?.value || '',
  };
}

async function loadProjects(ov, requireCreds) {
  err(ov, '');
  const c = creds(ov);
  if (requireCreds && (!c.baseUrl || !c.email || !c.token)) { err(ov, 'Enter base URL, email and API token.'); return; }
  const btn = ov.querySelector('#imp-load');
  if (btn) { btn.disabled = true; btn.textContent = 'Loading…'; }
  const r = await req('/api/jira/projects', { method: 'POST', body: JSON.stringify(c) });
  if (btn) { btn.disabled = false; btn.textContent = 'Load projects ▶'; }
  if (!r || !r.ok) { err(ov, (r && (await r.text()).trim()) || 'Could not reach Jira.'); return; }
  projects = (await r.json()) || [];
  selectedKeySet = new Set();
  if (!projects.length) { err(ov, 'No projects returned — check access (an invalid token reads as no access).'); return; }
  const rr = await req('/api/rosters');
  rosters = (rr && rr.ok) ? (await rr.json()) || [] : [];
  const sel = ov.querySelector('#imp-plan');
  const note = ov.querySelector('#imp-struct-note');
  const goBtn = ov.querySelector('#imp-go');
  if (!rosters.length) {
    sel.innerHTML = '';
    sel.disabled = true;
    note.innerHTML = '⚠️ No saved rosters yet. A JIRA import needs a dated roster — team composition changes over time, '
      + 'and a snapshot must be pinned to the roster as it stood then. Create one in Observe ▸ 👥 Rosters, then come back.';
    goBtn.disabled = true;
  } else {
    sel.disabled = false;
    goBtn.disabled = false;
    sel.innerHTML = rosters.map((r) => `<option value="${r.id}">${esc(r.name)} (${r.podCount} pods)</option>`).join('');
    note.innerHTML = 'Team structure (headcount, pairing, lanes) comes from this roster, joined to Jira by pod name. Manage rosters in Observe ▸ 👥 Rosters.';
  }
  const wipSel = ov.querySelector('#imp-wipmode');
  const wipNote = ov.querySelector('#imp-wipmode-note');
  const syncWipMode = () => {
    wipNote.textContent = wipSel.value === 'epic_or_parentless'
      ? 'An epic that’s in progress counts as 1, and its child stories/tasks aren’t counted separately — undercounts true concurrent work if an epic has several active children.'
      : 'An in-progress epic itself doesn’t count — every in-progress story/task/bug does, whether or not it has a parent epic. Matches what the freeze panel drill-down shows.';
  };
  wipSel.addEventListener('change', syncWipMode);
  syncWipMode();
  ov.querySelector('#imp-step2').hidden = false;
  renderProjects(ov);
}

function renderProjects(ov) {
  const filter = ov.querySelector('#imp-filter').value.trim().toLowerCase().replace(/\*+$/, '');
  const box = ov.querySelector('#imp-projects');
  const shown = projects.filter((p) => !filter || p.key.toLowerCase().includes(filter) || (p.name || '').toLowerCase().includes(filter));
  box.innerHTML = shown.map((p) => `<label class="imp-proj"><input type="checkbox" value="${esc(p.key)}" ${selectedKeySet.has(p.key) ? 'checked' : ''}> <b>${esc(p.key)}</b> <span class="hint">${esc(p.name || '')}</span></label>`).join('')
    || '<p class="hint">No projects match that filter.</p>';
  box.querySelectorAll('input').forEach((c) => c.addEventListener('change', () => {
    if (c.checked) selectedKeySet.add(c.value); else selectedKeySet.delete(c.value);
    updateCount(ov);
  }));
  ov.querySelector('#imp-filter').oninput = () => renderProjects(ov);
  ov.querySelector('#imp-all').onclick = () => { shown.forEach((p) => selectedKeySet.add(p.key)); renderProjects(ov); };
  ov.querySelector('#imp-none').onclick = () => { selectedKeySet.clear(); renderProjects(ov); };
  updateCount(ov);
}

function selectedKeys() {
  return [...selectedKeySet];
}
function updateCount(ov) {
  ov.querySelector('#imp-count').textContent = `${selectedKeySet.size} selected`;
}

async function runImport(ov) {
  err(ov, '');
  const status = ov.querySelector('#imp-status');
  const name = ov.querySelector('#imp-name').value.trim();
  const rosterId = ov.querySelector('#imp-plan').value;
  const keys = selectedKeys();
  if (!name) { err(ov, 'Name the snapshot.'); return; }
  if (!rosterId) { err(ov, 'Create and pick a roster before importing.'); return; }
  if (!keys.length) { err(ov, 'Pick at least one project.'); return; }
  const wipMode = ov.querySelector('#imp-wipmode').value;
  const btn = ov.querySelector('#imp-go');
  btn.disabled = true; btn.textContent = 'Importing…';
  status.innerHTML = '⏳ Starting import…';
  const body = { ...creds(ov), name, rosterId, wipMode, projects: keys };
  // the fetch can take minutes — the server runs it in the background and we poll
  const r = await req('/api/snapshots/import', { method: 'POST', body: JSON.stringify(body) });
  if (!r || !r.ok) { resetGo(ov); status.textContent = ''; err(ov, (r && (await r.text()).trim()) || 'Import failed.'); return; }
  const { jobId } = await r.json();
  pollImport(ov, jobId);
}

function resetGo(ov) {
  const btn = ov.querySelector('#imp-go');
  if (btn) { btn.disabled = false; btn.textContent = 'Import snapshot ▶'; }
}

function pollImport(ov, jobId) {
  const status = ov.querySelector('#imp-status');
  const tick = async () => {
    if (ov.hidden) return; // user closed the modal — stop polling
    const r = await req('/api/snapshots/import-status/' + jobId);
    if (!r || !r.ok) { resetGo(ov); status.textContent = ''; err(ov, 'Lost track of the import — try again.'); return; }
    const j = await r.json();
    if (j.status === 'running') {
      status.innerHTML = `⏳ ${esc(j.phase || 'Working…')}${j.issues ? ` — ${j.issues} issues` : ''}`;
      setTimeout(tick, 2000);
      return;
    }
    if (j.status === 'error') { resetGo(ov); status.textContent = ''; err(ov, j.error || 'Import failed.'); return; }
    // done
    status.innerHTML = '✓ Snapshot ready — opening it…';
    const u = new URL(location.href);
    u.searchParams.set('snapshot', j.snapshot);
    u.searchParams.delete('import');
    setTimeout(() => location.assign(u), 700);
  };
  tick();
}
