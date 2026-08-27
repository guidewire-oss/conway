import { heatColor } from './graph.js';
import { apiGet, getJiraBaseUrl } from './data.js';

const hlp = (t) => (t ? ` <span class="help" data-tip="${t.replace(/"/g, '&quot;')}">?</span>` : '');
// '' until getJiraBaseUrl() resolves, or if CONWAY_JIRA_BASE_URL is unset —
// jiraLink() below falls back to plain (unlinked) text in that case.
let JIRA = '';
const jiraLink = (key) => (JIRA ? `<a href="${JIRA}${key}" target="_blank">${key}</a>` : key);
const CATS = [
  ['unsized', 'Unsized open tasks', 'forecasts fall back to the pod median'],
  ['stale', 'Stale In-Progress (>14d)', 'inflates WIP, queue heat, and the freeze list'],
  ['unassigned', 'Unassigned In-Progress', 'zombie work nobody owns'],
  ['nooutcome', 'Epics w/o business outcome', 'nobody can triage or freeze what has no stated why'],
];

const issues = {}; // per-pod problem lists, fetched lazily on drill-down
let openPod = null;
let outcomeStats = null;

let unassocEpics = []; // epics whose owner pod isn't a team in this snapshot

export function initHygiene(state) {
  getJiraBaseUrl().then((u) => { JIRA = u ? u + '/browse/' : ''; });
  loadOutcomeStats().then(() => render(state)).catch(() => {});
  render(state);
}

// Org epic counts + the unassociated-epic list are computed server-side from
// the snapshot tables (no epic_meta blob).
async function loadOutcomeStats() {
  const [stats, unassoc] = await Promise.all([apiGet('epic-stats'), apiGet('unassoc-epics')]);
  unassocEpics = (unassoc || []).map((e) => ({ key: e.key, name: e.name || '', pod: e.pod || '' }));
  const s = stats || {};
  outcomeStats = {
    known: s.known ?? 0,
    missing: s.missing ?? 0,
    noDue: s.noDue ?? 0,
    overdue: s.overdue ?? 0,
    unassoc: unassocEpics.length,
  };
}

function orgCards(state) {
  const pods = state.pods.filter((p) => state.hygiene[p.name]);
  const tot = (cat) => pods.reduce((s, p) => s + (issues[p.name]?.[cat]?.length ?? 0), 0);
  const avg = pods.length
    ? pods.reduce((s, p) => s + (state.hygiene[p.name].score ?? 0), 0) / pods.length : 0;
  return `
    <div class="stat"><div class="l">Org hygiene${hlp('Average of the measurable data-quality signals across pods (sized %, fresh board, assigned). Rules of Flow & Phoenix Project: you cannot manage flow you cannot see — this is how trustworthy the data feeding every forecast is.')}</div><div class="v">${(avg * 100).toFixed(0)}%</div>
      <div class="hint">avg across ${pods.length} pods</div></div>
    ${CATS.map(([cat, label, why]) => `
      <div class="stat"><div class="l">${label}${hlp(`Count of issues in this category — ${why}. Cleaning these sharpens every forecast and freeze decision.`)}</div><div class="v">${tot(cat)}</div>
      <div class="hint">issues to fix</div></div>`).join('')}
    ${outcomeStats ? `
      <div class="stat">
      <div class="l">Epics w/o business outcome${hlp('Epics whose description has no stated why / cost-of-delay / success metric. The Goal & full-kit (Rules of Flow): work with no business outcome can\'t be prioritised, triaged, or honestly committed.')}</div>
      <div class="v">${outcomeStats.missing}<span class="hint" style="font-size:13px">/${outcomeStats.known}</span></div>
      <div class="hint">all in-flight epics · drill via pod rows</div></div>
      <div class="stat">
      <div class="l">Epics past due date${hlp('In-flight epics whose committed due date has passed. Trust (Rules of Flow): a missed date erodes trust and tends to turn future asks into escalations rather than plannable work.')}</div>
      <div class="v">${outcomeStats.overdue}</div>
      <div class="hint">${outcomeStats.noDue} of ${outcomeStats.known} have no due date at all</div></div>
      ${outcomeStats.unassoc ? `<div class="stat" style="border-color:var(--amber)">
      <div class="l">Epics with no team${hlp('Imported epics whose pod field is empty or names a team not in this snapshot\'s roster. They can\'t be placed on the network or forecast — fix the Jira pod field, or add the team to the roster and re-associate.')}</div>
      <div class="v" style="color:var(--amber)">${outcomeStats.unassoc}</div>
      <div class="hint">unmatched — see list below</div></div>` : ''}` : ''}`;
}

// cleanup panel: epics the import couldn't tie to a team in this snapshot.
function unassocPanel() {
  let el = document.getElementById('hygiene-unassoc');
  if (!el) {
    el = document.createElement('div');
    el.id = 'hygiene-unassoc';
    document.getElementById('hygiene-cards').after(el);
  }
  if (!unassocEpics.length) { el.innerHTML = ''; return; }
  const rows = unassocEpics.slice(0, 200).map((e) => `<tr>
    <td>${jiraLink(e.key)}</td>
    <td>${(e.name || '').replace(/[&<>]/g, '')}</td>
    <td class="hint">${e.pod ? `pod “${e.pod}” not in roster` : 'no pod field'}</td></tr>`).join('');
  el.innerHTML = `<div class="panel-card" style="margin-top:14px">
    <h3>Epics with no team <span class="hint">— ${unassocEpics.length} couldn’t be matched (Jira-import cleanup)</span></h3>
    <p class="hint">Either the Jira <b>pod field</b> is empty, or it names a team not in this snapshot’s roster. Fix the field in Jira, or add the team in Observe ▸ 👥 Rosters and re-associate the snapshot in Observe ▸ 🗂 Snapshots.</p>
    <table class="wip-table sortable"><thead><tr><th>Epic</th><th>Title</th><th>Why</th></tr></thead><tbody>${rows}</tbody></table>
    ${unassocEpics.length > 200 ? `<p class="hint">showing 200 of ${unassocEpics.length}</p>` : ''}</div>`;
}

function render(state) {
  document.getElementById('hygiene-cards').innerHTML = orgCards(state);
  unassocPanel();
  const pct = (v) => (v == null ? '<span class="hint">n/a</span>' : `${(v * 100).toFixed(0)}%`);
  const rows = state.pods
    .map((p) => ({ p, h: state.hygiene[p.name] }))
    .filter(({ h }) => h)
    .sort((a, b) => (a.h.score ?? 1) - (b.h.score ?? 1));
  const flagsFor = (h) => {
    const out = [];
    if (h.sizedPct != null && h.sizedPct < 0.5) out.push('<span class="flag red">mostly unsized</span>');
    if (h.staleWipPct != null && h.staleWipPct > 0.5) out.push('<span class="flag red">stale board</span>');
    if (h.unassignedWipPct != null && h.unassignedWipPct > 0.3) out.push('<span class="flag amber">zombie WIP</span>');
    if (h.linkDensity != null && h.linkDensity < 0.05) out.push('<span class="flag amber">links unused</span>');
    return out.join(' ');
  };
  document.getElementById('hygiene-table').innerHTML = `
    <thead><tr>
    <th>Pod${hlp('The team. Click a row to drill: category → the individual Jira issues to fix.')}</th>
    <th>Hygiene${hlp('Composite 0–100% of the measurable signals in this row. Phoenix Project: making work visible is the first step — low hygiene means the model (and you) are flying partly blind.')}</th>
    <th>Sized${hlp('% of recent epic child tasks carrying an estimate (of N sampled). Unsized work forces forecasts to fall back to the pod\'s median item, widening every date.')}</th>
    <th>Median pts${hlp('This pod\'s median story-point value. Used to normalize sizes per pod, so a "5" from a days-pointing team and a complexity-pointing team each map to that team\'s own scale — teams need not agree on units, only be self-consistent.')}</th>
    <th>Stale WIP &gt;14d${hlp('% of in-progress items untouched for >14 days. The Goal: this is hidden inventory — it inflates WIP and queue heat and pollutes the freeze list while looking like progress.')}</th>
    <th>Unassigned WIP${hlp('% of in-progress items with no assignee — zombie work nobody owns. The model otherwise counts it as load the team isn\'t really carrying.')}</th>
    <th>Link density${hlp('Issues with a blocking link ÷ resolved issues. Conway: low density means real cross-team coupling is invisible in the data, so the dependency graph and suggestions under-count it.')}</th>
    <th>Fix first${hlp('Auto-flags pointing at the cheapest high-impact cleanup for this pod.')}</th></tr></thead>
    <tbody>${rows.map(({ p, h }) => `<tr class="hyg-row" data-pod="${p.name}" style="cursor:pointer" title="click to drill down">
      <td>▸ ${p.name}</td>
      <td><span class="bar" style="width:${(h.score ?? 0) * 60}px;background:${heatColor(1.05 - (h.score ?? 0))}"></span> ${pct(h.score)}</td>
      <td>${pct(h.sizedPct)} <span class="hint">of ${h.sampleSized}</span></td>
      <td>${h.medianPoints ?? '<span class="hint">n/a</span>'}</td>
      <td>${pct(h.staleWipPct)}</td>
      <td>${pct(h.unassignedWipPct)}</td>
      <td>${pct(h.linkDensity)}</td>
      <td>${flagsFor(h)}</td>
    </tr>
    <tr class="hyg-drill" data-pod="${p.name}" hidden><td colspan="8"><div class="wip-drill" id="hyg-${p.name}"></div></td></tr>`).join('')}</tbody>`;
  document.querySelectorAll('.hyg-row').forEach((row) => {
    row.addEventListener('click', () => toggle(row.dataset.pod));
  });
  if (openPod) toggle(openPod, true);
}

async function toggle(pod, force = false) {
  const tr = document.querySelector(`.hyg-drill[data-pod="${pod}"]`);
  if (!tr) return;
  if (!tr.hidden && !force) { tr.hidden = true; openPod = null; return; }
  document.querySelectorAll('.hyg-drill').forEach((x) => { x.hidden = true; });
  tr.hidden = false;
  openPod = pod;
  // per-pod problem lists are computed in SQL, fetched once and cached
  if (!issues[pod]) {
    document.getElementById(`hyg-${pod}`).innerHTML = '<p class="hint">loading…</p>';
    issues[pod] = (await apiGet(`hygiene-issues?pod=${encodeURIComponent(pod)}`)) || {};
  }
  renderCats(pod, null);
}

function renderCats(pod, openCat) {
  const div = document.getElementById(`hyg-${pod}`);
  const data = issues[pod] ?? {};
  div.innerHTML = `
    <div style="margin:6px 0">${CATS.map(([cat, label, why]) => {
    const n = data[cat]?.length ?? 0;
    const active = cat === openCat;
    return `<button class="hyg-cat ${active ? 'primary' : ''}" data-cat="${cat}" ${n ? '' : 'disabled'}
        style="margin-right:8px">${label}: <b>${n}</b></button>
        ${active ? `<span class="hint">— ${why}</span>` : ''}`;
  }).join('')}</div>
    <div id="hyg-issues-${pod}"></div>`;
  div.querySelectorAll('.hyg-cat').forEach((b) => b.addEventListener('click', (ev) => {
    ev.stopPropagation();
    renderCats(pod, b.dataset.cat === openCat ? null : b.dataset.cat);
  }));
  if (openCat && data[openCat]?.length) {
    document.getElementById(`hyg-issues-${pod}`).innerHTML = `
      <table class="wip-table sortable"><thead><tr><th>Issue</th><th>Summary</th><th>Problem</th></tr></thead>
      <tbody>${data[openCat].map((i) => `<tr>
        <td>${jiraLink(i.key)}</td>
        <td>${i.summary}</td>
        <td class="hint">${i.detail}</td>
      </tr>`).join('')}</tbody></table>`;
  }
}
