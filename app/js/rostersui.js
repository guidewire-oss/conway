import { icon } from './icons.js';
import { openModal, closeModal } from './modal.js';
// Rosters: reusable, editable team-structure definitions (pods: name, site,
// pairing, headcount, lanes). Created/uploaded once, edited anytime, and
// associated with a Jira import. Manager-only.
import { authFetch } from './auth.js';
import { notifyMeasureSourcesChanged } from './measure-context.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
async function req(p, o) {
  try {
    const response = await authFetch(p, o);
    if (response.ok && ['POST', 'PATCH', 'DELETE'].includes(o?.method) && p.startsWith('/api/rosters')) notifyMeasureSourcesChanged();
    return response;
  } catch { return null; }
}

// Standalone "Rosters" modal — where rosters are created, uploaded, and edited.
export async function openRosters() {
  let ov = document.getElementById('rosters-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'rosters-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
    // no click-outside-to-close — the ✕ button is the deliberate exit.
  }
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>Team rosters</h2><button class="btn btn-secondary" id="rosters-close">${icon('close')}Close</button></div>
      <p class="hint">Reusable team structure — headcount, pairing, site and work-lanes. A Jira import
        joins a roster to live activity by pod name. Edit anytime; re-associate a snapshot from Measure ▸ Snapshots.</p>
      <p class="rosters-status" role="status" aria-live="polite"></p><div id="rosters-body"></div>
    </div>`;
  openModal(ov);
  ov.querySelector('#rosters-close').addEventListener('click', () => closeModal(ov));
  renderList(ov);
}

// Mounts the rosters section into an existing container — used standalone above,
// and by snapshotsui.js's combined "Snapshots" view (Rosters + Jira snapshots
// are both "things captured/uploaded"; Snapshots is where you see both at once).
export async function mountRosters(container) {
  container.innerHTML = `<p class="rosters-status" role="status" aria-live="polite"></p><div id="rosters-body"></div>`;
  renderList(container);
}

function showError(ov,message) { const el=ov.querySelector('.rosters-status'); if(el) { el.textContent=message.trim().slice(0,250); el.setAttribute('role','alert'); } }

async function renderList(ov) {
  const box = ov.querySelector('#rosters-body');
  const r = await req('/api/rosters');
  if(!r?.ok) { showError(ov,'Could not load rosters. Reopen Rosters to retry.'); return; }
  const rosters = (await r.json()) || [];
  const fmt = (ts) => (ts ? new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: '2-digit' }) : '');
  box.innerHTML = `
    <div class="games-create">
      <button id="ros-new" class="btn btn-primary primary">+ New roster</button>
      <button class="btn btn-secondary" id="ros-upload">${icon('upload')}New from CSV/XLSX</button>
      <a class="hint" href="/api/sample/roster.csv">Download sample format</a>
      <input class="form-control" id="ros-file" type="file" accept=".csv,.xlsx" hidden>
    </div>
    <table class="table table-sm wip-table"><thead><tr><th>Name</th><th>Pods</th><th>Visibility</th><th>Updated</th><th></th></tr></thead>
      <tbody>${rosters.map((r) => {
    const vis = r.public ? '<span class="badge bg-success-subtle text-success-emphasis flag">public</span>' : '<span class="badge text-bg-secondary flag">private</span>';
    return `<tr>
        <td><b>${esc(r.name)}</b>${!r.mine ? ` <span class="hint">· shared by ${r.owner ? esc(r.owner) : 'system'}</span>` : ''}</td>
        <td>${r.podCount}</td>
        <td>${r.mine ? `${vis} <button class="btn btn-secondary ros-pub" data-id="${r.id}" data-pub="${r.public ? 1 : 0}">${r.public ? 'make private' : 'make public'}</button>` : vis}</td>
        <td>${fmt(r.updatedAt)}</td>
        <td>${r.mine
      ? `<button class="btn btn-secondary ros-edit" data-id="${r.id}">edit</button> <button class="btn btn-secondary ros-del" data-id="${r.id}" data-name="${esc(r.name)}">delete</button>`
      : `<button class="btn btn-secondary ros-edit" data-id="${r.id}">view</button>`}</td>
      </tr>`;
  }).join('') || '<tr><td colspan="5" class="hint">No rosters yet — create one or upload your pod directory.</td></tr>'}
      </tbody></table>`;
  box.querySelector('#ros-new').addEventListener('click', () => editRoster(ov, { name: '', pods: [{ name: '', location: '', pairing: true, devCount: 0, streams: 0 }] }));
  const file = box.querySelector('#ros-file');
  box.querySelector('#ros-upload').addEventListener('click', () => file.click());
  file.addEventListener('change', async () => {
    if (!file.files.length) return;
    const fd = new FormData(); fd.append('file', file.files[0]);
    const rr = await req('/api/parse-roster', { method: 'POST', body: fd });
    file.value = '';
    if (!rr || !rr.ok) { alert((rr && (await rr.text()).trim()) || 'Could not read file'); return; }
    const d = await rr.json();
    editRoster(ov, { name: file.files[0]?.name?.replace(/\.\w+$/, '') || 'Roster', pods: d.teams });
  });
  box.querySelectorAll('.ros-pub').forEach((b) => b.addEventListener('click', async () => {
    const r=await req('/api/rosters/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ public: b.dataset.pub !== '1' }) });
    if(!r?.ok) { showError(ov,r ? await r.text() : 'Could not change visibility. Try again.'); return; }
    renderList(ov);
  }));
  box.querySelectorAll('.ros-edit').forEach((b) => b.addEventListener('click', async () => {
    const rr = await req('/api/rosters/' + b.dataset.id);
    if (!rr || !rr.ok) return;
    editRoster(ov, await rr.json());
  }));
  box.querySelectorAll('.ros-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm(`Delete roster "${b.dataset.name}"?`)) return;
    const r=await req('/api/rosters/' + b.dataset.id, { method: 'DELETE' });
    if(!r?.ok) { showError(ov,r ? await r.text() : 'Could not delete roster. Try again.'); return; }
    renderList(ov);
  }));
}

function podRow(p) {
  return `<tr>
    <td><input class="form-control rp-name" aria-label="Pod name" value="${esc(p.name)}" placeholder="Pod name"></td>
    <td><input class="form-control rp-loc" aria-label="Site" value="${esc(p.location || '')}" placeholder="Site"></td>
    <td style="text-align:center"><input class="form-check-input rp-pair" aria-label="Pairing enabled" type="checkbox" ${p.pairing ? 'checked' : ''}></td>
    <td><input class="form-control rp-dev" aria-label="Developer count" type="number" min="0" value="${p.devCount || 0}" style="width:56px"></td>
    <td><input class="form-control rp-lane" aria-label="Parallel work lanes" type="number" min="0" value="${p.streams || ''}" placeholder="auto" style="width:60px"></td>
    <td><button type="button" class="btn btn-secondary rp-del">Remove pod</button></td></tr>`;
}

function editRoster(ov, roster) {
  const box = ov.querySelector('#rosters-body');
  const readOnly = roster.id && roster.mine === false; // a shared roster owned by someone else
  box.innerHTML = `
    <div class="games-create">
      <label>Roster name <input class="form-control" id="ros-name" value="${esc(roster.name || '')}" placeholder="Roster name" style="min-width:220px" ${readOnly ? 'disabled' : ''}></label>
      ${readOnly ? '' : '<button class="btn btn-secondary" id="ros-add">+ Add pod</button> <button id="ros-save" class="btn btn-primary primary">Save roster</button>'}
      <button type="button" class="btn btn-link p-0 plan-back" id="ros-back">Back to rosters</button>
      <span id="ros-status" class="hint" role="status" aria-live="polite">${readOnly ? `read-only — shared by ${roster.owner ? 'another manager' : 'system'}` : ''}</span>
    </div>
    <table class="table table-sm wip-table"><thead><tr><th>Pod</th><th>Site</th><th>Pairing</th><th>Devs</th><th>Lanes</th><th></th></tr></thead>
      <tbody id="ros-rows">${(roster.pods && roster.pods.length ? roster.pods : [{}]).map(podRow).join('')}</tbody></table>
    <p class="hint">Lanes = parallel work-streams (capacity). Leave blank to derive from Devs + Pairing (pairing ≈ Devs÷2). Pod names must match the Jira pod field to join activity.</p>`;
  const rows = box.querySelector('#ros-rows');
  const bindDel = () => rows.querySelectorAll('.rp-del').forEach((b) => { b.onclick = () => b.closest('tr').remove(); });
  bindDel();
  box.querySelector('#ros-back').addEventListener('click', () => renderList(ov));
  box.querySelector('#ros-add')?.addEventListener('click', () => { rows.insertAdjacentHTML('beforeend', podRow({ pairing: true })); bindDel(); });
  box.querySelector('#ros-save')?.addEventListener('click', async () => {
    const name = box.querySelector('#ros-name').value.trim();
    if (!name) { box.querySelector('#ros-name').focus(); return; }
    const pods = [...rows.querySelectorAll('tr')].map((tr) => ({
      name: tr.querySelector('.rp-name').value.trim(),
      location: tr.querySelector('.rp-loc').value.trim(),
      pairing: tr.querySelector('.rp-pair').checked,
      devCount: +tr.querySelector('.rp-dev').value || 0,
      streams: +tr.querySelector('.rp-lane').value || 0,
    })).filter((p) => p.name);
    const body = JSON.stringify({ name, pods });
    const r = roster.id
      ? await req('/api/rosters/' + roster.id, { method: 'PATCH', body })
      : await req('/api/rosters', { method: 'POST', body });
    if (!r || !r.ok) { const status=box.querySelector('#ros-status'); status.textContent=(r ? (await r.text()).trim() : '') || 'Save failed. Your edits are still here; try again.'; status.setAttribute('role','alert'); return; }
    box.querySelector('#ros-status').innerHTML = `<span style="color:var(--green)">Saved ${pods.length} pods</span>`;
    setTimeout(() => renderList(ov), 600);
  });
}
