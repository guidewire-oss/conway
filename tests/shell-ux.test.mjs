import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

// Execute the real renderers against small DOM/network seams so unavailable
// evidence cannot silently regress into an all-clear (spec 017 FR-002).
function moduleContext(file, bindings) {
  const source = readFileSync(new URL('../app/js/' + file, import.meta.url), 'utf8')
    .replace(/^import .*?;\n/gm, '').replace(/export /g, '');
  const context = vm.createContext({ console, Date, Number, String, ...bindings });
  vm.runInContext(source, context); return context;
}
async function home({ stats = {}, hygiene = {}, manager = false, response } = {}) {
  const el = { innerHTML: '', querySelectorAll: () => [] };
  const context = moduleContext('home.js', {
    document: { getElementById: () => el },
    authUser: () => null, authRoles: () => [], authMode: () => 'none',
    hasRole: (role) => manager && role === 'manager',
    constraintScores: () => [], getSnapshot: () => 'snapshot-1',
    listSnapshots: async () => [], authFetch: async (url) => response ? response(url) : null,
  });
  await context.initHome({ pods: [{ name: 'Atlas' }], stats, hygiene, edges: [] });
  return el.innerHTML;
}

test('Home shows unknown load and quality instead of all-clear without evidence', async () => {
  const html = await home();
  assert.match(html, /Load evidence incomplete/);
  assert.match(html, /Data quality not fully checked/);
  assert.match(html, /0 of 1 teams/);
  assert.match(html, /Unknown/);
  assert.doesNotMatch(html, /Nothing needs attention|dates holding|data usable/);
});

test('Home distinguishes the high-load threshold from exceeding capacity', async () => {
  const html = await home({ stats: { Atlas: { rho0: 0.9, wip: 4 } }, hygiene: { Atlas: { score: 0.8 } } });
  assert.match(html, /1 team under high load/);
  assert.match(html, /0 at or above capacity/);
  assert.doesNotMatch(html, /1 pod over capacity/);
});

test('a failed plan request remains an explicit unchecked state', async () => {
  const html = await home({ manager: true, response: async () => ({ ok: false }) });
  assert.match(html, /Plan dates not checked/);
  assert.match(html, /could not be loaded/);
});

test('a populated plan without targets does not claim that dates hold', async () => {
  const html = await home({ manager: true, response: async (url) => ({ ok: true, json: async () => url === '/api/plan'
    ? [{ id: 'p1', name: 'Quarter', initiativeCount: 1, updatedAt: 1 }]
    : { initiatives: [{ name: 'Beacon', targetWeek: null, verdict: 'no-date' }] } }) });
  assert.match(html, /No target dates to check/);
  assert.doesNotMatch(html, /Every dated initiative/);
});

test('healthy plan dates are explicitly forecasts with named scope', async () => {
  const html = await home({ manager: true, response: async (url) => ({ ok: true, json: async () => url === '/api/plan'
    ? [{ id: 'p1', name: 'Quarter', initiativeCount: 1, updatedAt: 1 }]
    : { initiatives: [{ name: 'Beacon', targetWeek: 5, verdict: 'on-time' }] } }) });
  assert.match(html, /forecast on time/);
  assert.match(html, /not observed execution/);
  assert.match(html, /Only the most recently updated populated plan is checked/);
});

test('quality counts are unknown while loading and use server totals independent of drilldown', async () => {
  const elements = new Map(['hygiene-cards', 'hygiene-table', 'hygiene-unassoc'].map((id) => [id, { innerHTML: '', setAttribute() {} }]));
  let resolveCounts;
  const counts = new Promise((resolve) => { resolveCounts = resolve; });
  const context = moduleContext('hygiene.js', {
    document: { getElementById: (id) => elements.get(id), querySelectorAll: () => [] },
    getJiraBaseUrl: async () => '', getSnapshot: () => 'snapshot-1', heatColor: () => 'red',
    apiGet: async (path) => path === 'hygiene-counts' ? counts : path === 'unassoc-epics' ? [] : null,
  });
  context.initHygiene({ pods: [], hygiene: {} });
  assert.match(elements.get('hygiene-cards').innerHTML, /checking whole snapshot/);
  assert.match(elements.get('hygiene-cards').innerHTML, /Unknown/);
  resolveCounts({ unsized: 701, stale: 53, unassigned: 4, nooutcome: 12 });
  await new Promise((resolve) => setImmediate(resolve));
  assert.match(elements.get('hygiene-cards').innerHTML, />701</);
  assert.match(elements.get('hygiene-cards').innerHTML, /whole selected snapshot/);
});

test('quality aggregate request failure does not become zero issues', async () => {
  const el = { innerHTML: '', setAttribute() {} };
  const context = moduleContext('hygiene.js', {
    document: { getElementById: () => el, querySelectorAll: () => [] },
    getJiraBaseUrl: async () => '', getSnapshot: () => 'snapshot-2', heatColor: () => 'red', apiGet: async () => null,
  });
  const html = [];
  Object.defineProperty(el, 'innerHTML', { set(value) { html.push(value); }, get() { return html.at(-1); } });
  context.initHygiene({ pods: [], hygiene: {} });
  await new Promise((resolve) => setImmediate(resolve));
  assert.ok(html.some((value) => /count unavailable/.test(value)));
  assert.ok(!html.some((value) => /class="v">0</.test(value)));
});

test('all manual section links have real native destinations', () => {
  const manual = readFileSync(new URL('../app/docs.html', import.meta.url), 'utf8');
  const ids = new Set([...manual.matchAll(/id="([^"]+)"/g)].map((m) => m[1]));
  for (const link of manual.matchAll(/href="#([^"]+)"/g)) assert.ok(ids.has(link[1]), `Missing manual section ${link[1]}`);
  assert.doesNotMatch(manual, /data-anchor=/);
});

test('contextual help routes each view to its own existing manual section', () => {
  const manual = readFileSync(new URL('../app/docs.html', import.meta.url), 'utf8');
  const ids = new Set([...manual.matchAll(/id="([^"]+)"/g)].map((m) => m[1]));
  let clicked;
  let opened;
  let view;
  let planView;
  const context = moduleContext('docs.js', {
    document: {
      addEventListener(type, handler) { if (type === 'click') clicked = handler; },
      querySelector(selector) { return selector.startsWith('main') ? { id: view } : { id: planView }; },
    },
  });
  context.openDocs = (section) => { opened = section; };
  context.initDocs();
  const routes = [
    ['view-home', null, 'start'], ['view-network', null, 'network'],
    ['view-scoreboard', null, 'scoreboard'], ['view-hygiene', null, 'hygiene'],
    ['view-simulator', null, 'simulator'], ['view-flow', null, 'flow-actions'],
    ['view-game', null, 'learning'], ['view-plan', 'view-order', 'order'],
    ['view-plan', 'plan-view-network', 'plan-network'],
    ['view-plan', 'view-timeline', 'timeline'], ['view-plan', 'view-execution', 'execution'],
    ['view-plan', 'view-report', 'health-report'], ['view-plan', null, 'planning-loop'],
    ['view-unknown', null, 'what'],
  ];
  for (const [main, subview, expected] of routes) {
    view = main; planView = subview;
    clicked({ target: { closest: () => ({ dataset: { docs: 'context' } }) }, preventDefault() {} });
    assert.equal(opened, expected, `${main}/${subview}`);
    assert.ok(ids.has(opened), `Missing destination ${opened}`);
  }
});

test('manual Escape closes after both the initial document and later iframe navigations', () => {
  let load;
  let keydown;
  let hashChanged;
  let shown;
  let closed = 0;
  const positioned = [];
  const frameDocument = { getElementById(id) { return { scrollIntoView() { positioned.push(id); } }; }, documentElement: { setAttribute() {} }, addEventListener(type, handler) { if (type === 'keydown') keydown = handler; } };
  const frame = { dataset: {}, contentDocument: frameDocument, contentWindow: { location: { pathname: '/docs.html' }, document: frameDocument, addEventListener(type, handler) { if (type === 'hashchange') hashChanged = handler; } }, addEventListener(type, handler) { if (type === 'load') load = handler; } };
  const separate = {};
  const overlay = { addEventListener(type, handler) { if (type === 'shown.bs.modal') shown = handler; }, querySelector: (selector) => selector === '#docs-separate' ? separate : frame };
  const context = moduleContext('docs.js', {
    document: { createElement: () => overlay, body: { appendChild() {} }, documentElement: { getAttribute: () => 'dark' } },
    openModal() {}, closeModal(target) { assert.equal(target, overlay); closed++; },
  });
  context.openDocs('timeline');
  assert.equal(frame.src, 'docs.html#timeline');
  assert.equal(separate.href, 'docs.html#timeline');
  load(); keydown({ key: 'Escape', preventDefault() {} });
  assert.deepEqual(positioned, ['timeline'], 'a loaded document applies the requested section');
  shown();
  assert.deepEqual(positioned, ['timeline', 'timeline'], 'presentation restores positioning if loading happened while hidden');
  assert.equal(closed, 1);
  context.openDocs('execution');
  assert.equal(frame.contentWindow.location.hash, 'execution');
  assert.equal(positioned.at(-1), 'execution');
  context.openDocs('execution');
  assert.deepEqual(positioned.slice(-2), ['execution', 'execution'], 'reopening the same hash restores the section after scrolling elsewhere');
  load(); keydown({ key: 'Escape', preventDefault() {} });
  assert.equal(closed, 2);
  keydown({ key: 'Escape', defaultPrevented: true, preventDefault() {} });
  assert.equal(closed, 2, 'reader Escape dismisses search or contents before the outer guide');
  frame.contentWindow.location.hash = '#sites';
  hashChanged();
  assert.equal(separate.href, 'docs.html#sites', 'independent reading preserves the section selected inside the guide');
  frame.contentDocument = null;
  load();
  context.openDocs('concepts');
  assert.equal(frame.src, 'docs.html#concepts', 'an inaccessible frame is reloaded instead of reused');
});

test('closing a programmatically opened dialog returns focus to its invoker', () => {
  let restored = 0;
  const invoker = { isConnected: true, focus() { restored++; } };
  const classes = new Set();
  const attributes = new Map();
  let openedFocus = 0;
  const heading = { id: '' };
  const overlay = { id: 'example-dialog', setAttribute: (key, value) => attributes.set(key, value), hasAttribute: key => attributes.has(key), querySelector: () => heading, addEventListener() {}, removeEventListener() {}, focus() { openedFocus++; }, contains: () => false, firstElementChild: null, classList: { add: (value) => classes.add(value), remove: (value) => classes.delete(value), contains: (value) => classes.has(value) } };
  const context = moduleContext('modal.js', {
    window: {}, document: { activeElement: invoker, querySelectorAll: () => [], body: { classList: { remove() {} } } },
  });
  context.openModal(overlay);
  assert.equal(overlay.hidden, false);
  assert.equal(overlay.tabIndex, -1);
  assert.equal(attributes.get('role'), 'dialog');
  assert.equal(attributes.get('aria-modal'), 'true');
  assert.equal(attributes.get('aria-labelledby'), 'example-dialog-title');
  assert.equal(openedFocus, 1);
  context.closeModal(overlay);
  assert.equal(overlay.hidden, true);
  assert.equal(restored, 1);
});

test('Home capacity count uses uncapped load rather than clamped queue input', async () => {
  const html = await home({ stats: { Atlas: { load: 1.5, rho0: 0.97, wip: 6, synthetic: false } }, hygiene: { Atlas: { score: 0.8 } } });
  assert.match(html, /1 at or above capacity/);
});

test('Home never presents synthetic fallback load or WIP as measured evidence', async () => {
  const html = await home({ stats: { Atlas: { load: 0.5, rho0: 0.5, wip: 6, synthetic: true } } });
  assert.match(html, /Load evidence incomplete/);
  assert.match(html, /0 of 1 teams have a measured load/);
  assert.doesNotMatch(html, /Measured teams below high-load threshold/);
});

test('Close during the Bootstrap opening transition is completed when shown fires', () => {
  let instance;
  const listeners = new Map(), classes = new Set();
  const heading = { id: '' };
  const overlay = { id: 'transition-dialog', hidden: true, firstElementChild: { classList: { contains: () => true } },
    classList: { add: value => classes.add(value), remove: value => classes.delete(value), contains: value => classes.has(value) },
    contains: () => false, setAttribute() {}, hasAttribute: () => false, querySelector: () => heading,
    addEventListener: (type, handler) => listeners.set(type, handler), removeEventListener() {},
  };
  class Modal {
    constructor() { instance = this; }
    show() { this.transitioning = true; }
    finishShow() { this.transitioning = false; classes.add('show'); listeners.get('shown.bs.modal')(); }
    hide() { if (this.transitioning) return; classes.delete('show'); listeners.get('hidden.bs.modal')(); }
  }
  const context = moduleContext('modal.js', { window: { bootstrap: { Modal } }, document: {
    activeElement: null, querySelectorAll: () => [], body: { classList: { remove() {} } }, addEventListener() {}, removeEventListener() {},
  } });
  context.openModal(overlay); context.closeModal(overlay);
  assert.equal(overlay.hidden, false, 'framework opening animation is still running');
  instance.finishShow();
  assert.equal(overlay.hidden, true, 'pending close completes after the framework can hide');
});

test('role guidance handles an empty or synthetic-only snapshot without inventing insights', () => {
  const context = moduleContext('guide.js', { constraintScores: () => [] });
  for (const state of [{ stats: {}, pods: [], edges: [] }, { stats: { Atlas: { synthetic: true } }, pods: [{ name: 'Atlas' }], edges: [] }]) {
    const insights = context.computeInsights(state, {});
    assert.equal(insights.length, 1);
    assert.match(insights[0].obs, /No measured team-flow evidence/);
    assert.ok(insights[0].who.includes('planner'));
    assert.ok(insights[0].who.includes('pm'));
  }
});
