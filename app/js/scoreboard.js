import { heatColor } from './graph.js';

export function initScoreboard(state) {
  // inbound demand: who is waiting on this pod's work?
  const dependents = {}; const demand = {}; const upstreams = {};
  for (const e of state.edges) {
    (dependents[e.from] ??= new Set()).add(e.to);
    demand[e.from] = (demand[e.from] ?? 0) + e.count;
    (upstreams[e.to] ??= new Set()).add(e.from);
  }
  const depCount = (p) => dependents[p.name]?.size ?? 0;

  const cols = [
    ['Pod', (p, s) => p.name,
      "The team. Conway's law (the tool's namesake): an organization ships its communication structure — each pod is a node in that structure, and the edges between them are where delay lives."],
    ['Site', (p) => p.location.replace('*REMOTE - multicontinental*', 'Remote'),
      'Home location / timezone. Cross-site dependencies with little working-hour overlap are the slowest, costliest seams — the Unicorn Project\'s First Ideal (Locality & Simplicity) is about removing exactly these.'],
    ['Devs', (p) => p.devCount || '—',
      'Headcount from the uploaded roster (or ≈ distinct Jira assignees when no roster was given — a rough, low estimate for pairing teams). Pairing pods run ~devs ÷ 2 work-streams since a pair pulls one item together; capacity/Load uses work-streams, not headcount.'],
    ['Streams', (p) => p.streams,
      'Parallel work-streams = the real capacity. Pairing teams ≈ number of pairs; can be set explicitly in the roster (Work-streams column). Load = WIP ÷ (streams × 2).'],
    ['WIP', (p) => {
      const sp = state.wipSplit?.[p.name];
      if (!sp) return state.stats[p.name].wip;
      return `${sp.active + sp.waiting} <span class="hint">${sp.active}a/${sp.waiting}w</span>`;
    },
      'Work in progress — started-but-not-done items (Jira "In Progress" category; backlog/"To Do" is NOT counted). Split into active (In Progress, Testing) vs waiting (In Review, On Hold, Blocked — started work sitting in a queue). The Goal & Rules of Flow: WIP doesn\'t mean output; a high waiting share means a review/handoff bottleneck, not busy developers.'],
    ['Thru/wk', (p, s) => s.throughputWk.toFixed(1),
      'Throughput = items resolved in 180d ÷ 26 weeks. The Goal: throughput (value actually finished) is the real measure of a system\'s output — not how busy everyone looks.'],
    ['Cycle P50', (p, s) => `${s.p50.toFixed(1)}d`,
      'Median lead time, created→resolved (epics and >180d items removed, winsorized at the pod\'s 95th percentile). Half of this pod\'s work finishes within this many days.'],
    ['Cycle P85', (p, s) => `${s.p85.toFixed(1)}d`,
      '85th-percentile lead time — the date you can promise with confidence. Rules of Flow / probabilistic forecasting: commit the P85, not the optimistic P50, or you break promises half the time.'],
    ['Load ρ', (p, s) => (s.load ?? s.rho0),
      'Load = WIP ÷ (work-streams × 2) — the REAL ratio (we show it uncapped, so >1 means over capacity; the bar saturates at a full plate). The Goal & queueing theory: a system near or past full utilisation is not fast — it is where delay explodes.'],
    ['Kingman ×', (p, s) => {
      const load = s.load ?? s.rho0;
      return load >= 1 ? '∞' : `${(load / (1 - load)).toFixed(1)}×`;
    },
      'Wait-time multiplier ρ/(1−ρ) — but it only exists while load < 1 (a stable queue): at ρ=0.5 work waits ~1× its touch time, at 0.9 ~9×. At load ≥ 1 the queue is UNSTABLE — the backlog grows without bound, so there is no finite multiplier (shown as ∞). The Goal: a pod past 100% load never catches up until you cut its WIP below 1.'],
    ['Dependents', (p) => (depCount(p) ? `${depCount(p)} pods (×${demand[p.name]})` : '—'),
      'Distinct pods waiting on THIS pod\'s work (× total blocking links, last 12 months). The Goal: the pod everyone queues behind is your system constraint — improve it and the whole system speeds up.'],
    ['Upstreams', (p) => (upstreams[p.name]?.size ?? 0) || '—',
      'Pods THIS one waits on. High = at the mercy of others (e.g. UI pods blocked on operator pods). A prime candidate for an interface investment or ownership change (Unicorn: Locality).'],
    ['Flags', flags,
      'Auto-flags: dependency hub (≥5 dependents), hub under load (≥3 deps & ρ≥0.75 — the most dangerous cell), queue hot (ρ≥0.85), high variance (cycle-time σ>1.2), and zero-overlap deps (cross-site with no working-hour overlap).'],
  ];

  function flags(p, s, state2) {
    const out = [];
    const load = s.load ?? s.rho0;
    if (depCount(p) >= 5) out.push('<span class="flag red">dependency hub</span>');
    if (depCount(p) >= 3 && load >= 0.9) out.push('<span class="flag red">hub under load</span>');
    if (load >= 1) out.push('<span class="flag red">over capacity</span>');
    if (s.sigma > 1.2 && !s.synthetic) out.push('<span class="flag amber">high variance</span>');
    if (s.synthetic) out.push('<span class="flag amber">no data</span>');
    const blockedBy = state2.edges.filter((e) => e.to === p.name);
    const zero = blockedBy.filter((e) => (state2.overlap[p.name]?.[e.from] ?? 0) <= 0);
    if (zero.length) out.push(`<span class="flag red">${zero.length} zero-overlap deps</span>`);
    return out.join(' ');
  }

  let sortKey = 7; let sortDir = -1;
  const table = document.getElementById('score-table');

  function renderWipSummary() {
    const el = document.getElementById('wip-summary');
    if (!el || !state.wipSplit) return;
    let active = 0; let waiting = 0;
    for (const v of Object.values(state.wipSplit)) { active += v.active; waiting += v.waiting; }
    const total = active + waiting;
    if (!total) { el.innerHTML = ''; return; }
    const pctWait = Math.round((100 * waiting) / total);
    el.innerHTML = `Org WIP: <b>${total}</b> started items — <b>${active}</b> active vs
      <b style="color:${pctWait > 35 ? 'var(--amber)' : 'var(--text)'}">${waiting}</b> waiting
      (<b>${pctWait}%</b> of WIP is sitting in review/hold/blocked queues, not being worked).
      <span class="hint">High waiting share = a handoff/review bottleneck, not idle developers.</span>`;
  }

  function render() {
    renderWipSummary();
    const rows = state.pods.map((p) => ({ p, s: state.stats[p.name] }));
    rows.sort((a, b) => {
      const va = cols[sortKey][1](a.p, a.s, state);
      const vb = cols[sortKey][1](b.p, b.s, state);
      const na = parseFloat(va); const nb = parseFloat(vb);
      if (!Number.isNaN(na) || !Number.isNaN(nb)) {
        const xa = Number.isNaN(na) ? -Infinity : na;
        const xb = Number.isNaN(nb) ? -Infinity : nb;
        return (xa - xb) * sortDir;
      }
      return String(va).localeCompare(String(vb)) * sortDir;
    });
    const help = (t) => (t ? ` <span class="help" data-tip="${t.replace(/"/g, '&quot;')}">?</span>` : '');
    table.innerHTML = `<thead><tr>${cols.map(([h, , tip], i) =>
      `<th data-i="${i}">${h}${i === sortKey ? (sortDir > 0 ? ' ▲' : ' ▼') : ''}${help(tip)}</th>`).join('')}</tr></thead>` +
      `<tbody>${rows.map(({ p, s }) => `<tr>${cols.map(([h, fn], i) => {
        if (h === 'Load ρ') {
          const load = s.load ?? s.rho0;
          return `<td><span class="bar" style="width:${s.rho0 * 60}px;background:${heatColor(s.rho0)}"></span> ${load.toFixed(2)}</td>`;
        }
        return `<td>${fn(p, s, state)}</td>`;
      }).join('')}</tr>`).join('')}</tbody>`;
    table.querySelectorAll('th').forEach((th) => th.addEventListener('click', () => {
      const i = +th.dataset.i;
      if (i === sortKey) sortDir *= -1; else { sortKey = i; sortDir = -1; }
      render();
    }));
  }
  render();
}
