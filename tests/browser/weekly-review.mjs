import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

// Invoked by the existing isolated real-server acceptance harness after login.
export async function checkWeeklyReview(page,base,plan){
  const within=(promise,label)=>{
    let timer;
    return Promise.race([promise,new Promise((_resolve,reject)=>{timer=setTimeout(()=>reject(new Error('Timed out waiting for '+label)),15000);})]).finally(()=>clearTimeout(timer));
  };
  const api=async(path,options={})=>page.evaluate(async({path,options})=>{
    const response=await fetch(path,{...options,headers:{'Content-Type':'application/json',Authorization:'Bearer '+localStorage.getItem('conway_token')}});
    const body=await response.text();if(!response.ok)throw new Error(response.status+' '+body);return JSON.parse(body);
  },{path,options});
  // Let application bootstrap finish; only execution's own initial discovery
  // should be delayed, not the earlier global snapshot selection.
  await page.goto(base+'?view=plan&plan='+plan);
  await page.locator('#view-execution').waitFor();await page.waitForLoadState('networkidle');
  let releaseCatalog,catalogReady;
  const heldCatalog=new Promise(resolve=>{releaseCatalog=resolve;});
  const catalogPending=new Promise(resolve=>{catalogReady=resolve;});
  await page.route('**/api/snapshots',async route=>{
    const response=await route.fetch();catalogReady();await heldCatalog;await route.fulfill({response});
  });
  await page.locator('#view-execution').click();await within(catalogPending,'execution snapshot discovery');
  await page.locator('[data-weekly-preview]').waitFor();
  assert.equal(await page.locator('[data-weekly-preview]').isDisabled(),true,'review preparation waits for initial evidence selection');
  releaseCatalog();
  await page.locator('#execution-snapshot option[value=""]').filter({hasText:'Manual review'}).waitFor({state:'attached'});
  await page.unroute('**/api/snapshots');
  await page.locator('#execution-snapshot').selectOption('');
  await page.locator('.weekly-new-action > summary').click();
  const actionForm=page.locator('#execution-decision-form');
  await actionForm.locator('[name=action]').fill('Confirm Atlas readiness');
  await actionForm.locator('[name=owner]').fill('Team lead');
  await actionForm.locator('[name=reviewDate]').fill('2026-10-30');
  await actionForm.locator('[name=rationale]').fill('Confirm the dependency checkpoint with acceptance evidence.');
  await actionForm.locator('button[type=submit]').click();
  const action=page.locator('[data-action-id]').filter({hasText:'Confirm Atlas readiness'});
  await action.waitFor();const actionID=await action.getAttribute('data-action-id');
  await page.locator('#weekly-review-date').fill('2026-11-01');
  await page.locator('#weekly-review-timezone').fill('America/Los_Angeles');
  const prepare=page.locator('[data-weekly-preview]');
  const previewURL=base+'/api/plan/'+plan+'/reviews/preview';
  let releasePreview,previewReady;
  const heldPreview=new Promise(resolve=>{releasePreview=resolve;});
  const prepared=new Promise(resolve=>{previewReady=resolve;});
  await page.route(previewURL,async route=>{
    const response=await route.fetch();previewReady();await heldPreview;await route.fulfill({response});
  });
  await prepare.click();await within(prepared,'prepared review response');
  await page.locator('#weekly-outcome-note').fill('Keep this conclusion while the context changes.');
  await page.locator('#weekly-review-timezone').fill('UTC');
  releasePreview();await page.waitForLoadState('networkidle');
  assert.equal(await page.locator('[data-weekly-complete]').isDisabled(),true,'a late preview cannot reactivate completion after a context change');
  assert.equal(await page.locator('#weekly-summary').textContent(),'');
  assert.equal(await page.locator('#weekly-outcome-note').inputValue(),'Keep this conclusion while the context changes.');
  await page.unroute(previewURL);await page.locator('#weekly-review-timezone').fill('America/Los_Angeles');
  await prepare.focus();await page.keyboard.press('Enter');
  const complete=page.locator('[data-weekly-complete]');
  await page.waitForFunction(()=>!document.querySelector('[data-weekly-complete]').disabled);
  assert.match(await page.locator('#weekly-summary').textContent(),/Manual review.*unknown/);
  assert.match(await page.locator('#weekly-summary').textContent(),/Delivery exceptions/);
  assert.match(await page.locator('#weekly-summary').textContent(),/Data gaps/);
  assert.match(await page.locator('#weekly-summary').textContent(),/1 overdue/);
  const conclusion='Atlas readiness needs an owner checkpoint. <img src=x onerror="window.reviewInjected=1">';
  await page.locator('#weekly-outcome-note').fill(conclusion);
  await page.locator('#weekly-next-checkpoint').fill('2026-11-08');
  // An actual concurrent action write invalidates the prepared context.
  await api('/api/plan/'+plan+'/decisions',{method:'POST',body:JSON.stringify({action:'Confirm scope evidence',owner:'Delivery lead',reviewDate:'2026-11-08',rationale:'A follow-up arrived while the review was prepared.'})});
  await complete.click();await page.locator('#weekly-status[role=alert]').waitFor();
  assert.equal(await page.locator('#weekly-outcome-note').inputValue(),conclusion);
  assert.match(await page.locator('#weekly-status').textContent(),/retained|Prepare again/i);
  assert.equal((await api('/api/plan/'+plan+'/reviews')).reviews.length,0);
  await prepare.click();await page.waitForFunction(()=>!document.querySelector('[data-weekly-complete]').disabled);
  await complete.click();await page.locator('[data-review-id]').waitFor();
  const reviewID=await page.locator('[data-review-id]').getAttribute('data-review-id');
  const saved=await api('/api/plan/'+plan+'/reviews/'+reviewID);
  assert.equal(saved.outcomeNote,conclusion);
  assert.equal(saved.preview.actions.find(a=>a.id===actionID).status,'open');
  assert.equal(await page.locator('[data-review-id] img').count(),0);
  assert.equal(await page.evaluate(()=>window.reviewInjected),undefined);
  await action.locator('summary').click();
  const transition=action.locator('[data-action-transition]');
  await transition.locator('[name=status]').selectOption('resolved');
  assert.equal(await transition.locator('[name=evidence]').getAttribute('required'),'');
  await transition.locator('button[type=submit]').click();
  assert.equal((await api('/api/plan/'+plan+'/decisions/'+actionID+'/transitions')).transitions.length,0,'empty resolution evidence is refused');
  await transition.locator('[name=evidence]').fill('The team demonstrated acceptance for PROJ-2.');
  await api('/api/plan/'+plan+'/decisions/'+actionID+'/transitions',{method:'POST',body:JSON.stringify({expectedVersion:1,status:'in_progress',evidence:'A second manager started the follow-up.'})});
  await transition.locator('button[type=submit]').click();
  await transition.locator('[role=alert]').waitFor();
  assert.equal(await transition.locator('[name=evidence]').inputValue(),'The team demonstrated acceptance for PROJ-2.');
  assert.equal((await api('/api/plan/'+plan+'/decisions/'+actionID+'/transitions')).transitions.length,1,'a stale transition cannot append history');
  await page.locator('#execution-reload-actions').click();
  await action.locator('[data-action-transition][data-version="2"]').waitFor();
  assert.equal(await transition.locator('[name=evidence]').inputValue(),'The team demonstrated acceptance for PROJ-2.');
  await transition.locator('button[type=submit]').click();
  await page.waitForFunction(async({plan,actionID})=>{
    const r=await fetch('/api/plan/'+plan+'/decisions',{headers:{Authorization:'Bearer '+localStorage.getItem('conway_token')}});
    return (await r.json()).decisions.find(a=>a.id===actionID)?.status==='resolved';
  },{plan,actionID});
  await action.locator('h4 .tag').filter({hasText:'Resolved'}).waitFor();
  await action.locator('[data-action-history]').click();
  await action.locator('[data-action-events]').filter({hasText:'PROJ-2'}).waitFor();
  assert.deepEqual(await api('/api/plan/'+plan+'/reviews/'+reviewID),saved,'action resolution cannot rewrite the completed summary');
  await prepare.click();await page.waitForFunction(()=>!document.querySelector('[data-weekly-complete]').disabled);
  await page.locator('#weekly-outcome-note').fill('The action was resolved with acceptance evidence.');
  const reviewsURL=base+'/api/plan/'+plan+'/reviews';
  let releaseHistory,historyReady;
  const heldHistory=new Promise(resolve=>{releaseHistory=resolve;});
  const historyPending=new Promise(resolve=>{historyReady=resolve;});
  await page.route(reviewsURL,async route=>{
    if(route.request().method()!=='GET'){await route.continue();return;}
    const response=await route.fetch();historyReady();await heldHistory;await route.fulfill({response});
  });
  await complete.click();await within(historyPending,'completed review history refresh');
  const openedOlder=page.waitForResponse(base+'/api/plan/'+plan+'/reviews/'+reviewID);
  await page.locator('#weekly-history').selectOption(reviewID);await openedOlder;
  await page.locator(`[data-review-id="${reviewID}"]`).waitFor();
  releaseHistory();await page.waitForLoadState('networkidle');
  assert.equal(await page.locator('#weekly-history').inputValue(),reviewID,'a late completion history refresh preserves the selected older review');
  assert.equal(new URL(page.url()).searchParams.get('review'),reviewID);
  assert.equal(await page.locator('[data-review-id]').getAttribute('data-review-id'),reviewID);
  await page.unroute(reviewsURL);
  assert.equal((await api('/api/plan/'+plan+'/reviews')).reviews.length,2);
  const link=await page.locator('[data-review-id] a').getAttribute('href');
  await page.goto(link);await page.locator(`[data-review-id="${reviewID}"]`).waitFor();
  await page.locator('#execution-snapshot option[value=""]').filter({hasText:'Manual review'}).waitFor({state:'attached'});
  assert.equal(await page.locator('#execution-snapshot').inputValue(),'','a shared manual review must not select unrelated snapshot evidence on reload');
  assert.match(await page.locator('[data-review-id]').textContent(),/Read-only/);
  assert.match(await page.locator('[data-review-id]').textContent(),/2026-11-08/);
  assert.equal(await page.locator('#weekly-history').inputValue(),reviewID);
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-weekly-review-desktop.png'),fullPage:true});
  await page.setViewportSize({width:360,height:800});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-weekly-review-mobile.png'),fullPage:true});
  await page.setViewportSize({width:1280,height:960});
  console.log(JSON.stringify({weeklyManualReview:true,completionConflictRetainsNote:true,evidencedResolution:true,immutableReview:true,authorizedReviewLink:true,weeklyMobileOverflow:false}));
}
