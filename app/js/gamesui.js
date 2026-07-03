// Facilitator panel for multi-game: create games (with a scenario), share the
// join code/link, open rounds, project the per-game leaderboard, reset/delete.
import { authFetch } from './auth.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
async function req(p, o) { try { return await authFetch(p, o); } catch { return null; } }

export async function openGames() {
  let ov = document.getElementById('games-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'games-overlay';
    ov.innerHTML = `<div id="games-modal">
      <div class="guide-head"><h2>Games</h2><button id="games-close">✕</button></div>
      <div class="games-create">
        <input id="g-name" placeholder="Game name (e.g. Q3 Offsite)">
        <label class="hint">rounds <input id="g-rounds" type="number" min="1" max="8" value="4" style="width:46px"></label>
        <label class="hint">AP <input id="g-ap" type="number" min="2" max="6" value="5" style="width:42px"></label>
        <label class="hint">timer s <input id="g-timer" type="number" min="0" max="3600" value="300" style="width:62px"></label>
        <select id="g-scenario" title="Scenario / difficulty (seed)"></select>
        <button id="g-create" class="primary">Create game</button>
      </div>
      <div id="games-editor"></div>
      <div id="games-list"></div>
      <div id="games-roster"></div>
      <div id="games-scenarios"></div>
    </div>`;
    document.body.appendChild(ov);
    // no click-outside-to-close — an in-progress "create game" form or edit
    // shouldn't vanish on a stray click; the ✕ button is the deliberate exit.
    ov.querySelector('#games-close').addEventListener('click', () => { ov.hidden = true; });
    ov.querySelector('#g-create').addEventListener('click', createGame);
  }
  ov.hidden = false;
  refreshGames();
  populateScenario(document.getElementById('g-scenario'), 'default');
  renderScenarios();
}

// difficulty presets (seed the mined baseline world at varying hardness)
const PRESETS = [
  ['default', 'Default world'],
  ['balanced', 'Balanced'],
  ['constrained', 'Constrained'],
  ['crisis', 'Crisis'],
];

async function fetchPlans() {
  const r = await req('/api/plan'); // 403s for non-managers — treated as "no plans"
  if (!r || !r.ok) return [];
  return (await r.json()) || [];
}

// every snapshot/template the facilitator may seed from (own + public + baseline)
async function fetchSeeds() {
  const r = await req('/api/snapshots');
  if (!r || !r.ok) return [];
  return (await r.json()) || [];
}

const seedLabel = (s) => (s.name || s.id) + (s.public && !s.mine ? ` · shared by ${s.owner}` : s.public ? ' · public' : '');

// Build the grouped scenario <select>: difficulty presets, live org snapshots,
// scenario templates, and (if any) plans — each in a labelled optgroup.
async function populateScenario(sel, selected) {
  if (!sel) return;
  const [plans, seeds] = await Promise.all([fetchPlans(), fetchSeeds()]);
  const group = (label, opts) => (opts.length
    ? `<optgroup label="${label}">${opts.map(([v, l]) => `<option value="${esc(v)}" ${v === selected ? 'selected' : ''}>${esc(l)}</option>`).join('')}</optgroup>` : '');
  const live = seeds.filter((s) => s.source === 'jira' || s.source === 'baseline').map((s) => ['snap:' + s.id, seedLabel(s)]);
  const templates = seeds.filter((s) => s.source === 'template').map((s) => ['snap:' + s.id, seedLabel(s)]);
  const planOpts = plans.map((p) => ['plan:' + p.id, p.name || p.id]);
  let html = group('Difficulty presets', PRESETS) + group('Live org snapshots', live)
    + group('Scenario templates', templates) + group('Plans', planOpts);
  if (selected && !html.includes(`value="${esc(selected)}"`)) html += `<option value="${esc(selected)}" selected>${esc(selected)}</option>`;
  sel.innerHTML = html;
}

async function createGame() {
  const name = document.getElementById('g-name').value.trim();
  if (!name) { document.getElementById('g-name').focus(); alert('Enter a game name.'); return; }
  const body = {
    name,
    rounds: +document.getElementById('g-rounds').value || 4,
    ap: +document.getElementById('g-ap').value || 5,
    timerSecs: +document.getElementById('g-timer').value || 0,
    scenario: document.getElementById('g-scenario').value,
    expiryHours: 48,
  };
  const r = await req('/api/games', { method: 'POST', body: JSON.stringify(body) });
  if (!r || !r.ok) { alert((r && (await r.text()).trim()) || 'Could not create game'); return; }
  document.getElementById('g-name').value = '';
  refreshGames();
}

// inline editor: change a game's name, round rules and scenario seed.
async function editGame(gid) {
  const box = document.getElementById('games-editor');
  const r = await req('/api/games/' + gid);
  if (!r || !r.ok) { return; }
  const g = (await r.json()).game;
  box.innerHTML = `
    <h3 style="margin-top:16px">Edit “${esc(g.name)}” <a class="plan-back" id="edit-close">✕ close</a></h3>
    <div class="games-create">
      <input id="eg-name" value="${esc(g.name)}" placeholder="Game name">
      <label class="hint">rounds <input id="eg-rounds" type="number" min="1" max="8" value="${g.rounds}" style="width:46px"></label>
      <label class="hint">AP <input id="eg-ap" type="number" min="2" max="6" value="${g.ap}" style="width:42px"></label>
      <label class="hint">timer s <input id="eg-timer" type="number" min="30" max="3600" value="${g.timerSecs}" style="width:62px"></label>
      <select id="eg-scenario" title="Scenario / difficulty (seed)"></select>
      <button id="eg-save" class="primary">Save</button>
    </div>
    <p class="hint">Changing the scenario only re-seeds teams that begin play afterward.</p>`;
  populateScenario(box.querySelector('#eg-scenario'), g.scenario || 'default');
  box.querySelector('#edit-close').addEventListener('click', () => { box.innerHTML = ''; });
  box.querySelector('#eg-save').addEventListener('click', async () => {
    const name = box.querySelector('#eg-name').value.trim();
    if (!name) { box.querySelector('#eg-name').focus(); alert('Enter a game name.'); return; }
    const body = {
      name,
      rounds: +box.querySelector('#eg-rounds').value || 0,
      ap: +box.querySelector('#eg-ap').value || 0,
      timerSecs: +box.querySelector('#eg-timer').value || 0,
      scenario: box.querySelector('#eg-scenario').value,
    };
    const rr = await req('/api/games/' + gid, { method: 'PATCH', body: JSON.stringify(body) });
    if (rr && !rr.ok) { alert((await rr.text()).trim() || 'Could not save'); return; }
    box.innerHTML = '';
    refreshGames();
  });
}

const joinURL = (code) => `${location.origin}/?join=${code}`;

// per-game team roster: pre-add teams, each with its own join code/link
async function renderRoster(gid, name) {
  const box = document.getElementById('games-roster');
  const r = await req('/api/games/' + gid + '/teams');
  const teams = (r && r.ok) ? await r.json() : [];
  box.innerHTML = `
    <h3 style="margin-top:16px">Teams in “${esc(name)}” <a class="plan-back" id="roster-close">✕ close</a></h3>
    <div class="games-create">
      <input id="rt-name" placeholder="Team name (e.g. Team 1)">
      <button id="rt-add" class="primary">Add team</button>
      <span class="hint">each team gets its own join link to share</span>
    </div>
    <table class="wip-table"><thead><tr><th>Team</th><th>Join code</th><th>Status</th><th></th></tr></thead>
      <tbody>${(teams || []).map((t) => `<tr>
        <td>${esc(t.name)}</td>
        <td><b>${esc(t.code)}</b> <button class="rt-copy" data-code="${esc(t.code)}">copy link</button></td>
        <td>${t.joined ? `<span style="color:var(--green)">joined · round ${t.round}</span>` : 'not joined'}</td>
        <td><button class="rt-del" data-name="${esc(t.name)}">remove</button></td></tr>`).join('')
      || '<tr><td colspan="4" class="hint">No teams yet — add the teams that will play this game.</td></tr>'}
      </tbody></table>`;
  box.querySelector('#roster-close').addEventListener('click', () => { box.innerHTML = ''; });
  box.querySelector('#rt-add').addEventListener('click', async () => {
    const tn = box.querySelector('#rt-name').value.trim();
    if (!tn) return;
    const r = await req('/api/games/' + gid + '/teams', { method: 'POST', body: JSON.stringify({ name: tn }) });
    if (r && !r.ok) { alert((await r.text()).trim()); return; }
    renderRoster(gid, name);
  });
  box.querySelectorAll('.rt-copy').forEach((b) => b.addEventListener('click', () => {
    navigator.clipboard?.writeText(joinURL(b.dataset.code));
    b.textContent = 'copied!'; setTimeout(() => { b.textContent = 'copy link'; }, 1500);
  }));
  box.querySelectorAll('.rt-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm('Remove ' + b.dataset.name + '?')) return;
    await req('/api/games/' + gid + '/teams/' + encodeURIComponent(b.dataset.name), { method: 'DELETE' });
    renderRoster(gid, name);
  }));
}

async function refreshGames() {
  const list = document.getElementById('games-list');
  const r = await req('/api/games');
  if (!r || !r.ok) { list.innerHTML = '<p class="hint">Could not load games.</p>'; return; }
  const games = await r.json();
  if (!games || !games.length) { list.innerHTML = '<p class="hint">No games yet — create one above.</p>'; return; }
  list.innerHTML = `<table class="wip-table">
    <thead><tr><th>Game</th><th>Scenario</th><th>Join code</th><th>Status</th><th></th></tr></thead>
    <tbody>${games.map((g) => `<tr>
      <td>${esc(g.name)}</td>
      <td>${esc(g.scenario || 'default')}</td>
      <td><b>${esc(g.joinCode)}</b> <button class="g-copy" data-code="${esc(g.joinCode)}">copy link</button></td>
      <td>${g.open ? `<span style="color:var(--green)">open · round ${g.openRound}</span>` : 'closed'}</td>
      <td class="btn-row">
        <button class="g-round primary" data-id="${g.id}">open next round ▶</button>
        <button class="g-test" data-id="${g.id}">🧪 test</button>
        <button class="g-edit" data-id="${g.id}">✏️ edit</button>
        <button class="g-teams" data-id="${g.id}" data-name="${esc(g.name)}">🧑 teams</button>
        <button class="g-board" data-id="${g.id}">🏆 leaderboard</button>
        <button class="g-reset" data-id="${g.id}">reset</button>
        <button class="g-del" data-id="${g.id}">delete</button>
      </td></tr>`).join('')}</tbody></table>`;
  list.querySelectorAll('.g-edit').forEach((b) => b.addEventListener('click', () => editGame(b.dataset.id)));
  list.querySelectorAll('.g-teams').forEach((b) => b.addEventListener('click', () => renderRoster(b.dataset.id, b.dataset.name)));
  list.querySelectorAll('.g-copy').forEach((b) => b.addEventListener('click', () => {
    navigator.clipboard?.writeText(joinURL(b.dataset.code));
    b.textContent = 'copied!'; setTimeout(() => { b.textContent = 'copy link'; }, 1500);
  }));
  // guard against a slow response + a fast double-click leapfrogging a round
  list.querySelectorAll('.g-round').forEach((b) => b.addEventListener('click', async () => {
    if (b.disabled) return;
    b.disabled = true;
    try {
      const r = await req('/api/games/' + b.dataset.id + '/round', { method: 'POST' });
      if (r && !r.ok) alert((await r.text()).trim());
      await refreshGames();
    } finally {
      b.disabled = false;
    }
  }));
  // trial-run this specific game yourself, without touching the teams' game state
  list.querySelectorAll('.g-test').forEach((b) => b.addEventListener('click', async () => {
    const r = await req('/api/games/' + b.dataset.id + '/test', { method: 'POST' });
    if (!r || !r.ok) { alert((r && (await r.text()).trim()) || 'Could not start a test session'); return; }
    const { token } = await r.json();
    window.open('/?testtoken=' + encodeURIComponent(token), '_blank', 'noopener');
  }));
  list.querySelectorAll('.g-board').forEach((b) => b.addEventListener('click', () => {
    window.open('leaderboard.html?game=' + b.dataset.id, '_blank', 'noopener');
  }));
  list.querySelectorAll('.g-reset').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm('Reset this game? Clears all team play.')) return;
    await req('/api/games/' + b.dataset.id + '/reset', { method: 'POST' });
    refreshGames();
  }));
  list.querySelectorAll('.g-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm('Delete this game?')) return;
    await req('/api/games/' + b.dataset.id, { method: 'DELETE' });
    refreshGames();
  }));
}

// ---- scenario library: templates + public snapshots facilitators seed from ----

// authenticated file download (export/sample are auth-gated, so a plain link
// can't carry the bearer token — fetch the blob and save it).
async function downloadAuthed(url, filename) {
  const r = await req(url);
  if (!r || !r.ok) { alert('Download failed'); return; }
  const blob = await r.blob();
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

async function renderScenarios() {
  const box = document.getElementById('games-scenarios');
  if (!box) return;
  const seeds = (await fetchSeeds()).filter((s) => s.source !== 'baseline');
  const rows = seeds.map((s) => {
    const tmpl = s.source === 'template';
    const vis = s.public ? '<span class="flag" style="color:var(--green)">public</span>' : '<span class="flag">private</span>';
    const owner = !s.mine ? ` <span class="hint">· by ${esc(s.owner)}</span>` : '';
    return `<tr>
      <td><b>${esc(s.name || s.id)}</b>${owner}</td>
      <td>${tmpl ? 'template' : esc(s.source)}</td>
      <td>${s.mine ? vis : '<span class="hint">shared</span>'}</td>
      <td>
        <button class="sc-use" data-id="${s.id}" data-name="${esc(s.name || s.id)}">use in new game</button>
        <button class="sc-dl" data-id="${s.id}" data-name="${esc(s.name || s.id)}">⬇ download</button>
        <button class="sc-dup" data-id="${s.id}" data-name="${esc(s.name || s.id)}">📋 duplicate</button>
        ${s.mine && tmpl ? `<button class="sc-pub" data-id="${s.id}" data-pub="${s.public ? 1 : 0}">${s.public ? 'make private' : 'make public'}</button>
          <button class="sc-ren" data-id="${s.id}" data-name="${esc(s.name || '')}">rename</button>
          <button class="sc-del" data-id="${s.id}" data-name="${esc(s.name || s.id)}">delete</button>` : ''}
      </td></tr>`;
  }).join('');
  box.innerHTML = `
    <h3 style="margin-top:18px">Scenario library <span class="hint">— org snapshots &amp; editable templates you can seed games from</span></h3>
    <div class="games-create">
      <button id="sc-upload">⬆ Upload network file</button>
      <button id="sc-sample">⬇ Download sample format</button>
      <input id="sc-file" type="file" accept="application/json,.json" hidden>
      <span class="hint">Download a network → edit the JSON → upload it as a reusable template. Pods in the file are the teams.</span>
    </div>
    <table class="wip-table sortable"><thead><tr><th>Name</th><th>Type</th><th>Visibility</th><th data-nosort></th></tr></thead>
      <tbody>${rows || '<tr><td colspan="4" class="hint">No templates or shared snapshots yet — upload one, or duplicate a snapshot.</td></tr>'}</tbody></table>`;

  box.querySelector('#sc-sample').addEventListener('click', () => downloadAuthed('/api/sample/network.json', 'conway-sample.network.json'));
  const file = box.querySelector('#sc-file');
  box.querySelector('#sc-upload').addEventListener('click', () => file.click());
  file.addEventListener('change', async () => {
    if (!file.files.length) return;
    const name = prompt('Name this scenario template:', file.files[0].name.replace(/\.(network\.)?json$/i, ''));
    if (name == null) { file.value = ''; return; }
    const fd = new FormData();
    fd.append('file', file.files[0]);
    const r = await req('/api/snapshots/import-network?name=' + encodeURIComponent(name.trim()), { method: 'POST', body: fd });
    file.value = '';
    if (r && !r.ok) { alert((await r.text()).trim() || 'Upload failed'); return; }
    renderScenarios();
    populateScenario(document.getElementById('g-scenario'), document.getElementById('g-scenario')?.value);
  });
  box.querySelectorAll('.sc-use').forEach((b) => b.addEventListener('click', () => {
    const sel = document.getElementById('g-scenario');
    if (sel) sel.value = 'snap:' + b.dataset.id;
    const nm = document.getElementById('g-name'); if (nm) { nm.focus(); }
    box.scrollIntoView({ block: 'start' });
    document.getElementById('games-modal')?.scrollTo({ top: 0, behavior: 'smooth' });
  }));
  box.querySelectorAll('.sc-dl').forEach((b) => b.addEventListener('click', () => downloadAuthed('/api/snapshots/' + b.dataset.id + '/export', b.dataset.name.replace(/\W+/g, '-').toLowerCase() + '.network.json')));
  box.querySelectorAll('.sc-dup').forEach((b) => b.addEventListener('click', async () => {
    const name = prompt('Name the duplicate:', b.dataset.name + ' (copy)');
    if (name == null) return;
    const r = await req('/api/snapshots/' + b.dataset.id + '/clone', { method: 'POST', body: JSON.stringify({ name: name.trim() }) });
    if (r && !r.ok) { alert((await r.text()).trim() || 'Duplicate failed'); return; }
    renderScenarios();
    populateScenario(document.getElementById('g-scenario'), document.getElementById('g-scenario')?.value);
  }));
  box.querySelectorAll('.sc-pub').forEach((b) => b.addEventListener('click', async () => {
    await req('/api/snapshots/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ public: b.dataset.pub !== '1' }) });
    renderScenarios();
  }));
  box.querySelectorAll('.sc-ren').forEach((b) => b.addEventListener('click', async () => {
    const name = prompt('Rename template:', b.dataset.name);
    if (name == null || !name.trim()) return;
    await req('/api/snapshots/' + b.dataset.id, { method: 'PATCH', body: JSON.stringify({ name: name.trim() }) });
    renderScenarios();
    populateScenario(document.getElementById('g-scenario'), document.getElementById('g-scenario')?.value);
  }));
  box.querySelectorAll('.sc-del').forEach((b) => b.addEventListener('click', async () => {
    if (!confirm(`Delete template "${b.dataset.name}"? Games already started keep playing.`)) return;
    await req('/api/snapshots/' + b.dataset.id, { method: 'DELETE' });
    renderScenarios();
    populateScenario(document.getElementById('g-scenario'), document.getElementById('g-scenario')?.value);
  }));
}
