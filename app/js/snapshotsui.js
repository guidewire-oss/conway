// Snapshots: a single page to see everything captured/uploaded — rosters
// (from Observe ▸ 👥 Rosters) and dated Jira snapshots (from Observe ▸ 📥
// Import from Jira) — as two sections. Rename/delete the Jira snapshots you
// own; the baseline (mined seed) is protected from deletion. Manager/admin only.
import { authFetch } from './auth.js';
import { listSnapshots, getSnapshot } from './data.js';
import { mountRosters } from './rostersui.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
async function req(p, o) { try { return await authFetch(p, o); } catch { return null; } }

export async function openSnapshots() {
  let ov = document.getElementById('snapshots-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'snapshots-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
    // no click-outside-to-close — the ✕ button is the deliberate exit.
  }
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>🗂 Snapshots</h2><button id="snap-close">✕</button></div>
      <h3>👥 Rosters</h3>
      <p class="hint">Team structure — headcount, pairing, site and work-lanes — uploaded via Observe ▸ 👥 Rosters.</p>
      <div id="snap-rosters"></div>
      <h3 style="margin-top:20px">📥 Jira snapshots</h3>
      <p class="hint">Dated captures Observe renders and Train seeds from, built via Observe ▸ 📥 Import from Jira.
        Rename or delete the ones you own; the baseline (mined seed) is kept.</p>
      <div id="snap-list"></div>
    </div>`;
  ov.hidden = false;
  ov.querySelector('#snap-close').addEventListener('click', () => { ov.hidden = true; });
  mountRosters(ov.querySelector('#snap-rosters'));
  renderList(ov);
}

const scopeText = (s) => (Array.isArray(s.scope) && s.scope.length ? s.scope.join(', ') : '');

async function fetchRosters() {
  const r = await req('/api/rosters');
  return (r && r.ok) ? (await r.json()) || [] : [];
}

async function renderList(ov) {
  const box = ov.querySelector('#snap-list');
  const snaps = await listSnapshots();
  if (!snaps.length) { box.innerHTML = '<p class="hint">No snapshots yet.</p>'; return; }
  const rosters = await fetchRosters();
  const rosterCell = (s) => {
    if (s.source !== 'jira' || !s.mine) return rosters.find((r) => r.id === s.rosterId)?.name || '—';
    const opts = `<option value="">— roster —</option>` + rosters.map((r) => `<option value="${r.id}" ${r.id === s.rosterId ? 'selected' : ''}>${esc(r.name)}</option>`).join('');
    return `<select class="snap-roster" data-id="${s.id}">${opts}</select>`;
  };
  const fmtDate = (ts) => (ts ? new Date(ts * 1000).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : '');
  box.innerHTML = `<table class="wip-table">
    <thead><tr><th>Name</th><th>Source</th><th>Created by</th><th>Visibility</th><th>Roster</th><th>Created</th><th></th></tr></thead>
    <tbody>${snaps.map((s) => {
    const baseline = s.source === 'baseline';
    const current = s.id === getSnapshot() ? ' <span class="hint">(viewing)</span>' : '';
    const scope = scopeText(s);
    const vis = baseline ? '<span class="hint">built-in</span>'
      : s.public ? '<span class="flag" style="color:var(--green)">public</span>'
        : '<span class="flag">private</span>';
    const owned = s.mine && !baseline;
    return `<tr>
        <td><b>${esc(s.name || s.id)}</b>${current}${scope ? ` <span class="hint">(${esc(scope)})</span>` : ''}</td>
        <td>${esc(s.source)}</td>
        <td>${baseline ? '<span class="hint">system</span>' : esc(s.owner)}</td>
        <td>${vis}${owned ? ` <button class="snap-pub" data-id="${s.id}" data-pub="${s.public ? 1 : 0}">${s.public ? 'make private' : 'make public'}</button>` : ''}</td>
        <td>${rosterCell(s)}</td>
        <td>${baseline ? '—' : fmtDate(s.createdAt)}</td>
        <td>${owned ? `<button class="snap-rename" data-id="${s.id}" data-name="${esc(s.name || '')}">rename</button>
          <button class="snap-del" data-id="${s.id}" data-name="${esc(s.name || s.id)}">delete</button>` : ''}</td>
      </tr>`;
  }).join('')}</tbody></table>`;

  box.querySelectorAll('.snap-roster').forEach((sel) => sel.addEventListener('change', async () => {
    if (!sel.value) return; // structure must come from some roster — ignore the blank option
    const r = await req('/api/snapshots/' + sel.dataset.id, { method: 'PATCH', body: JSON.stringify({ rosterId: sel.value }) });
    if (r && !r.ok) { alert((await r.text()).trim() || 'Could not re-associate'); return; }
    // structure changed — if viewing this snapshot, reload so Observe re-reads it
    if (sel.dataset.id === getSnapshot()) location.reload(); else renderList(ov);
  }));
  box.querySelectorAll('.snap-pub').forEach((b) => b.addEventListener('click', async () => {
    const r = await req('/api/snapshots/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ public: b.dataset.pub !== '1' }) });
    if (r && !r.ok) { alert((await r.text()).trim() || 'Could not change visibility'); return; }
    renderList(ov);
  }));
  box.querySelectorAll('.snap-rename').forEach((b) => b.addEventListener('click', async () => {
    const name = prompt('Rename snapshot:', b.dataset.name);
    if (name == null) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    const r = await req('/api/snapshots/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ name: trimmed }) });
    if (r && !r.ok) { alert((await r.text()).trim() || 'Rename failed'); return; }
    renderList(ov);
  }));
  box.querySelectorAll('.snap-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm(`Delete snapshot "${b.dataset.name}"? Games already seeded from it keep playing; this only removes the stored capture.`)) return;
    const r = await req('/api/snapshots/' + b.dataset.id, { method: 'DELETE' });
    if (r && !r.ok) { alert((await r.text()).trim() || 'Delete failed'); return; }
    // if we deleted the snapshot currently being viewed, drop back to baseline
    if (b.dataset.id === getSnapshot()) {
      const u = new URL(location.href); u.searchParams.delete('snapshot'); location.assign(u); return;
    }
    renderList(ov);
  }));
}
