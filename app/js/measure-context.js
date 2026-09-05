const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

export const MEASURE_VIEWS = {
  home: 'The Measure cards summarize this snapshot. Plan checks identify their own working plan separately.',
  network: 'Org Network shows teams and blocking relationships from this snapshot. Organization what-ifs use the same source with your scenario changes.',
  scoreboard: 'WIP Scoreboard shows work in progress, throughput, cycle times and queue load from this dated capture.',
  hygiene: 'Data Quality reports missing or incomplete Jira evidence in this snapshot. It does not audit your uploaded plan workbook.',
  simulator: 'Feature Simulator uses this snapshot’s team statistics with the task scenario labeled below. It does not load the open Excel plan.',
  flow: 'Levers models organization changes against this snapshot. Results are scenarios; the captured data remains unchanged.',
};

export function snapshotSelectionURL(href, id) {
  const url = new URL(href);
  url.searchParams.set('snapshot', id);
  return url.href;
}

export function notifyMeasureSourcesChanged() {
  document.dispatchEvent(new Event('conway:measure-sources-changed'));
}

function sourceLabel(source) {
  if (source === 'jira') return 'Jira import';
  if (source === 'baseline' || source === 'template') return 'Example snapshot';
  return 'Source type unknown';
}

function capturedAt(value) {
  if (!Number.isFinite(value) || value <= 0) return 'Capture date unavailable';
  return new Date(value * 1000).toLocaleString();
}

// specs/021-measure-source-context.md:97: source identity is useful even when
// there is no second snapshot to switch to. Missing evidence stays explicit.
export function measureContextHTML({ snapshots = [], rosters = [], selectedId, state = {}, canManage = false,
  loading = false, error = '', rosterError = '', view = 'network' } = {}) {
  const selected = snapshots.find(snapshot => snapshot.id === selectedId);
  const roster = rosters.find(item => item.id === selected?.rosterId);
  const name = selected?.name || selected?.id || (loading ? 'Loading source details…' : 'Selected snapshot unavailable');
  const example = ['baseline', 'template'].includes(selected?.source);
  const pods = state.pods || [];
  const synthetic = pods.filter(pod => state.stats?.[pod.name]?.synthetic).length;
  const unknown = pods.filter(pod => !state.stats?.[pod.name]).length;
  const statistics = !pods.length ? 'No team data loaded.' : example
    ? 'Example data, not an observation of your organization.'
    : synthetic || unknown ? `${synthetic} of ${pods.length} teams use synthetic estimates; ${unknown} have unknown statistics.`
      : selected?.source === 'jira' ? `Historical team statistics loaded for ${pods.length} teams.`
        : 'Team statistics are loaded, but source provenance is unavailable.';
  const rosterLabel = !selected ? 'Roster association unavailable' : roster?.name || (selected.rosterId ? 'Associated roster name unavailable' : 'No saved roster associated');
  const scope = Array.isArray(selected?.scope) && selected.scope.length ? selected.scope.join(', ') : 'Project scope unavailable';
  return `<div class="measure-source-heading"><div><span class="hint">Measure data source</span><h2>${esc(name)}</h2></div>
    <span class="badge ${example || synthetic || unknown || !pods.length || !selected ? 'warn' : 'ok'}">${esc(sourceLabel(selected?.source))}</span></div>
    <p class="measure-view-purpose">${esc(MEASURE_VIEWS[view] || '')}</p>
    ${error ? `<p role="alert">${esc(error)}</p>` : ''}
    ${!loading && !error && !snapshots.length ? '<p>No snapshots are available. Import a dated capture to measure delivery.</p>' : ''}
    <dl class="measure-source-details"><div><dt>Captured</dt><dd>${esc(capturedAt(selected?.createdAt))}</dd></div>
      <div><dt>Jira projects</dt><dd>${esc(scope)}</dd></div>
      <div><dt>Associated roster</dt><dd>${esc(rosterLabel)}</dd></div>
      <div><dt>Data status</dt><dd>${esc(statistics)}</dd></div></dl>
    ${rosterError ? `<p class="hint" role="status">${esc(rosterError)}</p>` : ''}
    <div class="measure-source-actions"><label for="measure-snapshot">Viewing org snapshot
      <select id="measure-snapshot"${loading || !snapshots.length || (snapshots.length === 1 && selected) ? ' disabled' : ''}>
        ${!selected ? '<option value="">Select an available snapshot</option>' : ''}
        ${snapshots.map(snapshot => `<option value="${esc(snapshot.id)}"${snapshot.id === selectedId ? ' selected' : ''}>${esc(snapshot.name || snapshot.id)} · ${esc(sourceLabel(snapshot.source))} · ${esc(capturedAt(snapshot.createdAt))}</option>`).join('')}
      </select></label>
      ${canManage ? '<button type="button" data-measure-action="import">Import new snapshot</button><button type="button" data-measure-action="associations">Snapshot &amp; roster associations</button><button type="button" data-measure-action="rosters">Manage rosters</button>' : ''}
      ${error || rosterError ? '<button type="button" data-measure-action="retry">Retry source details</button>' : ''}</div>
    <p class="hint">${snapshots.length === 1 ? 'One snapshot is available. ' : ''}Changing the snapshot reloads all Measure views and resets the simulator’s unsaved scenario.</p>
    <details class="measure-association-help"><summary>How this connects to a plan</summary>
      <p>Measure reads a dated snapshot. A plan uses its own roster, estimates and scheduling assumptions. Sharing a roster does not bind initiatives to Jira work.</p>
      <ol><li>Open the plan’s <b>Execution</b> view and select its <b>Execution snapshot</b>.</li><li>Confirm Jira epic keys under each initiative’s <b>Epic bindings</b>.</li><li>Use a saved agreement to compare planned dates with observed delivery.</li></ol>
      <p>Execution has its own snapshot selection. Switching Measure does not change plan inputs or those bindings. Snapshot roster associations join teams by name. ${canManage ? 'Use Snapshot &amp; roster associations to inspect or change them.' : 'A manager can manage roster associations and plan bindings.'}</p>
      ${canManage ? '<button type="button" data-measure-action="plans">Open plans</button>' : ''}</details>`;
}

export function simulatorSourceHTML({ kind = 'example', epic = '', edited = false, dirty = false } = {}) {
  const origin = kind === 'epic' ? `Imported epic ${epic}` : kind === 'example' ? 'Example tasks' : 'Entered tasks';
  return `<b>Task scenario: ${esc(edited ? `Edited scenario — started from ${origin}` : origin)}</b>
    <span>${kind === 'example' ? 'Illustrative tasks, not imported delivery work. ' : ''}Team statistics come from the Measure snapshot above. This scenario is not associated with a saved plan.</span>
    ${dirty ? '<strong role="status">Inputs changed. Previous results are stale; run the simulation again.</strong>' : ''}`;
}

export function simulatorTeamOptionsHTML(pods, selected) {
  const missing = !pods.some(pod => pod.name === selected);
  return (missing ? `<option value="${esc(selected)}" selected>${selected ? `${esc(selected)} (not in snapshot roster)` : 'Choose a team — assignment missing'}</option>` : '') +
    pods.map(pod => `<option${pod.name === selected ? ' selected' : ''}>${esc(pod.name)}</option>`).join('');
}

export function mountMeasureContext(host, { selectedId, state, canManage, request, onSelect, actions = {}, view = 'home' }) {
  let model = { selectedId, state, canManage, view, loading: true };
  let ticket = 0;
  const paint = () => { host.hidden = !Object.hasOwn(MEASURE_VIEWS, model.view); host.innerHTML = measureContextHTML(model); };
  const read = async path => {
    const response = await request(path);
    if (!response?.ok) throw new Error('Source details could not be loaded. Retry to inspect this snapshot.');
    const result = await response.json();
    if (!Array.isArray(result)) throw new Error('Source details were unreadable. Retry to inspect this snapshot.');
    return result;
  };
  const refresh = async () => {
    const mine = ++ticket;
    model = { ...model, loading: true, error: '', rosterError: '' }; paint();
    const [snapshots, rosters] = await Promise.allSettled([read('/api/snapshots'), canManage ? read('/api/rosters') : Promise.resolve([])]);
    if (mine !== ticket) return;
    model = { ...model, loading: false,
      snapshots: snapshots.status === 'fulfilled' ? snapshots.value : [],
      rosters: rosters.status === 'fulfilled' ? rosters.value : [],
      error: snapshots.status === 'rejected' ? 'Snapshot details could not be loaded. Retry to identify the data shown below.' : '',
      rosterError: rosters.status === 'rejected' ? 'Roster details could not be loaded. The existing snapshot association has not changed.' : '' };
    paint();
  };
  host.addEventListener('change', event => {
    if (event.target.id === 'measure-snapshot' && event.target.value && event.target.value !== selectedId &&
      (model.snapshots || []).some(snapshot => snapshot.id === event.target.value)) onSelect(event.target.value);
  });
  host.addEventListener('click', event => {
    const action = event.target.closest('[data-measure-action]')?.dataset.measureAction;
    if (action === 'retry') refresh();
    else if (canManage && action && actions[action]) actions[action]();
  });
  return { ready: refresh(), refresh, setView(next) { model.view = next; paint(); } };
}
