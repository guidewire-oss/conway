import { simulateFeature, suggestDeps, fullKitCheck, relativeSize } from './sim.js';
import { apiGet } from './data.js';

// short numeric-suffix id for display: strips any "PROJECT-" prefix, not just one org's
const shortId = (key) => key.replace(/^[^-]+-/, '');

const KIT_TEMPLATE = `h2. FULL KIT — do not start this epic until every box is ticked
(Goldratt's Rules of Flow: starting without a full kit guarantees stop-start delay)

h3. Business
* Problem / outcome we are buying:
* Cost of delay (rough $/month or customers blocked):
* Success metric and target:
* Sponsor / single decision-maker:

h3. Scope
* In scope:
* Explicitly OUT of scope:
* [ ] All tasks created under this epic, one Assigned Pod each, all sized

h3. Dependencies
* Upstream pods + named contact per pod:
* [ ] Block links declared in Jira (not just known in someone's head)
* [ ] Interface contracts agreed — API spec / mock linked here:
* External: security PSA engaged [ ] · legal [ ] · vendor [ ]

h3. Readiness
* [ ] Design reviewed and linked
* [ ] Environments and access available for every involved pod
* [ ] SRE / production-readiness task created under this epic
* [ ] Drum slot: constraint pod(s) confirmed a start window
* [ ] Forecast attached: P50 ___ / P85 ___ (from the flow model — commit the P85)

h3. If this epic gets frozen
* Defrost criteria (what must be true to resume):`;
import { heatColor } from './graph.js';

const SIZES = { S: 0.5, M: 1, L: 2, XL: 4 };

// Pod names here are illustrative — swap in your own org's pod names once
// you've imported real Jira data (this canned list is just a demo starting point).
const EXAMPLE = {
  tasks: [
    { id: 'T1', pod: 'Atlas', size: 'M', deps: '' },               // architecture/design
    { id: 'T2', pod: 'Beacon', size: 'L', deps: 'T1' },            // core platform capacity
    { id: 'T3', pod: 'Cascade', size: 'L', deps: 'T1' },           // data service backend
    { id: 'T4', pod: 'Delta', size: 'L', deps: 'T2' },             // gateway integration
    { id: 'T5', pod: 'Ember', size: 'M', deps: 'T1' },             // authN integration
    { id: 'T6', pod: 'Fjord', size: 'L', deps: 'T3,T4,T5' },       // UI / project service
    { id: 'T7', pod: 'Granite', size: 'M', deps: 'T6' },           // SRE prod readiness
  ],
};

let state = null;
let rhoOverride = {};
let lastResult = null;

export function initSimulator(s) {
  state = s;
  document.getElementById('add-task').addEventListener('click', () => addRow());
  document.getElementById('load-example').addEventListener('click', loadExample);
  document.getElementById('run-sim').addEventListener('click', run);
  document.getElementById('import-epic').addEventListener('click', importEpic);
  // charts drawn while the tab is hidden get a 0-width container; redraw on activation
  document.querySelector('button[data-view=simulator]').addEventListener('click', () => {
    if (lastResult) requestAnimationFrame(() => renderAll(lastResult));
  });
  loadExample();
}

function podOptions(selected) {
  return state.pods.map((p) => `<option ${p.name === selected ? 'selected' : ''}>${p.name}</option>`).join('');
}

function addRow(t = null) {
  const tbody = document.querySelector('#task-table tbody');
  const n = tbody.children.length + 1;
  const tr = document.createElement('tr');
  const id = t?.id ?? `T${n}`;
  tr.innerHTML = `
    <td><input class="t-id" value="${id}" size="3"></td>
    <td><select class="t-pod">${podOptions(t?.pod ?? state.pods[0].name)}</select></td>
    <td><select class="t-size">${Object.keys(SIZES).map((k) => `<option ${k === (t?.size ?? 'M') ? 'selected' : ''}>${k}</option>`).join('')}</select></td>
    <td><input class="t-deps" value="${t?.deps ?? ''}" placeholder="T1,T2"></td>
    <td><button class="del" title="remove">✕</button></td>`;
  tr.querySelector('.del').addEventListener('click', () => tr.remove());
  tbody.appendChild(tr);
}

function pointsToSize(points, pod) {
  // normalize against the pod's own median: teams size in different units
  const median = state.hygiene?.[pod]?.medianPoints;
  if (median) {
    const f = relativeSize(points, median);
    if (f < 0.75) return 'S';
    if (f < 1.5) return 'M';
    if (f < 3) return 'L';
    return 'XL';
  }
  if (points == null) return 'M';
  if (points <= 2) return 'S';
  if (points <= 5) return 'M';
  if (points <= 8) return 'L';
  return 'XL';
}

async function importEpic() {
  const key = document.getElementById('epic-key').value.trim().toUpperCase();
  const status = document.getElementById('epic-status');
  if (!key) { status.textContent = 'enter an epic key'; return; }
  const epic = await apiGet(`epic/${encodeURIComponent(key)}`);
  if (!epic) {
    status.textContent = `no snapshot for ${key} — import it from Jira first`;
    return;
  }
  const open = epic.tasks.filter((t) => t.status !== 'Closed' && t.status !== 'Done');
  const chosen = open.length ? open : epic.tasks;
  const keys = new Set(chosen.map((t) => t.key));
  document.querySelector('#task-table tbody').innerHTML = '';
  for (const t of chosen) {
    const pod = (t.pod ?? '').trim();
    addRow({
      id: shortId(t.key),
      pod: state.pods.some((p) => p.name === pod) ? pod : state.pods[0].name,
      size: pointsToSize(t.points, pod),
      deps: t.blockedBy.filter((k) => keys.has(k)).map(shortId).join(','),
    });
  }
  status.textContent = `${chosen.length} open tasks imported${open.length ? '' : ' (all were closed; imported everything)'}`;
  renderKit({ ...epic, tasks: chosen });
  rhoOverride = {};
  run();
}

function renderKit(epic) {
  const srePods = state.pods.filter((p) => p.sre).map((p) => p.name);
  const kit = fullKitCheck(epic, { stats: state.stats, pods: state.pods, srePods });
  const cls = kit.score >= 0.8 ? 'g' : kit.score >= 0.5 ? 'a' : 'r';
  const ICON = { pass: '✓', warn: '◐', fail: '✗' };
  document.getElementById('fullkit').innerHTML = `
    <div class="kit-head">
      <span class="kit-score ${cls}">kit ${(kit.score * 100).toFixed(0)}%</span>
      <b>Full-kit check — ${epic.epic}</b>
      <span class="help" data-tip="Machine-checkable half of the full kit. The human half (business case, contracts, defrost criteria) is the template below — paste it into the epic description. Rule of thumb: don't start below 80%; a started epic without its kit becomes a stop-start zombie and burns buffer before progress (see the fever chart's top-left cluster).">?</span>
      <button id="kit-tmpl-btn">Jira template</button>
    </div>
    ${kit.items.map((i) => `<div class="kit-item ${i.status}">
      <span class="ki">${ICON[i.status]}</span><span>${i.label}</span>
      <span class="kd">— ${i.detail}</span></div>`).join('')}
    <textarea id="kit-template" hidden readonly>${KIT_TEMPLATE}</textarea>`;
  document.getElementById('kit-tmpl-btn').addEventListener('click', () => {
    const ta = document.getElementById('kit-template');
    ta.hidden = !ta.hidden;
    if (!ta.hidden) {
      ta.select();
      navigator.clipboard?.writeText(KIT_TEMPLATE).catch(() => {});
    }
  });
}

function loadExample() {
  document.querySelector('#task-table tbody').innerHTML = '';
  EXAMPLE.tasks.forEach((t) => addRow(t));
  rhoOverride = {};
  run();
}

function readFeature() {
  const tasks = []; const deps = [];
  for (const tr of document.querySelectorAll('#task-table tbody tr')) {
    const id = tr.querySelector('.t-id').value.trim();
    if (!id) continue;
    tasks.push({ id, pod: tr.querySelector('.t-pod').value, size: SIZES[tr.querySelector('.t-size').value] });
    for (const d of tr.querySelector('.t-deps').value.split(',').map((x) => x.trim()).filter(Boolean)) {
      deps.push([d, id]);
    }
  }
  return { tasks, deps };
}

function run() {
  const feature = readFeature();
  if (!feature.tasks.length) return;
  const podStats = {};
  for (const [name, st] of Object.entries(state.stats)) {
    podStats[name] = { mu: st.mu, sigma: st.sigma, rho0: st.rho0 };
  }
  let r;
  try {
    r = simulateFeature(feature, podStats, state.overlap, {
      trials: 10000, seed: 20260611, flowEff: 0.15, rhoOverride,
    });
  } catch (e) {
    document.getElementById('stat-cards').innerHTML = `<div class="stat"><div class="v" style="color:var(--red);font-size:14px">${e.message}</div></div>`;
    return;
  }
  lastResult = { r, feature };
  renderAll(lastResult);
}

function renderAll({ r, feature }) {
  renderStats(r);
  renderCdf(r.makespans);
  renderTornado(r.sensitivity);
  renderGantt(r.sampleTrial);
  renderCrit(r);
  renderSuggestions(feature);
  renderWhatIf(feature);
}

// past-inference: mined blocking history between this feature's pods that
// the current task graph does not declare
function renderSuggestions(feature) {
  const div = document.getElementById('suggestions');
  const sugg = suggestDeps(feature.tasks, feature.deps, state.edges, 2).slice(0, 5);
  if (!sugg.length) { div.innerHTML = ''; return; }
  div.innerHTML = '<h3>Suggested dependencies (from Jira history)</h3>' + sugg.map((s, i) => `
    <div class="suggestion">
      <span><b>${s.fromPod}</b> has blocked <b>${s.toPod}</b> ×${s.count} in the past 12 months,
      but no dependency is declared here.</span>
      <button data-i="${i}">add</button>
    </div>`).join('');
  div.querySelectorAll('button').forEach((b) => b.addEventListener('click', () => {
    const s = sugg[+b.dataset.i];
    for (const tr of document.querySelectorAll('#task-table tbody tr')) {
      if (tr.querySelector('.t-id').value.trim() === s.toTask) {
        const depsEl = tr.querySelector('.t-deps');
        const cur = depsEl.value.split(',').map((x) => x.trim()).filter(Boolean);
        if (!cur.includes(s.fromTask)) depsEl.value = [...cur, s.fromTask].join(',');
      }
    }
    run();
  }));
}

function chartWidth(svg, fallback = 900) {
  const w = svg.node().getBoundingClientRect().width;
  return w > 50 ? w : fallback;
}

function fmtDate(days) {
  const d = new Date();
  d.setDate(d.getDate() + Math.round(days * 7 / 5)); // working → calendar days
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function renderStats(r) {
  const hlp = (t) => ` <span class="help" data-tip="${t.replace(/"/g, '&quot;')}">?</span>`;
  const cards = [
    ['P50', r.p50, '', 'The completion day 50% of the 10,000 Monte-Carlo trials beat — the optimistic, coin-flip plan. Rules of Flow: do NOT commit this; you\'ll miss it half the time.'],
    ['P85 — commit this', r.p85, 'p85', 'The day 85% of trials finished by — the date to actually promise. It already prices in per-pod cycle-time variability, queue waits, and cross-site handoff delay. Probabilistic forecasting: commit a percentile, not a point.'],
    ['P95', r.p95, '', 'The near-worst-case day (95% of trials beat it). The gap P95−P50 is your risk/variability — wide gaps mean the plan is fragile to a bad event.'],
  ];
  document.getElementById('stat-cards').innerHTML = cards.map(([l, v, cls, tip]) => `
    <div class="stat ${cls}"><div class="l">${l}${hlp(tip)}</div>
    <div class="v">${v.toFixed(0)}d</div>
    <div class="hint">~${fmtDate(v)}</div></div>`).join('');
}

function renderCdf(makespans) {
  const svg = d3.select('#cdf');
  svg.selectAll('*').remove();
  const width = chartWidth(svg, 450); const height = 200;
  const m = { t: 8, r: 12, b: 24, l: 36 };
  const x = d3.scaleLinear().domain([makespans[0], makespans[makespans.length - 1]]).range([m.l, width - m.r]);
  const y = d3.scaleLinear().domain([0, 1]).range([height - m.b, m.t]);
  const pts = makespans.filter((_, i) => i % 50 === 0).map((v, i) => [v, (i * 50) / makespans.length]);
  svg.append('g').attr('transform', `translate(0,${height - m.b})`).call(d3.axisBottom(x).ticks(6)).attr('color', '#8aa0b4');
  svg.append('g').attr('transform', `translate(${m.l},0)`).call(d3.axisLeft(y).ticks(5).tickFormat(d3.format('.0%'))).attr('color', '#8aa0b4');
  svg.append('path').datum(pts)
    .attr('d', d3.line().x((p) => x(p[0])).y((p) => y(p[1])))
    .attr('fill', 'none').attr('stroke', '#4cc2ff').attr('stroke-width', 2);
  for (const q of [0.5, 0.85]) {
    const v = makespans[Math.floor(q * makespans.length)];
    svg.append('line').attr('x1', x(v)).attr('x2', x(v)).attr('y1', y(0)).attr('y2', y(q))
      .attr('stroke', '#f5b94c').attr('stroke-dasharray', '3,3');
  }
}

function renderTornado(sens) {
  const svg = d3.select('#tornado');
  svg.selectAll('*').remove();
  const width = chartWidth(svg, 450);
  const rows = sens.slice(0, 8);
  const h = 22; const m = { l: 110, r: 40 };
  svg.attr('height', rows.length * h + 10);
  const x = d3.scaleLinear().domain([0, d3.max(rows, (d) => Math.max(0.1, d.savedDays))]).range([0, width - m.l - m.r]);
  const g = svg.selectAll('g').data(rows).join('g').attr('transform', (_, i) => `translate(0,${i * h + 6})`);
  g.append('text').text((d) => d.pod).attr('x', m.l - 8).attr('y', 12).attr('text-anchor', 'end').attr('fill', '#e5eef7').attr('font-size', 12);
  g.append('rect').attr('x', m.l).attr('height', 14).attr('rx', 3)
    .attr('width', (d) => Math.max(2, x(Math.max(0, d.savedDays))))
    .attr('fill', (_, i) => (i === 0 ? '#f4655f' : '#4cc2ff'));
  g.append('text').text((d) => `${d.savedDays.toFixed(1)}d`)
    .attr('x', (d) => m.l + Math.max(2, x(Math.max(0, d.savedDays))) + 6).attr('y', 12)
    .attr('fill', '#8aa0b4').attr('font-size', 11);
}

function renderGantt(trial) {
  const svg = d3.select('#gantt');
  svg.selectAll('*').remove();
  const width = chartWidth(svg);
  const rows = trial.tasks; const h = 26; const m = { l: 150, r: 20, b: 22 };
  svg.attr('height', rows.length * h + m.b + 6);
  const x = d3.scaleLinear().domain([0, trial.makespan]).range([m.l, width - m.r]);
  const g = svg.selectAll('g.row').data(rows).join('g').attr('transform', (_, i) => `translate(0,${i * h + 4})`);
  g.append('text').text((d) => `${d.id} · ${d.pod}`).attr('x', m.l - 8).attr('y', 14)
    .attr('text-anchor', 'end').attr('fill', '#e5eef7').attr('font-size', 12);
  g.append('rect')
    .attr('x', (d) => x(d.start)).attr('width', (d) => Math.max(2, x(d.end) - x(d.start)))
    .attr('height', 16).attr('rx', 4)
    .attr('fill', (d) => (d.critical ? '#f4655f' : '#3a4d61'));
  g.append('title').text((d) => `${d.id} (${d.pod}): day ${d.start.toFixed(1)} → ${d.end.toFixed(1)}`);
  svg.append('g').attr('transform', `translate(0,${rows.length * h + 6})`)
    .call(d3.axisBottom(x).ticks(8).tickFormat((v) => `${v}d`)).attr('color', '#8aa0b4');
}

function renderCrit(r) {
  const entries = Object.entries(r.podCriticality).sort((a, b) => b[1] - a[1]);
  document.getElementById('crit-list').innerHTML =
    '<b>Criticality index</b> (probability the pod sits on the critical path): ' +
    entries.map(([pod, c]) => `${pod} <b>${(c * 100).toFixed(0)}%</b>`).join(' · ');
}

function renderWhatIf(feature) {
  const pods = [...new Set(feature.tasks.map((t) => t.pod))];
  const div = document.getElementById('whatif');
  div.innerHTML = '<h3>What-if: pod load (Kingman queue scaling)</h3>' + pods.map((p) => {
    const rho = rhoOverride[p] ?? state.stats[p].rho0;
    return `<label>${p} — ρ <b id="rv-${p}" style="color:${heatColor(rho)}">${rho.toFixed(2)}</b>
      (baseline ${state.stats[p].rho0.toFixed(2)})</label>
      <input type="range" min="0.30" max="0.97" step="0.01" value="${rho}" data-pod="${p}">`;
  }).join('');
  div.querySelectorAll('input[type=range]').forEach((el) => {
    el.addEventListener('input', () => {
      const p = el.dataset.pod;
      rhoOverride[p] = parseFloat(el.value);
      const lbl = document.getElementById(`rv-${p}`);
      lbl.textContent = rhoOverride[p].toFixed(2);
      lbl.style.color = heatColor(rhoOverride[p]);
    });
    el.addEventListener('change', run);
  });
}
