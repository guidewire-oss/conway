import { initGraph } from './graph.js';
import { initSimulator } from './simulator.js';
import { initScoreboard } from './scoreboard.js';
import { initFlow } from './flow.js';
import { initGuide } from './guide.js';
import { initHygiene } from './hygiene.js';
import { workStreams } from './sim.js';
import { initGameUI } from './gameui.js';
import { initAuth, isStaff, hasRole, authMode, authFetch, authUser, authToken, authGameID } from './auth.js';
import { mountAnnouncements } from './announcements.js';
import { initPlanUI, restorePlanLocation, openPlanDestination } from './planui.js';
import { readRoute, writeRoute, restoringRoute } from './navigation.js';
import { icon } from './icons.js';
import { setSnapshot, getSnapshot, dataJson, listSnapshots } from './data.js';
import { openImport } from './importui.js';
import { openSnapshots } from './snapshotsui.js';
import { openRosters } from './rostersui.js';
import { initHome } from './home.js';
import { mountMeasureContext, snapshotSelectionURL } from './measure-context.js';
import './sortable.js'; // delegated column sorting for tables.sortable

function syntheticStats(pod) {
  // fallback when a pod has no mined Jira history: typical issue ~7 days, wide spread
  return {
    mu: Math.log(7), sigma: 0.9, p50: 7, p85: 17, mean: 10,
    wip: Math.max(2, pod.devCount), throughputWk: pod.devCount * 0.8,
    resolved180: 0, synthetic: true,
  };
}

let measureContext = null;
let announcements = null;
const pendingAnnouncementVisits = new Map();
const announcementIdentity = () => authMode() === 'auth' && !authGameID() && authUser() ? `${authUser()}:${authToken()}` : null;
function flushAnnouncementVisits() {
  if (!announcements) return;
  const targets = new Set(announcements.state().features.map(feature => feature.action.target));
  for (const [target, identity] of pendingAnnouncementVisits) {
    if (identity !== announcementIdentity()) { pendingAnnouncementVisits.delete(target); continue; }
    if (!targets.has(target)) continue;
    pendingAnnouncementVisits.delete(target);
    void announcements.visit(target);
  }
}
window.addEventListener('conway:feature-opened', event => {
  const target = ({ guide: 'docs-btn', assistant: 'view-assistant', forecast: 'view-forecast', ready: 'view-ready', execution: 'view-execution', 'linked-sheets': 'plan-linked-sheets', 'plan-sample':'plan-init-sample', snapshots:'obs-snapshots' })[event.detail?.action];
  if (target && announcementIdentity()) {
    pendingAnnouncementVisits.set(target, announcementIdentity());
    flushAnnouncementVisits();
  }
});
window.addEventListener('pagehide', event => { if (!event.persisted) announcements?.dispose(); });
window.addEventListener('pageshow', event => {
  if (event.persisted) { announcements?.refreshIndicators(); flushAnnouncementVisits(); }
});
document.addEventListener('conway:measure-sources-changed', () => measureContext?.refresh());

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

  measureContext = mountMeasureContext(document.getElementById('measure-context'), {
    selectedId: getSnapshot(), state, canManage: authMode() !== 'auth' || hasRole('manager'), request: authFetch,
    onSelect: id => {
      localStorage.setItem('conway_snapshot', id);
      location.assign(snapshotSelectionURL(location.href, id));
    },
    actions: { import: openImport, associations: openSnapshots, rosters: openRosters,
      plans: () => document.querySelector('.tab[data-view="plan"]')?.click() },
  });
  syncMeasureContext();
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
  await restoreWorkspace();
  window.addEventListener('popstate', restoreWorkspace);
  // specs/022-feature-announcements.md:180 — show news after restoring work;
  // opening a plan picker is not evidence that its feature was visited.
  if (authMode() === 'auth' && !authGameID() && authUser()) {
    announcements = mountAnnouncements({ request: authFetch,
      getIdentity: announcementIdentity,
      onStateChange: () => queueMicrotask(flushAnnouncementVisits),
      replayButton: document.getElementById('whats-new-btn'),
      onAction: async action => {
        if (action.target === 'docs-btn') { openDocs(); return true; }
        if (action.target === 'obs-snapshots') { openSnapshots(); return true; }
        if (action.target === 'plan-init-sample') return openPlanDestination('setup');
        return openPlanDestination(action.target === 'view-forecast' ? 'forecast' : action.target === 'view-assistant' ? 'assistant' : action.target === 'view-execution' ? 'execution' : action.target === 'view-ready' ? 'ready' : 'linked-sheets');
      },
    });
    await announcements.ready;
    flushAnnouncementVisits();
    if (document.querySelector('#view-ready.active')) void announcements.visit('view-ready');
    if (document.querySelector('#view-execution.active')) void announcements.visit('view-execution');
  }
}

async function restoreWorkspace() {
  const route=readRoute(location.href);
  let target=route.view;
  if(authMode() === 'auth' && !isStaff()) target='game';
  else if(target === 'plan' && authMode() === 'auth' && !hasRole('manager')) target='home';
  await restoringRoute(async()=>{
    const selector = target === 'network'
      ? (route.networkLens === 'what-if' && (authMode() !== 'auth' || hasRole('manager')) ? '#net-plan' : '#net-observe')
      : `.tab[data-view="${target}"]`;
    document.querySelector(selector)?.click();
    if(target === 'plan') await restorePlanLocation(route);
  });
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

// Plan and Game own separate inputs. All snapshot-backed surfaces retain source context.
const syncMeasureContext = () => {
  const active = document.querySelector('.view.active');
  measureContext?.setView(active?.id.replace('view-', '') || 'home');
};
document.querySelectorAll('.tab[data-view]').forEach((b) => b.addEventListener('click', () => {
  document.querySelectorAll('.tab[data-view]').forEach((x) => {
    x.classList.toggle('active', x === b);
    if (x === b) x.setAttribute('aria-current', 'page');
    else x.removeAttribute('aria-current');
  });
  document.querySelectorAll('.view').forEach((v) => v.classList.toggle('active', v.id === `view-${b.dataset.view}`));
  syncMeasureContext();
  writeRoute({view:b.dataset.view, ...(b.dataset.view === 'network' ? {networkLens:b.id === 'net-plan' ? 'what-if' : 'observe'} : {})});
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
    btn.innerHTML = icon(next === 'light' ? 'sun' : 'moon') + (next === 'light' ? 'Light theme' : 'Dark theme');
  });
  if (btn) btn.innerHTML = icon(document.documentElement.dataset.bsTheme === 'light' ? 'sun' : 'moon') + (document.documentElement.dataset.bsTheme === 'light' ? 'Light theme' : 'Dark theme');
})();

// gate the app behind login when the server is present (dev/static: passes through)
// Bootstrap form adoption (spec 011 FR-001): class-inject before the first
// render so native focus/validation semantics load with the app.
import { initForms } from './forms.js';
import { initDocs, openDocs } from './docs.js';
initForms();
initDocs();
initAuth().then(load);
