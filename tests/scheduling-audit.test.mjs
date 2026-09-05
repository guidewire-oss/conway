import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { schedulingFromForm, schedulingFormHTML, compareScheduleCosts, objectiveView } from '../app/js/order.js';
import { remedyRowHTML } from '../app/js/remedyui.js';
import { remediesSectionHTML } from '../app/js/report.js';

test('editing visible assumptions preserves hidden policy and can clear a lead override', () => {
  const saved = { leadCapacity: { pm: 7, eng: 8, specialist: 9 }, customPolicy: { retain: true }, acceptedOrdering: 'engine', bufferPct: .4 };
  const before = structuredClone(saved);
  const form = { 'sched-lead-pm': '0', 'sched-lead-eng': '', 'sched-buffer': '' };
  const next = schedulingFromForm((id) => form[id] ?? '', saved);
  assert.equal(next.leadCapacity.pm, 0);
  assert.equal(next.leadCapacity.specialist, 9);
  assert.equal(next.leadCapacity.eng, undefined);
  assert.equal(next.bufferPct, undefined);
  assert.deepEqual(next.customPolicy, { retain: true });
  assert.equal(next.acceptedOrdering, 'engine');
  assert.deepEqual(saved, before);
  const html = schedulingFormHTML({ leadCapacity: { pm: 0 } });
  assert.match(html, /id="sched-lead-pm"[^>]*value="0"/);
  for (const role of ['pm', 'eng', 'architect', 'pgm']) assert.match(html, new RegExp(`id="sched-lead-${role}"`));
});

test('coverage improvements precede lateness and cannot be mislabeled as lateness savings', () => {
  assert.ok(compareScheduleCosts({ unscheduledWeight: 0, objective: 5 }, { unscheduledWeight: 2, objective: 0 }) < 0);
  assert.ok(compareScheduleCosts({ unscheduledWeight: 2, objective: 0 }, { unscheduledWeight: 0, objective: 5 }) > 0);
  const v = objectiveView({ unscheduledWeight: 0, statedOrderUnscheduledWeight: 2, objectiveScore: 5, statedOrderObjectiveScore: 0, initiatives: [{ statedRank: 1, targetWeek: 1 }] });
  assert.equal(v.better, true);
  const html = remedyRowHTML({ kind: 'raise-priority', target: 'Alpha', objectiveDelta: -5, unscheduledWeightDelta: 2, resultingVerdict: 'on-time' });
  assert.match(html, /ord-red/);
  assert.match(html, /weighted unstarted work/);
  const report = remediesSectionHTML({ remedies: [
    { kind: 'raise-priority', target: 'Drops work', objectiveDelta: -100, unscheduledWeightDelta: 2 },
    { kind: 'raise-priority', target: 'Preserves work', objectiveDelta: 1, unscheduledWeightDelta: -2 },
  ] });
  assert.ok(report.indexOf('Preserves work') < report.indexOf('Drops work'));
});

// Run the actual asynchronous handlers with controllable responses. This tests
// request ownership during network and JSON-body waits without saving a plan.
const source = readFileSync(new URL('../app/js/planui.js', import.meta.url), 'utf8');
const extract = (start, end) => {
  const from = source.indexOf(start), to = source.indexOf(end, from);
  assert.ok(from >= 0 && to > from);
  return source.slice(from, to);
};
function harness() {
  const pending = [], paints = [];
  const scope = vm.createContext({
    current: { id: 'one', initiatives: [], scheduling: {} }, orderEpoch: 1, previewTicket: 0, simulationTicket: 0,
    root: { querySelector: () => ({ insertAdjacentHTML() {} }) },
    document: { getElementById: () => null },
    FormData: class { append() {} }, setTimeout() {}, alert: assert.fail,
    req: () => new Promise((resolve) => pending.push(resolve)),
    staleOrder() { scope.orderEpoch++; }, renderPlan() { paints.push('plan'); }, paintDash() { paints.push('dash'); },
  });
  vm.runInContext(extract('async function previewInitiativesFile(', 'async function saveDraftInitiatives('), scope);
  vm.runInContext(extract('async function runSim()', 'const PODS ='), scope);
  return { scope, pending, paints };
}
const response = (name) => ({ ok: true, json: async () => ({ initiatives: [{ name }], network: {}, unknownTeams: [], sim: { name } }) });

for (const fn of ['previewInitiativesFile', 'runSim']) {
  test(`${fn} cannot overwrite another plan after a delayed response`, async () => {
    const { scope, pending, paints } = harness();
    const run = scope[fn]({ name: 'upload.xlsx' });
    scope.current = { id: 'two', initiatives: [{ name: 'Retained' }], sim: { name: 'Retained' } };
    pending[0](response('Old')); await run;
    assert.equal(scope.current.initiatives[0].name, 'Retained');
    assert.equal(scope.current.sim.name, 'Retained');
    assert.deepEqual(paints, []);
  });
  test(`${fn} ignores an older request after the newest response`, async () => {
    const { scope, pending } = harness();
    const old = scope[fn]({ name: 'old.xlsx' });
    const fresh = scope[fn]({ name: 'fresh.xlsx' });
    pending[1](response('Fresh')); await fresh;
    pending[0](response('Old')); await old;
    const actual = fn === 'runSim' ? scope.current.sim.initiatives[0].name : scope.current.initiatives[0].name;
    assert.equal(actual, 'Fresh');
  });
  test(`${fn} rechecks input ownership after the JSON body resolves`, async () => {
    const { scope, pending, paints } = harness();
    let finishBody;
    const run = scope[fn]({ name: 'upload.xlsx' });
    pending[0]({ ok: true, json: () => new Promise((resolve) => { finishBody = resolve; }) });
    await new Promise(setImmediate);
    scope.orderEpoch++;
    finishBody({ initiatives: [{ name: 'Old' }] }); await run;
    assert.equal(scope.current.initiatives.length, 0);
    assert.equal(scope.current.sim, undefined);
    assert.deepEqual(paints, []);
  });
}
