import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { measureContextHTML, mountMeasureContext, simulatorSourceHTML, simulatorTeamOptionsHTML, snapshotSelectionURL, MEASURE_VIEWS } from '../app/js/measure-context.js';

const snapshot = { id: 'capture-a', name: 'September capture', source: 'jira', createdAt: 1788600000, scope: ['PROJ'], rosterId: 'roster-a' };
const model = { snapshots: [snapshot], selectedId: snapshot.id, rosters: [{ id: 'roster-a', name: 'Atlas teams' }],
  state: { pods: [{ name: 'Atlas' }], stats: { Atlas: { synthetic: false } } }, canManage: true };

test('a single snapshot retains source identity, roster, scope, switching and association help', () => {
  for (const view of ['network', 'scoreboard', 'hygiene', 'simulator']) {
    const html = measureContextHTML({ ...model, view });
    for (const text of ['September capture', 'Jira import', 'Atlas teams', 'PROJ', 'Captured', 'Historical team statistics', 'Viewing org snapshot', 'Execution snapshot', 'Epic bindings']) assert.ok(html.includes(text), text);
    assert.match(html, /id="measure-snapshot" disabled/);
    assert.match(html, /data-measure-action="associations"/);
    assert.ok(html.includes(MEASURE_VIEWS[view]));
  }
});

test('multiple snapshots and a missing selected snapshot permit source recovery', () => {
  const multi = measureContextHTML({ ...model, snapshots: [snapshot, { ...snapshot, id: 'capture-b' }] });
  assert.doesNotMatch(multi, /id="measure-snapshot" disabled/);
  const missing = measureContextHTML({ ...model, selectedId: 'removed' });
  assert.match(missing, /Selected snapshot unavailable/);
  assert.doesNotMatch(missing, /id="measure-snapshot" disabled/);
});

test('source type and synthetic statistics are distinct, and missing metadata stays explicit', () => {
  assert.match(measureContextHTML({ ...model, snapshots: [{ ...snapshot, source: 'baseline' }] }), /Example data, not an observation/);
  assert.match(measureContextHTML({ ...model, state: { pods: [{ name: 'Atlas' }, { name: 'Beacon' }], stats: { Atlas: { synthetic: true } } } }), /1 of 2 teams use synthetic estimates; 1 have unknown statistics/);
  const empty = measureContextHTML({ selectedId: 'missing', error: 'Snapshot details could not be loaded.' });
  for (const text of ['Snapshot details could not be loaded.', 'Capture date unavailable', 'Project scope unavailable', 'No team data loaded.', 'Retry source details']) assert.ok(empty.includes(text));
  assert.doesNotMatch(empty, /Historical team statistics loaded/);
  assert.match(empty, /Roster association unavailable/);
  assert.doesNotMatch(empty, /No saved roster associated/);
});

test('read-only context permits selection without association-edit actions and escapes metadata', () => {
  const html = measureContextHTML({ ...model, canManage: false, snapshots: [{ ...snapshot, name: '<script>alert(1)</script>', id: '" onfocus="bad', scope: ['<PROJ>'] }], selectedId: '" onfocus="bad' });
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /&quot; onfocus=&quot;bad/);
  assert.doesNotMatch(html, /<script>|data-measure-action="(?:import|associations|rosters|plans)"/);
});

test('snapshot switching preserves the current view and independent execution selection', () => {
  const url = new URL(snapshotSelectionURL('http://localhost/?view=simulator&executionSnapshot=evidence-a&snapshot=old#details', 'capture-b'));
  assert.equal(url.searchParams.get('view'), 'simulator');
  assert.equal(url.searchParams.get('executionSnapshot'), 'evidence-a');
  assert.equal(url.searchParams.get('snapshot'), 'capture-b');
  assert.equal(url.hash, '#details');
});

test('simulator provenance separates examples, imported epics, edited tasks and stale results', () => {
  assert.match(simulatorSourceHTML(), /Example tasks/);
  assert.match(simulatorSourceHTML(), /not imported delivery work/);
  assert.match(simulatorSourceHTML({ kind: 'epic', epic: 'PROJ-123' }), /Imported epic PROJ-123/);
  const edited = simulatorSourceHTML({ kind: 'epic', epic: '<PROJ>', edited: true, dirty: true });
  assert.match(edited, /Edited scenario/);
  assert.match(edited, /&lt;PROJ&gt;/);
  assert.match(edited, /Previous results are stale/);
  assert.doesNotMatch(simulatorSourceHTML({ kind: 'epic', edited: true, dirty: false }), /Previous results are stale/);
});

test('imported missing or unknown teams cannot silently select the first roster team', () => {
  const pods = [{ name: 'Atlas' }];
  assert.match(simulatorTeamOptionsHTML(pods, ''), /value="" selected>Choose a team/);
  assert.match(simulatorTeamOptionsHTML(pods, 'Beacon'), /value="Beacon" selected>Beacon \(not in snapshot roster\)/);
  assert.match(simulatorTeamOptionsHTML(pods, 'Atlas'), /<option selected>Atlas/);
  assert.match(simulatorTeamOptionsHTML(pods, '<Unknown>'), /&lt;Unknown&gt;/);
});

test('successful snapshot and roster writes notify source context while reads and failures do not', async () => {
  for (const [file, path] of [['snapshotsui.js', '/api/snapshots/capture-a'], ['rostersui.js', '/api/rosters/roster-a']]) {
    const source = readFileSync(new URL(`../app/js/${file}`, import.meta.url), 'utf8');
    const start = source.indexOf('async function req(');
    const end = source.indexOf('\n}', start) + 2;
    let notifications = 0, ok = true;
    const scope = vm.createContext({ authFetch: async () => ({ ok }), notifyMeasureSourcesChanged: () => { notifications++; } });
    vm.runInContext(source.slice(start, end), scope);
    await scope.req(path); assert.equal(notifications, 0);
    await scope.req(path, { method: 'PATCH' }); assert.equal(notifications, 1);
    ok = false;
    await scope.req(path, { method: 'PATCH' }); assert.equal(notifications, 1);
  }
});

test('metadata failures can be retried without changing source or persisting data', async () => {
  const events = new Map(), paths = [];
  const host = { hidden: true, innerHTML: '', addEventListener: (name, handler) => events.set(name, handler) };
  let failing = true, selected = '', action = '';
  const controller = mountMeasureContext(host, { ...model, request: async path => {
    paths.push(path);
    return { ok: !failing, json: async () => path === '/api/snapshots' ? [snapshot, { ...snapshot, id: 'capture-b' }] : model.rosters };
  }, onSelect: id => { selected = id; }, actions: { associations: () => { action = 'associations'; } } });
  await controller.ready;
  assert.match(host.innerHTML, /Retry source details/);
  failing = false;
  await controller.refresh();
  assert.match(host.innerHTML, /September capture/);
  assert.doesNotMatch(host.innerHTML, /Retry source details/);
  events.get('change')({ target: { id: 'measure-snapshot', value: 'capture-b' } });
  assert.equal(selected, 'capture-b');
  events.get('click')({ target: { closest: () => ({ dataset: { measureAction: 'associations' } }) } });
  assert.equal(action, 'associations');
  controller.setView('plan'); assert.equal(host.hidden, true);
  controller.setView('scoreboard'); assert.equal(host.hidden, false);
  assert.ok(paths.every(path => ['/api/snapshots', '/api/rosters'].includes(path)));
});
