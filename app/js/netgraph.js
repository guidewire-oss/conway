// netgraph.js — the layered left-to-right dependency-network renderer shared
// by Observe's Org Network (graph.js) and Plan's dependency diagram
// (planui.js), so both speak the same visual language: columns by dependency
// depth, node size = load, ring color = utilization heat, pan/zoom, click to
// spotlight a node's dependency cone. Each caller still owns its own extra
// decorations (constraint rings, side panel, legend) — this module is only
// the shared layout math + drawing primitives, not a one-size-fits-all
// renderer.

export const DIM = 0.12;

export function heatColor(rho) {
  const t = Math.max(0, Math.min(1, (rho - 0.3) / 0.65));
  return d3.interpolateRgb('#3ecf8e', '#f4655f')(t);
}

// layerDag assigns each node a column (layer) by longest path from a root,
// breaking cycles by dropping back-edges (an edge whose target already
// reaches its source is skipped so the DAG stays acyclic).
export function layerDag(nodes, edges) {
  const adj = new Map(nodes.map((n) => [n, []]));
  const kept = [];
  const reaches = (from, to) => {
    const seen = new Set([from]);
    const stack = [from];
    while (stack.length) {
      const cur = stack.pop();
      if (cur === to) return true;
      for (const nxt of adj.get(cur) ?? []) if (!seen.has(nxt)) { seen.add(nxt); stack.push(nxt); }
    }
    return false;
  };
  for (const e of [...edges].sort((a, b) => b.count - a.count)) {
    if (reaches(e.to, e.from)) continue;
    adj.get(e.from).push(e.to);
    kept.push(e);
  }
  const layer = new Map(nodes.map((n) => [n, 0]));
  let changed = true;
  while (changed) {
    changed = false;
    for (const e of kept) {
      if (layer.get(e.to) < layer.get(e.from) + 1) {
        layer.set(e.to, layer.get(e.from) + 1);
        changed = true;
      }
    }
  }
  return { layer };
}

// layoutColumns positions each node left-to-right by dependency layer,
// top-to-bottom within a layer ordered by the barycenter of its upstream
// (incoming-edge) nodes — keeps most edges roughly horizontal without a full
// Sugiyama crossing-minimization pass.
export function layoutColumns(names, edges, width, height) {
  const { layer } = layerDag(names, edges);
  const cols = d3.groups(names, (n) => layer.get(n)).sort((a, b) => a[0] - b[0]);
  const pos = new Map();
  const colW = (width - 160) / Math.max(cols.length - 1, 1);
  for (const [li, members] of cols) {
    members.sort((a, b) => {
      const bary = (n) => {
        const ups = edges.filter((e) => e.to === n).map((e) => pos.get(e.from)?.y ?? height / 2);
        return ups.length ? ups.reduce((s, v) => s + v, 0) / ups.length : height / 2;
      };
      return bary(a) - bary(b) || a.localeCompare(b);
    });
    const gap = height / (members.length + 1);
    members.forEach((n, i) => pos.set(n, { x: 80 + li * colW, y: gap * (i + 1) }));
  }
  return pos;
}

// bezierEdgePath returns a path-generator fn(edge) -> SVG 'd' string, curving
// from a→b and stopping short of b's node circle so the arrowhead lands
// cleanly instead of burying itself under the node. When `edges` is given and
// an edge's reverse (b→a) is also present, the two curves are bowed apart to
// opposite sides of the straight line between the nodes — otherwise a
// reciprocal pair renders as two near-identical overlapping curves that are
// impossible to tell apart or click individually.
export function bezierEdgePath(pos, radiusOfTarget, edges = []) {
  const keys = new Set(edges.map((e) => `${e.from}→${e.to}`));
  const BOW = 22;
  return (e) => {
    const a = pos.get(e.from), b = pos.get(e.to);
    const rb = radiusOfTarget(e.to);
    const dx = b.x - a.x, dy = b.y - a.y;
    const len = Math.hypot(dx, dy) || 1;
    const k = (len - rb - 4) / len;
    const tx = a.x + dx * k, ty = a.y + dy * k;
    const mx = (a.x + tx) / 2;
    const bow = keys.has(`${e.to}→${e.from}`) ? BOW : 0;
    const nx = (-dy / len) * bow, ny = (dx / len) * bow;
    return `M${a.x},${a.y} C${mx + nx},${a.y + ny} ${mx + nx},${ty + ny} ${tx},${ty}`;
  };
}

// appendArrowMarker adds the <marker> def edges reference via marker-end.
export function appendArrowMarker(svg, id = 'arrow', color = '#56708a') {
  svg.append('defs').append('marker').attr('id', id).attr('viewBox', '0 -4 8 8')
    .attr('refX', 8).attr('markerWidth', 6.5).attr('markerHeight', 6.5).attr('orient', 'auto')
    .append('path').attr('d', 'M0,-4L8,0L0,4').attr('fill', color);
}

// enablePanZoom wires scroll-to-zoom + drag-to-pan on svg, applying the
// transform to g (a child <g> holding all drawn content). dblclick-to-zoom is
// disabled so a double-click doesn't surprise-zoom mid-interaction.
export function enablePanZoom(svg, g) {
  svg.call(d3.zoom().scaleExtent([0.5, 2.5]).on('zoom', (ev) => g.attr('transform', ev.transform)))
    .on('dblclick.zoom', null);
}

// enableNodeDrag lets a node be dragged to reposition it (edges follow via
// onMove), and fires a click handler for a plain press+release that never
// crosses the drag threshold. This does NOT use the browser's native 'click'
// event: in Chromium, a node with d3.drag attached can silently swallow the
// very first click's synthesized 'click' event (only pointerdown/mouseup
// fire, no 'click') — confirmed via direct testing — while every click after
// that fires normally. Driving the click off d3.drag's own 'end' event (which
// fires reliably on every pointerup) sidesteps that quirk entirely.
export function enableNodeDrag(nodeSel, pos, onMove) {
  let moved = false;
  let onClick = null;
  nodeSel.style('cursor', 'grab').call(d3.drag()
    .clickDistance(8)
    .on('start', function (event) { moved = false; event.sourceEvent?.stopPropagation(); d3.select(this).raise(); })
    .on('drag', function (event, d) {
      moved = true;
      const p = pos.get(d.name);
      p.x = event.x; p.y = event.y;
      d3.select(this).attr('transform', `translate(${p.x},${p.y})`);
      onMove();
    })
    .on('end', function (event, d) {
      if (!moved) onClick?.(event.sourceEvent, d);
    }));
  return {
    // registers the click handler — called from drag's 'end', not a native
    // 'click' listener (see note above).
    onClick: (handler) => { onClick = handler; },
  };
}

// makeSpotlight returns spotlight(name): dims every node/edge outside name's
// 1-hop dependency cone (both directions); spotlight(null) resets to full opacity.
export function makeSpotlight(nodeSel, linkSel, edges) {
  return function spotlight(name) {
    if (!name) {
      nodeSel.attr('opacity', 1);
      linkSel.attr('opacity', 1);
      return;
    }
    const cone = new Set([name]);
    for (const e of edges) {
      if (e.from === name) cone.add(e.to);
      if (e.to === name) cone.add(e.from);
    }
    nodeSel.attr('opacity', (d) => (cone.has(d.name) ? 1 : DIM));
    linkSel.attr('opacity', (e) => (e.from === name || e.to === name ? 1 : DIM));
  };
}
