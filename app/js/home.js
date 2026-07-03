// Home — a neutral, role-aware overview: at-a-glance org stats from the current
// snapshot plus the actions that matter for your role. Not the guide, not the
// game board. Reads the same `state` Observe builds.
import { authUser, authRoles, hasRole, authMode } from './auth.js';
import { constraintScores } from './sim.js';
import { getSnapshot, listSnapshots } from './data.js';

const pct = (v) => `${Math.round(v * 100)}%`;
const cap = (s) => (s ? s[0].toUpperCase() + s.slice(1) : s);

export async function initHome(state) {
  const el = document.getElementById('home-body');
  if (!el) return;

  const pods = state.pods || [];
  const stats = state.stats || {};
  const edges = state.edges || [];
  const totWip = pods.reduce((s, p) => s + (stats[p.name]?.wip || 0), 0);
  const hot = pods.filter((p) => (stats[p.name]?.rho0 || 0) >= 0.85).length;
  const hygVals = pods.map((p) => state.hygiene?.[p.name]?.score).filter((v) => v != null);
  const dq = hygVals.length ? hygVals.reduce((a, b) => a + b, 0) / hygVals.length : null;
  const constraints = constraintScores(stats, edges).filter((c) => pods.some((p) => p.name === c.pod)).slice(0, 3);
  const topEdges = [...edges].sort((a, b) => b.count - a.count).slice(0, 3);

  // which snapshot are we looking at?
  let snapNote = '';
  if (authMode() === 'auth') {
    const snaps = await listSnapshots();
    const cur = snaps.find((s) => s.id === getSnapshot()) || snaps.find((s) => s.id === 'baseline');
    if (cur) {
      const when = cur.id === 'baseline' ? 'mined baseline'
        : new Date(cur.createdAt * 1000).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
      snapNote = `Showing <b>${esc(cur.name || cur.id)}</b> <span class="hint">· ${when}</span>`;
    }
  }

  const roles = (authRoles() || []).filter((r) => r !== 'player');
  const who = authUser() ? `Welcome, <b>${esc(authUser())}</b>` : 'Welcome';
  const roleBadges = roles.map((r) => `<span class="flag" style="color:var(--accent)">${cap(r)}</span>`).join(' ');

  // first run: no org snapshot yet → a clear call to action instead of zeros
  if (!pods.length) {
    el.innerHTML = `
      <div class="home-hero">
        <h1>Conway</h1>
        <p class="home-sub">${who}. ${roleBadges}</p>
      </div>
      <div class="card" style="max-width:640px">
        <h3>No org snapshot yet</h3>
        <p>Conway renders an <b>org network</b> — pods and the dependencies between them —
          captured as a dated <b>snapshot</b>. There isn't one yet.</p>
        ${hasRole('manager')
        ? '<p>Capture the current state from Jira to get started:</p><button class="home-act" data-ctl="obs-import" style="max-width:280px"><b>📥 Import from Jira</b><span class="hint">build your first snapshot</span></button>'
        : '<p class="hint">Ask a manager to import a snapshot from Jira, or (facilitators) upload a scenario under Train ▸ Run games.</p>'}
      </div>`;
    el.querySelectorAll('[data-ctl]').forEach((b) => b.addEventListener('click', () => document.getElementById(b.dataset.ctl)?.click()));
    return;
  }

  const tile = (label, value, sub, color) => `
    <div class="home-tile">
      <div class="home-tile-v" ${color ? `style="color:${color}"` : ''}>${value}</div>
      <div class="home-tile-l">${label}</div>
      ${sub ? `<div class="hint">${sub}</div>` : ''}
    </div>`;

  const dqColor = dq == null ? '' : dq >= 0.66 ? 'var(--green)' : dq >= 0.4 ? 'var(--amber)' : 'var(--red)';
  const list = (items, empty) => (items.length
    ? `<ul class="home-list">${items.join('')}</ul>` : `<p class="hint">${empty}</p>`);

  // role-aware actions: each opens an existing nav item / control (only present
  // for roles that have it, so no extra gating needed here).
  const viewBtn = (view, label, desc) => `<button class="home-act" data-go="${view}"><b>${label}</b><span class="hint">${desc}</span></button>`;
  const ctlBtn = (id, label, desc) => `<button class="home-act" data-ctl="${id}"><b>${label}</b><span class="hint">${desc}</span></button>`;

  const observeActs = [
    viewBtn('network', 'Org Network', 'the cross-pod dependency map'),
    viewBtn('scoreboard', 'WIP Scoreboard', 'where work is piling up'),
    viewBtn('hygiene', 'Data Quality', 'how trustworthy the data is'),
    viewBtn('simulator', 'Feature Simulator', 'forecast an epic'),
  ];
  const roleActs = [];
  if (hasRole('manager')) {
    roleActs.push(ctlBtn('obs-rosters', '👥 Rosters', 'team structure: headcount, pairing, lanes'));
    roleActs.push(ctlBtn('obs-import', '📥 Import from Jira', 'capture a dated org snapshot (Observe)'));
    roleActs.push(ctlBtn('obs-snapshots', '🗂 Snapshots', 'rosters + Jira captures: view, compare & publish'));
    roleActs.push(viewBtn('flow', '🎚 Levers', 'what-if on the current state (Plan)'));
    roleActs.push(viewBtn('plan', '📋 Plan a future period', 'roster + initiatives (Plan)'));
  }
  if (hasRole('facilitator')) roleActs.push(ctlBtn('run-games-btn', '🎮 Run games', 'create & run a learning game'));
  if (hasRole('admin')) roleActs.push(ctlBtn('admin-btn', '⚙ Admin', 'users & roles'));
  roleActs.push(viewBtn('game', '🎲 Play the game', 'try a round yourself'));

  el.innerHTML = `
    <div class="home-hero">
      <h1>Conway</h1>
      <p class="home-sub">${who}. ${roleBadges} <span class="hint">— organizations ship their communication structure; this makes it visible.</span></p>
      ${snapNote ? `<p class="home-snap">${snapNote}</p>` : ''}
    </div>

    <div class="home-stats">
      ${tile('Pods', pods.length, 'teams in the network')}
      ${tile('Dependencies', edges.length, 'cross-pod blocking links')}
      ${tile('Open WIP', Math.round(totWip), 'items in flight')}
      ${tile('Hot pods', hot, 'load ρ ≥ 0.85', hot ? 'var(--amber)' : 'var(--green)')}
      ${tile('Data quality', dq == null ? '—' : pct(dq), 'avg pod hygiene', dqColor)}
    </div>

    <div class="home-cols">
      <div class="card">
        <h3>Top constraints <span class="hint">— where flow chokes first</span></h3>
        ${list(constraints.map((c) => `<li><b>${esc(c.pod)}</b> <span class="hint">queue ×${c.queueFactor.toFixed(1)} · ${c.dependents} dependents</span></li>`), 'No constraints detected.')}
      </div>
      <div class="card">
        <h3>Heaviest dependencies <span class="hint">— the costly seams</span></h3>
        ${list(topEdges.map((e) => `<li><b>${esc(e.from)}</b> → <b>${esc(e.to)}</b> <span class="hint">×${e.count}</span></li>`), 'No cross-pod dependencies.')}
      </div>
    </div>

    <h3 style="margin-top:18px">Explore</h3>
    <div class="home-acts">${observeActs.join('')}</div>
    <h3 style="margin-top:14px">Your tools</h3>
    <div class="home-acts">${roleActs.join('')}</div>`;

  el.querySelectorAll('[data-go]').forEach((b) => b.addEventListener('click', () => document.querySelector(`.tab[data-view="${b.dataset.go}"]`)?.click()));
  el.querySelectorAll('[data-ctl]').forEach((b) => b.addEventListener('click', () => document.getElementById(b.dataset.ctl)?.click()));
}

function esc(s) { return String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }
