// Pure simulation engine: lognormal task durations, Kingman queue scaling,
// timezone handoff penalties, PERT Monte Carlo. No DOM, no dependencies.

export function makeRng(seed) {
  let s = seed >>> 0;
  return function () {
    s |= 0; s = (s + 0x6D2B79F5) | 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function sampleNormal(rng) {
  let u = 0, v = 0;
  while (u === 0) u = rng();
  while (v === 0) v = rng();
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v);
}

export function sampleLognormal(rng, mu, sigma) {
  return Math.exp(mu + sigma * sampleNormal(rng));
}

export function percentile(xs, p) {
  const s = [...xs].sort((a, b) => a - b);
  if (s.length === 1) return s[0];
  const idx = (p / 100) * (s.length - 1);
  const lo = Math.floor(idx), hi = Math.ceil(idx);
  return s[lo] + (s[hi] - s[lo]) * (idx - lo);
}

export function fitLognormal(xs) {
  const logs = xs.map((x) => Math.log(Math.max(x, 0.25)));
  const mu = logs.reduce((a, b) => a + b, 0) / logs.length;
  const varr = logs.reduce((a, b) => a + (b - mu) ** 2, 0) / logs.length;
  return { mu, sigma: Math.sqrt(varr) };
}

const RHO_CAP = 0.97;

// Kingman ρ/(1−ρ) wait factor, relative to the pod's baseline ρ0.
export function kingmanScale(rho, rho0) {
  const f = (r) => { const c = Math.min(r, RHO_CAP); return c / (1 - c); };
  return f(rho) / f(rho0);
}

// Handoff delay in working days for one cross-pod dependency edge.
export function handoffDays(overlapHours, roundtrips) {
  let perTrip;
  if (overlapHours >= 4) perTrip = 0.25;
  else if (overlapHours >= 2) perTrip = 0.5;
  else if (overlapHours > 0) perTrip = 1.0;
  else perTrip = 1.5;
  return perTrip * roundtrips;
}

export function topoSort(ids, deps) {
  const indeg = new Map(ids.map((id) => [id, 0]));
  const out = new Map(ids.map((id) => [id, []]));
  for (const [from, to] of deps) {
    out.get(from).push(to);
    indeg.set(to, indeg.get(to) + 1);
  }
  const q = ids.filter((id) => indeg.get(id) === 0);
  const order = [];
  while (q.length) {
    const id = q.shift();
    order.push(id);
    for (const nxt of out.get(id)) {
      indeg.set(nxt, indeg.get(nxt) - 1);
      if (indeg.get(nxt) === 0) q.push(nxt);
    }
  }
  if (order.length !== ids.length) throw new Error('dependency cycle detected');
  return order;
}

const DEFAULTS = { trials: 5000, seed: 12345, roundtrips: 3, flowEff: 0.15 };

// feature: { tasks: [{id, pod, size}], deps: [[fromId, toId]] }
// podStats: { [pod]: { mu, sigma, rho0 } }
// overlap: { [podA]: { [podB]: hours } }
// opts: { trials, seed, roundtrips, flowEff, rhoOverride: {pod: rho} }
export function simulateFeature(feature, podStats, overlap, opts = {}) {
  const o = { ...DEFAULTS, ...opts };
  const { tasks, deps } = feature;
  const byId = new Map(tasks.map((t) => [t.id, t]));
  const order = topoSort(tasks.map((t) => t.id), deps);
  const preds = new Map(tasks.map((t) => [t.id, []]));
  for (const [from, to] of deps) preds.get(to).push(from);

  const waitScale = {};
  for (const t of tasks) {
    const st = podStats[t.pod];
    if (!st) throw new Error(`no stats for pod ${t.pod}`);
    const rho = o.rhoOverride?.[t.pod];
    waitScale[t.pod] = rho == null ? 1 : kingmanScale(rho, st.rho0 ?? 0.8);
  }

  const run = (rng, accelPod) => {
    const finish = new Map();
    const taskDur = new Map();
    for (const id of order) {
      const t = byId.get(id);
      const st = podStats[t.pod];
      let dur = sampleLognormal(rng, st.mu, st.sigma) * (t.size ?? 1);
      // lead time = wait + touch; rescale the wait part by Kingman factor
      const wait = dur * (1 - o.flowEff) * waitScale[t.pod];
      dur = dur * o.flowEff + wait;
      if (accelPod === t.pod) dur *= 0.75;
      let start = 0;
      for (const p of preds.get(id)) {
        const pt = byId.get(p);
        const h = pt.pod === t.pod ? 0
          : handoffDays(overlap[pt.pod]?.[t.pod] ?? 0, o.roundtrips);
        start = Math.max(start, finish.get(p) + h);
      }
      taskDur.set(id, dur);
      finish.set(id, start + dur);
    }
    let makespan = 0, last = null;
    for (const [id, f] of finish) if (f > makespan) { makespan = f; last = id; }
    // walk back the critical path: predecessor whose finish (+handoff) == start
    const critical = new Set();
    let cur = last;
    while (cur != null) {
      critical.add(cur);
      const t = byId.get(cur);
      const start = finish.get(cur) - taskDur.get(cur);
      let next = null;
      for (const p of preds.get(cur)) {
        const pt = byId.get(p);
        const h = pt.pod === t.pod ? 0
          : handoffDays(overlap[pt.pod]?.[t.pod] ?? 0, o.roundtrips);
        if (Math.abs(finish.get(p) + h - start) < 1e-9) { next = p; break; }
      }
      cur = next;
    }
    return { makespan, critical, finish, taskDur };
  };

  const rng = makeRng(o.seed);
  const makespans = [];
  const critCount = new Map(tasks.map((t) => [t.id, 0]));
  let sample = null;
  for (let i = 0; i < o.trials; i++) {
    const r = run(rng);
    makespans.push(r.makespan);
    for (const id of r.critical) critCount.set(id, critCount.get(id) + 1);
    if (sample == null) sample = r;
  }
  makespans.sort((a, b) => a - b);
  const p85 = percentile(makespans, 85);
  // keep the trial closest to p85 as the representative Gantt
  {
    const rng2 = makeRng(o.seed);
    let best = Infinity;
    for (let i = 0; i < Math.min(o.trials, 500); i++) {
      const r = run(rng2);
      if (Math.abs(r.makespan - p85) < best) { best = Math.abs(r.makespan - p85); sample = r; }
    }
  }

  const baseMean = makespans.reduce((a, b) => a + b, 0) / makespans.length;
  const pods = [...new Set(tasks.map((t) => t.pod))];
  const sensitivity = pods.map((pod) => {
    const rng3 = makeRng(o.seed);
    let sum = 0;
    const n = Math.min(o.trials, 1000);
    for (let i = 0; i < n; i++) sum += run(rng3, pod).makespan;
    return { pod, savedDays: baseMean - sum / n };
  }).sort((a, b) => b.savedDays - a.savedDays);

  const criticality = {};
  const podCrit = {};
  for (const t of tasks) {
    criticality[t.id] = critCount.get(t.id) / o.trials;
    podCrit[t.pod] = Math.max(podCrit[t.pod] ?? 0, criticality[t.id]);
  }

  return {
    p50: percentile(makespans, 50),
    p85,
    p95: percentile(makespans, 95),
    mean: baseMean,
    makespans,
    criticality,
    podCriticality: podCrit,
    sensitivity,
    sampleTrial: {
      tasks: order.map((id) => ({
        id,
        pod: byId.get(id).pod,
        start: sample.finish.get(id) - sample.taskDur.get(id),
        end: sample.finish.get(id),
        critical: sample.critical.has(id),
      })),
      makespan: sample.makespan,
    },
  };
}

// Past-inference: historical pod-to-pod blocking edges that the feature's
// task graph does not declare. tasks: [{id, pod}], existingDeps: [[from,to]],
// edges: [{from, to, count}] mined from Jira. Returns suggestions with a
// representative task pair so the UI can offer one-click "add dependency".
export function suggestDeps(tasks, existingDeps, edges, minCount = 2) {
  const podsInFeature = new Map();
  for (const t of tasks) {
    if (!podsInFeature.has(t.pod)) podsInFeature.set(t.pod, t.id);
  }
  const byId = new Map(tasks.map((t) => [t.id, t]));
  const declaredPodPairs = new Set();
  for (const [from, to] of existingDeps) {
    const a = byId.get(from), b = byId.get(to);
    if (a && b) declaredPodPairs.add(`${a.pod}→${b.pod}`);
  }
  const out = [];
  for (const e of edges) {
    if (e.count < minCount) continue;
    if (e.from === e.to) continue;
    if (!podsInFeature.has(e.from) || !podsInFeature.has(e.to)) continue;
    if (declaredPodPairs.has(`${e.from}→${e.to}`)) continue;
    out.push({
      fromPod: e.from, toPod: e.to, count: e.count,
      fromTask: podsInFeature.get(e.from), toTask: podsInFeature.get(e.to),
    });
  }
  return out.sort((a, b) => b.count - a.count);
}

// Theory-of-Constraints helpers (Goldratt: identify -> exploit -> subordinate)

// The system constraint is where queue delay and downstream reach multiply:
// score = Kingman queue factor x (1 + distinct dependent pods).
export function constraintScores(stats, edges) {
  const dependents = {}; const demand = {};
  for (const e of edges) {
    (dependents[e.from] ??= new Set()).add(e.to);
    demand[e.from] = (demand[e.from] ?? 0) + e.count;
  }
  return Object.entries(stats).map(([pod, s]) => {
    const rho = Math.min(s.rho0 ?? 0.5, RHO_CAP);
    const queueFactor = rho / (1 - rho);
    const reach = dependents[pod]?.size ?? 0;
    return {
      pod, queueFactor, dependents: reach, demand: demand[pod] ?? 0,
      score: queueFactor * (1 + reach),
    };
  }).sort((a, b) => b.score - a.score);
}

// Little's law: time in system = WIP / throughput. Freezing WIP down to a
// cap shortens flow time proportionally without anyone working faster.
export function freezeProjection(wip, throughputPerWeek, targetWip) {
  const perDay = throughputPerWeek / 5;
  if (perDay <= 0) return { currentDays: null, projectedDays: null, frozen: Math.max(0, wip - targetWip) };
  return {
    currentDays: wip / perDay,
    projectedDays: Math.min(wip, targetWip) / perDay,
    frozen: Math.max(0, wip - targetWip),
  };
}

// CCPM fever point: buffer = p85 - p50; consumed = (elapsed - p50*pct)/buffer.
// Zone by burn ratio (buffer consumed vs progress made).
export function feverPoint(pctComplete, elapsedDays, p50, p85) {
  const buffer = Math.max(p85 - p50, 1e-9);
  const consumed = Math.max(0, (elapsedDays - p50 * pctComplete) / buffer);
  const ratio = consumed / Math.max(pctComplete, 0.05);
  const zone = ratio < 0.5 ? 'green' : ratio < 1.0 ? 'yellow' : 'red';
  return { consumed, ratio, zone };
}

// --- Org-design simulation: absorb one pod's scope into another -----------
// model: { pods: [{name, devCount, location, ...}], stats, edges, overlap,
//          residualTax } — treated as immutable; every operation returns a copy.

// transferFraction: share of the absorbed pod's headcount that moves with the
// scope (1 = whole team, 0 = scope only). Scope (WIP, demand) always moves
// fully; capacity moves partially — so low fractions heat the merged queue.
export function mergePods(model, absorber, absorbed, transferFraction = 1) {
  const a = model.pods.find((p) => p.name === absorber);
  const b = model.pods.find((p) => p.name === absorbed);
  if (!a || !b) throw new Error('unknown pod in merge');
  const name = `${absorber}+${absorbed}`;
  const sa = model.stats[absorber], sb = model.stats[absorbed];

  const wa = sa.resolved180 || 1, wb = sb.resolved180 || 1;
  const mu = (sa.mu * wa + sb.mu * wb) / (wa + wb);
  const sigma = (sa.sigma * wa + sb.sigma * wb) / (wa + wb);
  const devCount = a.devCount + Math.round(b.devCount * transferFraction);
  const wip = sa.wip + sb.wip;
  const merged = {
    ...a,
    name,
    devCount,
    location: a.location === b.location ? a.location : `${a.location} / ${b.location}`,
    area: [a.area, b.area].filter(Boolean).join(' + '),
    simulated: true,
  };
  const stats = { ...model.stats };
  delete stats[absorber]; delete stats[absorbed];
  stats[name] = {
    ...sa,
    mu, sigma, wip,
    throughputWk: (sa.throughputWk || 0) + (sb.throughputWk || 0) * transferFraction,
    resolved180: (sa.resolved180 || 0) + (sb.resolved180 || 0),
    p50: Math.exp(mu),
    p85: Math.exp(mu + 1.0364 * sigma),
    rho0: Math.max(0.3, Math.min(0.92, wip / Math.max(1, devCount * 2))),
    synthetic: sa.synthetic && sb.synthetic,
  };

  // cross-site merge: the people did not move, so internalized links keep
  // half their handoff cost as a residual
  const abOverlap = model.overlap[absorber]?.[absorbed] ?? 0;
  let residualTax = model.residualTax ?? 0;
  const rename = (n) => (n === absorber || n === absorbed ? name : n);
  const folded = new Map();
  for (const e of model.edges) {
    const from = rename(e.from), to = rename(e.to);
    if (from === to) {
      if (abOverlap < 2) residualTax += e.count * handoffDays(abOverlap, 3) * 0.5;
      continue;
    }
    const k = `${from}→${to}`;
    folded.set(k, (folded.get(k) ?? 0) + e.count);
  }
  const edges = [...folded.entries()].map(([k, count]) => {
    const [from, to] = k.split('→');
    return { from, to, count };
  }).sort((x, y) => y.count - x.count);

  // merged team can talk to X whenever either original site could
  const overlap = {};
  const others = model.pods.filter((p) => p !== a && p !== b).map((p) => p.name);
  for (const o of others) {
    overlap[o] = { ...model.overlap[o] };
    const best = Math.max(model.overlap[o]?.[absorber] ?? 0, model.overlap[o]?.[absorbed] ?? 0);
    overlap[o][name] = best;
    delete overlap[o][absorber]; delete overlap[o][absorbed];
  }
  overlap[name] = { [name]: 8 };
  for (const o of others) {
    overlap[name][o] = Math.max(model.overlap[absorber]?.[o] ?? 0, model.overlap[absorbed]?.[o] ?? 0);
  }

  return {
    pods: [...model.pods.filter((p) => p !== a && p !== b), merged],
    stats, edges, overlap, residualTax,
  };
}

// Org Flow Score in day-units; lower is better.
// coordTax = handoff days implied by all cross-pod blocking links
// queueTax = links weighted by the blocking pod's Little's-law wait
//            (WIP / daily throughput) — unclamped, so capacity changes
//            always register
export function orgFlowScore(model, roundtrips = 3) {
  let coordTax = 0, queueTax = 0;
  for (const e of model.edges) {
    coordTax += e.count * handoffDays(model.overlap[e.from]?.[e.to] ?? 0, roundtrips);
    const s = model.stats[e.from] ?? {};
    // capped at the mining window: waits beyond it are data artifacts
    // (pods that deliver outside the tracked project), not real queues
    const waitDays = Math.min(s.throughputWk > 0 ? s.wip / (s.throughputWk / 5) : 30, 180);
    queueTax += e.count * waitDays;
  }
  coordTax += model.residualTax ?? 0;
  return { coordTax, queueTax, total: coordTax + queueTax, edgeCount: model.edges.length };
}

// Greedy search for headcount-neutral consolidations (no hiring): each round,
// apply the coupling-internalizing merge with the best score delta.
export function suggestMerges(model, { maxDevs = 12, rounds = 3, transferFraction = 1 } = {}) {
  const base0 = orgFlowScore(model);
  // same 50/50 blended index the UI shows, in points (baseline = 100)
  const idx = (s) => 50 * (s.coordTax / Math.max(base0.coordTax, 1e-9))
    + 50 * (s.queueTax / Math.max(base0.queueTax, 1e-9));
  const moves = [];
  let cur = model;
  for (let r = 0; r < rounds; r++) {
    const coupling = new Map();
    for (const e of cur.edges) {
      const k = [e.from, e.to].sort().join('|');
      coupling.set(k, (coupling.get(k) ?? 0) + e.count);
    }
    const before = idx(orgFlowScore(cur));
    let best = null;
    for (const [k, c] of coupling) {
      if (c < 2) continue;
      const [x, y] = k.split('|');
      const px = cur.pods.find((p) => p.name === x), py = cur.pods.find((p) => p.name === y);
      if (!px || !py) continue;
      // bigger team absorbs smaller (less disruption)
      const [abs, abd] = px.devCount >= py.devCount ? [x, y] : [y, x];
      let next;
      try { next = mergePods(cur, abs, abd, transferFraction); } catch { continue; }
      const mergedDevs = next.pods.find((p) => p.name === `${abs}+${abd}`).devCount;
      if (mergedDevs > maxDevs) continue;
      const delta = before - idx(orgFlowScore(next));
      if (delta > 0 && (!best || delta > best.delta)) {
        best = { absorber: abs, absorbed: abd, delta, mergedDevs, coupling: c, model: next };
      }
    }
    if (!best) break;
    cur = best.model;
    const { model: _m, ...move } = best;
    moves.push(move);
  }
  return moves;
}

// Full-kit check (Rules of Flow): is everything needed to FINISH this epic
// present before it starts? Machine-checkable half; the human half (business
// case, contracts, defrost criteria) lives in the epic description template.
// epic: { epic, tasks: [{key, pod, points, status, blockedBy, blocks}] }
// ctx: { stats, pods, srePods }
export function fullKitCheck(epic, ctx) {
  const tasks = epic.tasks;
  const items = [];
  const add = (id, label, status, detail) => items.push({ id, label, status, detail });

  const noPod = tasks.filter((t) => !(t.pod || '').trim());
  add('pods-assigned', 'Every task has an Assigned Pod',
    noPod.length ? 'fail' : 'pass',
    noPod.length ? `${noPod.length} unassigned: ${noPod.slice(0, 4).map((t) => t.key).join(', ')}` : `${tasks.length} tasks`);

  const unsized = tasks.filter((t) => t.points == null);
  add('sized', 'Tasks are estimated',
    unsized.length / Math.max(tasks.length, 1) > 0.2 ? 'fail' : unsized.length ? 'warn' : 'pass',
    unsized.length ? `${unsized.length} of ${tasks.length} unsized` : 'all sized');

  add('outcome', 'Business outcome written in the epic',
    epic.hasOutcome === true ? 'pass' : epic.hasOutcome === false ? 'fail' : 'warn',
    epic.hasOutcome === true ? 'description states the why'
      : epic.hasOutcome === false ? 'description empty or no outcome/CoD/metric — whose problem does this solve?'
        : 'epic description not synced — unknown');

  const keys = new Set(tasks.map((t) => t.key));
  const external = tasks.flatMap((t) => (t.blockedBy ?? []).filter((k) => !keys.has(k)));
  add('blockers', 'No unresolved upstream blockers',
    external.length ? 'warn' : 'pass',
    external.length ? `${external.length} blocker(s) outside this epic — verify resolved: ${external.slice(0, 4).join(', ')}` : 'none declared');

  const srePods = new Set(ctx.srePods ?? []);
  const hasSre = tasks.some((t) => srePods.has((t.pod || '').trim()));
  add('sre', 'Production-readiness (SRE) task included',
    hasSre ? 'pass' : 'fail',
    hasSre ? 'yes' : 'no SRE pod task — ops work will surface as unplanned later');

  const involved = [...new Set(tasks.map((t) => (t.pod || '').trim()).filter(Boolean))];
  const saturated = involved.filter((p) => {
    const s = ctx.stats[p];
    const pod = ctx.pods.find((x) => x.name === p);
    return s && pod && s.wip >= (pod.streams ?? pod.devCount) * 2;
  });
  add('headroom', 'Involved pods have queue headroom (drum slot)',
    saturated.length ? 'fail' : 'pass',
    saturated.length ? `saturated: ${saturated.join(', ')} — starting now means queueing, not working` : 'all have capacity');

  const declaredDeps = tasks.some((t) => (t.blockedBy ?? []).some((k) => keys.has(k)));
  add('deps-declared', 'Cross-pod sequencing declared',
    involved.length > 1 && !declaredDeps ? 'warn' : 'pass',
    involved.length > 1 && !declaredDeps
      ? `${involved.length} pods but zero internal block links — order is implicit in someone's head`
      : 'ok');

  const weight = { pass: 1, warn: 0.5, fail: 0 };
  const score = items.reduce((s, i) => s + weight[i.status], 0) / items.length;
  return { items, score };
}

// Teams size differently (complexity, days, not at all). Points are only
// meaningful relative to the SAME pod's median, so the size factor is
// points / pod-median, clamped — and missing data falls back to 1x (median).
export function relativeSize(points, podMedianPoints) {
  if (points == null || !podMedianPoints) return 1;
  return Math.max(0.25, Math.min(4, points / podMedianPoints));
}

// Pairing halves a pod's independent work streams (pairs, not people, pull
// items). Healthy WIP and queue-heat proxies should use streams, not devs.
export function workStreams(devCount, pairing) {
  return pairing ? Math.max(1, devCount / 2) : devCount;
}
