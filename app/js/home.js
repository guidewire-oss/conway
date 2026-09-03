// Home — a neutral, role-aware overview: at-a-glance org stats from the current
// snapshot plus the actions that matter for your role. Not the guide, not the
// game board. Reads the same `state` Observe builds.
import { authUser, authRoles, hasRole, authMode, authFetch } from './auth.js';
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
      <div class="panel-card" style="max-width:640px">
        <h3>No org snapshot yet</h3>
        <p>Conway has three surfaces: <b>Measure</b> (what's happening now, mined from
          Jira), <b>Plan</b> (what you intend to run — rosters and initiatives, priced
          by capacity), and <b>Learn</b> (the multi-team flow game). Everything starts
          with an <b>org network</b> — pods and the dependencies between them —
          captured as a dated <b>snapshot</b>. There isn't one yet.</p>
        ${hasRole('manager')
        ? '<p>Capture the current state from Jira to get started — each import creates a <b>dated snapshot</b> that is yours, and the Measure screens render whichever one you pick at the top:</p><button class="home-act" data-ctl="obs-import" style="max-width:280px"><b>📥 Import from Jira</b><span class="hint">build your first snapshot</span></button><p class="hint" style="margin-top:8px">New here? The <b>Docs</b> tab (top right) has a 15-minute walkthrough on the demo plan.</p>'
        : '<p class="hint">Ask a manager to import a snapshot from Jira, or (facilitators) upload a scenario under Train ▸ Run games.</p>'}
      </div>`;
    el.querySelectorAll('button[data-ctl]').forEach((b) => b.addEventListener('click', () => document.getElementById(b.dataset.ctl)?.click()));
    return;
  }

  // ── Status alerts (IA #4): "is anything wrong right now?" — the only
  // question a returning user has. Org-level (hot pods, data quality) from
  // the snapshot; plan-level (dates at risk) from the manager's own most
  // recently updated plan with a schedule. Cards deep-link to the views.
  const alertCard = (level, title, sub, go) =>
    `<button class="home-alert home-alert-${level}" data-go="${go}">
      <b>${title}</b><span class="hint">${sub}</span></button>`;

  let planAlert = '';
  if (hasRole('manager')) {
    try {
      const r = await authFetch('/api/plan');
      if (r && r.ok) {
        const plans = (await r.json()) || [];
        // The most recent plan WITH initiatives — an empty scaffold (just
        // created, nothing uploaded) is not a status anyone can act on, and
        // its "no dates" state would swallow the card.
        const latest = [...plans].filter((p) => (p.initiativeCount || 0) > 0)
          .sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0))[0];
        if (latest) {
          const sr = await authFetch(`/api/plan/${encodeURIComponent(latest.id)}/schedule`, { method: 'POST', body: '{}' });
          if (sr && sr.ok) {
            const sched = await sr.json();
            const dated = (sched.initiatives || []).filter((i) => i.targetWeek !== null && i.targetWeek !== undefined);
            const missing = dated.filter((i) => i.verdict !== 'on-time');
            if (missing.length) {
              planAlert = alertCard('miss',
                `${missing.length} of ${dated.length} dates at risk`,
                `in ${latest.name || 'your latest plan'} under the best order found`, 'plan');
            } else if (dated.length) {
              planAlert = alertCard('ok', 'Every dated initiative holds',
                `in ${latest.name || 'your latest plan'}`, 'plan');
            }
          }
        }
      }
    } catch { /* the plan pillar is optional; no alert rather than a broken home */ }
  }

  const alerts = [];
  if (hot > 0) alerts.push(alertCard('miss', `${hot} pod${hot > 1 ? 's' : ''} over capacity`,
    'load ρ ≥ 0.85 — where flow chokes first', 'scoreboard'));
  if (dq != null && dq < 0.4) alerts.push(alertCard('warn', 'Data quality is low',
    'decisions on this data inherit its gaps', 'hygiene'));
  if (planAlert) alerts.push(planAlert);
  const alertsHTML = alerts.length
    ? `<div class="home-alerts">${alerts.join('')}</div>`
    : `<div class="home-alerts">${alertCard('ok', 'Nothing needs attention', 'pods under load, dates holding, data usable', 'network')}</div>`;

  const tile = (label, value, sub, color) => `
    <div class="home-tile">
      <div class="home-tile-v" ${color ? `style="color:${color}"` : ''}>${value}</div>
      <div class="home-tile-l">${label}</div>
      ${sub ? `<div class="hint">${sub}</div>` : ''}
    </div>`;

  const dqColor = dq == null ? '' : dq >= 0.66 ? 'var(--green)' : dq >= 0.4 ? 'var(--amber)' : 'var(--red)';
  const list = (items, empty) => (items.length
    ? `<ul class="home-list">${items.join('')}</ul>` : `<p class="hint">${empty}</p>`);

  // IA #3: Home is a dashboard, not a directory — the nav (Measure/Plan/Learn)
  // is the way around now. Two intent cards answer "what do I do from here",
  // each deep-linking into the nav's own destinations.
  const viewBtn = (view, label, desc) => `<button class="home-act" data-go="${view}"><b>${label}</b><span class="hint">${desc}</span></button>`;
  const ctlBtn = (id, label, desc) => `<button class="home-act" data-ctl="${id}"><b>${label}</b><span class="hint">${desc}</span></button>`;

  el.innerHTML = `
    <div class="home-hero">
      <h1>Conway</h1>
      <p class="home-sub">${who}. ${roleBadges} <span class="hint">— organizations ship their communication structure; this makes it visible.</span></p>
      ${snapNote ? `<p class="home-snap">${snapNote}</p>` : ''}
    </div>

    ${alertsHTML}

    <div class="home-stats">
      ${tile('Pods', pods.length, 'teams in the network')}
      ${tile('Dependencies', edges.length, 'cross-pod blocking links')}
      ${tile('Open WIP', Math.round(totWip), 'items in flight')}
      ${tile('Hot pods', hot, 'load ρ ≥ 0.85', hot ? 'var(--amber)' : 'var(--green)')}
      ${tile('Data quality', dq == null ? '—' : pct(dq), 'avg pod hygiene', dqColor)}
    </div>

    <div class="home-cols">
      <div class="panel-card">
        <h3>Top constraints <span class="hint">— where flow chokes first</span></h3>
        ${list(constraints.map((c) => {
          const q = Math.min(1, c.queueFactor / 40); // 40x = full bar
          return `<li class="home-bar-row"><b>${esc(c.pod)}</b>
            <span class="home-bar"><span class="home-bar-fill" style="width:${Math.round(q * 100)}%"></span></span>
            <span class="hint">×${c.queueFactor.toFixed(1)} · ${c.dependents} deps</span></li>`;
        }), 'No constraints detected.')}
      </div>
      <div class="panel-card">
        <h3>Heaviest dependencies <span class="hint">— the costly seams</span></h3>
        ${list(topEdges.map((e) => {
          const w = topEdges[0].count ? Math.min(1, e.count / topEdges[0].count) : 0;
          return `<li class="home-bar-row"><b>${esc(e.from)}</b> → <b>${esc(e.to)}</b>
            <span class="home-bar"><span class="home-bar-fill" style="width:${Math.round(w * 100)}%"></span></span>
            <span class="hint">×${e.count}</span></li>`;
        }), 'No cross-pod dependencies.')}
      </div>
    </div>

    <h3 style="margin-top:18px">Start here</h3>
    <div class="home-acts">
      ${viewBtn('network', 'Measure what\'s happening', 'the dependency map, WIP and data quality')}
      ${hasRole('manager') ? viewBtn('plan', 'Plan the next period', 'your plans, orders and baselines') : ''}
      ${hasRole('facilitator') ? ctlBtn('run-games-btn', 'Run a learning game', 'scenarios, join codes, rounds') : ''}
      ${viewBtn('game', 'Learn the game', 'try a round yourself')}
    </div>
    ${hasRole('manager') ? `<p class="hint" style="margin-top:10px">Data tools: <a href="#" data-ctl="obs-rosters">rosters</a> · <a href="#" data-ctl="obs-import">import from Jira</a> · <a href="#" data-ctl="obs-snapshots">snapshots</a>${hasRole('admin') ? ' · <a href="#" data-ctl="admin-btn">admin</a>' : ''}</p>` : ''}`;

  el.querySelectorAll('[data-go]').forEach((b) => b.addEventListener('click', () => document.querySelector(`.tab[data-view="${b.dataset.go}"]`)?.click()));
  el.querySelectorAll('a[data-ctl]').forEach((a) => a.addEventListener('click', (ev) => { ev.preventDefault(); document.getElementById(a.dataset.ctl)?.click(); }));
  el.querySelectorAll('button[data-ctl]').forEach((b) => b.addEventListener('click', () => document.getElementById(b.dataset.ctl)?.click()));
}

function esc(s) { return String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }
