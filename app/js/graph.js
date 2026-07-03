import { constraintScores, mergePods, orgFlowScore, suggestMerges } from './sim.js';
import { listSnapshots, getSnapshot, snapshotDataJson, apiGet, getJiraBaseUrl } from './data.js';
import {
  heatColor, layerDag, layoutColumns, bezierEdgePath, appendArrowMarker,
  enablePanZoom, enableNodeDrag, makeSpotlight,
} from './netgraph.js';

// re-exported: other views (gamenet.js, hygiene.js, scoreboard.js, simulator.js)
// import heatColor/layerDag from here — canonical definitions now live in
// netgraph.js (shared with Plan's dependency diagram), this just forwards them.
export { heatColor, layerDag };

const MUTED = '#2e3f51';
const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
// jiraLink() falls back to plain (unlinked) text when CONWAY_JIRA_BASE_URL is unset.
const jiraLink = (base, key) => (base ? `<a href="${base}/browse/${key}" target="_blank">${key}</a>` : key);

export function initGraph(state) {
  const baseModel = {
    pods: state.pods, stats: state.stats, edges: state.edges,
    overlap: state.overlap, residualTax: 0,
  };
  let simModel = null;
  let appliedMoves = [];
  let lastFraction = 1;
  let diff = null; // compare-mode diff vs another snapshot (null = off)
  const hidden = new Set(); // session-only: pods temporarily hidden from the view
  const model = () => simModel ?? baseModel;

  const controls = document.getElementById('net-controls');
  controls.innerHTML = `
    <label>min links <input id="net-min" type="range" min="1" max="6" step="1" value="1">
    <b id="net-min-v">1</b></label>
    <label><input id="net-iso" type="checkbox" checked> hide pods with no visible links</label>
    <span class="hint">flow runs left→right. Click a pod to spotlight its cone; background click resets.</span>`;
  controls.querySelector('#net-min').addEventListener('input', () => {
    controls.querySelector('#net-min-v').textContent = controls.querySelector('#net-min').value;
    render();
  });
  controls.querySelector('#net-iso').addEventListener('change', render);

  // ---- compare mode: diff this snapshot against another (now vs then) ----
  const cmpWrap = document.createElement('div');
  cmpWrap.className = 'net-compare';
  cmpWrap.hidden = true;
  controls.after(cmpWrap);
  mountCompare();

  // ---- session-only node hiding (focus on a subgraph; Reset restores all) ----
  const hideBar = document.createElement('div');
  hideBar.className = 'net-compare';
  hideBar.hidden = true;
  cmpWrap.after(hideBar);
  function hide(name) { hidden.add(name); render(); }
  function renderHideBar() {
    if (!hidden.size) { hideBar.hidden = true; hideBar.innerHTML = ''; return; }
    hideBar.hidden = false;
    hideBar.innerHTML = `<b>Hidden:</b> ${[...hidden].map((n) => `<span class="flag">${n}</span>`).join(' ')}
      <button id="net-show-all">Reset (show all)</button>`;
    hideBar.querySelector('#net-show-all').addEventListener('click', () => { hidden.clear(); render(); });
  }

  async function mountCompare() {
    const snaps = await listSnapshots();
    const others = snaps.filter((s) => s.id !== getSnapshot());
    if (!others.length) return; // nothing to compare against
    const fmt = (s) => (s.id === 'baseline' ? s.name : (s.name || s.id));
    const label = document.createElement('label');
    label.innerHTML = `compare to <select id="net-cmp"><option value="">— off —</option>`
      + others.map((s) => `<option value="${s.id}">${fmt(s)}</option>`).join('') + '</select>';
    controls.appendChild(label);
    controls.querySelector('#net-cmp').addEventListener('change', (e) => {
      loadCompare(e.target.value, e.target.selectedOptions[0]?.textContent || e.target.value);
    });
  }

  const ekey = (e) => `${e.from}→${e.to}`;

  async function loadCompare(bId, bName) {
    if (!bId) { diff = null; renderCompareSummary(); render(); return; }
    const [bp, bs, be] = await Promise.all([
      snapshotDataJson(bId, 'pods.json'),
      snapshotDataJson(bId, 'pod_stats.json'),
      snapshotDataJson(bId, 'edges.json'),
    ]);
    if (!bs || !be) { diff = null; renderCompareSummary('Could not load that snapshot.'); render(); return; }
    const aEdges = baseModel.edges;
    const aPods = new Set(baseModel.pods.map((p) => p.name));
    const aEdgeKeys = new Set(aEdges.map(ekey));
    const bEdgeKeys = new Set((be || []).map(ekey));
    const addedEdges = new Set(aEdges.filter((e) => !bEdgeKeys.has(ekey(e))).map(ekey));
    const removedAll = (be || []).filter((e) => !aEdgeKeys.has(ekey(e)));
    const removedEdges = removedAll.filter((e) => aPods.has(e.from) && aPods.has(e.to));
    const podDelta = new Map();
    for (const p of baseModel.pods) {
      const b = bs[p.name];
      if (b) podDelta.set(p.name, (baseModel.stats[p.name].wip || 0) - (b.wip_count || 0));
    }
    const bPods = new Set(((bp && bp.pods) || []).map((p) => p.name));
    const added = new Set([...aPods].filter((n) => !bPods.has(n)));
    const removed = new Set([...bPods].filter((n) => !aPods.has(n)));
    diff = {
      bName, addedEdges, removedEdges, podDelta, added, removed,
      totals: { addedE: addedEdges.size, removedE: removedAll.length },
    };
    renderCompareSummary();
    render();
  }

  function renderCompareSummary(msg) {
    if (msg) { cmpWrap.hidden = false; cmpWrap.innerHTML = `<span class="hint">${msg}</span>`; return; }
    if (!diff) { cmpWrap.hidden = true; cmpWrap.innerHTML = ''; return; }
    cmpWrap.hidden = false;
    const movers = [...diff.podDelta.entries()].filter(([, d]) => d !== 0)
      .sort((a, b) => Math.abs(b[1]) - Math.abs(a[1])).slice(0, 6)
      .map(([n, d]) => `${n} ${d > 0 ? '+' : ''}${d}`).join(' · ');
    cmpWrap.innerHTML = `<b>vs ${diff.bName}:</b>
      <span class="flag" style="color:#3ecf8e">+${diff.totals.addedE} edges</span>
      <span class="flag red">−${diff.totals.removedE} edges</span>
      ${diff.added.size ? `<span class="flag" style="color:#3ecf8e">${diff.added.size} pods new</span>` : ''}
      ${diff.removed.size ? `<span class="flag red">${diff.removed.size} pods gone</span>` : ''}
      ${movers ? `<span class="hint">ΔWIP: ${movers}</span>` : ''}`;
  }

  // ---- org simulation panel ----
  const sim = document.getElementById('org-sim');
  function renderSimPanel(suggestions = null) {
    const pods = [...model().pods].sort((a, b) => a.name.localeCompare(b.name));
    const opts = (sel) => pods.map((p) => `<option ${p.name === sel ? 'selected' : ''}>${p.name}</option>`).join('');
    const base = orgFlowScore(baseModel);
    const cur = orgFlowScore(model());
    // 50/50 blend so neither tax can drown the other
    const idx = 50 * (cur.coordTax / Math.max(base.coordTax, 1e-9))
              + 50 * (cur.queueTax / Math.max(base.queueTax, 1e-9));
    const active = !!simModel;
    sim.innerHTML = `
      <div class="sim-bar ${active ? 'sim-active' : ''}">
        <span class="sim-title">${active ? '⚠ SIMULATION — org changes are hypothetical' : 'Org simulation'}
          <span class="help" data-tip="Absorb one pod's entire scope into another (headcount-neutral — no hiring). The absorbed pod's dependencies re-point to the absorber; coupling between the two becomes internal and stops costing handoffs — unless their sites barely overlap, in which case half the cost remains (people did not move). Watch the Org Flow Index: baseline = 100, lower is better.">?</span></span>
        <span class="sim-score">Org Flow Index <b class="${idx < 99.5 ? 'good' : ''}">${idx.toFixed(1)}</b>
          <span class="hint">(coord ${cur.coordTax.toFixed(0)}d + queue ${cur.queueTax.toFixed(0)}d · ${cur.edgeCount} edges${active ? ` · baseline 100 = coord ${base.coordTax.toFixed(0)}d + queue ${base.queueTax.toFixed(0)}d` : ''})</span></span>
        <span class="sim-controls">
          <select id="sim-absorber">${opts()}</select> absorbs
          <select id="sim-absorbed">${opts(pods[1]?.name)}</select> with
          <select id="sim-fraction" title="share of the absorbed pod's headcount that moves with the scope">
            ${[100, 75, 50, 25, 0].map((f) => `<option value="${f / 100}" ${f === lastFraction * 100 ? 'selected' : ''}>${f}% of people</option>`).join('')}
          </select>
          <button id="sim-apply">Apply</button>
          <button id="sim-suggest">Suggest moves</button>
          <button id="sim-reset" ${active ? '' : 'disabled'}>Reset</button>
        </span>
      </div>
      ${appliedMoves.length ? `<div class="sim-chips">${appliedMoves
        .map((m) => `<span class="chip">${m.absorber} ⊕ ${m.absorbed} <i>(${Math.round(m.fraction * 100)}% ppl)</i></span>`).join('')}
        <span class="hint">applied in order — Reset reverts everything</span></div>` : ''}
      <div id="sim-suggestions">${suggestions === null ? '' : suggestions.length === 0
        ? '<p class="hint">No headcount-neutral merge improves the score under the 12-dev team cap.</p>'
        : suggestions.map((s, i) => `
          <div class="suggestion">
            <span><b>${s.absorber}</b> absorbs <b>${s.absorbed}</b>
              <span class="hint">(coupling ×${s.coupling}, merged team ${s.mergedDevs} devs)</span>
              → index −${s.delta.toFixed(1)} pts</span>
            <button data-i="${i}">apply</button>
          </div>`).join('')}</div>`;

    sim.querySelector('#sim-apply').addEventListener('click', () => {
      const a = sim.querySelector('#sim-absorber').value;
      const b = sim.querySelector('#sim-absorbed').value;
      lastFraction = +sim.querySelector('#sim-fraction').value;
      if (a === b) return;
      applyMove(a, b, lastFraction);
    });
    sim.querySelector('#sim-suggest').addEventListener('click', () => {
      lastFraction = +sim.querySelector('#sim-fraction').value;
      const moves = suggestMerges(model(), { maxDevs: 12, rounds: 4, transferFraction: lastFraction });
      renderSimPanel(moves);
    });
    sim.querySelector('#sim-reset').addEventListener('click', () => {
      simModel = null; appliedMoves = [];
      renderSimPanel(); render();
      document.getElementById('netpanel').innerHTML = '<p class="hint">Simulation reset — showing real org.</p>';
    });
    if (suggestions) {
      sim.querySelectorAll('#sim-suggestions button').forEach((b) => b.addEventListener('click', () => {
        const s = suggestions[+b.dataset.i];
        applyMove(s.absorber, s.absorbed, lastFraction);
      }));
    }
  }

  function applyMove(absorber, absorbed, fraction = 1) {
    try {
      simModel = mergePods(model(), absorber, absorbed, fraction);
      appliedMoves.push({ absorber, absorbed, fraction });
      renderSimPanel(); render();
    } catch (e) {
      sim.querySelector('.sim-bar').insertAdjacentHTML('beforeend', `<span class="flag red">${e.message}</span>`);
    }
  }

  function render() {
    const m = model();
    const minCount = +controls.querySelector('#net-min').value;
    const hideIso = controls.querySelector('#net-iso').checked;
    const svg = d3.select('#netsvg');
    svg.selectAll('*').remove();
    const rect = svg.node().getBoundingClientRect();
    const width = rect.width > 50 ? rect.width : 1100;
    const height = rect.height > 50 ? rect.height : 700;

    if (!m.pods.length) { // first run / empty snapshot
      svg.append('text').attr('x', width / 2).attr('y', height / 2).attr('text-anchor', 'middle')
        .attr('fill', '#8fa6bd').attr('font-size', 14)
        .text('No org snapshot yet — managers can Import from Jira (Observe ▾) to build one.');
      return;
    }

    const ranked = constraintScores(m.stats, m.edges);
    const constraintRank = new Map(ranked.slice(0, 3)
      .filter((r) => m.pods.some((p) => p.name === r.pod))
      .map((r, i) => [r.pod, i]));

    renderHideBar();
    // hidden pods drop out entirely, taking their in/out edges with them (edges
    // are filtered to surviving nodes below).
    const edges = m.edges.filter((e) => e.count >= minCount && !hidden.has(e.from) && !hidden.has(e.to));
    const linked = new Set(edges.flatMap((e) => [e.from, e.to]));
    const pods = m.pods.filter((p) => !hidden.has(p.name) && (p.simulated || !hideIso || linked.has(p.name)));
    const names = pods.map((p) => p.name);
    const visEdges = edges.filter((e) => names.includes(e.from) && names.includes(e.to));
    const pos = layoutColumns(names, visEdges, width, height);

    appendArrowMarker(svg);

    const g = svg.append('g');
    // pan/zoom via scroll + drag-background; disable dblclick-to-zoom so a
    // double-click doesn't surprise-zoom when someone's trying to select.
    enablePanZoom(svg, g);
    svg.on('click', () => spotlight(null));

    const r = (p) => 8 + Math.sqrt(m.stats[p.name].wip || 1) * 2.0;
    const wMax = d3.max(visEdges, (e) => e.count) ?? 1;
    const podByName = new Map(pods.map((p) => [p.name, p]));

    // node coupling: distinct teams a pod depends on (fan-in) vs. teams that depend on it (fan-out).
    // computed over the full model, not the min-links view, so the score is stable.
    const dependsOn = new Map();   // pod -> Set of teams it waits on (it is blocked by them)
    const dependents = new Map();  // pod -> Set of teams that wait on it (it blocks them)
    for (const e of m.edges) {
      (dependents.get(e.from) ?? dependents.set(e.from, new Set()).get(e.from)).add(e.to);
      (dependsOn.get(e.to) ?? dependsOn.set(e.to, new Set()).get(e.to)).add(e.from);
    }
    const fanIn = (name) => dependsOn.get(name)?.size ?? 0;
    const fanOut = (name) => dependents.get(name)?.size ?? 0;

    const edgePath = bezierEdgePath(pos, (name) => r(podByName.get(name)), visEdges);
    const linkSel = g.append('g').selectAll('path').data(visEdges).join('path')
      .attr('d', edgePath)
      .attr('fill', 'none')
      .attr('stroke', (e) => (diff && diff.addedEdges.has(`${e.from}→${e.to}`) ? '#3ecf8e' : '#56708a'))
      .attr('stroke-opacity', 0.4)
      .attr('stroke-width', (e) => 1 + 4.5 * (e.count / wMax))
      .attr('marker-end', 'url(#arrow)');

    // invisible wide-stroke overlay so a double-click lands even on thin edges
    // (the visible stroke can be as narrow as 1px, too thin to reliably hit)
    g.append('g').selectAll('path').data(visEdges).join('path')
      .attr('d', edgePath).attr('fill', 'none').attr('stroke', 'transparent')
      .attr('stroke-width', 14).style('cursor', 'pointer')
      .on('dblclick', (ev, e) => { ev.stopPropagation(); showEdgeIssuesModal(e); });

    // compare: dropped edges (present then, gone now) as red dashed ghosts
    if (diff) {
      const ghosts = diff.removedEdges.filter((e) => pos.get(e.from) && pos.get(e.to) && podByName.get(e.to));
      g.append('g').selectAll('path').data(ghosts).join('path')
        .attr('d', edgePath).attr('fill', 'none')
        .attr('stroke', '#f4655f').attr('stroke-opacity', 0.55)
        .attr('stroke-width', 1.5).attr('stroke-dasharray', '5,4');
    }

    const nodeSel = g.append('g').selectAll('g').data(pods).join('g')
      .attr('transform', (p) => `translate(${pos.get(p.name).x},${pos.get(p.name).y})`);
    const drag = enableNodeDrag(nodeSel, pos, () => linkSel.attr('d', edgePath));
    drag.onClick((ev, p) => {
      ev?.stopPropagation(); spotlight(p.name); showPanel(p, m, !!simModel, diff, hide);
    });

    nodeSel.append('circle')
      .attr('r', (p) => r(p))
      .attr('fill', (p) => (p.simulated ? '#3d2f57' : MUTED))
      .attr('stroke', (p) => heatColor(m.stats[p.name].rho0))
      .attr('stroke-width', 2.5);

    nodeSel.filter((p) => constraintRank.has(p.name)).append('circle')
      .attr('r', (p) => r(p) + 5)
      .attr('fill', 'none')
      .attr('stroke', (p) => (constraintRank.get(p.name) === 0 ? '#f4655f' : '#f5b94c'))
      .attr('stroke-width', 2).attr('stroke-dasharray', '5,3');

    nodeSel.filter((p) => p.simulated).append('circle')
      .attr('r', (p) => r(p) + 9)
      .attr('fill', 'none').attr('stroke', '#b58cff')
      .attr('stroke-width', 1.5).attr('stroke-dasharray', '2,4');

    // compare: new pods get a green dashed ring; WIP movers a colored halo
    // (red = busier now, green = lighter now)
    if (diff) {
      nodeSel.filter((p) => diff.added.has(p.name)).append('circle')
        .attr('r', (p) => r(p) + 9).attr('fill', 'none').attr('stroke', '#3ecf8e')
        .attr('stroke-width', 1.5).attr('stroke-dasharray', '2,3');
      nodeSel.filter((p) => diff.podDelta.get(p.name)).append('circle')
        .attr('r', (p) => r(p) + 13).attr('fill', 'none')
        .attr('stroke', (p) => (diff.podDelta.get(p.name) > 0 ? '#f4655f' : '#3ecf8e'))
        .attr('stroke-width', 2).attr('stroke-opacity', 0.7);
    }

    nodeSel.append('text').text((p) => p.name)
      .attr('dy', (p) => -r(p) - 8).attr('text-anchor', 'middle')
      .attr('fill', (p) => (p.simulated ? '#d8c7ff' : '#cfdcea')).attr('font-size', 11.5);
    nodeSel.append('text').text((p) => m.stats[p.name].wip ?? 0)
      .attr('dy', 3.5).attr('text-anchor', 'middle').attr('pointer-events', 'none')
      .attr('fill', '#eaf2fb').attr('font-size', 10).attr('font-weight', 700);
    nodeSel.filter((p) => constraintRank.get(p.name) === 0).append('text')
      .text('CONSTRAINT')
      .attr('dy', (p) => r(p) + 17).attr('text-anchor', 'middle')
      .attr('fill', '#f4655f').attr('font-size', 9).attr('font-weight', 700)
      .attr('letter-spacing', 1.5);

    nodeSel.append('text')
      .attr('dy', (p) => r(p) + (constraintRank.get(p.name) === 0 ? 31 : 18))
      .attr('text-anchor', 'middle').attr('font-size', 9.5).attr('fill', '#8fa6bd')
      .attr('pointer-events', 'none')
      .text((p) => `⬇${fanIn(p.name)} ⬆${fanOut(p.name)}`)
      .append('title')
      .text((p) => `${p.name} depends on ${fanIn(p.name)} team(s) (fan-in / blocked by) · `
        + `${fanOut(p.name)} team(s) depend on it (fan-out / it blocks)`);

    const spotlight = makeSpotlight(nodeSel, linkSel, visEdges);
  }

  renderSimPanel();
  render();
  document.querySelector('button[data-view=network]').addEventListener('click', () => {
    requestAnimationFrame(render);
  });

  const legend = document.getElementById('netlegend');
  legend.innerHTML = [
    '<span><span class="sw" style="border:2px dashed #f4655f;background:none"></span>system constraint</span>',
    '<span><span class="sw" style="border:2px dashed #f5b94c;background:none"></span>candidates #2–3</span>',
    '<span><span class="sw" style="border:2px dashed #b58cff;background:none"></span>simulated merge</span>',
    '<span><span class="sw" style="background:linear-gradient(90deg,#3ecf8e,#f4655f)"></span>ring: queue heat ρ</span>',
    '<span>node: size + center number = WIP</span>',
    '<span>under node: ⬇ fan-in (teams it depends on) · ⬆ fan-out (teams that depend on it)</span>',
    '<span>edge width: blocking links (12 mo)</span>',
    '<span><span class="sw" style="background:#3ecf8e"></span>compare: green = new edge/pod · <span class="sw" style="border:1px dashed #f4655f;background:none"></span>red dashed = dropped edge · halo = ΔWIP</span>',
  ].join('');
}

// compare annotation: how this pod's WIP moved vs the other snapshot
function compareWipNote(cmp, name) {
  const delta = cmp.podDelta.get(name);
  if (!delta) return ` <span class="hint">(no change vs ${cmp.bName})</span>`;
  const col = delta > 0 ? 'var(--red)' : 'var(--green)';
  return ` <span class="flag" style="color:${col}">${delta > 0 ? '+' : ''}${delta} vs ${cmp.bName}</span>`;
}

function showPanel(d, m, simulated, cmp, onHide) {
  const s = m.stats[d.name];
  const inAll = m.edges.filter((e) => e.to === d.name);
  const outAll = m.edges.filter((e) => e.from === d.name);
  const inbound = [...inAll].sort((a, b) => b.count - a.count).slice(0, 6);
  const outbound = [...outAll].sort((a, b) => b.count - a.count).slice(0, 6);
  const fanIn = new Set(inAll.map((e) => e.from)).size;
  const fanOut = new Set(outAll.map((e) => e.to)).size;
  const instability = fanIn + fanOut ? fanIn / (fanIn + fanOut) : 0;
  const zeroOverlap = [...inbound.map((e) => e.from), ...outbound.map((e) => e.to)]
    .filter((other) => (m.overlap[d.name]?.[other] ?? 0) <= 0);
  const flags = [];
  if (simulated) flags.push('<span class="flag" style="background:rgba(181,140,255,.15);color:#b58cff">SIMULATION</span>');
  if (s.rho0 >= 0.85) flags.push('<span class="flag red">queue hot (ρ≥0.85)</span>');
  if (zeroOverlap.length) flags.push(`<span class="flag red">${zeroOverlap.length} zero-overlap dependencies</span>`);
  if (s.synthetic) flags.push('<span class="flag amber">no mined data</span>');
  if (s.sigma > 1.2 && !s.synthetic) flags.push('<span class="flag amber">high variability (σ&gt;1.2)</span>');

  const fmtEdge = (other, count, dir) => {
    const ov = m.overlap[d.name]?.[other] ?? '?';
    return `<dd>${dir} <b>${other}</b> ×${count} <span class="hint">(${ov}h overlap)</span></dd>`;
  };

  document.getElementById('netpanel').innerHTML = `
    <h2>${d.name}</h2>
    <div>${flags.join(' ') || '<span class="flag" style="color:var(--green)">healthy</span>'}</div>
    <dl>
      <dt>Work area</dt><dd>${d.area || '—'}</dd>
      <dt>Site</dt><dd>${d.location} · ${d.devCount} devs</dd>
      <dt>Flow (180d)</dt><dd>${s.synthetic ? 'synthetic estimate' : `${s.resolved180} resolved · ${s.throughputWk.toFixed(1)}/wk`}</dd>
      <dt>Cycle time</dt><dd>P50 ${s.p50.toFixed(1)}d · P85 ${s.p85.toFixed(1)}d</dd>
      <dt>WIP / queue heat</dt><dd>${s.wip} items · ρ≈${s.rho0.toFixed(2)}${cmp && cmp.podDelta.has(d.name) ? compareWipNote(cmp, d.name) : ''}</dd>
      <dt>Coupling</dt><dd>depends on ${fanIn} · ${fanOut} depend on it
        <span class="hint">(instability ${instability.toFixed(2)} — 1 = at others' mercy, 0 = others rely on it)</span></dd>
      <dt>Blocked by (top)</dt>${inbound.map((e) => fmtEdge(e.from, e.count, '⬅ blocked by')).join('') || '<dd>—</dd>'}
      <dt>Blocks (top)</dt>${outbound.map((e) => fmtEdge(e.to, e.count, '➡ blocks')).join('') || '<dd>—</dd>'}
    </dl>
    ${onHide ? '<button id="np-hide">🙈 Hide this pod</button> <span class="hint">temporarily, this session</span>' : ''}`;
  if (onHide) document.getElementById('np-hide').addEventListener('click', () => {
    onHide(d.name);
    document.getElementById('netpanel').innerHTML = `<p class="hint">${d.name} hidden — use “Reset (show all)” above the graph to restore it.</p>`;
  });
}

// showEdgeIssuesModal drills into a double-clicked from→to edge: fetches the
// individual (blocker, blocked) Jira issue pairs behind that edge's count and
// lists them. Server mode only (apiGet is a no-op in static/offline mode).
function edgeModal() {
  let ov = document.getElementById('edge-issues-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'edge-issues-overlay';
    ov.className = 'modal-overlay';
    ov.hidden = true;
    document.body.appendChild(ov);
    // no click-outside-to-close — the ✕ button is the deliberate exit.
  }
  return ov;
}

async function showEdgeIssuesModal(e) {
  const ov = edgeModal();
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>${esc(e.from)} → ${esc(e.to)}</h2><button id="edge-issues-close">✕</button></div>
      <p class="hint">Jira issues in <b>${esc(e.from)}</b> blocking work in <b>${esc(e.to)}</b> (×${e.count} counted into this edge).</p>
      <div id="edge-issues-body"><p class="hint">Loading…</p></div>
    </div>`;
  ov.hidden = false;
  ov.querySelector('#edge-issues-close').addEventListener('click', () => { ov.hidden = true; });

  const body = ov.querySelector('#edge-issues-body');
  const [rows, jiraBase] = await Promise.all([
    apiGet(`edge-issues?from=${encodeURIComponent(e.from)}&to=${encodeURIComponent(e.to)}`),
    getJiraBaseUrl(),
  ]);
  if (!ov.contains(body)) return; // modal closed/replaced while the fetch was in flight
  if (rows === null) {
    body.innerHTML = '<p class="hint">Could not load — Jira drill-down needs the server-backed snapshot store.</p>';
    return;
  }
  if (!rows.length) {
    body.innerHTML = '<p class="hint">No individual issue links found for this edge.</p>';
    return;
  }
  body.innerHTML = `<dl>${rows.map((r) => `
      <dt>${jiraLink(jiraBase, esc(r.blockerKey))} ${esc(r.blockerSummary)}</dt>
      <dd>⬇ blocks ${jiraLink(jiraBase, esc(r.blockedKey))} ${esc(r.blockedSummary)}</dd>`).join('')}</dl>`;
}
