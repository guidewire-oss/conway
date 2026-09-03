// The Usage analytics page (spec 016): the admin's read side of the telemetry.
// KPI cards, an actives/events line chart (d3, vendored), the plan-setup
// funnel, and a per-user activity table — sliceable by range (7/30/90 days).
// HEART-informed: Adoption = funnel, Engagement = daily actives, Retention =
// this week vs last week, Task success = schedules/baselines.

import { authFetch } from './auth.js';
import { esc } from './order.js';

let range = 30;

export function openUsage() {
  let ov = document.getElementById('usage-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'usage-overlay';
    ov.innerHTML = `
      <div id="usage-modal">
        <div class="guide-head"><h2>Usage analytics</h2>
          <span id="usage-range" class="usage-range">
            <button class="usage-btn" data-days="7">7d</button>
            <button class="usage-btn active" data-days="30">30d</button>
            <button class="usage-btn" data-days="90">90d</button>
          </span>
          <button id="usage-close">✕</button>
        </div>
        <div id="usage-kpis" class="usage-kpis"></div>
        <div id="usage-chart" class="usage-chart"></div>
        <div id="usage-funnel"></div>
        <div id="usage-users"></div>
        <p class="hint" id="usage-foot"></p>
      </div>`;
    document.body.appendChild(ov);
    ov.querySelector('#usage-close').addEventListener('click', closeUsage);
    ov.querySelectorAll('.usage-btn').forEach((b) =>
      b.addEventListener('click', () => {
        range = parseInt(b.dataset.days, 10);
        ov.querySelectorAll('.usage-btn').forEach((x) => x.classList.toggle('active', x === b));
        loadUsage();
      }));
  }
  ov.classList.add('open');
  loadUsage();
}

export function closeUsage() {
  document.getElementById('usage-overlay')?.classList.remove('open');
}

// fmtDate: short local date for the axis labels
const dayLabel = (d) => d.slice(5); // MM-DD

const KPI_DEFS = [
  { key: 'users', label: 'active users (range)' },
  { key: 'activeThisWeek', label: 'active this week' },
  { key: 'activeLastWeek', label: 'active last week' },
  { key: 'plansCreated', label: 'plans created' },
  { key: 'schedulesComputed', label: 'schedules computed' },
  { key: 'baselinesSaved', label: 'baselines saved' },
  { key: 'jiraImportsDone', label: 'Jira imports' },
  { key: 'totalEvents', label: 'total events' },
];

// render draws the KPI cards, the d3 line chart (daily actives + events),
// the funnel bars, and the per-user table from one aggregate response.
export function render(d) {
  const counts = d.eventCounts || {};
  const kpis = [
    { label: 'users active in range', value: (d.users || []).length },
    { label: 'active this week', value: d.activeThisWeek },
    { label: 'active last week', label2: 'retention', value: d.activeLastWeek },
    { label: 'plans created', value: counts.plan_created || 0 },
    { label: 'schedules computed', value: counts.schedules_computed || 0 },
    { label: 'baselines saved', value: counts.baselines_saved || 0 },
    { label: 'Jira imports', value: counts.jira_import_done || 0 },
    { label: 'total events', value: d.totalEvents },
  ];
  document.getElementById('usage-kpis').innerHTML =
    kpis.map((k) => `<div class="usage-kpi"><div class="usage-kpi-num">${esc(String(k.value))}</div><div class="usage-kpi-label">${esc(k.label)}</div></div>`).join('');

  drawLine(d.dailyActives || []);

  // The adoption funnel: distinct users per ordered setup step. The step-over-
  // step drop is the drop-off detector — a plan created but never scheduled is
  // the story without interpretation.
  const funnel = d.funnel || [];
  const maxU = Math.max(1, ...funnel.map((f) => f.users));
  document.getElementById('usage-funnel').innerHTML = funnel
    .map((f) => `<div class="usage-funnel-row">
      <span class="usage-funnel-label">${esc(f.event.replace(/_/g, ' '))}</span>
      <div class="usage-funnel-bar"><div style="width:${Math.round((f.users / maxU) * 100)}%"></div></div>
      <span class="usage-funnel-num">${f.users}</span>
    </div>`).join('');

  const users = d.users || [];
  document.getElementById('usage-users').innerHTML =
    `<table class="wip-table"><thead><tr><th>User</th><th>Events</th><th>Distinct features</th><th>Plans touched</th><th>Last seen</th></tr></thead>
     <tbody>${users.map((u) => `<tr>
        <td>${esc(u.user)}</td><td>${u.events}</td><td>${u.distinct}</td><td>${u.plans}</td>
        <td class="hint">${new Date(u.last).toLocaleString()}</td>
      </tr>`).join('') || '<tr><td colspan="5" class="hint">no user activity in this range</td></tr>'}</tbody></table>`;

  document.getElementById('usage-foot').textContent =
    `${d.totalEvents} events · ${d.from.slice(0, 10)} → ${d.to.slice(0, 10)}`;
}

// drawLine renders the daily actives + events series with vendored d3: two
// lines sharing an x axis, dots for non-zero days, and a hover title per dot.
export function drawLine(points) {
  const el = document.getElementById('usage-chart');
  if (!el) return;
  el.innerHTML = '';
  const w = el.clientWidth || 800, h = 220, pad = { t: 10, r: 12, b: 24, l: 36 };
  const n = Math.max(1, points.length);
  const x = (i) => pad.l + (i * (w - pad.l - pad.r)) / Math.max(1, n - 1 || 1);
  const maxV = Math.max(1, ...points.map((p) => Math.max(p.users, p.count)));
  const y = (v) => h - pad.b - (v / maxV) * (h - pad.t - pad.b);
  const svg = d3.select(el).append('svg').attr('width', w).attr('height', h);

  // gridlines
  for (let g = 0; g <= 4; g++) {
    const v = Math.round((maxV / 4) * g);
    svg.append('line').attr('x1', pad.l).attr('x2', w - pad.r)
      .attr('y1', y(v)).attr('y2', y(v))
      .attr('stroke', 'var(--border)').attr('opacity', 0.4);
    svg.append('text').attr('x', pad.l - 6).attr('y', y(v) + 4)
      .attr('text-anchor', 'end').attr('font-size', 10)
      .attr('fill', 'var(--muted)').text(v);
  }
  const line = (vals) => d3.line()(points.map((p, i) => [x(i), y(vals(p))]));
  svg.append('path').attr('d', line((p) => p.count))
    .attr('fill', 'none').attr('stroke', 'var(--accent)').attr('stroke-width', 1.5);
  svg.append('path').attr('d', line((p) => p.users))
    .attr('fill', 'none').attr('stroke', 'var(--bs-success)').attr('stroke-width', 1.5);
  // hover titles per point (simple, no crosshair machinery)
  points.forEach((p, i) => {
    svg.append('circle').attr('cx', x(i)).attr('cy', y(p.users)).attr('r', 2)
      .attr('fill', 'var(--bs-success)')
      .append('title').text(`${p.day}: ${p.users} active users, ${p.count} events`);
  });
  const ticks = [0, Math.floor(n / 2), n - 1];
  ticks.forEach((i) => {
    if (points[i]) svg.append('text').attr('x', x(i)).attr('y', h - 6)
      .attr('text-anchor', i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle')
      .attr('font-size', 10).attr('fill', 'var(--muted)')
      .text(points[i].day.slice(5));
  });
}

// loadUsage fetches the aggregate for the selected range and paints.
export async function loadUsage() {
  document.getElementById('usage-kpis').innerHTML = '<div class="hint">loading…</div>';
  const r = await authFetch(`/api/admin/analytics?days=${range}`);
  if (!r || !r.ok) {
    document.getElementById('usage-kpis').innerHTML =
      `<div class="plan-warn">analytics unavailable${r ? ` (${r.status})` : ''}</div>`;
    return;
  }
  const d = await r.json();
  render(d);
}

