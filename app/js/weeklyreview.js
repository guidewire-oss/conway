// One review context, with immutable completion and evidenced action transitions.
// specs/024-weekly-execution-review.md:287
import { esc } from './order.js';
import { writeRoute } from './navigation.js';

const labels = {open:'Open', in_progress:'In progress', resolved:'Resolved', superseded:'Superseded'};
const dateToday = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')}`; };
const time = (n, timeZone) => n ? new Date(n * 1000).toLocaleString(undefined, timeZone ? {timeZone} : undefined) : 'Unknown';
const contextName = item => esc(item?.name || item?.id || 'None');

export function reviewSummaryHTML(review) {
  const c = review.context || {}, counts = review.counts || {};
  const entries = (items, empty) => items?.length ? `<ul>${items.map(item => `<li>${item.initiative ? `<b>${esc(item.initiative)}</b>: ` : ''}${esc(item.reason)}${item.team ? ` <span class="tag">${esc(item.team)}</span>` : ''}</li>`).join('')}</ul>` : `<p class="hint">${empty}</p>`;
  return `<div class="weekly-context"><p><b>${esc(c.reviewDate)}</b> · ${esc(c.timezone)}<br>Snapshot: ${contextName(c.snapshot)}${c.snapshot ? ` · captured ${esc(time(c.snapshot.capturedAt,c.timezone))}` : ' · Manual review; measured progress is unknown'}<br>Agreement: ${contextName(c.baseline)}<br>Review scope: ${review.filters?.team ? `team ${esc(review.filters.team)}` : 'all teams'}${review.filters?.initiative ? ` · initiative ${esc(review.filters.initiative)}` : ''}</p><p>${esc(c.comparisonLabel || 'No previous completed review.')}</p>${c.previousReview ? `<p class="hint">Previous review: ${esc(c.previousReview.reviewDate)} · snapshot ${contextName(c.previousReview.snapshot)} · agreement ${contextName(c.previousReview.baseline)}</p>` : ''}</div>
    <div class="weekly-counts" aria-label="Review totals"><span><b>${Number(counts.delivery) || 0}</b> delivery exceptions</span><span><b>${Number(counts.gaps) || 0}</b> data gaps</span><span><b>${Number(counts.openActions) || 0}</b> open actions</span><span><b>${Number(counts.overdueActions) || 0}</b> overdue</span></div>
    <div class="weekly-agenda"><section><h4>Delivery exceptions</h4>${entries(review.agenda?.delivery, 'No delivery exceptions identified in the available evidence. This does not establish that all delivery is on track.')}</section><section><h4>Data gaps</h4>${entries(review.agenda?.gaps, 'No additional gaps identified by this review.')}</section></div>
    <details><summary>Action states at preparation (${review.actions?.length || 0})</summary>${(review.actions || []).map(a => `<p><b>${esc(a.action)}</b> · ${esc(a.owner)} · ${esc(labels[a.status] || 'Open')}${a.overdue ? ' · Overdue' : ''} · due ${esc(a.reviewDate)}</p>`).join('') || '<p>No actions recorded.</p>'}</details>`;
}

export function actionCardsHTML(actions) {
  return (actions || []).map(a => `<article class="execution-decision" data-action-id="${esc(a.id)}"><h4>${esc(a.action)} <span class="tag">${esc(labels[a.status] || 'Open')}</span></h4><p>${esc(a.owner)} · review ${esc(a.reviewDate)}${a.initiative ? ` · ${esc(a.initiative)}` : ''}</p><p>${esc(a.rationale)}</p><small>Recorded ${esc(time(a.createdAt))} by ${esc(a.createdBy)} · snapshot ${esc(a.snapshotId || 'not selected')} · baseline ${esc(a.baselineId || 'none')}</small>
    <details><summary>Update status or view history</summary><form data-action-transition data-version="${Number(a.version) || 1}" class="weekly-transition"><label>New status <select name="status">${Object.entries(labels).filter(([s])=>s !== (a.status || 'open')).map(([s,label])=>`<option value="${s}">${label}</option>`).join('')}</select></label><label>Evidence / explanation <textarea name="evidence" maxlength="10000" rows="2"></textarea></label><p class="hint">Evidence is required to resolve or supersede an action. Reopening preserves its earlier history.</p><button type="submit">Save status</button><p role="status" aria-live="polite"></p></form><button type="button" data-action-history>Load transition history</button><div data-action-events></div></details></article>`).join('') || '<p class="hint">No actions recorded yet. Add the next concrete follow-up below.</p>';
}

export function bindActionCards(host, {json, base, live, onChanged}) {
  host.querySelectorAll('[data-action-id]').forEach(card => {
    const form = card.querySelector('form'), select = form.elements.status, evidence = form.elements.evidence;
    const requireEvidence = () => { evidence.required = ['resolved','superseded'].includes(select.value); };
    select.addEventListener('change',requireEvidence); requireEvidence();
    form.addEventListener('submit', async event => {
      event.preventDefault(); const button = form.querySelector('button'), note = form.querySelector('[role]');
      if(button.disabled) return;
      button.disabled = true; note.setAttribute('role','status'); note.textContent = 'Saving status…';
      const submitted = {status:select.value,evidence:evidence.value};
      try {
        await json(`${base}/decisions/${encodeURIComponent(card.dataset.actionId)}/transitions`, {method:'POST',body:JSON.stringify({expectedVersion:Number(form.dataset.version),...submitted})});
        if(!live()) return;
        if(select.value === submitted.status && evidence.value === submitted.evidence) evidence.value = '';
        await onChanged();
      } catch(e) { if(live()) { note.setAttribute('role','alert'); note.textContent = `Could not update action: ${e.message}. Your evidence is retained. Reload actions to obtain the latest version before retrying a conflict.`; } }
      finally { button.disabled = false; }
    });
    card.querySelector('[data-action-history]').addEventListener('click',async event => {
      const button = event.currentTarget, target = card.querySelector('[data-action-events]');
      button.disabled = true;
      try {
        const result = await json(`${base}/decisions/${encodeURIComponent(card.dataset.actionId)}/transitions`);
        if(live()) target.innerHTML = (result.transitions || []).map(t => `<p><b>${esc(labels[t.fromStatus] || t.fromStatus)} → ${esc(labels[t.status] || t.status)}</b> · ${esc(time(t.createdAt))} · ${esc(t.createdBy)}<br>${esc(t.evidence)}</p>`).join('') || '<p>No status changes yet.</p>';
      } catch(e) { if(live()) target.textContent = `History unavailable: ${e.message}`; }
      finally {button.disabled = false;}
    });
  });
}

export function mountWeeklyReview(host, {plan, json, live, getContext, ready = true}) {
  const base = `/api/plan/${encodeURIComponent(plan.id)}`;
  let preview = null, generation = 0, historyGeneration = 0, listGeneration = 0;
  host.innerHTML = `<section class="panel-card weekly-review"><div class="weekly-heading"><div><h3>Weekly execution review</h3><p class="hint">Prepare the agenda, follow up actions, then save the conclusion your team agreed.</p></div><button class="usage-link" data-anchor="weekly-review" type="button">Review guide</button></div>
    <div class="weekly-controls"><label>Review date <input id="weekly-review-date" type="date" required value="${dateToday()}"></label><label>Timezone <input id="weekly-review-timezone" required value="${esc(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')}" autocomplete="off"></label><button type="button" class="primary" data-weekly-preview ${ready ? '' : 'disabled'}>Prepare review</button></div>
    <p id="weekly-status" role="status" aria-live="polite">Use the snapshot and team controls above, or choose a manual review without a snapshot.</p><div id="weekly-summary"></div>
    <div data-weekly-actions></div><form id="weekly-completion"><label>Review conclusion <textarea id="weekly-outcome-note" name="outcomeNote" required maxlength="10000" rows="3" placeholder="What did the team agree, and why?"></textarea></label><label>Next checkpoint (optional) <input id="weekly-next-checkpoint" name="nextCheckpoint" type="date"></label><button type="submit" class="primary" data-weekly-complete disabled>Complete review</button><p id="weekly-completion-feedback"></p><p class="hint">Completion saves an immutable summary of this preview. Record new actions before preparing the final preview.</p></form>
    <details class="weekly-history"><summary>Completed reviews</summary><label>Saved review <select id="weekly-history"><option value="">Choose a completed review</option></select></label><p id="weekly-history-status" role="status"></p><div id="weekly-saved"></div></details></section>`;
  const status = host.querySelector('#weekly-status'), summary = host.querySelector('#weekly-summary'), complete = host.querySelector('[data-weekly-complete]'), prepare = host.querySelector('[data-weekly-preview]');
  const date = host.querySelector('#weekly-review-date'), zone = host.querySelector('#weekly-review-timezone'), completionForm = host.querySelector('#weekly-completion');
  const completionFeedback = host.querySelector('#weekly-completion-feedback');
  const picker = host.querySelector('#weekly-history'), saved = host.querySelector('#weekly-saved'), historyStatus = host.querySelector('#weekly-history-status');
  const inputs = () => ({reviewDate:date.value,timezone:zone.value,...getContext()});
  function invalidate() { generation++; preview = null; complete.disabled = true; summary.innerHTML = ''; status.setAttribute('role','status'); status.textContent = 'Context changed. Prepare the review again; your conclusion is retained.'; }
  date.addEventListener('change',invalidate); zone.addEventListener('input',invalidate);
  prepare.addEventListener('click',async () => {
    if(!date.reportValidity() || !zone.reportValidity() || prepare.disabled) return;
    const mine = ++generation, context = inputs(); preview = null; complete.disabled = true; prepare.disabled = true; summary.innerHTML = '';
    status.setAttribute('role','status'); status.textContent = 'Preparing the review…';
    try {
      const result = await json(`${base}/reviews/preview`,{method:'POST',body:JSON.stringify(context)});
      if(!live() || mine !== generation) return;
      preview = {result,context}; summary.innerHTML = reviewSummaryHTML(result); complete.disabled = false;
      status.textContent = 'Agenda prepared. Review the exceptions and action states, then record your conclusion.';
    } catch(e) { if(live() && mine === generation) {status.setAttribute('role','alert'); status.textContent = `Could not prepare review: ${e.message}`;} }
    finally {prepare.disabled = false; if(live()) completionFeedback.textContent = status.textContent;}
  });
  async function openReview(id) {
    const mine = ++historyGeneration; saved.innerHTML = ''; if(!id) return;
    historyStatus.textContent = 'Loading completed review…';
    try {
      const review = await json(`${base}/reviews/${encodeURIComponent(id)}`);
      if(!live() || mine !== historyGeneration) return;
      const url = new URL(location.href); url.searchParams.set('view','plan'); url.searchParams.set('plan',plan.id); url.searchParams.set('planView','execution'); url.searchParams.set('review',review.id); url.searchParams.set('executionSnapshot',review.snapshotId || 'manual'); url.searchParams.set('team',review.preview.filters?.team || '');
      saved.innerHTML = `<article data-review-id="${esc(review.id)}"><h4>Completed review · ${esc(review.reviewDate)}</h4><p class="hint">Read-only · completed ${esc(time(review.createdAt))} by ${esc(review.createdBy)}</p>${reviewSummaryHTML(review.preview)}<h4>Conclusion</h4><p class="weekly-note">${esc(review.outcomeNote)}</p>${review.nextCheckpoint ? `<p>Next checkpoint: ${esc(review.nextCheckpoint)}</p>` : ''}<a href="${esc(url.href)}">Link to this review</a><p class="hint">This link requires the plan's existing access permissions.</p></article>`;
      host.querySelector('.weekly-history').open = true; historyStatus.textContent = ''; writeRoute({review:review.id},true);
      const article = saved.querySelector('article'); article.tabIndex = -1; article.focus();
    } catch(e) { if(live() && mine === historyGeneration) historyStatus.textContent = `Could not open review: ${e.message}`; }
  }
  async function loadHistory(selected = '', expectedIntent = historyGeneration) {
    const mine = ++listGeneration;
    try {
      const result = await json(`${base}/reviews`); if(!live() || mine !== listGeneration) return;
      const chosen = picker.value, unchanged = expectedIntent === historyGeneration;
      picker.innerHTML = '<option value="">Choose a completed review</option>' + (result.reviews || []).map(r=>`<option value="${esc(r.id)}">${esc(r.reviewDate)} · ${esc(time(r.createdAt))}</option>`).join('');
      if(!unchanged) {picker.value = chosen; return;}
      if(selected) {picker.value = selected; await openReview(selected);}
      else historyStatus.textContent = result.reviews?.length ? '' : 'No completed reviews yet.';
    } catch(e) { if(live() && mine === listGeneration && expectedIntent === historyGeneration) historyStatus.textContent = `Review history unavailable: ${e.message}`; }
  }
  picker.addEventListener('change',()=>{writeRoute({review:picker.value},true); openReview(picker.value);});
  completionForm.addEventListener('submit',async event => {
    event.preventDefault(); if(!preview || complete.disabled) return;
    const current = preview, mine = generation, historyIntent = historyGeneration, values = Object.fromEntries(new FormData(completionForm)); complete.disabled = true; prepare.disabled = true;
    status.setAttribute('role','status'); status.textContent = 'Completing review…'; completionFeedback.textContent = status.textContent;
    try {
      const review = await json(`${base}/reviews`,{method:'POST',body:JSON.stringify({...current.context,...values,expectedFingerprint:current.result.fingerprint})});
      if(!live()) return;
      if(mine === generation) {preview = null; summary.innerHTML = '';}
      status.textContent = 'Review completed. Its saved context and conclusion are preserved below.'; completionFeedback.textContent = status.textContent;
      await loadHistory(review.id,historyIntent);
    } catch(e) { if(live()) {preview = null; status.setAttribute('role','alert'); status.textContent = `Could not complete review: ${e.message}. Your conclusion is retained. Prepare again before retrying.`;} }
    finally {prepare.disabled = false; if(live()) completionFeedback.textContent = status.textContent;}
  });
  loadHistory(new URL(location.href).searchParams.get('review') || '');
  return {invalidate,ready:()=>{prepare.disabled = false; status.textContent = 'Choose the review date and context, then prepare an agenda. Actions and manual reviews remain available without measured evidence.';}};
}
