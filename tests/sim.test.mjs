import test from 'node:test';
import assert from 'node:assert/strict';
import {
  makeRng, sampleLognormal, percentile, kingmanScale,
  handoffDays, topoSort, simulateFeature, fitLognormal,
} from '../app/js/sim.js';

test('makeRng is deterministic and in [0,1)', () => {
  const a = makeRng(42), b = makeRng(42);
  for (let i = 0; i < 100; i++) {
    const x = a();
    assert.equal(x, b());
    assert.ok(x >= 0 && x < 1);
  }
});

test('sampleLognormal matches target median and spread', () => {
  const rng = makeRng(7);
  const mu = Math.log(5), sigma = 0.8;
  const xs = Array.from({ length: 20000 }, () => sampleLognormal(rng, mu, sigma));
  const med = percentile(xs, 50);
  assert.ok(Math.abs(med - 5) < 0.3, `median ${med} ≉ 5`);
  const meanLog = xs.reduce((s, x) => s + Math.log(x), 0) / xs.length;
  assert.ok(Math.abs(meanLog - mu) < 0.05);
});

test('percentile interpolates correctly', () => {
  assert.equal(percentile([1, 2, 3, 4, 5], 50), 3);
  assert.equal(percentile([1, 2, 3, 4], 100), 4);
  assert.equal(percentile([10], 85), 10);
});

test('fitLognormal recovers parameters', () => {
  const rng = makeRng(11);
  const xs = Array.from({ length: 5000 }, () => sampleLognormal(rng, Math.log(8), 0.6));
  const { mu, sigma } = fitLognormal(xs);
  assert.ok(Math.abs(mu - Math.log(8)) < 0.05, `mu ${mu}`);
  assert.ok(Math.abs(sigma - 0.6) < 0.05, `sigma ${sigma}`);
});

test('kingmanScale: 1 at baseline, grows with rho, capped', () => {
  assert.equal(kingmanScale(0.8, 0.8), 1);
  assert.ok(kingmanScale(0.9, 0.8) > 1.5);
  assert.ok(kingmanScale(0.5, 0.8) < 0.5);
  assert.ok(kingmanScale(0.999, 0.8) <= kingmanScale(0.97, 0.8) + 1e-9); // cap
});

test('handoffDays from overlap hours', () => {
  const rt = 3;
  assert.equal(handoffDays(8, rt), 0.75);
  assert.equal(handoffDays(3, rt), 1.5);
  assert.equal(handoffDays(1, rt), 3);
  assert.equal(handoffDays(0, rt), 4.5);
});

test('topoSort orders dependencies and rejects cycles', () => {
  const order = topoSort(['a', 'b', 'c'], [['a', 'c'], ['b', 'c']]);
  assert.ok(order.indexOf('c') > order.indexOf('a'));
  assert.ok(order.indexOf('c') > order.indexOf('b'));
  assert.throws(() => topoSort(['a', 'b'], [['a', 'b'], ['b', 'a']]));
});

const POD_STATS = {
  Alpha: { mu: Math.log(4), sigma: 0.001, rho0: 0.8 },
  Beta: { mu: Math.log(6), sigma: 0.001, rho0: 0.8 },
};
const OVERLAP = { Alpha: { Alpha: 8, Beta: 1 }, Beta: { Beta: 8, Alpha: 1 } };

test('simulateFeature: serial chain ≈ sum of durations + handoff', () => {
  const feature = {
    tasks: [
      { id: 't1', pod: 'Alpha', size: 1 },
      { id: 't2', pod: 'Beta', size: 1 },
    ],
    deps: [['t1', 't2']],
  };
  const r = simulateFeature(feature, POD_STATS, OVERLAP, { trials: 2000, seed: 1, roundtrips: 3, flowEff: 1 });
  // 4 + 6 + handoff(1h overlap → 1d × 3rt = 3d) = 13, near-zero sigma
  assert.ok(Math.abs(r.p50 - 13) < 0.5, `p50 ${r.p50}`);
  assert.ok(r.p85 >= r.p50 && r.p95 >= r.p85);
});

test('simulateFeature: parallel tasks take the max, criticality sums to ~1 per cut', () => {
  const feature = {
    tasks: [
      { id: 'short', pod: 'Alpha', size: 1 },
      { id: 'long', pod: 'Beta', size: 1 },
    ],
    deps: [],
  };
  const r = simulateFeature(feature, POD_STATS, OVERLAP, { trials: 2000, seed: 2, flowEff: 1 });
  assert.ok(Math.abs(r.p50 - 6) < 0.5, `p50 ${r.p50}`);
  assert.ok(r.criticality.long > 0.95);
  assert.ok(r.criticality.short < 0.05);
});

test('simulateFeature: raising rho on a pod increases completion (what-if)', () => {
  const feature = { tasks: [{ id: 't', pod: 'Alpha', size: 1 }], deps: [] };
  const base = simulateFeature(feature, POD_STATS, OVERLAP, { trials: 2000, seed: 3, flowEff: 0.15 });
  const hot = simulateFeature(feature, POD_STATS, OVERLAP, {
    trials: 2000, seed: 3, flowEff: 0.15, rhoOverride: { Alpha: 0.95 },
  });
  assert.ok(hot.p50 > base.p50 * 1.5, `hot ${hot.p50} vs base ${base.p50}`);
});

test('simulateFeature: sensitivity ranks the dominant pod first', () => {
  const feature = {
    tasks: [
      { id: 't1', pod: 'Alpha', size: 1 },
      { id: 't2', pod: 'Beta', size: 4 },
    ],
    deps: [['t1', 't2']],
  };
  const r = simulateFeature(feature, POD_STATS, OVERLAP, { trials: 1000, seed: 4, flowEff: 1 });
  assert.ok(r.sensitivity[0].pod === 'Beta');
  assert.ok(r.sensitivity[0].savedDays > 0);
});

test('simulateFeature: size factor scales duration', () => {
  const small = simulateFeature({ tasks: [{ id: 't', pod: 'Alpha', size: 0.5 }], deps: [] }, POD_STATS, OVERLAP, { trials: 1000, seed: 5, flowEff: 1 });
  const large = simulateFeature({ tasks: [{ id: 't', pod: 'Alpha', size: 2 }], deps: [] }, POD_STATS, OVERLAP, { trials: 1000, seed: 5, flowEff: 1 });
  assert.ok(Math.abs(large.p50 / small.p50 - 4) < 0.2);
});

test('suggestDeps proposes historical edges missing from the feature', async () => {
  const { suggestDeps } = await import('../app/js/sim.js');
  const tasks = [
    { id: 'a1', pod: 'Alpha' }, { id: 'b1', pod: 'Beta' }, { id: 'c1', pod: 'Gamma' },
  ];
  const edges = [
    { from: 'Alpha', to: 'Beta', count: 14 },   // missing -> suggest
    { from: 'Alpha', to: 'Gamma', count: 1 },   // below threshold -> skip
    { from: 'Beta', to: 'Gamma', count: 9 },    // already declared -> skip
    { from: 'Alpha', to: 'Delta', count: 20 },  // Delta not in feature -> skip
  ];
  const existing = [['b1', 'c1']];
  const sugg = suggestDeps(tasks, existing, edges, 2);
  assert.equal(sugg.length, 1);
  assert.equal(sugg[0].fromPod, 'Alpha');
  assert.equal(sugg[0].toPod, 'Beta');
  assert.equal(sugg[0].count, 14);
  assert.equal(sugg[0].fromTask, 'a1');
  assert.equal(sugg[0].toTask, 'b1');
});

test('constraintScores ranks hot hubs first', async () => {
  const { constraintScores } = await import('../app/js/sim.js');
  const stats = {
    Hub: { rho0: 0.9 }, Quiet: { rho0: 0.9 }, Busy: { rho0: 0.5 },
  };
  const edges = [
    { from: 'Hub', to: 'A', count: 5 }, { from: 'Hub', to: 'B', count: 3 },
    { from: 'Busy', to: 'A', count: 9 },
  ];
  const r = constraintScores(stats, edges);
  assert.equal(r[0].pod, 'Hub');           // hot AND wide reach
  assert.ok(r[0].score > r[1].score);
  const quiet = r.find((x) => x.pod === 'Quiet');
  const busy = r.find((x) => x.pod === 'Busy');
  assert.ok(quiet.score < r[0].score);     // hot but no dependents
  assert.equal(busy.dependents, 1);
});

test('freezeProjection applies Littles law', async () => {
  const { freezeProjection } = await import('../app/js/sim.js');
  const r = freezeProjection(40, 10, 15);  // wip 40, 10/wk, cap 15
  assert.ok(Math.abs(r.currentDays - 20) < 1e-9);   // 40/(10/5)
  assert.ok(Math.abs(r.projectedDays - 7.5) < 1e-9); // 15/2
  assert.equal(r.frozen, 25);
  const noThru = freezeProjection(40, 0, 15);
  assert.equal(noThru.currentDays, null);
});

test('feverPoint zones', async () => {
  const { feverPoint } = await import('../app/js/sim.js');
  // p50=100, p85=150 -> buffer 50
  const green = feverPoint(0.5, 55, 100, 150);   // elapsed ~ on plan
  assert.equal(green.zone, 'green');
  const yellow = feverPoint(0.5, 65, 100, 150);  // consumed (65-50)/50=0.3
  assert.equal(yellow.zone, 'yellow');
  const red = feverPoint(0.2, 80, 100, 150);     // consumed (80-20)/50=1.2
  assert.equal(red.zone, 'red');
  assert.ok(red.consumed > 1);
});

const ORG_MODEL = () => ({
  pods: [
    { name: 'A', devCount: 5, location: 'X' },
    { name: 'B', devCount: 4, location: 'X' },
    { name: 'C', devCount: 6, location: 'Y' },
  ],
  stats: {
    A: { mu: Math.log(6), sigma: 0.8, rho0: 0.8, wip: 10, throughputWk: 5, resolved180: 130 },
    B: { mu: Math.log(10), sigma: 0.6, rho0: 0.9, wip: 12, throughputWk: 3, resolved180: 78 },
    C: { mu: Math.log(8), sigma: 0.7, rho0: 0.6, wip: 8, throughputWk: 6, resolved180: 156 },
  },
  edges: [
    { from: 'B', to: 'A', count: 6 },
    { from: 'A', to: 'B', count: 2 },
    { from: 'C', to: 'B', count: 4 },
    { from: 'B', to: 'C', count: 3 },
  ],
  overlap: {
    A: { A: 8, B: 8, C: 1 },
    B: { A: 8, B: 8, C: 1 },
    C: { A: 1, B: 1, C: 8 },
  },
  residualTax: 0,
});

test('mergePods repoints, internalizes, and sums capacity', async () => {
  const { mergePods } = await import('../app/js/sim.js');
  const m = mergePods(ORG_MODEL(), 'A', 'B');
  const merged = m.pods.find((p) => p.name === 'A+B');
  assert.ok(merged);
  assert.equal(merged.devCount, 9);
  assert.ok(!m.pods.some((p) => p.name === 'A' || p.name === 'B'));
  // A<->B edges internalized (same site, full overlap -> gone)
  assert.ok(!m.edges.some((e) => e.from === e.to));
  const inbound = m.edges.find((e) => e.from === 'C' && e.to === 'A+B');
  assert.equal(inbound.count, 4);
  const outbound = m.edges.find((e) => e.from === 'A+B' && e.to === 'C');
  assert.equal(outbound.count, 3);
  assert.equal(m.residualTax, 0); // same-site merge: no residual
  const s = m.stats['A+B'];
  assert.equal(s.wip, 22);
  assert.ok(Math.abs(s.throughputWk - 8) < 1e-9);
  assert.ok(m.overlap['A+B'].C >= 1);
});

test('mergePods across zero-overlap sites keeps residual coordination cost', async () => {
  const { mergePods } = await import('../app/js/sim.js');
  const base = ORG_MODEL();
  base.pods[1].location = 'Z';
  base.overlap.A.B = 0; base.overlap.B.A = 0;
  const m = mergePods(base, 'A', 'B');
  assert.ok(m.residualTax > 0, 'cross-site internalization is not free');
});

test('orgFlowScore drops when a heavy coupling is internalized', async () => {
  const { mergePods, orgFlowScore } = await import('../app/js/sim.js');
  const base = ORG_MODEL();
  const before = orgFlowScore(base);
  const after = orgFlowScore(mergePods(base, 'A', 'B'));
  assert.ok(before.total > 0);
  assert.ok(after.total < before.total, `after ${after.total} !< before ${before.total}`);
  assert.ok(after.coordTax <= before.coordTax);
});

test('suggestMerges internalizes the most expensive coupling first and respects dev cap', async () => {
  const { suggestMerges } = await import('../app/js/sim.js');
  const moves = suggestMerges(ORG_MODEL(), { maxDevs: 12, rounds: 2 });
  assert.ok(moves.length >= 1);
  const names = [moves[0].absorber, moves[0].absorbed].sort().join(',');
  // B-C has fewer links than A-B (7 vs 8) but crosses a 1h-overlap site
  // boundary, so internalizing it removes far more coordination cost
  assert.equal(names, 'B,C');
  assert.ok(moves[0].delta > 0);
  assert.ok(moves.every((mv) => mv.mergedDevs <= 12));
});

test('mergePods transferFraction moves scope fully but capacity partially', async () => {
  const { mergePods } = await import('../app/js/sim.js');
  const half = mergePods(ORG_MODEL(), 'A', 'B', 0.5);
  const m = half.pods.find((p) => p.name === 'A+B');
  assert.equal(m.devCount, 7);                 // 5 + round(4*0.5)
  assert.equal(half.stats['A+B'].wip, 22);     // scope moves fully
  assert.ok(Math.abs(half.stats['A+B'].throughputWk - 6.5) < 1e-9); // 5 + 3*0.5

  const none = mergePods(ORG_MODEL(), 'A', 'B', 0);
  assert.equal(none.pods.find((p) => p.name === 'A+B').devCount, 5);
  const full = mergePods(ORG_MODEL(), 'A', 'B', 1);
  assert.ok(none.stats['A+B'].rho0 >= full.stats['A+B'].rho0, 'less capacity -> hotter queue');
});

test('orgFlowScore tradeoff: zero-transfer merge cuts coord tax but raises queue tax', async () => {
  const { mergePods, orgFlowScore } = await import('../app/js/sim.js');
  const before = orgFlowScore(ORG_MODEL());
  const none = orgFlowScore(mergePods(ORG_MODEL(), 'A', 'B', 0));
  const full = orgFlowScore(mergePods(ORG_MODEL(), 'A', 'B', 1));
  assert.ok(none.coordTax < before.coordTax);            // internalization still helps
  assert.ok(none.queueTax >= full.queueTax);             // but the queue pays for it
});

test('fullKitCheck flags missing pods, sizes, SRE, saturation, and undeclared deps', async () => {
  const { fullKitCheck } = await import('../app/js/sim.js');
  const epic = {
    epic: 'GWCP-1',
    tasks: [
      { key: 'GWCP-2', pod: 'Alpha', points: 3, status: 'Open', blockedBy: [], blocks: [] },
      { key: 'GWCP-3', pod: 'Beta', points: null, status: 'Open', blockedBy: ['GWCP-99'], blocks: [] },
      { key: 'GWCP-4', pod: null, points: 5, status: 'Open', blockedBy: [], blocks: [] },
    ],
  };
  const stats = {
    Alpha: { wip: 30, rho0: 0.92 },   // saturated
    Beta: { wip: 2, rho0: 0.4 },
  };
  const pods = [
    { name: 'Alpha', devCount: 5 }, { name: 'Beta', devCount: 4 },
  ];
  const r = fullKitCheck(epic, { stats, pods, srePods: ['Cooperstown'] });
  const by = (id) => r.items.find((i) => i.id === id);
  assert.equal(by('pods-assigned').status, 'fail');     // GWCP-4 has no pod
  assert.equal(by('sized').status, 'fail');             // 1 of 3 unsized
  assert.equal(by('blockers').status, 'warn');          // external blocker unknown
  assert.equal(by('sre').status, 'fail');               // no SRE task
  assert.equal(by('headroom').status, 'fail');          // Alpha saturated
  assert.equal(by('deps-declared').status, 'warn');     // 2+ pods, zero deps
  assert.equal(by('outcome').status, 'warn');           // hasOutcome unknown
  assert.ok(r.score >= 0 && r.score <= 1);
  assert.ok(r.score < 0.5);
});

test('fullKitCheck passes a well-formed kit', async () => {
  const { fullKitCheck } = await import('../app/js/sim.js');
  const epic = {
    epic: 'GWCP-1',
    hasOutcome: true,
    tasks: [
      { key: 'GWCP-2', pod: 'Alpha', points: 3, status: 'Open', blockedBy: [], blocks: ['GWCP-3'] },
      { key: 'GWCP-3', pod: 'Beta', points: 2, status: 'Open', blockedBy: ['GWCP-2'], blocks: [] },
      { key: 'GWCP-5', pod: 'Cooperstown', points: 2, status: 'Open', blockedBy: ['GWCP-3'], blocks: [] },
    ],
  };
  const stats = {
    Alpha: { wip: 4, rho0: 0.4 }, Beta: { wip: 2, rho0: 0.3 }, Cooperstown: { wip: 3, rho0: 0.4 },
  };
  const pods = [
    { name: 'Alpha', devCount: 5 }, { name: 'Beta', devCount: 4 }, { name: 'Cooperstown', devCount: 6 },
  ];
  const r = fullKitCheck(epic, { stats, pods, srePods: ['Cooperstown'] });
  assert.ok(r.items.every((i) => i.status === 'pass'), JSON.stringify(r.items.filter((i) => i.status !== 'pass')));
  assert.equal(r.score, 1);
});

test('relativeSize normalizes points against the pod median', async () => {
  const { relativeSize } = await import('../app/js/sim.js');
  assert.equal(relativeSize(5, 5), 1);     // a median item is 1x
  assert.equal(relativeSize(10, 5), 2);    // double the median
  assert.equal(relativeSize(1, 5), 0.25);  // clamped floor
  assert.equal(relativeSize(80, 5), 4);    // clamped ceiling
  assert.equal(relativeSize(null, 5), 1);  // unsized -> median
  assert.equal(relativeSize(8, null), 1);  // pod has no point history -> median
});

test('workStreams halves for pairing pods', async () => {
  const { workStreams } = await import('../app/js/sim.js');
  assert.equal(workStreams(6, true), 3);
  assert.equal(workStreams(5, true), 2.5);
  assert.equal(workStreams(6, false), 6);
  assert.equal(workStreams(1, true), 1); // floor: a team is at least one stream
});
