import {waitForAsyncFunction} from './async-condition.mjs';
// specs/025-team-ready-work-queue.md:129: actual server, retained evidence,
// explicit operational decisions, keyboard navigation and narrow-screen use.
import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

export async function checkReadyQueue(page,base,plan) {
  const path='/api/plan/'+plan+'/ready-queue', url=base+path;
  const releases=[];
  const api=(suffix='',body)=>page.evaluate(async({path,suffix,body})=>{
    const response=await fetch(path+suffix,{method:body?'POST':'GET',headers:{'Content-Type':'application/json',Authorization:'Bearer '+localStorage.getItem('conway_token')},...(body?{body:JSON.stringify(body)}:{})});
    const value=await response.json();if(!response.ok)throw new Error(JSON.stringify(value));return value;
  },{path,suffix,body});
  const card=()=>page.locator('[data-ready-initiative="Atlas"]');
  const group=state=>page.locator(`[data-ready-group="${state}"] [data-ready-initiative="Atlas"]`);
  const refresh=async()=>{
    await Promise.all([page.waitForResponse(r=>r.url().startsWith(url+'?')&&r.request().method()==='GET'),page.locator('#ready-refresh').click()]);
    await page.locator('#ready-status').filter({hasText:/Queue loaded/}).waitFor();
  };
  const decide=async(kind)=>{
    const q=await api('?team=Team%20A&asOfWeek=0');
    return api('/decisions',{team:'Team A',initiative:'Atlas',asOfWeek:0,expectedFingerprint:q.fingerprint,decision:kind,owner:'Team lead',evidence:'Confirmed at the team planning review.'});
  };
  try {
    await page.goto(base+'?view=plan&plan='+plan+'&planView=ready&team=Team%20A&readyWeek=0');
    await group('waiting').waitFor();
    assert.equal(await page.locator('#ready-team').inputValue(),'Team A');
    assert.equal(await page.locator('#ready-week').inputValue(),'0');
    assert.match(await page.locator('#ready-context').textContent(),/Week 0/);
    await card().locator('.ready-checklist > summary').click();
    const confirmation=()=>card().locator('[data-ready-confirmation]');
    for(const key of ['scope_ready','dependencies_accepted','team_available']) {
      await confirmation().locator(`[name=${key}_checked]`).check();
      await confirmation().locator(`[name=${key}_owner]`).fill('Team lead');
      await confirmation().locator(`[name=${key}_evidence]`).fill('Acceptance and availability reviewed.');
    }
    // A rejected write must preserve each typed field and require a fresh queue.
    await page.route(url+'/confirmations',route=>route.fulfill({status:409,body:'Planning context changed; refresh the queue.'}),{times:1});
    await confirmation().locator('[data-ready-write]').click();
    await page.locator('#ready-status[role=alert]').waitFor();
    assert.equal(await confirmation().locator('[name=scope_ready_evidence]').inputValue(),'Acceptance and availability reviewed.');
    assert.equal(await confirmation().locator('[data-ready-write]').isDisabled(),true);
    await refresh();
    assert.equal(await confirmation().locator('[name=scope_ready_owner]').inputValue(),'Team lead');
    await confirmation().locator('[data-ready-write]').click();
    await group('ready').waitFor();

    // Concurrent reconsideration removes the drafted option. It must not turn
    // retained evidence into a newly selected release without a deliberate choice.
    await decide('defer');await refresh();await group('deferred').waitFor();
    await card().locator('.ready-decision > summary').click();
    const decision=()=>card().locator('[data-ready-decision]');
    await decision().locator('[name=owner]').fill('Team lead');
    await decision().locator('[name=evidence]').fill('Review this deferral before choosing the next decision.');
    assert.equal(await decision().locator('[name=decision]').inputValue(),'reconsider');
    await decide('reconsider');
    await decision().locator('[data-ready-write]').click();
    await page.locator('#ready-status[role=alert]').waitFor();await refresh();await group('ready').waitFor();
    assert.equal(await decision().locator('[name=decision]').inputValue(),'');
    assert.equal(await decision().locator('[name=decision]').evaluate(el=>el.checkValidity()),false);
    assert.equal(await decision().locator('[name=evidence]').inputValue(),'Review this deferral before choosing the next decision.');
    await decision().locator('[name=decision]').selectOption('release');

    // Keep a release request pending while Refresh displays pre-commit state.
    // The later successful write must still refresh this same queue context.
    let releaseWrite,writeArrived;
    const held=new Promise(resolve=>{releaseWrite=resolve;releases.push(resolve);});
    const arrived=new Promise(resolve=>{writeArrived=resolve;});
    await page.route(url+'/decisions',async route=>{writeArrived();await held;await route.continue();},{times:1});
    await decision().locator('[data-ready-write]').click();
    await Promise.race([arrived,new Promise((_,reject)=>setTimeout(()=>reject(new Error('Release request did not arrive')),10000))]);
    await refresh();await group('ready').waitFor();
    releaseWrite();
    await group('waiting').waitFor();
    assert.match(await card().textContent(),/start remains unconfirmed|start unconfirmed/);
    await card().locator('.ready-history > summary').click();
    await card().locator('[data-ready-history]').click();
    await card().locator('[data-ready-events]').filter({hasText:'Full-kit confirmation'}).waitFor();
    const history=await api('/history?team=Team%20A&initiative=Atlas');
    assert.equal(history.confirmations.length,1);assert.equal(history.decisions.length,3);
    assert.equal(history.decisions.at(-1).decision,'release');
    await waitForAsyncFunction(page,async()=>{const response=await fetch('/api/announcements',{headers:{Authorization:'Bearer '+localStorage.getItem('conway_token')}});return (await response.json()).features.find(f=>f.id==='team-ready-work-v1')?.visited;});

    await page.locator('#ready-week').fill('1');await page.locator('#ready-week').press('Tab');
    await page.locator('#ready-context').filter({hasText:'Week 1'}).waitFor();
    assert.match(await card().textContent(),/Historical permission/);
    assert.equal(new URL(page.url()).searchParams.get('readyWeek'),'1');
    await page.reload();await page.locator('#ready-context').filter({hasText:'Week 1'}).waitFor();
    assert.equal(await page.locator('#ready-week').inputValue(),'1');
    await card().locator('[data-ready-inspect]').focus();await page.keyboard.press('Enter');
    await page.waitForURL(u=>u.searchParams.get('planView')==='timeline');
    assert.equal(new URL(page.url()).searchParams.get('team'),'Team A');
    await page.goto(base+'?view=plan&plan='+plan+'&planView=ready&team=Team%20A&readyWeek=0');
    await group('waiting').waitFor();
    // A deleted/renamed team is an explicit invalid destination; loading it
    // must not reinterpret the request as permission to inspect another team.
    const queueRequests=[];
    const observe=request=>{if(request.url().startsWith(url+'?'))queueRequests.push(request.url());};
    page.on('request',observe);
    try {
      await page.goto(base+'?view=plan&plan='+plan+'&planView=ready&team=Team%20Z&readyWeek=0');
      await page.locator('#ready-status').filter({hasText:'Team Z'}).waitFor();
      assert.equal(await page.locator('#ready-team').inputValue(),'');
      assert.match(await page.locator('#ready-team option:checked').textContent(),/Choose a team from this plan/);
      assert.equal(await page.locator('#ready-team option[value="Team Z"]').count(),0);
      assert.deepEqual(queueRequests,[]);
    } finally {page.off('request',observe);}
    await page.locator('#ready-team').selectOption('Team A');
    await group('waiting').waitFor();
    const acceptedRoute=page.url();
    const historyLength=await page.evaluate(()=>window.history.length);
    await refresh();
    assert.equal(page.url(),acceptedRoute);
    assert.equal(await page.evaluate(()=>window.history.length),historyLength,'same-context refresh must not add navigation entries');
    await page.route(url+'?team=Team%20A&asOfWeek=2',route=>route.fulfill({status:503,body:'Queue temporarily unavailable.'}),{times:1});
    await page.locator('#ready-week').fill('2');await page.locator('#ready-week').press('Tab');
    await page.locator('#ready-status[role=alert]').waitFor();
    assert.equal(page.url(),acceptedRoute,'failed queue requests must retain the accepted route');
    await page.locator('#ready-week').fill('0');await page.locator('#ready-week').press('Tab');
    await group('waiting').waitFor();
    await page.locator('#ready-team').selectOption('Team B');
    await page.locator('#ready-context').filter({hasText:'Team B'}).waitFor();
    await page.locator('#view-execution').click();
    await page.locator('#execution-team option').filter({hasText:'Team A'}).waitFor({state:'attached'});
    await page.locator('#execution-team').selectOption('Team A');
    await page.locator('#view-ready').click();
    await page.locator('#ready-context').filter({hasText:'Team A'}).waitFor();
    assert.equal(await page.locator('#ready-team').inputValue(),'Team A');
    assert.equal(new URL(page.url()).searchParams.get('team'),'Team A');
    await group('waiting').waitFor();
    await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-ready-queue-desktop.png'),fullPage:true});
    await page.setViewportSize({width:360,height:800});
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
    await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-ready-queue-mobile.png'),fullPage:true});
    await page.setViewportSize({width:1280,height:960});
    console.log(JSON.stringify({readyChecklist:true,staleEvidenceRetained:true,explicitReconsideration:true,lateReleaseRefresh:true,releaseNotObservedStart:true,readyHistory:true,readyKeyboardTimeline:true,invalidTeamExplicit:true,failedQueuePreservesRoute:true,executionTeamRoundTrip:true,refreshHistoryUnchanged:true,readyMobileOverflow:false}));
  } finally {
    for(const release of releases)release();
    if(!page.isClosed()) {
      await page.unroute(url+'/confirmations');await page.unroute(url+'/decisions');
      await page.unroute(url+'?team=Team%20A&asOfWeek=2');
    }
  }
}
