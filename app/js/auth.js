import { openModal, closeModal } from './modal.js';
// Login gate + admin panel. Detects whether the Conway Go server is present:
// - server present, no/invalid token  -> show login overlay (enforced)
// - server present, valid token        -> proceed, expose role
// - no server (plain static hosting)   -> dev mode, no auth (same files either way)
import { openGames } from './gamesui.js';

const TOKEN_KEY = 'conway_token';
let roles = ['player'];
let username = null;
let gameId = '';
let mode = 'none';
// a facilitator's "🧪 test this game" link carries its token in the URL and
// keeps it in memory only — never localStorage — so opening it in a new tab
// can't clobber the facilitator's own signed-in session in another tab.
let testToken = null;

export function authToken() { return testToken || localStorage.getItem(TOKEN_KEY); }
export function authGameID() { return gameId; } // non-empty for a joined team
export function authUser() { return username; }
export function authRoles() { return roles; }
// admin is a superuser, so it satisfies every role check.
export function hasRole(r) { return roles.includes(r) || roles.includes('admin'); }
// staff = anyone who isn't a plain team player (admin / facilitator / manager).
export function isStaff() { return hasRole('admin') || hasRole('facilitator') || hasRole('manager'); }
// representative role (kept for callers that want a single label)
export function authRole() { return roles.includes('admin') ? 'admin' : (roles[0] || 'player'); }
export function authMode() { return mode; }

async function api(path, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  const tok = authToken();
  if (tok) headers.Authorization = `Bearer ${tok}`;
  // JSON by default, but let the browser set the multipart boundary for uploads
  if (opts.body && !(opts.body instanceof FormData)) headers['Content-Type'] = 'application/json';
  return fetch(path, { ...opts, headers });
}
export { api as authFetch };

// After an SSO round-trip the server bounces back to /#sso=<token>. Read it,
// persist it, and strip the fragment so a reload or shared URL can't leak or
// replay the token. Returns true when a token was picked up.
function pickUpSSOToken() {
  const h = location.hash || '';
  const m = h.match(/[#&]sso=([^&]+)/);
  if (!m) return false;
  try { localStorage.setItem(TOKEN_KEY, decodeURIComponent(m[1])); } catch { return false; }
  history.replaceState(null, '', location.pathname + location.search);
  return true;
}

// A denied/failed SSO attempt returns to /?sso_error=<reason>. Map it to a
// human message for the login overlay, and strip the param.
function ssoErrorMessage() {
  const p = new URLSearchParams(location.search);
  const reason = p.get('sso_error');
  if (!reason) return '';
  const msg = {
    no_role: 'Your account has no Conway role. Ask an admin to grant access in your identity provider.',
    invalid_token: 'Sign-in failed: the identity provider response could not be verified.',
    exchange_failed: 'Sign-in failed while contacting the identity provider. Try again.',
    invalid_request: 'Sign-in link was invalid or expired. Try again.',
    access_denied: 'Sign-in was cancelled.',
  }[reason] || 'Single sign-on failed. Try again.';
  const u = new URL(location.href); u.searchParams.delete('sso_error'); history.replaceState(null, '', u);
  return msg;
}

export async function initAuth() {
  pickUpSSOToken();
  const linkToken = new URLSearchParams(location.search).get('testtoken');
  if (linkToken) {
    testToken = linkToken;
    try {
      const r = await api('/api/me');
      if (r.ok) { const d = await r.json(); roles = d.roles || ['player']; username = d.username; gameId = d.gameId || ''; mode = 'auth'; return mountChip(); }
    } catch { /* fall through to the normal sign-in flow */ }
    testToken = null;
  }
  const tok = authToken();
  if (tok) {
    try {
      const r = await api('/api/me');
      if (r.ok) { const d = await r.json(); roles = d.roles || ['player']; username = d.username; gameId = d.gameId || ''; mode = 'auth'; return mountChip(); }
      localStorage.removeItem(TOKEN_KEY);
    } catch { /* fall through */ }
  }
  // probe for a server (clean 200, no console noise)
  try {
    const r = await fetch('/api/config');
    if (r.ok) { const cfg = await r.json(); await showLogin(cfg); return mountChip(); }
  } catch { /* no server */ }
  mode = 'none';
  return Promise.resolve();
}

function showLogin(cfg = {}) {
  return new Promise((resolve) => {
    const safeCode = (new URLSearchParams(location.search).get('join') || '').replace(/[^A-Za-z0-9]/g, '').toUpperCase();
    const joinFirst = safeCode !== '';
    const ov = document.createElement('div');
    ov.id = 'login-overlay';
    ov.innerHTML = `
      <div id="login-box">
        <h2>Conway</h2>
        <div class="login-tabs">
          <button class="tab ${joinFirst ? '' : 'active'}" id="tab-signin">Sign in</button>
          <button class="tab ${joinFirst ? 'active' : ''}" id="tab-join">Join a game</button>
        </div>
        <form id="signin-form" ${joinFirst ? 'hidden' : ''}>
          <p class="hint">Facilitators, managers &amp; admins.</p>
          ${cfg.oidc ? `<button type="button" id="login-sso" class="primary sso-btn">Sign in with SSO</button>
          <div class="sso-divider"><span>or</span></div>` : ''}
          <input id="login-user" placeholder="username" autocomplete="username">
          <input id="login-pass" type="password" placeholder="password" autocomplete="current-password">
          <button type="submit" class="primary">Sign in</button>
        </form>
        <form id="join-form" ${joinFirst ? '' : 'hidden'}>
          <p class="hint">Enter your join code. A team name is only needed for a shared game code.</p>
          <input id="join-code" placeholder="join code" value="${safeCode}" style="text-transform:uppercase">
          <input id="join-team" placeholder="team name (optional)">
          <button type="submit" class="primary">Join</button>
        </form>
        <div id="login-err" class="login-err"></div>
      </div>`;
    document.body.appendChild(ov);
    const err = (m) => { ov.querySelector('#login-err').textContent = m; };
    const ssoBtn = ov.querySelector('#login-sso');
    if (ssoBtn) ssoBtn.addEventListener('click', async () => {
      err('');
      try {
        const r = await fetch('/api/oidc/start');
        if (!r.ok) { err('SSO is unavailable right now.'); return; }
        const d = await r.json();
        location.assign(d.url); // hand off to the identity provider
      } catch { err('Cannot reach server.'); }
    });
    const show = (which) => {
      ov.querySelector('#signin-form').hidden = which !== 'signin';
      ov.querySelector('#join-form').hidden = which !== 'join';
      ov.querySelector('#tab-signin').classList.toggle('active', which === 'signin');
      ov.querySelector('#tab-join').classList.toggle('active', which === 'join');
      err('');
    };
    ov.querySelector('#tab-signin').addEventListener('click', () => show('signin'));
    ov.querySelector('#tab-join').addEventListener('click', () => show('join'));
    // Surface a denied/failed SSO round-trip (e.g. no recognized role).
    const ssoErr = ssoErrorMessage();
    if (ssoErr) { if (!joinFirst) show('signin'); err(ssoErr); }
    ov.querySelector('#signin-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const u = ov.querySelector('#login-user').value.trim();
      const p = ov.querySelector('#login-pass').value;
      try {
        const r = await fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: u, password: p }) });
        if (!r.ok) { err('Invalid credentials or expired account.'); return; }
        const d = await r.json();
        localStorage.setItem(TOKEN_KEY, d.token);
        roles = d.roles || ['player']; username = d.username; gameId = d.gameId || ''; mode = 'auth';
        ov.remove(); resolve();
      } catch { err('Cannot reach server.'); }
    });
    ov.querySelector('#join-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const code = ov.querySelector('#join-code').value.trim().toUpperCase();
      const team = ov.querySelector('#join-team').value.trim();
      if (!code) { err('Enter your join code.'); return; }
      try {
        const r = await fetch('/api/games/join', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code, team }) });
        if (!r.ok) { err((await r.text()).trim() || 'Could not join that game.'); return; }
        const d = await r.json();
        localStorage.setItem(TOKEN_KEY, d.token);
        roles = ['player']; username = d.team; gameId = d.gameId; mode = 'auth';
        ov.remove(); resolve();
      } catch { err('Cannot reach server.'); }
    });
  });
}

function logout() {
  localStorage.removeItem(TOKEN_KEY);
  if (testToken) { // drop ?testtoken= too, or reload would just re-authenticate as the tester
    const u = new URL(location.href); u.searchParams.delete('testtoken'); location.assign(u); return;
  }
  location.reload();
}

function mountChip() {
  if (mode !== 'auth') return;
  const nav = document.querySelector('header nav');
  const chip = document.createElement('span');
  chip.className = 'auth-chip';
  const label = testToken ? `testing as ${username.replace(/^__test__:/, '')}` : username;
  chip.innerHTML = `${esc(label)} · <a id="auth-logout">sign out</a>`;
  nav.appendChild(chip);
  chip.querySelector('#auth-logout').addEventListener('click', logout);
  // facilitators (and admins, as superusers) get the game-ops console, grouped
  // under Train ▾ alongside "Play the game" (role-gated — players never see it).
  if (hasRole('facilitator')) {
    const trainMenu = document.getElementById('train-menu');
    const games = document.createElement('button');
    games.id = 'run-games-btn';
    games.className = 'tab dropdown-item';
    games.textContent = '🎮 Run games';
    games.title = 'Create and run games — scenarios, join codes, rounds, leaderboard';
    games.addEventListener('click', openGames);
    if (trainMenu) trainMenu.appendChild(games); else nav.appendChild(games);
  }
  // Admin panel is users & roles only — admins.
  if (hasRole('admin')) {
    const guideBtn = document.getElementById('guide-btn');
    const b = document.createElement('button');
    b.id = 'admin-btn';
    b.className = 'tab admin-tab';
    b.textContent = '⚙ Admin';
    b.addEventListener('click', openAdmin);
    if (guideBtn) guideBtn.before(b); else nav.appendChild(b);
  }
}

// ---- admin panel (users & roles) ---------------------------------------

function openAdmin() {
  let ov = document.getElementById('admin-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'admin-overlay';
    ov.innerHTML = `
      <div id="admin-modal">
        <div class="guide-head"><h2>Admin — users &amp; roles</h2><button id="admin-close">✕</button></div>
        <div id="admin-accounts">
          <div class="admin-create">
            <input id="admin-disp" placeholder="Name (person or team)">
            <span class="role-pick" title="A user can hold several roles">
              <label><input type="checkbox" class="role-cb" value="facilitator" checked> Facilitator</label>
              <label><input type="checkbox" class="role-cb" value="manager"> Manager</label>
              <label><input type="checkbox" class="role-cb" value="admin"> Admin</label>
            </span>
            <label class="hint">expires <input id="admin-exp" type="date" value="${defaultExpiry()}" style="width:140px"></label>
            <button id="admin-add" class="primary">Create user</button>
            <span id="admin-new" class="admin-new"></span>
          </div>
          <table id="admin-users" class="wip-table"></table>
        </div>
        <p class="hint">Run games — rounds, the team roster &amp; the leaderboard — from the 🎮 Games panel.</p>
      </div>`;
    document.body.appendChild(ov);
    // no click-outside-to-close — the ✕ button is the deliberate exit.
    ov.querySelector('#admin-close').addEventListener('click', closeAdmin);
    ov.querySelector('#admin-add').addEventListener('click', addUser);
  }
  openModal(ov);
  refreshUsers();
}
function closeAdmin() {
  closeModal(document.getElementById('admin-overlay'));
}

// defaultExpiry: today + 30 days, ISO date — the form's default horizon.
function defaultExpiry() {
  const d = new Date(Date.now() + 30 * 24 * 3600 * 1000);
  return d.toISOString().slice(0, 10);
}

// hoursUntil: whole hours from now to the picked date (min 1 — an expiry in
// the past is a mistake, not a request to expire someone now).
function hoursUntil(dateStr) {
  const t = new Date(dateStr + 'T23:59:59').getTime();
  return Math.max(1, Math.ceil((t - Date.now()) / 3600000));
}

// hoursFrom: hours needed to move an account whose expiry is `fromEpoch`
// (seconds) onto the picked date — Extend() adds to the current expiry, so the
// request must be the delta, not the absolute horizon.
function hoursFrom(dateStr, fromEpoch) {
  const t = new Date(dateStr + 'T23:59:59').getTime();
  const base = fromEpoch > 0 ? fromEpoch * 1000 : Date.now();
  return Math.max(1, Math.ceil((t - base) / 3600000));
}

async function addUser() {
  const disp = document.getElementById('admin-disp').value.trim();
  const exp = document.getElementById('admin-exp').value ? hoursUntil(document.getElementById('admin-exp').value) : 720;
  const roles = Array.from(document.querySelectorAll('.role-cb:checked')).map((c) => c.value);
  if (!disp) return;
  if (!roles.length) { document.getElementById('admin-new').textContent = 'pick at least one role'; return; }
  const r = await api('/api/admin/users', { method: 'POST', body: JSON.stringify({ display: disp, expiryHours: exp, roles }) });
  const d = await r.json();
  document.getElementById('admin-new').innerHTML =
    `created <b>${esc(d.username)}</b> (${esc((d.roles || []).join(', '))}) · password <code>${esc(d.password)}</code> <span class="hint">(copy now — not shown again)</span>`;
  document.getElementById('admin-disp').value = '';
  refreshUsers();
}

// HTML-escape untrusted values before interpolating into innerHTML. Account
// fields like display name and username are IdP-controlled for SSO users (the
// name claim is often self-editable), so they must never be treated as markup.
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));
}

async function refreshUsers() {
  const r = await api('/api/admin/users');
  const users = await r.json();
  const fmt = (ts) => (ts ? new Date(ts * 1000).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'never');
  const BADGE = { admin: 'var(--violet)', facilitator: 'var(--accent)', manager: 'var(--green)', player: 'var(--muted)' };
  const label = (r) => r === 'player' ? 'team' : r;
  const roleBadges = (rs) => (rs || []).map((r) => `<span class="flag" style="color:${BADGE[r] || 'var(--muted)'}">${esc(label(r))}</span>`).join(' ');
  // The extend picker defaults to the user's current expiry (or +1 day when
  // none), so pushing a date out is a delta on what's already true.
  const extDefault = (u) => {
    const base = u.expiresAt > 0 ? u.expiresAt * 1000 : Date.now() + 24 * 3600 * 1000;
    return new Date(base).toISOString().slice(0, 10);
  };
  document.getElementById('admin-users').innerHTML = `
    <thead><tr><th>Username</th><th>Name</th><th>Roles</th><th>Expires</th><th>Status</th><th></th></tr></thead>
    <tbody>${users.map((u) => `<tr>
      <td>${esc(u.username)}${u.sso ? ' <span class="flag" style="color:var(--violet)">SSO</span>' : ''}</td>
      <td>${esc(u.display)}</td><td>${roleBadges(u.roles)}</td><td>${u.sso ? '<span class="hint">via IdP</span>' : fmt(u.expiresAt)}</td>
      <td>${u.expired ? '<span class="flag red">expired</span>' : '<span class="flag" style="color:var(--green)">active</span>'}
          ${u.hasState ? '<span class="hint">playing</span>' : ''}</td>
      <td>${u.sso ? '' : `<span class="admin-ext"><input type="date" class="admin-ext-date" data-ext="${esc(u.username)}" value="${extDefault(u)}" title="new expiry date"><button data-extbtn="${esc(u.username)}">extend</button></span>`}${u.username === 'admin' ? '' : ` <button data-del="${esc(u.username)}">revoke</button>`}</td>
    </tr>`).join('') || '<tr><td colspan="6" class="hint">No accounts yet.</td></tr>'}`;
  document.querySelectorAll('#admin-users [data-extbtn]').forEach((b) => b.addEventListener('click', async () => {
    const u = users.find((x) => x.username === b.dataset.extbtn);
    const input = document.querySelector(`.admin-ext-date[data-ext="${CSS.escape(b.dataset.extbtn)}"]`);
    // "Extend to <date>" must land ON that date, not add (date - today) to the
    // current expiry — the picker defaults to the current expiry, so the
    // overshoot was exactly (current - today) ≈ months.
    const hours = input && input.value ? hoursFrom(input.value, u && u.expiresAt) : 24;
    await api(`/api/admin/users/${b.dataset.extbtn}/extend`, { method: 'POST', body: JSON.stringify({ hours }) }); refreshUsers();
  }));
  document.querySelectorAll('#admin-users [data-del]').forEach((b) => b.addEventListener('click', async () => {
    await api(`/api/admin/users/${b.dataset.del}`, { method: 'DELETE' }); refreshUsers();
  }));
}

