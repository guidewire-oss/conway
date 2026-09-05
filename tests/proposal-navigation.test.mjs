import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { readRoute, writeRoute, restoringRoute } from '../app/js/navigation.js';

const source = readFileSync(new URL('../app/js/planui.js', import.meta.url), 'utf8');
const main = readFileSync(new URL('../app/js/main.js', import.meta.url), 'utf8');
const extract = (start, end) => source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)));

test('route validation preserves known views, selection and independent filters', () => {
  const route = readRoute('https://example.test/?view=plan&plan=one&planView=timeline&lens=pod&initiative=Alpha&team=Atlas&selected=Alpha');
  assert.equal(route.planView, 'timeline');
  assert.equal(route.lens, 'pod');
  assert.equal(route.initiative, 'Alpha');
  assert.equal(route.team, 'Atlas');
  assert.equal(route.selected, 'Alpha');
  assert.equal(readRoute('https://example.test/?view=invalid').view, 'home');
  assert.equal(readRoute('https://example.test/?plan=one&planView=invalid').planView, 'order');
});

test('a user navigation during asynchronous route loading still writes browser history', async () => {
  const previousLocation = globalThis.location, previousHistory = globalThis.history;
  const calls = [];
  globalThis.location = { href: 'https://example.test/?view=plan&plan=one' };
  globalThis.history = { pushState(_a,_b,url) { calls.push(url.href); globalThis.location.href=url.href; } };
  try {
    let finish;
    const loading = restoringRoute(async () => {
      writeRoute({ view: 'timeline' });
      await new Promise((resolve) => { finish = resolve; });
    });
    assert.equal(calls.length, 0, 'programmatic synchronous navigation is suppressed');
    writeRoute({ view: 'home' });
    assert.equal(new URL(calls[0]).searchParams.get('view'), 'home');
    finish(); await loading;
  } finally { globalThis.location = previousLocation; globalThis.history = previousHistory; }
});

test('workspace restoration opens the selected plan and preserves role-based navigation', async () => {
  const restored = [], clicked = [];
  const scope = vm.createContext({
    location: { href: 'https://example.test/?view=plan&plan=one&planView=timeline&team=Atlas' },
    readRoute, restoringRoute: async (fn) => fn(),
    authMode: () => 'auth', isStaff: () => true, hasRole: () => true,
    document: { querySelector: (selector) => ({ click: () => clicked.push(selector) }) },
    restorePlanLocation: async (route) => restored.push(route),
  });
  vm.runInContext(main.slice(main.indexOf('async function restoreWorkspace()'), main.indexOf('// Rosters, Import, and Snapshots')), scope);
  await scope.restoreWorkspace();
  assert.equal(restored[0].plan, 'one');
  assert.equal(restored[0].planView, 'timeline');
  assert.equal(restored[0].team, 'Atlas');
  scope.hasRole = () => false;
  await scope.restoreWorkspace();
  assert.match(clicked.at(-1), /data-view="home"/);
  assert.equal(restored.length, 1, 'a staff member without planning permission cannot restore plan data');
  scope.isStaff = () => false;
  await scope.restoreWorkspace();
  assert.match(clicked.at(-1), /data-view="game"/);
  assert.equal(restored.length, 1);
});

test('a failed older plan load cannot replace the newer loaded plan', async () => {
  let finishOld;
  const scope = vm.createContext({ current: null, planLoadTicket: 0, root: { innerHTML: '' },
    staleOrder() {}, loadBaselines: async () => {}, localStorage: { getItem: () => null }, rememberPlanRoute() {},
    renderPlan() { scope.root.innerHTML = scope.current.name; },
    req: async (path) => path.endsWith('/old') ? new Promise((r) => { finishOld = r; }) : { ok: true, json: async () => ({ id: 'new', name: 'New plan' }) },
  });
  vm.runInContext(extract('async function openPlan(', 'function uploadField('), scope);
  const old = scope.openPlan('old');
  await scope.openPlan('new');
  finishOld({ ok: false }); await old;
  assert.equal(scope.root.innerHTML, 'New plan');
  assert.equal(scope.current.id, 'new');
});

test('a reused proposal modal rejects an older preview response', async () => {
  let finishOld;
  const status = { textContent: '', outerHTML: '' }, button = { addEventListener() {} };
  const overlay = { hidden: false, querySelector: (selector) => selector === '#remedy-apply' ? button : status };
  const scope = vm.createContext({ current: { id: 'one' }, orderEpoch: 1,
    proposalModal() { overlay.proposalToken = Symbol(); return overlay; },
    esc: (s) => s, compareTableHTML: ({comparison}) => comparison.name,
    req: async (_path, opts) => JSON.parse(opts.body).remedy.target === 'Old'
      ? new Promise((r) => { finishOld = r; })
      : { ok: true, json: async () => ({ comparison: { name: 'New preview' } }) },
  });
  vm.runInContext(extract('async function previewRemedy(', 'function renderExecution('), scope);
  const old = scope.previewRemedy({ kind: 'descope', target: 'Old' });
  await scope.previewRemedy({ kind: 'descope', target: 'New' });
  assert.match(status.outerHTML, /New preview/);
  finishOld({ ok: true, json: async () => ({ comparison: { name: 'Old preview' } }) });
  await old;
  assert.match(status.outerHTML, /New preview/);
  assert.doesNotMatch(status.outerHTML, /Old preview/);
});

test('accepting an old ordering response cannot overwrite newer scheduling settings', async () => {
  let finish;
  const scope = vm.createContext({ current: { id: 'one', scheduling: { estimateModel: 'effort' } }, orderEpoch: 1,
    staleOrder() { scope.orderEpoch++; }, req: async () => new Promise((r) => { finish = r; }),
    view: () => 'order', renderOrder: async () => { throw new Error('Stale ordering must not rerender'); },
  });
  const fn = vm.runInContext(extract('  const setAcceptedOrdering = async', "  document.getElementById('ord-unoptimize')") + '\nsetAcceptedOrdering;', scope);
  const pending = fn('engine');
  scope.current.scheduling = { estimateModel: 'wall-clock', acceptedOrdering: 'stated' };
  scope.orderEpoch++;
  finish({ ok: true }); await pending;
  assert.equal(scope.current.scheduling.acceptedOrdering, 'stated');
  assert.equal(scope.current.scheduling.estimateModel, 'wall-clock');
});

test('save feedback does not claim baselines unchanged and delayed errors stay on their originating plan', async () => {
  let textDone;
  const notices = [];
  const scope = vm.createContext({ current: { id: 'one' }, planNotice: (...args) => notices.push(args),
    authFetch: async () => ({ ok: true }),
  });
  vm.runInContext(extract('async function req(', 'function rememberPlanRoute('), scope);
  await scope.req('/api/plan/one/baselines', { method: 'POST' });
  assert.deepEqual(notices.at(-1), ['Changes saved.', false]);
  notices.length = 0;
  await scope.req('/api/plan/one/schedule', { method: 'POST' });
  assert.equal(notices.length, 0, 'read-only computation never claims a write');
  scope.authFetch = async () => ({ ok: false, clone: () => ({ text: () => new Promise((r) => { textDone = r; }) }) });
  const pending = scope.req('/api/plan/one/initiatives', { method: 'PATCH' });
  await new Promise(setImmediate);
  scope.current = { id: 'two' }; textDone('Old plan failure'); await pending;
  assert.equal(notices.length, 1);
  assert.equal(notices[0][0], 'Saving…');
});

test('roster reload preserves planning context while replacing server inputs', async () => {
  const levers = [{ type: 'addCapacity', pod: 'Atlas', n: 2 }];
  let renders = 0;
  const scope = vm.createContext({
    current: { id: 'one', view: 'timeline', tlLens: 'pod', tlInitiativeFilter: 'Alpha', tlTeamFilter: 'Atlas', selectedInitiative: 'Alpha', levers, schedule: { old: true }, sim: { old: true } },
    planLoadTicket: 0, dragUndo: {}, dragHistory: [{}],
    staleOrder() { scope.current.schedule = null; }, loadBaselines: async () => {}, renderPlan() { renders++; },
    req: async () => ({ ok: true, json: async () => ({ id: 'one', teams: [{ name: 'Atlas', tracks: 4 }] }) }),
  });
  vm.runInContext(extract('async function reloadPlan()', 'function renderLeverTarget()'), scope);
  await scope.reloadPlan();
  assert.equal(scope.current.view, 'timeline');
  assert.equal(scope.current.tlLens, 'pod');
  assert.equal(scope.current.tlInitiativeFilter, 'Alpha');
  assert.equal(scope.current.tlTeamFilter, 'Atlas');
  assert.equal(scope.current.selectedInitiative, 'Alpha');
  assert.equal(scope.current.levers, levers);
  assert.equal(scope.current.teams[0].tracks, 4);
  assert.equal(scope.current.schedule, null);
  assert.equal(scope.current.sim, null, 'roster edits invalidate the network simulation as well as the schedule');
  assert.equal(scope.dragHistory.length, 0);
  assert.equal(renders, 1);
});

test('late roster reload cannot overwrite a different active plan', async () => {
  let finish;
  const scope = vm.createContext({
    current: { id: 'one', view: 'timeline' }, planLoadTicket: 0,
    staleOrder() {}, req: async () => new Promise((resolve) => { finish = resolve; }),
    loadBaselines: async () => assert.fail('stale plan must not load its baselines'), renderPlan: () => assert.fail('stale plan must not render'),
  });
  vm.runInContext(extract('async function reloadPlan()', 'function renderLeverTarget()'), scope);
  const pending = scope.reloadPlan();
  scope.current = { id: 'two', name: 'Current plan' };
  finish({ ok: true, json: async () => ({ id: 'one', name: 'Older plan' }) });
  await pending;
  assert.equal(scope.current.id, 'two');
  assert.equal(scope.current.name, 'Current plan');
});

test('failed team save retains the open editor and its pending field values', async () => {
  const tracks = { value: '7' }, pairs = { checked: true }, requests = [];
  const scope = vm.createContext({ current: { id: 'one', editPod: 'Atlas' },
    document: { getElementById: (id) => id === 'pe-tracks' ? tracks : pairs },
    req: async (_path, opts) => { requests.push(JSON.parse(opts.body)); return { ok: false }; },
    reloadPlan: () => assert.fail('failed save must not reload away pending fields'),
  });
  vm.runInContext(extract('async function savePod(', '// reloadPlan re-fetches'), scope);
  await scope.savePod('Atlas');
  assert.deepEqual(requests[0], { name: 'Atlas', pairs: true, tracks: 7 });
  assert.equal(scope.current.editPod, 'Atlas');
  assert.equal(tracks.value, '7');
  assert.equal(pairs.checked, true);
});
