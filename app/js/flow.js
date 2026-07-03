import { constraintScores, freezeProjection, feverPoint, simulateFeature } from './sim.js';
import { apiGet, getJiraBaseUrl } from './data.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

export function initFlow(state) {
  getJiraBaseUrl().then((u) => { JIRA = u ? u + '/browse/' : ''; });
  renderConstraints(state);
  const slider = document.getElementById('cap-slider');
  slider.addEventListener('input', () => {
    document.getElementById('cap-v').textContent = (+slider.value).toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
    renderFreeze(state, +slider.value);
  });
  renderFreeze(state, +slider.value);
  renderFever(state);
  document.getElementById('fever-count')?.addEventListener('change', () => renderFever(state));
  document.querySelector('button[data-view=flow]').addEventListener('click', () => {
    requestAnimationFrame(() => renderFever(state));
  });
}

function renderConstraints(state) {
  const real = Object.fromEntries(
    Object.entries(state.stats).filter(([name]) => state.pods.some((p) => p.name === name)),
  );
  const top = constraintScores(real, state.edges).slice(0, 3);
  const podOf = (n) => state.pods.find((p) => p.name === n);
  document.getElementById('constraint-cards').innerHTML = top.map((c, i) => {
    const p = podOf(c.pod); const s = state.stats[c.pod];
    const downstream = state.edges.filter((e) => e.from === c.pod)
      .sort((a, b) => b.count - a.count).slice(0, 4).map((e) => e.to);
    const cap = Math.max(2, Math.round((p.streams ?? p.devCount) * 1.5));
    return `
    <div class="constraint-card ${i > 0 ? 'rank2' : ''}">
      <h3>#${i + 1} ${c.pod} <span class="hint">· ${p.location} · ${p.devCount} devs</span></h3>
      <div class="metrics">load ${((s.load ?? s.rho0)).toFixed(1)}× ${(s.load ?? s.rho0) >= 1 ? '(queue unstable — grows until WIP drops below 1)' : `(wait ×${c.queueFactor.toFixed(1)})`}
        · blocks ${c.dependents} pods (×${c.demand} links) · WIP ${s.wip} · P85 ${s.p85.toFixed(0)}d</div>
      <ul>
        <li><b>Exploit:</b> cap WIP at ~${cap} (now ${s.wip}); shield from interrupts;
            finish before starting. Every hour lost here is lost for ${c.dependents} pods.</li>
        <li><b>Subordinate:</b> ${downstream.join(', ') || 'downstream pods'} sequence requests
            to ${c.pod}'s cadence — batch asks, stop drive-by tickets, full-kit before handing over.</li>
        <li><b>Elevate</b> (only after the above): add capacity, or split the
            ${p.area ? `"${p.area}"` : 'domain'} fracture plane so demand divides.</li>
      </ul>
    </div>`;
  }).join('');
}

function renderFreeze(state, capPerDev) {
  const rows = state.pods
    .map((p) => ({ p, s: state.stats[p.name] }))
    .filter(({ s }) => s.wip > 0 && !s.synthetic)
    .sort((a, b) => b.s.wip - a.s.wip).slice(0, 10);
  const maxWip = rows[0]?.s.wip ?? 1;
  let frozenTotal = 0; let gains = [];
  document.getElementById('freeze-bars').innerHTML = rows.map(({ p, s }) => {
    const cap = Math.max(2, Math.round((p.streams ?? p.devCount) * capPerDev));
    const proj = freezeProjection(s.wip, s.throughputWk, cap);
    frozenTotal += proj.frozen;
    const healthyW = (Math.min(s.wip, cap) / maxWip) * 100;
    const excessW = (proj.frozen / maxWip) * 100;
    let gain = '';
    if (proj.currentDays != null && proj.frozen > 0) {
      gains.push(1 - proj.projectedDays / proj.currentDays);
      gain = `<span class="freeze-gain">${proj.currentDays.toFixed(0)}d → ${proj.projectedDays.toFixed(0)}d flow time</span>`;
    } else if (proj.frozen === 0) {
      gain = '<span class="hint">within cap</span>';
    } else {
      gain = '<span class="hint">no throughput data</span>';
    }
    return `<div class="freeze-row" data-pod="${p.name}" title="click to inspect WIP issues">
      <span>▸ ${p.name} <span class="hint">${s.wip} wip</span></span>
      <div class="freeze-track">
        <span class="healthy" style="width:${healthyW}%"></span>
        <span class="excess" style="left:${healthyW}%;width:${excessW}%"></span>
      </div>
      <span>${gain}</span>
    </div>
    <div class="wip-drill" id="drill-${p.name}" hidden></div>`;
  }).join('');
  document.querySelectorAll('.freeze-row').forEach((row) => {
    row.addEventListener('click', () => toggleDrill(row.dataset.pod));
  });
  const avgGain = gains.length ? (gains.reduce((a, b) => a + b, 0) / gains.length) * 100 : 0;
  document.getElementById('freeze-summary').textContent =
    `Freezing ${frozenTotal} items org-wide (red segments) projects an average ${avgGain.toFixed(0)}% `
    + 'flow-time reduction for what remains — nobody works faster, the work just stops waiting. '
    + 'Defrost as capacity frees (Rules of Flow: triage, then full-kit before defrosting).';
}

// '' until getJiraBaseUrl() resolves, or if CONWAY_JIRA_BASE_URL is unset —
// jiraLink() below falls back to plain (unlinked) text in that case.
let JIRA = '';
const jiraLink = (key) => (JIRA ? `<a href="${JIRA}${key}" target="_blank">${key}</a>` : key);
const VERDICT_BADGE = {
  freeze: '<span class="flag red">freeze candidate</span>',
  review: '<span class="flag amber">review</span>',
  keep: '<span class="flag" style="color:var(--green)">keep</span>',
};

const DRILL_PER_PAGE = 15;

// Each drill page is a filtered/paginated SQL query — the server orders freeze
// candidates first and returns only this page's rows, so a pod with thousands
// of WIP issues never ships more than DRILL_PER_PAGE rows to the browser.
function toggleDrill(pod) {
  const div = document.getElementById(`drill-${pod}`);
  if (!div) return;
  if (!div.hidden) { div.hidden = true; return; }
  const rowHtml = (i) => `
    <tr class="v-${i.verdict}">
      <td>${jiraLink(i.key)}</td>
      <td>${i.summary}</td>
      <td>${i.assignee || '<span class="flag amber">unassigned</span>'}</td>
      <td>${i.ageDays?.toFixed(0) ?? '?'}d</td>
      <td>${i.staleDays?.toFixed(0) ?? '?'}d</td>
      <td>${i.blocksKeys?.length ? `${i.blocksKeys.length} issue(s)` : '—'}</td>
      <td>${VERDICT_BADGE[i.verdict]}</td>
    </tr>`;
  const renderPage = async (page) => {
    div.innerHTML = '<p class="hint">loading…</p>';
    const r = await apiGet(`wip?pod=${encodeURIComponent(pod)}&page=${page}&size=${DRILL_PER_PAGE}`);
    const items = r?.items ?? [];
    const total = r?.total ?? 0;
    const pages = r?.pages ?? 1;
    const shown = page * DRILL_PER_PAGE + items.length;
    div.innerHTML = `
      <p class="hint">${total} in-progress issues · <b style="color:var(--red)">${r?.freezable ?? 0} look freezable</b>
      (stale &gt;14d or unassigned, and nothing waits on them). "Keep" = another issue blocks on it — freezing it freezes them too.</p>
      ${total ? `<table class="wip-table">
        <thead><tr><th>Issue</th><th>Summary</th><th>Assignee</th><th>Age</th><th>Stale</th><th>Blocks</th><th>Verdict</th></tr></thead>
        <tbody>${items.map(rowHtml).join('')}</tbody></table>
        ${pages > 1 ? `<div class="row-actions">
          <button id="drill-prev-${pod}" ${page === 0 ? 'disabled' : ''}>‹ Prev</button>
          <span class="hint">page ${page + 1} of ${pages} · showing ${page * DRILL_PER_PAGE + 1}–${shown} of ${total}</span>
          <button id="drill-next-${pod}" ${page >= pages - 1 ? 'disabled' : ''}>Next ›</button></div>` : ''}`
      : '<p class="hint">No in-progress issues for this pod in the snapshot.</p>'}`;
    if (pages > 1) {
      document.getElementById(`drill-prev-${pod}`)?.addEventListener('click', () => renderPage(page - 1));
      document.getElementById(`drill-next-${pod}`)?.addEventListener('click', () => renderPage(page + 1));
    }
  };
  renderPage(0);
  div.hidden = false;
}

// The server returns exactly the N newest in-flight epics that will render
// (≥1 child in a known pod), each with its child tasks + blocking links — so
// the dropdown count == the number of dots, and no whole-blob transfer.
async function loadEpics(cap) {
  const r = await apiGet(`fever?n=${cap}`);
  return { epics: r?.epics ?? [], total: r?.total ?? 0 };
}

const SIZE_OF_POINTS = (pts) => (pts == null ? 1 : pts <= 2 ? 0.5 : pts <= 5 ? 1 : pts <= 8 ? 2 : 4);

async function renderFever(state) {
  const svg = d3.select('#fever');
  svg.selectAll('*').remove();
  const rect = svg.node().getBoundingClientRect();
  const width = rect.width > 50 ? rect.width : 900;
  const height = 260;
  const m = { t: 12, r: 24, b: 34, l: 46 };
  const x = d3.scaleLinear().domain([0, 1]).range([m.l, width - m.r]);
  const y = d3.scaleLinear().domain([0, 1.5]).range([height - m.b, m.t]);

  // ratio zones matching feverPoint: consumed/progress <0.5 green, <1 yellow
  const zone = (ratio, color) => svg.append('path')
    .attr('d', `M${x(0)},${y(0)} L${x(1)},${y(Math.min(ratio, 1.5))} L${x(1)},${y(0)} Z`)
    .attr('fill', color).attr('opacity', 0.16);
  svg.append('rect').attr('x', x(0)).attr('y', y(1.5)).attr('width', x(1) - x(0))
    .attr('height', y(0) - y(1.5)).attr('fill', '#f4655f').attr('opacity', 0.13);
  zone(1.0, '#f5b94c');
  zone(0.5, '#3ecf8e');

  svg.append('g').attr('transform', `translate(0,${height - m.b})`)
    .call(d3.axisBottom(x).ticks(5).tickFormat(d3.format('.0%'))).attr('color', '#8aa0b4');
  svg.append('g').attr('transform', `translate(${m.l},0)`)
    .call(d3.axisLeft(y).ticks(5).tickFormat(d3.format('.0%'))).attr('color', '#8aa0b4');
  svg.append('text').text('work complete (size-weighted)').attr('x', (x(0) + x(1)) / 2).attr('y', height - 4)
    .attr('text-anchor', 'middle').attr('fill', '#8aa0b4').attr('font-size', 11);
  svg.append('text').text('buffer consumed').attr('transform', `translate(12,${(y(0) + y(1.5)) / 2}) rotate(-90)`)
    .attr('text-anchor', 'middle').attr('fill', '#8aa0b4').attr('font-size', 11);

  const cap = +(document.getElementById('fever-count')?.value) || 25;
  const loading = document.getElementById('fever-loading');
  if (loading) loading.hidden = false;
  const { epics, total } = await loadEpics(cap);
  if (loading) loading.hidden = true;
  if (!epics.length) {
    svg.append('text').text('no epics in this snapshot — import one from Jira (Observe ▸ Import)')
      .attr('x', (x(0) + x(1)) / 2).attr('y', y(0.75)).attr('text-anchor', 'middle')
      .attr('fill', '#8aa0b4').attr('font-size', 12);
    return;
  }
  if (total > epics.length) {
    svg.append('text').text(`showing the ${epics.length} newest in-flight of ${total} epics`)
      .attr('x', width - m.r).attr('y', m.t + 2).attr('text-anchor', 'end')
      .attr('fill', '#8aa0b4').attr('font-size', 10);
  }
  const ZONE_COLOR = { green: '#3ecf8e', yellow: '#f5b94c', red: '#f4655f' };
  const points = [];
  for (const epic of epics) {
    const tasks = epic.tasks.filter((t) => t.pod && state.stats[(t.pod || '').trim()]);
    if (!tasks.length) continue;
    const sized = epic.tasks.map((t) => ({ t, w: SIZE_OF_POINTS(t.points) }));
    const total = sized.reduce((s, v) => s + v.w, 0);
    const done = sized.filter(({ t }) => ['Closed', 'Done', 'Resolved'].includes(t.status))
      .reduce((s, v) => s + v.w, 0);
    const pct = total ? done / total : 0;
    const createds = epic.tasks.map((t) => t.created).filter(Boolean).map((d) => new Date(d));
    if (!createds.length) continue;
    const start = new Date(Math.min(...createds));
    const elapsed = ((Date.now() - start) / 86400000) * (5 / 7); // working days

    const keys = new Set(tasks.map((t) => t.key));
    const feature = {
      tasks: tasks.map((t) => ({ id: t.key, pod: t.pod.trim(), size: SIZE_OF_POINTS(t.points) })),
      deps: tasks.flatMap((t) => t.blockedBy.filter((k) => keys.has(k)).map((k) => [k, t.key])),
    };
    const podStats = Object.fromEntries(Object.entries(state.stats)
      .map(([k, v]) => [k, { mu: v.mu, sigma: v.sigma, rho0: v.rho0 }]));
    let sim;
    try {
      // fever needs only p50/p85 — a light trial count keeps the whole pass fast
      sim = simulateFeature(feature, podStats, state.overlap, { trials: 120, seed: 7 });
    } catch { continue; }
    const fp = feverPoint(pct, elapsed, sim.p50, sim.p85);
    // date risk vs the COMMITTED date (epic due date), not just the forecast
    let dateRisk = null;
    if (epic.duedate) {
      const wdToDue = ((new Date(epic.duedate) - Date.now()) / 86400000) * (5 / 7);
      const remainingP85 = Math.max(0, sim.p85 * (1 - pct));
      if (wdToDue < 0) dateRisk = 'overdue';
      else if (remainingP85 > wdToDue) dateRisk = 'at risk';
    }
    points.push({
      epic: epic.epic, name: epic.name ?? '', pct, elapsed, fp,
      p50: sim.p50, p85: sim.p85, open: epic.tasks.length - Math.round(pct * epic.tasks.length),
      duedate: epic.duedate ?? null, dateRisk, hasOutcome: epic.hasOutcome,
    });
  }

  // deterministic jitter so identical (pct, consumed) dots stay distinguishable
  points.forEach((p, i) => {
    const cy = Math.min(p.fp.consumed, 1.45);
    const jx = ((i % 5) - 2) * 4, jy = (Math.floor(i / 5) % 5 - 2) * 4;
    const dot = svg.append('circle')
      .attr('cx', x(p.pct) + jx).attr('cy', y(cy) + jy).attr('r', 7)
      .attr('fill', ZONE_COLOR[p.fp.zone]).attr('stroke', '#0f1419').attr('stroke-width', 2)
      .style('cursor', 'pointer')
      .on('click', () => showFeverEpicModal(p));
    dot.append('title').text(
      `${p.epic} ${p.name}\n${(p.pct * 100).toFixed(0)}% complete · `
      + `${(p.fp.consumed * 100).toFixed(0)}% buffer consumed (${p.fp.zone})\n`
      + `elapsed ${p.elapsed.toFixed(0)}wd · forecast P50 ${p.p50.toFixed(0)}d / P85 ${p.p85.toFixed(0)}d`
      + (p.duedate ? `\ndue ${p.duedate}${p.dateRisk ? ` — ${p.dateRisk.toUpperCase()} (remaining P85 vs due, approx)` : ''}` : '\nno due date set'),
    );
  });

  const hot = points.filter((p) => p.fp.zone !== 'green' || p.dateRisk)
    .sort((a, b) => (b.dateRisk === 'overdue') - (a.dateRisk === 'overdue') || b.fp.ratio - a.fp.ratio)
    .slice(0, cap);
  document.getElementById('fever-list').innerHTML = hot.length ? `
    <table class="wip-table"><thead><tr><th>Needs attention</th><th>Epic</th>
    <th>Complete</th><th>Buffer burned</th><th>Zone</th><th>Due</th><th>Outcome?</th></tr></thead><tbody>
    ${hot.map((p) => `<tr>
      <td>${jiraLink(p.epic)}</td>
      <td>${p.name || '—'}</td>
      <td>${(p.pct * 100).toFixed(0)}%</td>
      <td>${(Math.min(p.fp.consumed, 9.99) * 100).toFixed(0)}%</td>
      <td><span class="flag ${p.fp.zone === 'red' ? 'red' : 'amber'}">${p.fp.zone}</span></td>
      <td>${p.duedate ?? '<span class="hint">none</span>'} ${p.dateRisk ? DATE_BADGE[p.dateRisk] : ''}</td>
      <td>${p.hasOutcome === true ? '✓' : p.hasOutcome === false ? '<span class="flag red">missing</span>' : '<span class="hint">?</span>'}</td>
    </tr>`).join('')}</tbody></table>` : '<p class="hint">All in-flight epics are in the green zone.</p>';
}

const DATE_BADGE = {
  overdue: '<span class="flag red">overdue</span>',
  'at risk': '<span class="flag amber">date at risk</span>',
};

// Clicking a fever-chart dot shows the epic it represents — id + title, plus
// the same stats already on the dot's hover tooltip, in a form you can select/copy.
function feverEpicModal() {
  let ov = document.getElementById('fever-epic-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'fever-epic-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
    // no click-outside-to-close — the ✕ button is the deliberate exit.
  }
  return ov;
}

function showFeverEpicModal(p) {
  const ov = feverEpicModal();
  const zoneBadge = p.fp.zone === 'red' ? '<span class="flag red">red</span>'
    : p.fp.zone === 'yellow' ? '<span class="flag amber">yellow</span>'
      : '<span class="flag" style="color:var(--green)">green</span>';
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>${jiraLink(p.epic)} — ${esc(p.name || '(no title)')}</h2><button id="fever-epic-close">✕</button></div>
      <p class="hint">${(p.pct * 100).toFixed(0)}% complete · ${(p.fp.consumed * 100).toFixed(0)}% buffer consumed
        ${zoneBadge}</p>
      <p class="hint">Elapsed ${p.elapsed.toFixed(0)} working days · forecast P50 ${p.p50.toFixed(0)}d / P85 ${p.p85.toFixed(0)}d</p>
      <p class="hint">${p.duedate ? `Due ${esc(p.duedate)} ${p.dateRisk ? DATE_BADGE[p.dateRisk] : ''}` : 'No due date set'}</p>
    </div>`;
  ov.hidden = false;
  ov.querySelector('#fever-epic-close').addEventListener('click', () => { ov.hidden = true; });
}
