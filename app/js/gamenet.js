// Game-scoped dependency network. Renders from the sanitized game view
// (pods + edges the server sends), so every team starts with the same shape
// and it reshapes as their levers fire (WIP moves, interfaces get built).
// This is deliberately separate from the real-org Network tab (graph.js),
// which shows the current mined state of the actual org.
import { heatColor, layerDag } from './graph.js';
import { constraintScores } from './sim.js';

const MUTED = '#2e3f51';
const DIM = 0.12;

// el: a container element holding (or to hold) an <svg>. view: a gameView.
// opts.panel: an element to render the click-to-inspect details into.
export function renderGameNetwork(el, view, opts = {}) {
  if (!el) return;
  const panel = opts.panel || null;
  let svg = el.querySelector('svg');
  if (!svg) {
    svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    el.appendChild(svg);
  }
  const sel = d3.select(svg);
  sel.selectAll('*').remove();

  if (!view || !view.pods || !view.pods.length) {
    el.querySelector('.gamenet-empty')?.remove();
    const p = document.createElement('p');
    p.className = 'hint gamenet-empty';
    p.textContent = 'No active game yet — start a game to see your dependency network.';
    el.appendChild(p);
    if (panel) panel.innerHTML = '<p class="hint">Start a game, then click a team to inspect it.</p>';
    return;
  }
  el.querySelector('.gamenet-empty')?.remove();

  const rect = svg.getBoundingClientRect();
  const width = rect.width > 50 ? rect.width : 1000;
  const height = rect.height > 50 ? rect.height : 640;

  const pods = view.pods;
  const names = pods.map((p) => p.name);
  const nameSet = new Set(names);
  const edges = (view.edges || []).filter((e) => nameSet.has(e.from) && nameSet.has(e.to) && e.from !== e.to);

  // coupling: distinct teams each pod depends on (fan-in) vs. that depend on it (fan-out)
  const dependsOn = new Map();
  const dependents = new Map();
  for (const e of edges) {
    (dependents.get(e.from) ?? dependents.set(e.from, new Set()).get(e.from)).add(e.to);
    (dependsOn.get(e.to) ?? dependsOn.set(e.to, new Set()).get(e.to)).add(e.from);
  }
  const fanIn = (n) => dependsOn.get(n)?.size ?? 0;
  const fanOut = (n) => dependents.get(n)?.size ?? 0;

  // constraint ranking — same Kingman queue-factor × downstream-reach model the
  // admin network uses, fed from the game's current per-pod load.
  const stats = {};
  for (const p of pods) stats[p.name] = { rho0: p.rho };
  const ranked = constraintScores(stats, edges);
  const constraintRank = new Map(ranked.slice(0, 3).map((r, i) => [r.pod, i]));

  // deterministic left→right layered layout (active dependencies drive the DAG)
  const active = edges.filter((e) => !e.interfaced);
  const { layer } = layerDag(names, active.length ? active : edges);
  const podByName = new Map(pods.map((p) => [p.name, p]));
  const cols = d3.groups(pods, (p) => layer.get(p.name)).sort((a, b) => a[0] - b[0]);
  const pos = new Map();
  const colW = (width - 160) / Math.max(cols.length - 1, 1);
  for (const [li, members] of cols) {
    members.sort((a, b) => a.name.localeCompare(b.name));
    const gap = height / (members.length + 1);
    members.forEach((p, i) => pos.set(p.name, { x: 80 + li * colW, y: gap * (i + 1) }));
  }

  const wipMax = d3.max(pods, (p) => p.wip) || 1;
  const r = (p) => 8 + Math.sqrt(p.wip || 1) * (2 + 6 / Math.sqrt(wipMax + 1));
  const cMax = d3.max(edges, (e) => e.count) ?? 1;

  const defs = sel.append('defs');
  defs.append('marker').attr('id', 'gn-arrow').attr('viewBox', '0 -4 8 8')
    .attr('refX', 8).attr('markerWidth', 6.5).attr('markerHeight', 6.5).attr('orient', 'auto')
    .append('path').attr('d', 'M0,-4L8,0L0,4').attr('fill', '#56708a');

  const g = sel.append('g');
  sel.on('click', () => { spotlight(null); resetPanel(); });

  const edgePath = (e) => {
    const a = pos.get(e.from), b = pos.get(e.to);
    const tb = podByName.get(e.to);
    const dx = b.x - a.x, dy = b.y - a.y;
    const len = Math.hypot(dx, dy) || 1;
    const k = (len - r(tb) - 4) / len;
    const tx = a.x + dx * k, ty = a.y + dy * k;
    const mx = (a.x + tx) / 2;
    return `M${a.x},${a.y} C${mx},${a.y} ${mx},${ty} ${tx},${ty}`;
  };
  const linkSel = g.append('g').selectAll('path').data(edges).join('path')
    .attr('d', edgePath)
    .attr('fill', 'none')
    .attr('stroke', (e) => (e.interfaced ? '#3ecf8e' : '#56708a'))
    .attr('stroke-opacity', (e) => (e.interfaced ? 0.5 : 0.4))
    .attr('stroke-dasharray', (e) => (e.interfaced ? '4,4' : null))
    .attr('stroke-width', (e) => 1 + 4.5 * (e.count / cMax))
    .attr('marker-end', 'url(#gn-arrow)');
  linkSel.append('title').text((e) => `${e.from} → ${e.to}: ${e.count} blocking link(s)`
    + (e.interfaced ? ' — interface built (handoff drag removed)' : ''));

  let dragMoved = false;
  const nodeSel = g.append('g').selectAll('g').data(pods).join('g')
    .attr('transform', (p) => `translate(${pos.get(p.name).x},${pos.get(p.name).y})`)
    .style('cursor', 'grab')
    .on('click', (ev, p) => {
      if (dragMoved) { dragMoved = false; return; } // ignore the click that ends a drag
      ev.stopPropagation(); spotlight(p.name); showInfo(p);
    })
    .call(d3.drag()
      .on('start', function () { dragMoved = false; d3.select(this).raise(); })
      .on('drag', function (event, p) {
        dragMoved = true;
        const np = pos.get(p.name);
        np.x = event.x; np.y = event.y;
        d3.select(this).attr('transform', `translate(${np.x},${np.y})`);
        linkSel.attr('d', edgePath); // edges follow the node
      }));

  nodeSel.append('circle')
    .attr('r', (p) => r(p))
    .attr('fill', (p) => (p.attrited ? '#3a2330' : MUTED))
    .attr('stroke', (p) => heatColor(p.rho))
    .attr('stroke-width', 2.5);

  // constraint rings (red = the system constraint, amber = #2–3 candidates)
  nodeSel.filter((p) => constraintRank.has(p.name)).append('circle')
    .attr('r', (p) => r(p) + 5)
    .attr('fill', 'none')
    .attr('stroke', (p) => (constraintRank.get(p.name) === 0 ? '#f4655f' : '#f5b94c'))
    .attr('stroke-width', 2).attr('stroke-dasharray', '5,3');

  nodeSel.append('text').text((p) => p.name)
    .attr('dy', (p) => -r(p) - 8).attr('text-anchor', 'middle')
    .attr('fill', '#cfdcea').attr('font-size', 11.5);
  nodeSel.append('text').text((p) => p.wip)
    .attr('dy', 3.5).attr('text-anchor', 'middle').attr('pointer-events', 'none')
    .attr('fill', '#eaf2fb').attr('font-size', 10).attr('font-weight', 700);
  nodeSel.filter((p) => constraintRank.get(p.name) === 0).append('text')
    .text('CONSTRAINT')
    .attr('dy', (p) => r(p) + 17).attr('text-anchor', 'middle')
    .attr('fill', '#f4655f').attr('font-size', 9).attr('font-weight', 700).attr('letter-spacing', 1.5);
  nodeSel.append('text')
    .attr('dy', (p) => r(p) + (constraintRank.get(p.name) === 0 ? 31 : 18))
    .attr('text-anchor', 'middle').attr('font-size', 9.5).attr('fill', '#8fa6bd')
    .attr('pointer-events', 'none')
    .text((p) => `⬇${fanIn(p.name)} ⬆${fanOut(p.name)}`)
    .append('title')
    .text((p) => `${p.name} depends on ${fanIn(p.name)} team(s) · ${fanOut(p.name)} team(s) depend on it`);

  function spotlight(name) {
    if (!name) { nodeSel.attr('opacity', 1); linkSel.attr('opacity', 1); return; }
    const cone = new Set([name]);
    for (const e of edges) {
      if (e.from === name) cone.add(e.to);
      if (e.to === name) cone.add(e.from);
    }
    nodeSel.attr('opacity', (p) => (cone.has(p.name) ? 1 : DIM));
    linkSel.attr('opacity', (e) => (e.from === name || e.to === name ? 1 : DIM));
  }

  function resetPanel() {
    if (panel) panel.innerHTML = '<p class="hint">Click a team to inspect its load, coupling and dependencies.</p>';
  }

  function showInfo(p) {
    if (!panel) return;
    const inbound = edges.filter((e) => e.to === p.name).sort((a, b) => b.count - a.count);
    const outbound = edges.filter((e) => e.from === p.name).sort((a, b) => b.count - a.count);
    const tot = fanIn(p.name) + fanOut(p.name);
    const inst = tot ? fanIn(p.name) / tot : 0;
    const flags = [];
    if (constraintRank.get(p.name) === 0) flags.push('<span class="flag red">system constraint</span>');
    else if (constraintRank.has(p.name)) flags.push('<span class="flag amber">constraint candidate</span>');
    if (p.rho >= 1) flags.push('<span class="flag red">queue overloaded (ρ≥1)</span>');
    else if (p.rho >= 0.85) flags.push('<span class="flag amber">queue hot</span>');
    if (p.attrited) flags.push('<span class="flag red">attrition</span>');
    const edgeLi = (other, count, interfaced, dir) =>
      `<dd>${dir} <b>${other}</b> ×${count}${interfaced ? ' <span class="hint">(interface built)</span>' : ''}</dd>`;
    panel.innerHTML = `
      <h3>${p.name}${p.isSre ? ' <span class="hint">SRE</span>' : ''}</h3>
      <div>${flags.join(' ') || '<span class="flag" style="color:var(--green)">healthy</span>'}</div>
      <dl>
        <dt>Site</dt><dd>${(p.location || '—').replace('*REMOTE - multicontinental*', 'Remote')}${p.pairing ? '' : ' · solo'}</dd>
        <dt>WIP / load</dt><dd>${p.wip} items · ρ ${p.rho > 3 ? '3+' : p.rho.toFixed(2)}</dd>
        <dt>Morale / Readiness</dt><dd>${Math.round(p.morale * 100)}% · ${Math.round(p.readiness * 100)}%</dd>
        <dt>Interrupt / KTLO</dt><dd>${p.interrupt.toFixed(1)}d · ${p.ktlo.toFixed(1)}d</dd>
        <dt>Hygiene</dt><dd>${Math.round(p.hygiene * 100)}%</dd>
        <dt>Coupling</dt><dd>depends on ${fanIn(p.name)} · ${fanOut(p.name)} depend on it
          <span class="hint">(instability ${inst.toFixed(2)})</span></dd>
        <dt>Blocked by</dt>${inbound.slice(0, 6).map((e) => edgeLi(e.from, e.count, e.interfaced, '⬅')).join('') || '<dd>—</dd>'}
        <dt>Blocks</dt>${outbound.slice(0, 6).map((e) => edgeLi(e.to, e.count, e.interfaced, '➡')).join('') || '<dd>—</dd>'}
      </dl>`;
  }

  resetPanel();
}

export const GAMENET_LEGEND = [
  '<span><span class="sw" style="border:2px dashed #f4655f;background:none"></span> system constraint · '
    + '<span class="sw" style="border:2px dashed #f5b94c;background:none"></span> candidates #2–3</span>',
  '<span>node: size + center number = WIP · ring colour = queue heat (load ρ)</span>',
  '<span>under node: ⬇ teams it depends on · ⬆ teams that depend on it · click a team to inspect</span>',
  '<span>edge width: blocking links · <span style="color:#3ecf8e">green dashed</span> = interface built (drag removed)</span>',
].join('');
