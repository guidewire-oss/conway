import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

// Exercise the real undo implementation with a controlled transport and view.
// This observes request bodies and state after failure and delayed responses.
const source = readFileSync(new URL('../app/js/planui.js', import.meta.url), 'utf8');
const functions = source.slice(source.indexOf('function snapshotDragUndo('), source.indexOf('async function renderList()'));

function harness(respond) {
  const requests = [], notes = [];
  const scope = vm.createContext({
    current: { id: 'plan-one', initiatives: [{ name: 'Alpha', work: { Atlas: { weeks: 8 }, Beacon: { weeks: 4 } } }] },
    orderEpoch: 0, dragUndo: null, dragHistory: [], timelineMutationPending: false,
    req: async (path, opts) => { requests.push(JSON.parse(opts.body)); return respond(); },
    dragNote: (s) => notes.push(s), staleOrder() { scope.orderEpoch++; scope.current.schedule = null; },
    view: () => 'timeline', renderTimeline: async () => {}, renderOrder: async () => {}, renderDash: async () => {},
  });
  vm.runInContext(functions, scope);
  return { scope, requests, notes };
}

test('precise-edit snapshots include all prior efforts and copy pin maps', () => {
  const { scope } = harness(() => {});
  scope.current.initiatives[0].pinnedStarts = { Atlas: 2 };
  const snap = scope.snapshotDragUndo(scope.current.initiatives[0]);
  assert.equal(JSON.stringify(snap.estimateEdits), '{"Atlas":8,"Beacon":4}');
  scope.current.initiatives[0].pinnedStarts.Atlas = 9;
  assert.equal(snap.pinnedStarts.Atlas, 2);
  assert.equal(JSON.stringify(scope.snapshotDragUndo(scope.current.initiatives[0], 'Atlas').estimateEdits), '{"Atlas":8}');
});

test('failed undo preserves history and retry restores the most recent edit before the older one', async () => {
  let fail = true;
  const { scope, requests, notes } = harness(() => fail
    ? { ok: false, text: async () => 'Conflicting lane placement' }
    : { ok: true, json: async () => ({ initiatives: scope.current.initiatives }) });
  scope.dragHistory.push(scope.snapshotDragUndo(scope.current.initiatives[0], 'Beacon'));
  scope.dragHistory.push(scope.snapshotDragUndo(scope.current.initiatives[0], 'Atlas'));
  await scope.undoDrag();
  assert.equal(scope.dragHistory.length, 2);
  assert.match(notes.at(-1), /Conflicting/);
  assert.equal(scope.timelineMutationPending, false);
  fail = false;
  await scope.undoDrag();
  assert.equal(scope.dragHistory.length, 1);
  assert.deepEqual(requests.at(-1).initiatives[0].estimateEdits, { Atlas: 8 });
  await scope.undoDrag();
  assert.equal(scope.dragHistory.length, 0);
  assert.deepEqual(requests.at(-1).initiatives[0].estimateEdits, { Beacon: 4 });
});

test('a delayed undo response cannot replace another plan or consume its history', async () => {
  let resolve;
  const { scope } = harness(() => new Promise((r) => { resolve = r; }));
  scope.dragHistory.push(scope.snapshotDragUndo(scope.current.initiatives[0]));
  const pending = scope.undoDrag();
  scope.current = { id: 'plan-two', initiatives: [{ name: 'Beta' }] };
  resolve({ ok: true, json: async () => ({ initiatives: [{ name: 'Alpha' }] }) });
  await pending;
  assert.equal(scope.current.initiatives[0].name, 'Beta');
  assert.equal(scope.dragHistory.length, 1);
  assert.equal(scope.timelineMutationPending, false);
});
