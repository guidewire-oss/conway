// Operational release decisions reuse the accepted schedule, not a second planner.
// specs/025-team-ready-work-queue.md:263
import {esc, weekToDate} from './order.js';
import {initiativeMatch} from './filter.js';

const groups = [
  ['in_progress', 'In progress', 'Reported initiative carryover; team activity has not been independently confirmed.'],
  ['ready', 'Ready to pull', 'Current full-kit evidence and planned placement permit a release decision.'],
  ['waiting', 'Waiting', 'Resolve the stated condition or inspect the plan before requesting release.'],
  ['deferred', 'Deliberately deferred', 'Reconsidering checks all current gates; it does not release work.'],
  ['complete', 'Declared complete', 'Retained context from the initiative’s reported progress, not a new release opportunity.'],
];
const checkKeys = ['scope_ready', 'dependencies_accepted', 'team_available'];
const decisionLabels = {release:'Release', defer:'Defer', reconsider:'Reconsider'};
const when = timestamp => timestamp ? new Date(timestamp * 1000).toLocaleString() : 'Not recorded';
const placement = (week, start) => Number.isInteger(week)
  ? `Week ${week}${start ? ` · ${weekToDate(week, start)}` : ''}` : 'Not placed';

function checklistHTML(item) {
  return `<details class="ready-checklist border-top mt-3"><summary class="py-2">Confirm full kit</summary>
    <p class="hint">These operational confirmations do not change the plan's readiness percentage or bypass its release gate.</p>
    ${item.confirmation ? `<p class="hint">Last recorded ${esc(when(item.confirmation.createdAt))} by ${esc(item.confirmation.createdBy)} for week ${Number(item.confirmation.asOfWeek)}. ${item.confirmationCurrent ? 'Matches this planning context.' : 'Earlier context: review and reconfirm for this plan and week.'}</p>` : '<p class="hint">No operational confirmation recorded for this team and initiative.</p>'}
    <form data-ready-confirmation class="d-flex flex-column gap-3">
      ${(item.checklist || []).map(check => `<fieldset class="border rounded p-3 d-flex flex-column gap-3"><legend class="float-none w-auto px-2 fs-6">${esc(check.label)}</legend>
        <label class="ready-check form-check mb-0"><input class="form-check-input" type="checkbox" name="${esc(check.key)}_checked" ${check.checked ? 'checked' : ''}> Confirmed for this week</label>
        <label class="form-label d-grid gap-1 mb-0">Responsible owner <input class="form-control" name="${esc(check.key)}_owner" value="${esc(check.owner || '')}" maxlength="200"></label>
        <label class="form-label d-grid gap-1 mb-0">Evidence / acceptance <textarea class="form-control" name="${esc(check.key)}_evidence" rows="2" maxlength="10000">${esc(check.evidence || '')}</textarea></label>
      </fieldset>`).join('')}
      <button class="btn btn-secondary text-wrap align-self-start" type="submit" data-ready-write>Save full-kit confirmation</button>
      <p class="ready-form-status" role="status" aria-live="polite"></p>
    </form>
  </details>`;
}

export function readyItemHTML(item, context) {
  const choices = item.state === 'deferred' ? ['reconsider']
    : ['in_progress','complete'].includes(item.state) ? []
      : item.canRelease ? ['release','defer'] : ['defer'];
  return `<article class="card p-3 panel-card ready-item my-3" data-ready-initiative="${esc(item.initiative)}">
    <div class="ready-item-heading d-flex flex-wrap align-items-baseline gap-2"><h4 class="fs-6 text-body">${esc(item.initiative)}</h4>${item.kind === 'milestone' ? '<span class="tag badge text-bg-secondary text-wrap text-start">Acceptance checkpoint</span>' : ''}</div>
    <p><b>Planned start:</b> ${esc(placement(item.plannedStartWeek, context.periodStart))}<br>
      <b>Planned finish:</b> ${esc(placement(item.plannedFinishWeek, context.periodStart))}</p>
    ${item.kind === 'milestone' ? '<p class="hint">Zero-effort checkpoint: no work week or lane is invented.</p>' : ''}
    <ul class="ready-reasons ps-3">${(item.reasons || []).map(reason => `<li>${esc(reason.message)}${reason.owner ? ` <small class="d-block text-body-secondary">Owner: ${esc(reason.owner)}</small>` : ''}</li>`).join('')}</ul>
    ${item.lastDecision ? `<p class="hint">Last decision: ${esc(decisionLabels[item.lastDecision.decision] || item.lastDecision.decision)} · ${esc(item.lastDecision.owner)} · ${esc(when(item.lastDecision.createdAt))}${item.lastDecision.decision === 'release' ? item.releaseCurrent ? ' · Permission recorded; start unconfirmed.' : ' · Historical permission; recheck the current context.' : ''}</p>` : ''}
    <div class="row-actions d-flex flex-wrap gap-2"><button class="btn btn-secondary text-wrap" type="button" data-ready-inspect>Inspect in timeline</button><button class="btn btn-secondary text-wrap" type="button" data-ready-review>Review execution</button></div>
    ${!['in_progress','complete'].includes(item.state) ? checklistHTML(item) : ''}
    ${choices.length ? `<details class="ready-decision border-top mt-3"><summary class="py-2">${item.canRelease ? 'Record release or defer' : item.state === 'deferred' ? 'Reconsider this deferral' : 'Record a deferral'}</summary>
      <form data-ready-decision class="d-flex flex-column gap-3">
        <label class="form-label d-grid gap-1 mb-0">Decision <select class="form-select" name="decision" required>${choices.map(value=>`<option value="${value}">${decisionLabels[value]}</option>`).join('')}</select></label>
        <label class="form-label d-grid gap-1 mb-0">Accountable owner <input class="form-control" name="owner" maxlength="200" required></label>
        <label class="form-label d-grid gap-1 mb-0">Decision evidence / reason <textarea class="form-control" name="evidence" maxlength="10000" rows="3" required></textarea></label>
        <p class="hint">Release records permission, not an observed start. Deferral changes this queue's recommendation; it does not move the forecast.</p>
        <button type="submit" class="btn ${item.canRelease ? 'btn-primary primary' : 'btn-secondary'} align-self-start text-wrap" data-ready-write>Record decision</button>
        <p class="ready-form-status" role="status" aria-live="polite"></p>
      </form>
    </details>` : ''}
    <details class="ready-history border-top mt-3"><summary class="py-2">Readiness and release history</summary>
      <button class="btn btn-secondary text-wrap" type="button" data-ready-history>Load history</button><div data-ready-events role="status"></div>
    </details>
  </article>`;
}

export function readyHistoryHTML(history) {
  const records = [
    ...(history.confirmations || []).map(record=>({...record, type:'confirmation'})),
    ...(history.decisions || []).map(record=>({...record, type:'decision'})),
  ].sort((a,b)=>(b.eventOrder || 0)-(a.eventOrder || 0) || (b.createdAt || 0)-(a.createdAt || 0));
  return records.map(record=>`<article class="py-3 border-top"><h5 class="fs-6">${record.type === 'confirmation' ? 'Full-kit confirmation' : esc(decisionLabels[record.decision] || record.decision)}</h5>
    <p>Week ${Number(record.asOfWeek)} · ${esc(when(record.createdAt))} · ${esc(record.createdBy)}</p>
    ${record.type === 'confirmation' ? `<ul>${(record.checks || []).map(check=>`<li>${esc(check.key.replaceAll('_',' '))}: ${check.checked ? 'Confirmed' : 'Not confirmed'} · ${esc(check.owner || 'Owner unassigned')}<p>${esc(check.evidence || '')}</p></li>`).join('')}</ul>` : `<p>${esc(record.owner)}: ${esc(record.evidence)}</p>`}
  </article>`).join('') || '<p class="hint">No readiness or release history yet.</p>';
}

export function mountReadyQueue(host, {plan, request, onContext, onInspect, onReview}) {
  const teams = (plan.teams || []).map(team=>typeof team === 'string' ? team : team.name).filter(Boolean);
  const route = new URL(location.href).searchParams;
  const requestedTeam = route.get('team') || '';
  const unavailableTeam = requestedTeam && !teams.includes(requestedTeam);
  const horizon = Math.max(1,Math.ceil(plan.horizonWeeks || 26));
  const requestedWeek = Number(route.get('readyWeek') || 0);
  const initialWeek = Number.isInteger(requestedWeek) && requestedWeek >= 0 && requestedWeek < horizon ? requestedWeek : 0;
  host.innerHTML = `<section class="ready-queue">
    <div class="ready-heading d-flex flex-wrap justify-content-between align-items-start gap-3"><div><h3 class="fs-5 text-body">Next work</h3><p>Finish active work, prepare what is waiting, and release only what the current plan can support.</p></div><button class="btn btn-link usage-link text-wrap" data-anchor="next-work" type="button">Next work guide</button></div>
    <div class="ready-controls row g-3 align-items-end mb-3"><label class="form-label d-grid gap-1 mb-0 col-12 col-md">Team <select class="form-select" id="ready-team">${unavailableTeam && teams.length ? '<option value="" disabled selected>Choose a team from this plan</option>' : ''}${teams.length ? teams.map(team=>`<option>${esc(team)}</option>`).join('') : '<option value="">No teams in this plan</option>'}</select></label>
      <label class="form-label d-grid gap-1 mb-0 col-12 col-md">As-of planning week <input class="form-control" type="number" id="ready-week" min="0" max="${horizon-1}" step="1" value="${initialWeek}" required></label>
      <div class="col-12 col-md-auto d-grid"><button class="btn btn-secondary text-wrap" type="button" id="ready-refresh">Refresh queue</button></div>
    </div>
    <p class="hint">Week 0 is the period start. Choose the week you want to assess; this view is not a live observation of team activity.</p>
    <p id="ready-status" role="status" aria-live="polite"></p><div id="ready-context"></div>
    <div class="ready-filters row g-3 align-items-end mb-3"><label class="form-label d-grid gap-1 mb-0 col-12 col-md">Find an initiative <input class="form-control" type="search" id="ready-search" placeholder="Filter initiative names"></label><div class="col-12 col-md"><label class="ready-check form-check mb-0"><input class="form-check-input" type="checkbox" id="ready-show-all"> Show all items in each group</label></div></div>
    <div id="ready-items"></div>
  </section>`;
  const root = host.querySelector('.ready-queue');
  const live = () => host.isConnected && host.querySelector('.ready-queue') === root;
  const team = root.querySelector('#ready-team');
  const week = root.querySelector('#ready-week');
  const status = root.querySelector('#ready-status');
  const body = root.querySelector('#ready-items');
  const search = root.querySelector('#ready-search');
  const showAll = root.querySelector('#ready-show-all');
  if(requestedTeam && !unavailableTeam) team.value = requestedTeam;
  let queue = null, generation = 0, busy = false;
  const drafts = new Map();
  const base = `/api/plan/${encodeURIComponent(plan.id)}/ready-queue`;
  const contextKey = () => JSON.stringify([team.value,Number(week.value)]);
  const draftKey = (initiative,kind) => JSON.stringify([team.value,Number(week.value),initiative,kind]);
  const json = async (url,options) => {
    const response = await request(url,options);
    if(!response?.ok) throw new Error(response ? (await response.text()).slice(0,300) || `Server returned ${response.status}` : 'Could not reach the server.');
    return response.json();
  };
  function lockWrites() {
    root.querySelectorAll('[data-ready-write]').forEach(button=>{button.disabled = busy || !queue;});
  }
  function readChecks(form) {
    return checkKeys.map(key=>({key,checked:form.elements.namedItem(`${key}_checked`).checked,
      owner:form.elements.namedItem(`${key}_owner`).value,evidence:form.elements.namedItem(`${key}_evidence`).value}));
  }
  function paint() {
    if(!queue || !live()) return;
    const items = (queue.items || []).filter(item=>initiativeMatch(search.value,item.initiative));
    body.innerHTML = `<p class="hint">${items.length} of ${queue.counts.total} assigned items match. ${showAll.checked ? 'All matching items shown.' : 'Showing up to three per group; use Show all to see the rest.'}</p>` + groups.map(([key,label,hint])=>{
      const matches = items.filter(item=>item.state === key);
      const visible = showAll.checked ? matches : matches.slice(0,3);
      if(!matches.length) return `<details class="ready-group ready-empty mt-3 border-bottom" data-ready-group="${key}"><summary class="py-2">${label} <span class="tag badge text-bg-secondary text-wrap text-start">0</span><span class="hint ms-2 fw-normal">No matching items</span></summary><p class="hint">${hint}</p></details>`;
      return `<section class="ready-group mt-4" data-ready-group="${key}"><h3 class="fs-5 text-body">${label} <span class="tag badge text-bg-secondary text-wrap text-start">${matches.length}</span></h3><p class="hint">${hint}</p>
        ${visible.map(item=>readyItemHTML(item,queue.context)).join('') || '<p class="hint">No matching items in this group.</p>'}
        ${visible.length < matches.length ? `<button class="btn btn-secondary text-wrap" type="button" data-ready-show-rest>Show all ${matches.length} ${label.toLowerCase()} items</button>` : ''}</section>`;
    }).join('');
    body.querySelectorAll('[data-ready-show-rest]').forEach(button=>button.addEventListener('click',()=>{showAll.checked = true; paint();}));
    body.querySelectorAll('[data-ready-initiative]').forEach(card=>wireCard(card));
    lockWrites();
  }
  function wireCard(card) {
    const initiative = card.dataset.readyInitiative;
    card.querySelector('[data-ready-inspect]').addEventListener('click',()=>onInspect?.(team.value,initiative));
    card.querySelector('[data-ready-review]').addEventListener('click',()=>onReview?.(team.value,initiative));
    for(const kind of ['confirmation','decision']) {
      const form = card.querySelector(`[data-ready-${kind}]`);
      if(!form) continue;
      const key = draftKey(initiative,kind);
      const read = () => kind === 'confirmation' ? {checks:readChecks(form)} : Object.fromEntries(new FormData(form));
      const draft = drafts.get(key);
      if(draft) {
        if(kind === 'confirmation') {
          for(const check of draft.checks) {
            form.elements.namedItem(`${check.key}_checked`).checked = check.checked;
            form.elements.namedItem(`${check.key}_owner`).value = check.owner;
            form.elements.namedItem(`${check.key}_evidence`).value = check.evidence;
          }
        } else {
          for(const [name,value] of Object.entries(draft)) {
            if(name === 'decision' && ![...form.elements.decision.options].some(option=>option.value === value)) {
              const placeholder = document.createElement('option');
              placeholder.value = ''; placeholder.textContent = 'Previous decision unavailable — choose explicitly';
              placeholder.disabled = true; form.elements.decision.prepend(placeholder);
              form.elements.decision.value = '';
              continue;
            }
            form.elements.namedItem(name).value = value;
          }
        }
        form.closest('details').open = true;
        form.querySelector('.ready-form-status').textContent = 'Unsaved evidence retained. Review it against the refreshed queue before saving.';
      }
      const requiredEvidence = () => {
        if(kind !== 'confirmation') return;
        for(const checkKey of checkKeys) {
          const checked = form.elements.namedItem(`${checkKey}_checked`).checked;
          form.elements.namedItem(`${checkKey}_owner`).required = checked;
          form.elements.namedItem(`${checkKey}_evidence`).required = checked;
        }
      };
      requiredEvidence();
      const retain = () => {requiredEvidence(); drafts.set(key,read());};
      form.addEventListener('input',retain);
      form.addEventListener('change',retain);
      form.addEventListener('submit',async event=>{
        event.preventDefault();
        if(busy || !queue || !form.reportValidity()) return;
        const submitted = read(), source = contextKey();
        const payload = {team:team.value,initiative,asOfWeek:Number(week.value),expectedFingerprint:queue.fingerprint,...submitted};
        drafts.set(key,submitted);
        busy = true; lockWrites();
        const note = form.querySelector('.ready-form-status');
        note.setAttribute('role','status'); note.textContent = 'Saving…';
        try {
          await json(`${base}/${kind === 'confirmation' ? 'confirmations' : 'decisions'}`,{method:'POST',body:JSON.stringify(payload)});
          if(!live()) return;
          if(JSON.stringify(drafts.get(key)) === JSON.stringify(submitted)) drafts.delete(key);
          if(source !== contextKey()) return;
          await load(`Saved ${kind === 'confirmation' ? 'full-kit confirmation' : 'decision'} for ${initiative}.`);
        } catch(error) {
          if(live() && source === contextKey()) {
            generation++; queue = null;
            const message = `Could not save: ${error.message} Your evidence is retained. Refresh the queue, inspect any changes, then retry.`;
            note.setAttribute('role','alert'); note.textContent = message;
            status.setAttribute('role','alert'); status.textContent = message;
          }
        } finally {busy = false; if(live()) lockWrites();}
      });
    }
    const historyButton = card.querySelector('[data-ready-history]');
    historyButton.addEventListener('click',async()=>{
      const target = card.querySelector('[data-ready-events]');
      historyButton.disabled = true; target.textContent = 'Loading history…';
      try {
        const history = await json(`${base}/history?team=${encodeURIComponent(team.value)}&initiative=${encodeURIComponent(initiative)}`);
        if(live() && card.isConnected) target.innerHTML = readyHistoryHTML(history);
      } catch(error) {if(live() && card.isConnected) target.textContent = `History unavailable: ${error.message}`;}
      finally {historyButton.disabled = false;}
    });
  }
  async function load(success = '') {
    if(!live()) return;
    const mine = ++generation;
    queue = null; lockWrites();
    if(!team.value || !week.reportValidity()) {
      status.textContent = !teams.length ? 'Add teams to this plan before assessing its work.' : !team.value && unavailableTeam ? `The requested team “${requestedTeam}” is not in this plan. Choose a team from this plan to assess its work.` : 'Choose a team and a valid planning week.';
      body.innerHTML = ''; root.querySelector('#ready-context').innerHTML = ''; return;
    }
    const selected = {team:team.value,asOfWeek:Number(week.value)};
    status.setAttribute('role','status'); status.textContent = 'Evaluating the current plan and readiness evidence…';
    body.innerHTML = ''; root.querySelector('#ready-context').innerHTML = '';
    try {
      const result = await json(`${base}?team=${encodeURIComponent(selected.team)}&asOfWeek=${selected.asOfWeek}`);
      if(!live() || mine !== generation) return;
      queue = result;
      onContext?.(selected.team,selected.asOfWeek);
      root.querySelector('#ready-context').innerHTML = `<div class="card p-3 panel-card ready-context border-start border-3 border-primary my-3"><p><b>${esc(queue.context.team)}</b> · ${esc(placement(queue.context.asOfWeek,queue.context.periodStart))} · ${esc(queue.context.acceptedOrdering || 'stated')} ordering</p><p class="hint">${queue.counts.total} assigned initiatives · ${queue.counts.ready} ready · ${queue.counts.waiting} waiting · ${queue.counts.deferred} deferred.</p><details><summary class="py-2">How this queue is assessed</summary><p>${esc(queue.context.basis)}</p><p class="hint">Free lanes alone do not establish permission to begin work.</p></details></div>`;
      status.textContent = success || 'Queue loaded. Confirm full kit before recording a release; inspect the timeline for changes to placement.';
      paint();
    } catch(error) {
      if(live() && mine === generation) {status.setAttribute('role','alert'); status.textContent = `Queue unavailable: ${error.message} Your unsaved evidence is retained for this team and week. Refresh to try again.`;}
    }
  }
  team.addEventListener('change',()=>load());
  week.addEventListener('change',()=>load());
  week.addEventListener('input',()=>{generation++; queue = null; lockWrites(); status.textContent = 'Planning week changed. Refresh the queue to evaluate it.';});
  root.querySelector('#ready-refresh').addEventListener('click',()=>load());
  search.addEventListener('input',paint);
  showAll.addEventListener('change',paint);
  return load();
}
