// specs/027-evidence-linked-planning-assistant.md:190: answers retain source and view ownership.
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const tasks={schedule:'Explain this schedule',changes:'What changed since agreement?',review:'Prepare my review agenda'};
export function safeAssistantSource(value,planID){
 if(typeof value!=='string'||!value.startsWith('?'))return false;
 const u=new URL(value,'https://conway.invalid/');
 return u.searchParams.get('view')==='plan'&&u.searchParams.get('plan')===planID&&['timeline','order','execution'].includes(u.searchParams.get('planView'));
}
export function assistantAnswerHTML(answer){
 const c=answer.context||{},review=c.review;
 const stamp=n=>n?new Date(n*1000).toLocaleString():'Unknown';
 return `<section aria-label="Assistant answer" class="mt-3"><h3 class="h5">${esc(answer.summary)}</h3><div class="border-start border-3 border-primary ps-3 small mb-3"><p class="mb-1">Saved plan: <b>${esc(c.planName)}</b> · ${esc(c.team||'All teams')} · ${esc(c.initiative||'All initiatives')}</p><p class="mb-1">${c.periodStart?'Period starts '+esc(c.periodStart):'Undated schedule: relative weeks'} · Prepared ${esc(stamp(c.preparedAt))}</p>${review?`<p class="mb-1">Review ${esc(review.reviewDate)} · ${esc(review.timezone)} · ${review.snapshot?'Snapshot '+esc(review.snapshot.name)+' · captured '+esc(stamp(review.snapshot.capturedAt)):'Manual review: no snapshot'} · Agreement ${esc(review.baseline?.name||'None')}</p>`:''}<details><summary>Evidence context</summary><p class="text-break">Saved input fingerprint: ${esc(c.planFingerprint)}<br>Answer fingerprint: ${esc(answer.fingerprint)}</p></details></div>
 <div class="d-flex flex-wrap gap-2 mb-3">${(answer.sources||[]).filter(source=>safeAssistantSource(source.url,c.planId)).map(source=>`<a class="btn btn-outline-primary btn-sm" href="${esc(source.url)}">${esc(source.label)}</a>`).join('')}</div>
 ${(answer.gaps||[]).length?`<div class="alert alert-warning"><h4 class="h6">Limits and missing evidence</h4><ul class="mb-0 ps-3">${answer.gaps.map(g=>`<li>${esc(g)}</li>`).join('')}</ul></div>`:''}
 <div class="d-grid gap-3">${(answer.facts||[]).map(f=>`<article class="card"><div class="card-body"><span class="badge bg-secondary-subtle text-secondary-emphasis mb-2">${esc(f.kind)}</span><h4 class="h6 text-break">${esc(f.title)}</h4><ul class="ps-3">${(f.details||[]).map(d=>`<li class="text-break">${esc(d)}</li>`).join('')}</ul>${safeAssistantSource(f.source?.url,c.planId)?`<a class="btn btn-outline-primary btn-sm" href="${esc(f.source.url)}">${esc(f.source.label)}</a>`:''}</div></article>`).join('')||'<p>No additional facts were returned for this scope. Review the limits above.</p>'}</div></section>`;
}
export function mountPlanningAssistant(host,{plan,request,getIdentity,live=()=>true}){
 const identity=getIdentity(),base='/api/plan/'+encodeURIComponent(plan.id)+'/assistant';
 let ticket=0,loadTicket=0,loading=false,initialized=false,disposed=false,config=null,abort,loadAbort;
 const section=document.createElement('section');section.className='card p-3 my-3';host.replaceChildren(section);
 const current=()=>!disposed&&live()&&section.isConnected&&identity===getIdentity();
 const today=()=>{const d=new Date();return [d.getFullYear(),String(d.getMonth()+1).padStart(2,'0'),String(d.getDate()).padStart(2,'0')].join('-');};
 section.innerHTML=`<div class="d-flex flex-wrap align-items-start justify-content-between gap-2"><div><h2 class="h4">Planning assistant</h2><p>Explain a schedule, inspect agreement changes, or prepare a review from Conway evidence.</p></div><a class="btn btn-link" href="docs.html#planning-assistant" target="_blank" rel="noopener">Assistant guide</a></div><p class="alert alert-info">Answers use this plan's <b>saved inputs</b>. Save your changes first to include them. Asking does not change your plan or complete a review.</p>
 <form data-assistant-form><div class="row g-3"><div class="col-12 col-md-6"><label class="form-label" for="assistant-task">Question</label><select class="form-select" id="assistant-task">${Object.entries(tasks).map(([k,v])=>`<option value="${k}">${v}</option>`).join('')}</select></div><div class="col-12 col-md-3"><label class="form-label" for="assistant-team">Team</label><select class="form-select" id="assistant-team"><option value="">All teams</option></select></div><div class="col-12 col-md-3"><label class="form-label" for="assistant-initiative">Initiative</label><select class="form-select" id="assistant-initiative"><option value="">All initiatives</option></select></div></div>
 <div data-assistant-question hidden class="mt-3"><label class="form-label" for="assistant-question">Your question</label><textarea class="form-control" id="assistant-question" maxlength="2000" rows="3"></textarea><div class="form-check mt-2"><input class="form-check-input" type="checkbox" id="assistant-consent"><label class="form-check-label" for="assistant-consent">Send this question to the configured AI provider for interpretation.</label></div><p class="small text-body-secondary" data-assistant-disclosure></p></div>
 <div data-assistant-review hidden class="row g-3 mt-1"><div class="col-12 col-md-6"><label class="form-label" for="assistant-snapshot">Evidence for review questions</label><select class="form-select" id="assistant-snapshot"><option value="">Choose evidence</option><option value="manual">Manual review (no snapshot)</option></select></div><div class="col-6 col-md-3"><label class="form-label" for="assistant-date">Review date</label><input class="form-control" id="assistant-date" type="date" value="${today()}"></div><div class="col-6 col-md-3"><label class="form-label" for="assistant-zone">Timezone</label><input class="form-control" id="assistant-zone" value="${esc(Intl.DateTimeFormat().resolvedOptions().timeZone||'UTC')}"></div></div>
 <div class="d-flex flex-wrap gap-2 mt-3"><button class="btn btn-primary" type="submit" data-assistant-ask disabled>Ask assistant</button><button class="btn btn-outline-secondary" type="button" data-assistant-refresh>Refresh saved context</button></div></form><p role="status" aria-live="polite" class="mt-3" data-assistant-status>Loading saved context…</p><div data-assistant-answer></div>`;
 const form=section.querySelector('form'),task=section.querySelector('#assistant-task'),team=section.querySelector('#assistant-team'),initiative=section.querySelector('#assistant-initiative'),snapshot=section.querySelector('#assistant-snapshot'),date=section.querySelector('#assistant-date'),zone=section.querySelector('#assistant-zone'),question=section.querySelector('#assistant-question'),consent=section.querySelector('#assistant-consent'),ask=section.querySelector('[data-assistant-ask]'),status=section.querySelector('[data-assistant-status]'),answer=section.querySelector('[data-assistant-answer]');
 const requestedSnapshot=new URL(location.href).searchParams.get('executionSnapshot')||'';
 if(requestedSnapshot&&requestedSnapshot!=='manual')snapshot.add(new Option('Requested snapshot (not in available list)',requestedSnapshot));
 snapshot.value=requestedSnapshot;
 const showStatus=(message,error=false)=>{status.textContent=message;status.setAttribute('role',error?'alert':'status');};
 function invalidate(){ticket++;abort?.abort();answer.replaceChildren();ask.disabled=!config||loading;showStatus('Context changed. Ask again to use these selections.');}
 async function json(url,options){const response=await request(url,options);if(!response?.ok){let message='Connection failed. Retry when connected.';try{message=(await response?.text())?.trim()||message;}catch{}throw Error(message.slice(0,300));}return response.json();}
 function taskChanged(){section.querySelector('[data-assistant-question]').hidden=task.value!=='question';const reviewing=['review','question'].includes(task.value);section.querySelector('[data-assistant-review]').hidden=!reviewing;snapshot.required=task.value==='review';date.required=task.value==='review';zone.required=task.value==='review';question.required=task.value==='question';consent.required=task.value==='question';}
 form.addEventListener('input',()=>{invalidate();taskChanged();});
 form.addEventListener('change',()=>{invalidate();taskChanged();});
 async function load(){const mine=++loadTicket;ticket++;abort?.abort();loadAbort?.abort();loadAbort=new AbortController();loading=true;config=null;ask.disabled=true;answer.replaceChildren();showStatus('Loading saved context…');
  try{const next=await json(base,{signal:loadAbort.signal});if(!current()||mine!==loadTicket)return;config=next;
   for(const [select,names,initial] of [[team,next.teams,plan.tlTeamFilter],[initiative,next.initiatives,plan.selectedInitiative]]){const chosen=initialized?select.value:(select.value||initial||'');select.innerHTML=`<option value="">All ${select===team?'teams':'initiatives'}</option>`+names.map(n=>`<option value="${esc(n)}">${esc(n)}</option>`).join('');if(names.includes(chosen))select.value=chosen;}
   initialized=true;
   if(next.externalAvailable&&!task.querySelector('[value="question"]'))task.insertAdjacentHTML('beforeend','<option value="question">Ask in your own words</option>');
   section.querySelector('[data-assistant-disclosure]').textContent=next.externalDisclosure||'Guided questions are processed within Conway.';
   showStatus('Saved context ready. Choose a question and inspect the supporting evidence.');
   try{const snapshots=await json('/api/snapshots',{signal:loadAbort.signal});if(!current()||mine!==loadTicket)return;const chosen=snapshot.value;snapshot.innerHTML='<option value="">Choose evidence</option><option value="manual">Manual review (no snapshot)</option>'+snapshots.map(s=>`<option value="${esc(s.id)}">${esc(s.name||s.id)}</option>`).join('');if(chosen&&chosen!=='manual'&&!snapshots.some(s=>s.id===chosen))snapshot.add(new Option('Requested snapshot (not in available list)',chosen));snapshot.value=chosen;}catch{if(current()&&mine===loadTicket)showStatus('Saved plan ready. Snapshot list unavailable; refresh to retry, or explicitly choose Manual review.',true);}
  }catch(e){if(current()&&mine===loadTicket)showStatus('Could not load context: '+e.message,true);}
 finally{if(current()&&mine===loadTicket){loading=false;ask.disabled=!config;}}
 }
 form.addEventListener('submit',async event=>{event.preventDefault();if(!config||ask.disabled||!form.reportValidity())return;
  const mine=++ticket;abort?.abort();abort=new AbortController();const body={task:task.value,question:question.value,allowExternal:consent.checked,team:team.value,initiative:initiative.value,snapshotId:snapshot.value,reviewDate:date.value,timezone:zone.value,expectedFingerprint:config.fingerprint};ask.disabled=true;answer.replaceChildren();showStatus('Preparing an evidence-linked answer…');
  try{const result=await json(base,{method:'POST',body:JSON.stringify(body),signal:abort.signal});if(!current()||mine!==ticket)return;answer.innerHTML=assistantAnswerHTML(result);showStatus('Answer prepared from the context shown below. Nothing was changed.');}
  catch(e){if(current()&&mine===ticket)showStatus(e.message+' Your question is retained. Retry, or refresh saved context if it changed.',true);}
  finally{if(current()&&mine===ticket)ask.disabled=false;}
 });
 const leaveView=event=>{if(event.target.closest?.('.tab[data-view]')?.dataset.view && event.target.closest('.tab[data-view]').dataset.view!=='plan')invalidate();};
 document.addEventListener('click',leaveView);
 section.querySelector('[data-assistant-refresh]').addEventListener('click',load);taskChanged();load();
 return ()=>{document.removeEventListener('click',leaveView);disposed=true;ticket++;loadTicket++;abort?.abort();loadAbort?.abort();};
}
