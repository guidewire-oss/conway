// Snapshot evidence and review decisions. Missing measurements stay unknown.
// specs/017-planning-and-execution-usability.md:87
import { esc, weekToDate } from './order.js';
import { icon } from './icons.js';

const num = (n, suffix = '') => Number.isFinite(n) ? `${Math.round(n * 10) / 10}${suffix}` : 'Unknown';
const delta = n => Number.isFinite(n) ? `${n > 0 ? '+' : ''}${num(n)} weeks` : 'Unknown';
const when = ts => Number.isFinite(ts) && ts > 0 && Number.isFinite(new Date(ts * 1000).getTime()) ? new Date(ts * 1000).toLocaleString() : 'Unknown capture time';
const dateWeek = (n, start) => Number.isFinite(n) ? `${weekToDate(n, start) || 'Undated'} (week ${num(n)})` : 'Unknown';
const gapsHTML = gaps => (gaps || []).length ? `<ul class="hint">${gaps.map(g => `<li>${esc(g)}</li>`).join('')}</ul>` : '';
const statuses = {'not-tracked':'Not tracked', unknown:'Unknown', 'on-track':'On track (inferred)', 'at-risk':'At risk (inferred)', late:'Late / buffer exhausted (inferred)'};

function slicesHTML(slices, start) {
  return `<div class="execution-table-wrap" tabindex="0" aria-label="Team execution evidence, scroll horizontally for all measures"><table class="wip-table"><thead><tr><th scope="col">Team</th><th scope="col">Completed children</th><th scope="col">Agreed start / finish</th><th scope="col">Inferred start / actual finish</th><th scope="col">Start / finish variance</th><th scope="col">Elapsed-time estimate variance</th><th scope="col">Estimated remaining / conditional finish</th><th scope="col">Buffer used</th><th scope="col">Evidence</th></tr></thead><tbody>${slices.map(s => `<tr>
    <th scope="row">${esc(s.pod)}</th><td>${num(s.percentComplete, '%')}<br><small>${num(s.doneCount)} / ${num(s.issueCount)} children</small></td>
    <td>${esc(dateWeek(s.baselineStartWeek,start))}<br>${esc(dateWeek(s.baselineFinishWeek,start))}</td>
    <td>${esc(dateWeek(s.actualStartWeek,start))}<small>${Number.isFinite(s.actualStartWeek) ? s.startInferred ? "Inferred from issue activity" : "Start provenance not supplied" : "Start not measured"}</small><br>${esc(dateWeek(s.actualFinishWeek,start))}<small>${Number.isFinite(s.actualFinishWeek) ? "Resolution timestamps in snapshot" : "Finish not measured"}</small></td>
    <td>${delta(s.startVarianceWeeks)}<br>${delta(s.finishVarianceWeeks)}</td><td>${num(s.estimateVariancePct,'%')}<br><small>Calendar duration proxy</small></td>
    <td>${num(s.remainingWeeks,' calendar weeks remaining')}<br>${esc(dateWeek(s.forecastFinishWeek,start))}<small>${esc(s.forecastBasis || 'No remaining-work forecast basis supplied')}</small></td>
    <td>${num(s.bufferUsedPct,'%')}<br>${esc(statuses[s.status] || 'Unknown')}</td><td>${esc(s.confidence || 'unknown')} confidence${gapsHTML(s.gaps)}</td>
  </tr>`).join('')}</tbody></table></div>`;
}

export function executionEvidenceHTML(data, {team = '', plan = {}} = {}) {
  const c = data.coverage || {};
  const snap = data.snapshot || {};
  const base = data.baseline;
  const start = base?.periodStart || '';
  const assignedTeams = (it) => Object.entries((plan.initiatives || []).find(p => p.name === it.name)?.work || {}).filter(([,slice]) => slice.inPath).map(([pod]) => pod);
  const initiatives = (data.initiatives || []).filter(it => !team || [...(it.slices || []), ...(it.originalScopeSlices || [])].some(s => s.pod === team) || assignedTeams(it).includes(team));
  return `<div class="execution-evidence">
    <h3>Execution against the agreed plan</h3>
    ${snap.source === 'template' || snap.source === 'baseline' ? '<p class="plan-warn">Synthetic example evidence: this snapshot is a scenario or shipped demo, not an observation of your organization. Select a Jira import for a delivery review.</p>' : !snap.source ? '<p class="hint">Snapshot source type is unknown. Confirm its provenance before drawing delivery conclusions.</p>' : ''}
    <p><b>Snapshot:</b> ${esc(snap.name || snap.id || 'Unknown')} · ${esc(when(snap.createdAt))} · ${num(snap.ageDays)} days old. This is a saved capture, not live Jira.</p>
    <p><b>Agreement:</b> ${base ? `${esc(base.name)} · saved ${esc(when(base.createdAt))}` : 'No active baseline. Save an agreement in Plan commitments to compare variance.'}</p>
    <p role="status"><b>${num(c.tracked)} of ${num(c.total)} initiatives have evidence</b> · ${num(c.bound)} explicitly bound. ${team ? `Showing team ${esc(team)}; coverage above is for the whole plan.` : 'All initiatives are included, even when untracked.'}</p>
    <p class="hint">Progress counts completed child issues, not effort delivered. Starts are inferred from issue activity; elapsed-time variance and calibration are proxies. Missing transition history cannot establish exact work starts. Use these signals to investigate with teams.</p>
    ${gapsHTML(data.gaps)}
    ${initiatives.map((it, index) => `<article class="panel-card execution-initiative">
      <h4>${esc(it.name)} <span class="tag">${esc(statuses[it.status] || (it.tracked ? 'Evidence available' : 'Not tracked'))}</span></h4>
      <p>${num(it.percentComplete,'%')} completed children · start variance ${delta(it.startVarianceWeeks)} · finish variance ${delta(it.finishVarianceWeeks)} · buffer ${num(it.bufferUsedPct,'%')}</p>
      ${(it.addedEpics || []).length || (it.removedEpics || []).length || (it.unplannedPods || []).length ? `<p class="plan-warn">Scope changed: added epics ${esc(it.addedEpics?.join(', ') || 'none')}; removed epics ${esc(it.removedEpics?.join(', ') || 'none')}; unplanned teams ${esc(it.unplannedPods?.join(', ') || 'none')}. Compare original scope separately below.</p>` : ''}
      ${gapsHTML(it.gaps)}
      ${(it.slices || []).length ? slicesHTML(it.slices.filter(s => !team || s.pod === team),start) : '<p class="hint">No child-issue evidence. Bind epics below or import a snapshot containing their children.</p>'}
      ${(it.originalScopeSlices || []).length ? `<details><summary>Original agreed scope</summary>${slicesHTML(it.originalScopeSlices.filter(s=>!team || s.pod === team),start)}</details>` : ''}
      <details><summary>Epic bindings · ${esc((it.epicKeys || []).join(', ') || 'None')}</summary>
        <form class="execution-binding" data-initiative="${esc(it.name)}"><label for="epic-keys-${index}">Confirmed Jira epic keys</label>
          <input id="epic-keys-${index}" name="epicKeys" value="${esc((it.epicKeys || []).join(', '))}" placeholder="PROJ-123, PROJ-456" autocomplete="off">
          <p class="hint">Comma-separated keys. Saving changes the working plan; the agreed baseline retains its original scope.</p>
          ${(it.suggestions || []).length ? `<p>Suggested name matches — select and save to confirm:</p>${it.suggestions.map(s => `<label class="execution-suggestion"><input type="checkbox" name="suggestion" value="${esc(s.key)}">${esc(s.key)} — ${esc(s.summary)}</label>`).join('')}` : ''}
          <button type="submit">${icon('save')}Save confirmed bindings</button><p class="execution-binding-status" role="status" aria-live="polite"></p>
        </form>
      </details>
    </article>`).join('') || '<p>No initiatives have evidence for this team. Clear the team filter to see untracked initiatives.</p>'}
    <details class="panel-card"><summary>Order adherence and calibration</summary>
      <p>Inferred order adherence: ${num(data.adherence?.percent,'%')} (${num(data.adherence?.followed)} of ${num(data.adherence?.compared)} comparable team pairs). Out-of-order teams: ${esc(data.adherence?.outOfOrderPods?.join(', ') || 'None identified; missing evidence may limit comparison')}.</p>
      <p class="hint">Calibration compares elapsed calendar duration with estimates. It is not measured effort or an individual performance score. Review sample count and scope before testing a change in a scenario copy.</p>
      <div class="execution-table-wrap" tabindex="0" aria-label="Calibration table, scroll horizontally"><table class="wip-table"><thead><tr><th scope="col">Team</th><th scope="col">Observed duration / estimate</th><th scope="col">Samples</th></tr></thead><tbody>${(data.calibration || []).map(c => `<tr><td>${esc(c.pod)}</td><td>${num(c.factor,'×')} (inferred)</td><td>${num(c.sampleCount)}</td></tr>`).join('') || '<tr><td colspan="3">No comparable completed slices.</td></tr>'}</tbody></table></div>
    </details>
  </div>`;
}

export function decisionsHTML(decisions) {
  return (decisions || []).map(d => `<article class="execution-decision"><h4>${esc(d.action)}</h4><p>${esc(d.owner)} · review ${esc(d.reviewDate)}${d.initiative ? ` · ${esc(d.initiative)}` : ''}</p><p>${esc(d.rationale)}</p><small>Recorded ${esc(when(d.createdAt))} by ${esc(d.createdBy)} · snapshot ${esc(d.snapshotId || 'not selected')} · baseline ${esc(d.baselineId || 'none')}</small></article>`).join('') || '<p class="hint">No decisions recorded yet.</p>';
}

export function confirmedEpicKeys(form) {
  const keys = String(form.get('epicKeys') || '').split(/[\s,;]+/).filter(Boolean).concat(form.getAll('suggestion')).map(k => k.trim().toUpperCase());
  if (keys.some(k => !/^[A-Z][A-Z0-9_]*-[0-9]+$/.test(k))) throw new Error('Use Jira epic keys such as PROJ-123, separated by commas.');
  return [...new Set(keys)];
}

export async function mountExecution(host, {plan, request, onBindingsSaved, onImport, onAgreement, onSnapshot, onTeam}) {
  let ticket = 0, data = null, snapshots = [], mountedRoot;
  const requestedTeam = new URL(location.href).searchParams.get('team') || '';
  const drafts = new Map();
  const draftFrom = form => ({ text: form.elements.epicKeys.value, suggestions: [...form.querySelectorAll('[name="suggestion"]')].filter(input => input.checked).map(input => input.value) });
  const live = () => host.isConnected && host.querySelector('.execution-review') === mountedRoot;
  const json = async (url, options) => {
    const r = await request(url, options);
    if (!r?.ok) throw new Error(r ? (await r.text()).slice(0,250) || `Server returned ${r.status}; please retry.` : 'Could not reach the server. Try again.');
    return r.json();
  };
  host.innerHTML = `<section class="execution-review"><div class="row-actions"><label>Execution snapshot <select id="execution-snapshot"><option value="">Loading snapshots…</option></select></label><button id="execution-refresh">Refresh saved evidence</button><button id="execution-import">${icon('upload')}Import new snapshot</button><button id="execution-agreement">Review agreement</button><label>Team <select id="execution-team"><option value="">All teams, including untracked</option></select></label></div><p id="execution-status" role="status" aria-live="polite"></p><div id="execution-evidence"></div>
    <section class="panel-card"><h3>Record the next action</h3><p class="hint">Capture a decision with an owner and review date. This app records the action; it does not send notifications or change Jira.</p><form id="execution-decision-form" class="execution-decision-form">
      <label>Action <input name="action" required maxlength="1000"></label><label>Owner <input name="owner" required maxlength="200"></label><label>Review date <input name="reviewDate" type="date" required></label>
      <label>Initiative <select name="initiative"><option value="">Whole plan</option>${(plan.initiatives || []).map(it=>`<option>${esc(it.name)}</option>`).join('')}</select></label>
      <label class="execution-rationale">Rationale / evidence <textarea name="rationale" required maxlength="10000" rows="3"></textarea></label><button type="submit">${icon('save')}Record decision</button><p id="execution-decision-status" role="status" aria-live="polite"></p></form><h3>Previous decisions</h3><div id="execution-decisions">Loading decisions…</div></section></section>`;
  mountedRoot = host.querySelector('.execution-review');
  const select = host.querySelector('#execution-snapshot'), teamSelect = host.querySelector('#execution-team'), status = host.querySelector('#execution-status');
  const paint = () => {
    if (!live()) return;
    host.querySelector('#execution-evidence').innerHTML = data ? executionEvidenceHTML(data,{team:teamSelect.value,plan}) : '';
    host.querySelectorAll('.execution-binding').forEach(form => {
      const name = form.dataset.initiative;
      const draft = drafts.get(name);
      if (draft) {
        form.elements.epicKeys.value = draft.text;
        form.querySelectorAll('[name="suggestion"]').forEach(input => { input.checked = draft.suggestions.includes(input.value); });
        form.closest('details').open = true;
        form.querySelector('[role=status]').textContent = 'Unsaved bindings retained. Save to apply them to the working plan.';
      }
      const retain = () => { drafts.set(name, draftFrom(form)); form.querySelector('.execution-binding-status').textContent = 'Unsaved bindings'; };
      form.addEventListener('input', retain); form.addEventListener('change', retain);
      form.addEventListener('submit', async ev => {
        ev.preventDefault(); const button = form.querySelector('button'), note = form.querySelector('.execution-binding-status');
        if (button.disabled) return;
        drafts.set(name, draftFrom(form));
        const pendingDraft = drafts.get(name);
        try {
          const epicKeys = confirmedEpicKeys(new FormData(form)); button.disabled = true; note.setAttribute('role','status'); note.textContent = 'Saving bindings…';
          const result = await json(`/api/plan/${encodeURIComponent(plan.id)}/initiatives`,{method:'PATCH',body:JSON.stringify({initiatives:[{name,epicKeys}]})});
          if (!live()) return;
          // Preserve edits typed after this request was dispatched.
          if (JSON.stringify(drafts.get(name)) === JSON.stringify(pendingDraft)) drafts.delete(name);
          onBindingsSaved?.(result); await refresh();
          if (live()) status.textContent += ` Bindings saved for ${name}.${drafts.has(name) ? ' Newer unsaved edits remain in the form.' : ''}`;
        } catch (e) { if(live()) { note.textContent = `Could not save: ${e.message}. Your input is retained.`; note.setAttribute('role','alert'); } }
        finally { button.disabled = false; }
      });
    });
  };
  async function refresh() {
    if (!live()) return;
    const mine = ++ticket; data = null; paint(); status.setAttribute('role','status');
    if (!select.value) { status.textContent = 'Choose a snapshot, or import one to review execution.'; return; }
    status.textContent = 'Loading snapshot evidence…';
    try {
      const result = await json(`/api/plan/${encodeURIComponent(plan.id)}/actuals?snapshot=${encodeURIComponent(select.value)}`);
      if (!live() || mine !== ticket) return;
      data = {...result, snapshot: {...snapshots.find(snapshot => snapshot.id === select.value), ...result.snapshot}}; status.textContent = 'Saved evidence loaded. To capture newer Jira data, choose Import new snapshot.';
      const prior = teamSelect.dataset.chosen === '1' ? teamSelect.value : requestedTeam;
      const teams = [...new Set([...(data.initiatives || []).flatMap(it=>[...(it.slices || []), ...(it.originalScopeSlices || [])].map(s=>s.pod)), ...(plan.initiatives || []).flatMap(it=>Object.entries(it.work || {}).filter(([,slice]) => slice.inPath).map(([pod]) => pod))])].filter(Boolean).sort();
      teamSelect.innerHTML = '<option value="">All teams, including untracked</option>' + teams.map(t=>`<option>${esc(t)}</option>`).join('');
      if (teams.includes(prior)) teamSelect.value = prior;
      paint();
    } catch (e) { if(live() && mine === ticket) { status.textContent = `Execution evidence unavailable: ${e.message}`; status.setAttribute('role','alert'); } }
  }
  select.addEventListener('change',()=>{onSnapshot?.(select.value); refresh();});
  teamSelect.addEventListener('change',()=>{teamSelect.dataset.chosen='1'; onTeam?.(teamSelect.value); paint();});
  host.querySelector('#execution-refresh').addEventListener('click',refresh);
  host.querySelector('#execution-import').addEventListener('click',()=>onImport?.());
  host.querySelector('#execution-agreement').addEventListener('click',()=>onAgreement?.());
  async function loadDecisions() {
    try { const result = await json(`/api/plan/${encodeURIComponent(plan.id)}/decisions`); if(live()) host.querySelector('#execution-decisions').innerHTML = decisionsHTML(result.decisions); }
    catch(e) { if(live()) host.querySelector('#execution-decisions').textContent = `Decision history unavailable: ${e.message}`; }
  }
  host.querySelector('#execution-decision-form').addEventListener('submit',async ev=>{
    ev.preventDefault(); const form=ev.currentTarget, button=form.querySelector('button'), note=host.querySelector('#execution-decision-status');
    if (button.disabled) return;
    if (!data) { note.textContent='Load execution evidence before recording a decision, so its source is retained.'; return; }
    button.disabled=true; note.setAttribute('role','status'); note.textContent='Recording decision…';
    try {
      const values=Object.fromEntries(new FormData(form));
      await json(`/api/plan/${encodeURIComponent(plan.id)}/decisions`,{method:'POST',body:JSON.stringify({...values,snapshotId:data.snapshot.id,baselineId:data.baseline?.id || ''})});
      if(!live()) return; form.reset(); note.textContent='Decision recorded. Previous decisions are preserved.'; await loadDecisions();
    } catch(e) { note.textContent=`Could not record decision: ${e.message}`; note.setAttribute('role','alert'); }
    finally {button.disabled=false;}
  });
  const decisions = loadDecisions();
  try {
    snapshots = await json('/api/snapshots'); if(!live()) return;
    select.innerHTML = '<option value="">Choose a snapshot</option>' + (snapshots || []).map(s=>`<option value="${esc(s.id)}">${esc(s.name || s.id)} · ${esc(s.source === 'jira' ? 'Jira import' : s.source === 'baseline' || s.source === 'template' ? 'Synthetic example' : 'Source unknown')} · ${esc(when(s.createdAt))}</option>`).join('');
    const requested = new URL(location.href).searchParams.get('executionSnapshot');
    if((snapshots || []).some(s=>s.id === requested)) select.value=requested;
    else if(snapshots?.length) select.value=[...snapshots].sort((a,b)=>(Number(b.source === 'jira') - Number(a.source === 'jira')) || (b.createdAt || 0) - (a.createdAt || 0))[0].id;
    await refresh();
  } catch(e) { if(live()) { select.innerHTML='<option value="">Snapshots unavailable</option>'; status.textContent=`Could not load snapshots: ${e.message}`; status.setAttribute('role','alert'); } }
  await decisions;
}
