import { initGraph } from './graph.js';
import { initSimulator } from './simulator.js';
import { initScoreboard } from './scoreboard.js';
import { initFlow } from './flow.js';
import { initGuide } from './guide.js';
import { initHygiene } from './hygiene.js';
import { workStreams } from './sim.js';
import { initGameUI } from './gameui.js';
import { initAuth, isStaff, hasRole, authMode } from './auth.js';
import { initPlanUI } from './planui.js';
import { setSnapshot, getSnapshot, dataJson, listSnapshots } from './data.js';
import { openImport } from './importui.js';
import { openSnapshots } from './snapshotsui.js';
import { openRosters } from './rostersui.js';
import { initHome } from './home.js';
import './sortable.js'; // delegated column sorting for tables.sortable

function syntheticStats(pod) {
  // fallback when a pod has no mined Jira history: typical issue ~7 days, wide spread
  return {
    mu: Math.log(7), sigma: 0.9, p50: 7, p85: 17, mean: 10,
    wip: Math.max(2, pod.devCount), throughputWk: pod.devCount * 0.8,
    resolved180: 0, synthetic: true,
  };
}

export const state = { pods: [], overlap: {}, stats: {}, edges: [], mined: false };

async function load() {
  // The top "Viewing" picker is the source of truth for which snapshot every
  // view renders. Resolve it: an explicit ?snapshot wins; else the last picked
  // (sticky across reloads); else baseline; else the newest snapshot. Always
  // validate the choice still exists (it may have been deleted / cleared).
  const param = new URLSearchParams(location.search).get('snapshot');
  let snapId = param || (authMode() === 'auth' && localStorage.getItem('conway_snapshot')) || 'baseline';
  if (authMode() === 'auth') {
    const snaps = await listSnapshots(); // newest-first
    const ids = snaps.map((s) => s.id);
    if (snaps.length && !ids.includes(snapId)) snapId = ids.includes('baseline') ? 'baseline' : snaps[0].id;
  }
  setSnapshot(snapId);
  if (authMode() === 'auth') localStorage.setItem('conway_snapshot', snapId);

  const podsFile = await dataJson('pods.json');
  state.pods = podsFile?.pods || []; // empty when no snapshot yet (first run)
  state.overlap = podsFile?.overlap || {};

  const stats = await dataJson('pod_stats.json');
  const edges = await dataJson('edges.json');
  state.hygiene = (await dataJson('hygiene.json')) ?? {};
  state.wipSplit = (await dataJson('wip_split.json')) ?? {};
  state.mined = !!(stats && edges && state.pods.length);

  for (const p of state.pods) {
    const m = stats?.[p.name];
    state.stats[p.name] = m ? {
      mu: m.lognormal.mu, sigma: m.lognormal.sigma,
      p50: m.cycle_time_days.p50, p85: m.cycle_time_days.p85, mean: m.cycle_time_days.mean,
      wip: m.wip_count, throughputWk: m.throughput_per_week,
      resolved180: m.resolved_count_180d, synthetic: false,
    } : syntheticStats(p);
    p.streams = p.streams || workStreams(p.devCount, p.pairing); // explicit work-streams (pairs) win
    const s = state.stats[p.name];
    // true load: WIP per healthy concurrency (a pair pulls one item). Can exceed
    // 1 — an overloaded pod — and we DISPLAY the real number.
    s.load = s.wip / Math.max(1, p.streams * 2);
    // utilization used for the Kingman wait factor and heat color: must stay
    // below 1 (ρ/(1−ρ) diverges at 1). This is a math input, not the display.
    s.rho0 = Math.max(0.05, Math.min(0.97, s.load));
  }
  state.edges = (edges ?? []).filter(
    (e) => state.stats[e.from] && state.stats[e.to] && e.from !== e.to,
  );

  const badge = document.getElementById('data-badge');
  if (!state.pods.length) {
    badge.textContent = 'no org snapshot yet';
    badge.className = 'badge warn';
  } else if (state.mined) {
    badge.textContent = `${state.pods.length} pods · ${state.edges.length} cross-pod edges`;
    badge.className = 'badge ok';
  } else {
    badge.textContent = 'no stats in this snapshot — using synthetic estimates';
    badge.className = 'badge warn';
  }
  mountSnapshotPicker(badge);
  wireSnapshotControls();

  initGuide(state);
  initGameUI();
  initPlanUI();
  // Plan pillar is for managers (admins included as superusers) and dev/static mode
  if (authMode() !== 'auth' || hasRole('manager')) {
    document.getElementById('plan-group')?.removeAttribute('hidden');
  }
  // Players only ever see Guide + Flow Game. Skip initialising the heavy
  // analytics views (org network, scoreboard, simulator, hygiene, flow) they
  // never see — that init (esp. the 31-pod d3 network) is the bulk of load cost.
  if (!(authMode() === 'auth' && !isStaff())) {
    initFlow(state);
    initGraph(state);
    initSimulator(state);
    initScoreboard(state);
    initHygiene(state);
    initHome(state); // staff landing dashboard
  }
  applyRoleGating();
}

// Rosters, Import, and Snapshots are observation tools (capturing & comparing
// reality), so they live under Observe ▾ — but only for managers (who can
// import/manage).
function wireSnapshotControls() {
  const imp = document.getElementById('obs-import');
  const ros = document.getElementById('obs-rosters');
  const snap = document.getElementById('obs-snapshots');
  imp?.addEventListener('click', openImport);
  ros?.addEventListener('click', openRosters);
  snap?.addEventListener('click', openSnapshots);
  if (authMode() !== 'auth' || hasRole('manager')) {
    imp?.removeAttribute('hidden');
    ros?.removeAttribute('hidden');
    snap?.removeAttribute('hidden');
  }
  // returning from the Jira SSO redirect → reopen the import modal (now connected)
  if (authMode() === 'auth' && new URLSearchParams(location.search).get('import') === '1') openImport();
}

// Snapshot picker: the single control for "which org capture every Observe
// screen renders". Always shown in server mode (even with just the baseline, so
// it's discoverable and labels what you're viewing). Changing it reloads with
// ?snapshot=<id> so every view re-reads cleanly.
async function mountSnapshotPicker(badge) {
  if (authMode() !== 'auth' || !badge) return;
  const snaps = await listSnapshots();
  if (!snaps.length) return;
  const fmt = (s) => s.source === 'baseline' ? (s.name || 'Baseline')
    : `${s.name || s.id} -- ${new Date(s.createdAt * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })} -- ${s.owner}`;
  const wrap = document.createElement('span');
  wrap.className = 'snapshot-pick';
  wrap.innerHTML = '<label class="hint" for="snapshot-pick-sel">Viewing org snapshot</label>';
  const sel = document.createElement('select');
  sel.id = 'snapshot-pick-sel';
  sel.title = 'The org snapshot every Observe screen renders';
  const cur = getSnapshot();
  sel.innerHTML = snaps.map((s) => `<option value="${s.id}" ${s.id === cur ? 'selected' : ''}>${fmt(s)}</option>`).join('');
  const fitWidth = () => {
    const tmp = document.createElement('canvas').getContext('2d');
    tmp.font = getComputedStyle(sel).font;
    const text = sel.options[sel.selectedIndex]?.text ?? '';
    sel.style.width = (tmp.measureText(text).width + 36) + 'px';
  };
  sel.addEventListener('change', () => {
    fitWidth();
    localStorage.setItem('conway_snapshot', sel.value); // sticky across reloads
    const u = new URL(location.href);
    u.searchParams.set('snapshot', sel.value);
    location.assign(u);
  });
  wrap.appendChild(sel);
  badge.after(wrap);
  requestAnimationFrame(fitWidth);
  // The picker mounts after async snapshot data, later than the initial
  // syncSnapshotPicker() call — a player landing on the game view would see it
  // mount visible over the game. Sync here, at the moment it exists.
  syncSnapshotPicker();
}

// Role-based landing: a plain team player sees only the game (which embeds its
// own network); staff (admin/manager/facilitator) land on Observe → Org Network,
// not the player board. dev/static mode is fully open and left on the default view.
function applyRoleGating() {
  if (authMode() !== 'auth') return; // dev/static: show everything, stay on Home
  if (!isStaff()) {
    // players run only the game: hide Home + the Explore menu, relabel Guide
    document.getElementById('home-tab')?.setAttribute('hidden', '');
    document.getElementById('explore-group')?.setAttribute('hidden', '');
    const gb = document.getElementById('guide-btn'); if (gb) gb.textContent = 'How to play';
    document.querySelector('.tab[data-view="game"]')?.click(); // activate + size the game view
    return;
  }
  // staff land on Home (the default active view) — nothing to switch
}

// Bootstrap Tooltip (spec 011 FR-002): delegated init on document.body.
// BS handles positioning, viewport clamping, hover + focus triggers, and
// dynamic content natively — replacing the custom tooltip div. The app
// theme bridge maps bs-* vars, so tooltips follow the light/dark theme.
new bootstrap.Tooltip(document.body, {
  selector: '[data-bs-toggle="tooltip"], [data-tip], .help',
  title: (el) => el.dataset.bsTitle ?? el.dataset.tip ?? '',
  trigger: 'hover focus',
  placement: 'bottom'
});

// The "Viewing" picker is the org snapshot every OBSERVE screen renders. Plan
// and Games carry their own data (a plan's roster and initiatives, a game's
// scenario network) and never consult it — on those views the picker would
// imply a connection that does not exist, so it hides.
const SNAPSHOT_AGNOSTIC_VIEWS = new Set(['plan', 'game']);
const syncSnapshotPicker = () => {
  const pick = document.querySelector('.snapshot-pick');
  if (!pick) return;
  const active = document.querySelector('.view.active');
  const hide = active && SNAPSHOT_AGNOSTIC_VIEWS.has(active.id.replace('view-', ''));
  pick.toggleAttribute('hidden', !!hide);
};
// Initial sync: role gating may land the page on a snapshot-agnostic view
// (players go straight to the game) before any tab is clicked — but the picker
// itself mounts later, after its async listSnapshots, so the first real sync
// happens on the first tab click; this one covers a picker that mounted fast.
setTimeout(syncSnapshotPicker, 0);
document.querySelectorAll('.tab[data-view]').forEach((b) => b.addEventListener('click', () => {
  document.querySelectorAll('.tab[data-view]').forEach((x) => x.classList.toggle('active', x === b));
  document.querySelectorAll('.view').forEach((v) => v.classList.toggle('active', v.id === `view-${b.dataset.view}`));
  syncSnapshotPicker();
}));

// Explore ▾ dropdown: groups the analytics views under one menu so the top bar
// stays focused on running the game. Click to toggle; a selection or an outside
// click closes it.
// Nav dropdowns are Bootstrap dropdowns now (data-bs-toggle in the markup,
// bootstrap.bundle.js): ESC, arrow keys, focus handling and outside-click
// close come from the framework. Dynamically injected items (🎮 Run games)
// work because BS's dropdown close listener is delegated at document level.
// The `hidden` attribute initially gates the plan GROUP in index.html; load()
// in this file removes it for managers, so Bootstrap only ever toggles the menu.

// The org network is one view seen two ways: read-only under Observe (no
// simulation panel) and as the what-if tool under Plan (panel shown).
document.getElementById('net-observe')?.addEventListener('click', () => document.getElementById('view-network')?.classList.add('readonly'));
document.getElementById('net-plan')?.addEventListener('click', () => document.getElementById('view-network')?.classList.remove('readonly'));

// Theme toggle wiring (the saved mode itself is applied by the inline script
// in <head>, before first paint — this module loads too late for that).
(() => {
  const btn = document.getElementById('theme-btn');
  btn?.addEventListener('click', () => {
    const next = document.documentElement.dataset.bsTheme === 'light' ? 'dark' : 'light';
    document.documentElement.dataset.bsTheme = next;
    localStorage.setItem('conway-theme', next);
    btn.textContent = next === 'light' ? '☀' : '☾';
  });
  if (btn) btn.textContent = document.documentElement.dataset.bsTheme === 'light' ? '☀' : '☾';
})();

// gate the app behind login when the server is present (dev/static: passes through)
// Bootstrap form adoption (spec 011 FR-001): class-inject before the first
// render so native focus/validation semantics load with the app.
import { initForms } from './forms.js';
import { initDocs } from './docs.js';
initForms();
initDocs();
initAuth().then(load);
