import { openModal, closeModal } from './modal.js';
// Rosters: reusable, editable team-structure definitions (pods: name, site,
// pairing, headcount, lanes). Created/uploaded once, edited anytime, and
// associated with a Jira import. Manager-only.
import { authFetch } from './auth.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
async function req(p, o) { try { return await authFetch(p, o); } catch { return null; } }

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
      <div class="modal-head"><h2>👥 Team rosters</h2><button id="rosters-close">✕</button></div>
      <p class="hint">Reusable team structure — headcount, pairing, site and work-lanes. A Jira import
        joins a roster to live activity by pod name. Edit anytime; re-associate a snapshot from Observe ▸ 🗂 Snapshots.</p>
      <div id="rosters-body"></div>
    </div>`;
  openModal(ov);
  ov.querySelector('#rosters-close').addEventListener('click', () => closeModal(ov));
  renderList(ov);
}

// Mounts the rosters section into an existing container — used standalone above,
// and by snapshotsui.js's combined "Snapshots" view (Rosters + Jira snapshots
// are both "things captured/uploaded"; Snapshots is where you see both at once).
export async function mountRosters(container) {
  container.innerHTML = `<div id="rosters-body"></div>`;
  renderList(container);
}

async function renderList(ov) {
  const box = ov.querySelector('#rosters-body');
  const r = await req('/api/rosters');
  const rosters = (r && r.ok) ? (await r.json()) || [] : [];
  const fmt = (ts) => (ts ? new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: '2-digit' }) : '');
  box.innerHTML = `
    <div class="games-create">
      <button id="ros-new" class="primary">+ New roster</button>
      <button id="ros-upload">⬆ New from CSV/XLSX</button>
      <a class="hint" href="/api/sample/roster.csv">⬇ sample format</a>
      <input id="ros-file" type="file" accept=".csv,.xlsx" hidden>
    </div>
    <table class="wip-table"><thead><tr><th>Name</th><th>Pods</th><th>Visibility</th><th>Updated</th><th></th></tr></thead>
      <tbody>${rosters.map((r) => {
    const vis = r.public ? '<span class="flag" style="color:var(--green)">public</span>' : '<span class="flag">private</span>';
    return `<tr>
        <td><b>${esc(r.name)}</b>${!r.mine ? ` <span class="hint">· shared by ${r.owner ? esc(r.owner) : 'system'}</span>` : ''}</td>
        <td>${r.podCount}</td>
        <td>${r.mine ? `${vis} <button class="ros-pub" data-id="${r.id}" data-pub="${r.public ? 1 : 0}">${r.public ? 'make private' : 'make public'}</button>` : vis}</td>
        <td>${fmt(r.updatedAt)}</td>
        <td>${r.mine
      ? `<button class="ros-edit" data-id="${r.id}">edit</button> <button class="ros-del" data-id="${r.id}" data-name="${esc(r.name)}">delete</button>`
      : `<button class="ros-edit" data-id="${r.id}">view</button>`}</td>
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
    await req('/api/rosters/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ public: b.dataset.pub !== '1' }) });
    renderList(ov);
  }));
  box.querySelectorAll('.ros-edit').forEach((b) => b.addEventListener('click', async () => {
    const rr = await req('/api/rosters/' + b.dataset.id);
    if (!rr || !rr.ok) return;
    editRoster(ov, await rr.json());
  }));
  box.querySelectorAll('.ros-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm(`Delete roster "${b.dataset.name}"?`)) return;
    await req('/api/rosters/' + b.dataset.id, { method: 'DELETE' });
    renderList(ov);
  }));
}

function podRow(p) {
  return `<tr>
    <td><input class="rp-name" value="${esc(p.name)}" placeholder="Pod name"></td>
    <td><input class="rp-loc" value="${esc(p.location || '')}" placeholder="Site"></td>
    <td style="text-align:center"><input class="rp-pair" type="checkbox" ${p.pairing ? 'checked' : ''}></td>
    <td><input class="rp-dev" type="number" min="0" value="${p.devCount || 0}" style="width:56px"></td>
    <td><input class="rp-lane" type="number" min="0" value="${p.streams || ''}" placeholder="auto" style="width:60px"></td>
    <td><button class="rp-del">✕</button></td></tr>`;
}

function editRoster(ov, roster) {
  const box = ov.querySelector('#rosters-body');
  const readOnly = roster.id && roster.mine === false; // a shared roster owned by someone else
  box.innerHTML = `
    <div class="games-create">
      <input id="ros-name" value="${esc(roster.name || '')}" placeholder="Roster name" style="min-width:220px" ${readOnly ? 'disabled' : ''}>
      ${readOnly ? '' : '<button id="ros-add">+ Add pod</button> <button id="ros-save" class="primary">Save roster</button>'}
      <a class="plan-back" id="ros-back">✕ back to list</a>
      <span id="ros-status" class="hint">${readOnly ? `read-only — shared by ${roster.owner ? 'another manager' : 'system'}` : ''}</span>
    </div>
    <table class="wip-table"><thead><tr><th>Pod</th><th>Site</th><th>Pairing</th><th>Devs</th><th>Lanes</th><th></th></tr></thead>
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
    if (!r || !r.ok) { alert((r && (await r.text()).trim()) || 'Save failed'); return; }
    box.querySelector('#ros-status').innerHTML = `<span style="color:var(--green)">✓ saved ${pods.length} pods</span>`;
    setTimeout(() => renderList(ov), 600);
  });
}
