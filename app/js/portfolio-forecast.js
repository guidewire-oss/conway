const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const known = row => row && !row.provisional && row.slices?.length && !['unschedulable','beyond-horizon'].includes(row.verdict);
export function forecastDate(origin, week) {
  if (!Number.isFinite(week)) return 'Unknown';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(origin || '')) return `Week ${week}`;
  const date = new Date(`${origin}T00:00:00Z`);
  if (!Number.isFinite(date.getTime())) return `Week ${week}`;
  date.setUTCDate(date.getUTCDate() + week * 7);
  return `${date.toISOString().slice(0,10)} (week ${week})`;
}
export function forecastHTML(result, planID) {
  const scenarios = result.scenarios, origin = scenarios[0].schedule.periodStart;
  const names = [...new Set(scenarios.flatMap(s => s.schedule.initiatives.map(i => i.name)))];
  const cells = scenarios.map(s => new Map(s.schedule.initiatives.map(i => [i.name, i])));
  return `<div class="alert alert-info"><strong>Scenario comparison · Not calibrated</strong><p class="mb-0">These are constrained planning scenarios, not confidence intervals or guaranteed bounds. Each scenario runs the same scheduler.</p></div>
    <p class="small text-body-secondary">Saved inputs ${esc(result.fingerprint.slice(0,12))} · ${origin ? `Period starts ${esc(origin)}` : 'Undated plan: relative weeks'} · Estimates ${esc(result.settings.lowerFactor)}× / 1× / ${esc(result.settings.upperFactor)}× · Additional shared disruption ${Math.round(result.settings.disruption*100)}%</p>
    <div class="row g-3 mb-4">${scenarios.map(s => `<div class="col-12 col-lg-4"><div class="card h-100"><div class="card-body"><h3 class="h6">${esc(s.name)}</h3><p class="mb-1">Portfolio commitment: <strong>${esc(forecastDate(origin,s.commitWeek))}</strong></p><p class="small mb-1">Finish before buffer: ${esc(forecastDate(origin,s.rawFinishWeek))}</p><p class="small mb-0">${s.unknown} of ${s.schedule.initiatives.length} initiatives with unknown or provisional placement</p></div></div></div>`).join('')}</div>
    <div class="table-responsive" role="region" aria-label="Initiative scenario comparison" tabindex="0"><table class="table table-striped align-middle"><caption>Commitments include buffers. Unknown work remains visible. Timeline links open the saved current plan.</caption><thead><tr><th scope="col">Initiative</th>${scenarios.map(s=>`<th scope="col">${esc(s.name)}</th>`).join('')}</tr></thead><tbody>${names.map(name=>`<tr><th scope="row"><a href="?view=plan&amp;plan=${encodeURIComponent(planID)}&amp;planView=timeline&amp;selected=${encodeURIComponent(name)}&amp;initiative=${encodeURIComponent(name)}">${esc(name)}</a></th>${cells.map(map=>{const row=map.get(name);return `<td>${known(row)?esc(forecastDate(origin,row.commitWeek)):'Unknown'}<div class="small text-body-secondary">${esc(row?.provisional?'Provisional':row?.verdict||'No placement')}${row?.bindingConstraint?` · ${esc(row.bindingConstraint)}`:''}</div></td>`;}).join('')}</tr>`).join('')}</tbody></table></div>
    <details class="mt-3"><summary>Assumptions and limitations</summary><ul>${result.limitations.map(s=>`<li>${esc(s)}</li>`).join('')}${scenarios.flatMap(s=>[...(s.schedule.warnings||[]),...(s.schedule.assumptions||[])].map(line=>`<li>${esc(s.name)}: ${esc(line)}</li>`)).join('')}</ul></details>`;
}
export function forecastEvidenceHTML(value) {
  if (value.snapshot.source !== 'jira') return `<p><strong>${esc(value.snapshot.name)}</strong> · Source: ${esc(value.snapshot.source || 'Unknown')}</p><p>This snapshot is not imported Jira execution evidence. Example or unrecognized sources are excluded from historical diagnostics. Choose an imported capture.</p>`;
  return `<p><strong>${esc(value.snapshot.name)}</strong> · Source: Jira · ${Math.floor(value.snapshot.ageDays)} days old · Agreement: ${esc(value.baseline?.name || 'None')}</p><p>Evidence covers ${value.coverage.tracked} of ${value.coverage.total} initiatives. These ratios compare inferred elapsed calendar time with agreed duration; they do not measure effort or validate prediction accuracy.</p>${value.calibration.length ? `<ul>${value.calibration.map(c=>`<li>${esc(c.pod)}: ${Number(c.factor).toFixed(2)}× agreed duration; ${c.sampleCount} completed team slices (inferred starts).</li>`).join('')}</ul>`:'<p>No eligible completed-work samples. Confirm epic bindings, agreement scope, known issue status and resolution timestamps in Review execution.</p>'}<ul>${value.gaps.map(g=>`<li>${esc(g)}</li>`).join('')}</ul><p class="small">Predictive calibration still requires forecasts recorded before outcomes and interval coverage on comparable completed cohorts. These samples do not automatically change the scenario settings.</p>`;
}

// specs/028-portfolio-forecasts.md:146: each async response belongs to a mounted
// plan, identity and form generation; diagnostics never become a new association.
export function mountPortfolioForecast(host,{plan,request,getIdentity,live}) {
  const owner=getIdentity();let disposed=false,ticket=0,evidenceTicket=0,abort;
  const section=document.createElement('section');section.className='card card-body';host.replaceChildren(section);
  const current=()=>!disposed&&owner&&owner===getIdentity()&&section.isConnected&&live();
  section.innerHTML=`<div class="d-flex flex-wrap justify-content-between gap-2 mb-3"><div><h2 class="h4">Portfolio forecasts</h2><p class="mb-0">${esc(plan.name)} · Saved planning inputs</p></div><a class="btn btn-outline-secondary align-self-start" href="docs.html#portfolio-forecasts" target="_blank" rel="noopener">Forecast guide</a></div>
    <p>Compare uncertainty before negotiating scope or dates. Start with the example factors below, then adjust them to your team's evidence. This does not change your plan or agreement.</p>
    <form data-forecast-form><div class="row g-3"><div class="col-12 col-md-4"><label class="form-label" for="forecast-lower">Shorter estimate factor</label><input class="form-control" id="forecast-lower" type="number" min="0.25" max="1" step="0.05" value="0.8" required aria-describedby="forecast-factor-help"></div><div class="col-12 col-md-4"><label class="form-label" for="forecast-upper">Longer estimate factor</label><input class="form-control" id="forecast-upper" type="number" min="1" max="3" step="0.05" value="1.3" required aria-describedby="forecast-factor-help"></div><div class="col-12 col-md-4"><label class="form-label" for="forecast-disruption">Additional shared disruption (%)</label><input class="form-control" id="forecast-disruption" type="number" min="0" max="75" step="1" value="10" required aria-describedby="forecast-disruption-help"></div></div><p id="forecast-factor-help" class="form-text">1× keeps estimates unchanged; 0.8× makes known estimates 20% shorter; 1.3× makes them 30% longer.</p><p id="forecast-disruption-help" class="form-text">In the longer-estimate scenario only, disruption removes this share of remaining productive capacity from every team, in addition to existing losses and calendars.</p><button class="btn btn-primary" type="submit" data-forecast-run>Compare scenarios</button></form><p role="status" class="mt-3" data-forecast-status></p><div data-forecast-result></div>
    <details class="mt-4"><summary>Check historical evidence</summary><p class="mt-2">Optional diagnostics from the existing execution review evidence. Selecting a snapshot here does not change your review association.</p><label class="form-label" for="forecast-snapshot">Execution snapshot</label><select class="form-select mb-2" id="forecast-snapshot"><option value="">Choose evidence</option></select><button class="btn btn-outline-secondary" type="button" data-forecast-evidence-refresh>Refresh available snapshots</button><p role="status" data-forecast-evidence-status></p><div data-forecast-evidence></div></details>`;
  const $=sel=>section.querySelector(sel),form=$('form'),button=$('[data-forecast-run]'),status=$('[data-forecast-status]'),result=$('[data-forecast-result]'),snapshot=$('#forecast-snapshot'),evidence=$('[data-forecast-evidence]'),evidenceStatus=$('[data-forecast-evidence-status]');
  const message=(node,text,error=false)=>{node.textContent=text;node.setAttribute('role',error?'alert':'status');};
  async function json(url,options){const r=await request(url,options);if(!r?.ok)throw Error((await r?.text())?.trim().slice(0,300)||'Connection failed. Retry.');return r.json();}
  form.addEventListener('input',()=>{ticket++;abort?.abort();button.disabled=false;result.replaceChildren();message(status,'Settings changed. Compare again to use these values.');});
  form.addEventListener('submit',async event=>{
    event.preventDefault();if(button.disabled||!form.reportValidity())return;
    const mine=++ticket;abort?.abort();abort=new AbortController();button.disabled=true;result.replaceChildren();message(status,'Comparing three constrained scenarios…');
    const settings={lowerFactor:Number($('#forecast-lower').value),upperFactor:Number($('#forecast-upper').value),disruption:Number($('#forecast-disruption').value)/100};
    try {const value=await json(`/api/plan/${encodeURIComponent(plan.id)}/forecast`,{method:'POST',body:JSON.stringify(settings),signal:abort.signal});if(!current()||mine!==ticket)return;result.innerHTML=forecastHTML(value,plan.id);message(status,'Comparison ready. Your plan and agreement are unchanged.');}
    catch(e){if(current()&&mine===ticket)message(status,e.message+' Adjust settings or retry.',true);}
    finally{if(current()&&mine===ticket)button.disabled=false;}
  });
  async function loadSnapshots(){const mine=++evidenceTicket;snapshot.disabled=true;evidence.replaceChildren();message(evidenceStatus,'Loading available snapshots…');
    try{const available=await json('/api/snapshots');const rows=available.filter(s=>s.source==='jira');if(!current()||mine!==evidenceTicket)return;snapshot.innerHTML='<option value="">Choose evidence</option>'+rows.map(s=>`<option value="${esc(s.id)}">${esc(s.name)}</option>`).join('');message(evidenceStatus,rows.length?'Choose a capture to inspect its evidence.':'No imported Jira snapshots available. Import evidence from Measure to begin.');}
    catch(e){if(current()&&mine===evidenceTicket)message(evidenceStatus,e.message+' Refresh available snapshots to retry.',true);}
    finally{if(current()&&mine===evidenceTicket)snapshot.disabled=false;}
  }
  snapshot.addEventListener('change',async()=>{const mine=++evidenceTicket;evidence.replaceChildren();if(!snapshot.value){message(evidenceStatus,'Choose evidence.');return;}message(evidenceStatus,'Inspecting evidence…');
    try{const value=await json(`/api/plan/${encodeURIComponent(plan.id)}/actuals?snapshot=${encodeURIComponent(snapshot.value)}`);if(!current()||mine!==evidenceTicket)return;evidence.innerHTML=forecastEvidenceHTML(value);message(evidenceStatus,'Diagnostic evidence only; scenarios remain uncalibrated.');}
    catch(e){if(current()&&mine===evidenceTicket)message(evidenceStatus,e.message+' Choose another snapshot or retry.',true);}
  });
  $('[data-forecast-evidence-refresh]').addEventListener('click',loadSnapshots);
  let loaded=false;section.querySelector('details').addEventListener('toggle',event=>{if(event.target.open&&!loaded){loaded=true;loadSnapshots();}});
  const leave=event=>{
    const tab=event.target.closest?.('.tab[data-view]');
    if(!tab||tab.dataset.view==='plan')return;
    ticket++;evidenceTicket++;abort?.abort();button.disabled=false;snapshot.disabled=false;
    result.replaceChildren();evidence.replaceChildren();
    message(status,'Returned to this plan? Compare again to use saved inputs.');
    message(evidenceStatus,'Choose evidence again or refresh available snapshots.');
  };
  document.addEventListener('click',leave);
  return ()=>{document.removeEventListener('click',leave);disposed=true;ticket++;evidenceTicket++;abort?.abort();};
}
