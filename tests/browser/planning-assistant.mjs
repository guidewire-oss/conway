import {checkPortfolioForecast} from './portfolio-forecast.mjs';
import {chooseUpdate,readAllUpdates} from './announcement-navigation.mjs';
import assert from 'node:assert/strict';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_BROWSER_CHANNEL?{channel:process.env.PLAYWRIGHT_BROWSER_CHANNEL}:{})});
const page=await browser.newPage({viewport:{width:1280,height:960}}),errors=[];
page.on('pageerror',e=>errors.push(e.message));page.setDefaultTimeout(10000);
const base=process.env.CONWAY_TEST_BASE_URL,plan=process.env.CONWAY_TEST_PLAN_ID,endpoint=base+'/api/plan/'+plan+'/assistant';
const assistantURL=base+'?view=plan&plan='+plan+'&planView=assistant';
const releases=[];
async function bounded(promise,label,ms=10000){let timer;try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error(label+' timed out')),ms);})]);}finally{clearTimeout(timer);}}
async function holdAnswer(url=endpoint,method='POST',holdMs=10000){
 let release,readyResolve,doneResolve,used=false;
 const held=new Promise(r=>release=r),ready=new Promise(r=>readyResolve=r),done=new Promise(r=>doneResolve=r);
 releases.push(release);
 const handler=async route=>{
  if(used||route.request().method()!==method){await route.continue();return;}
  used=true;
  try{const response=await route.fetch({timeout:10000});readyResolve();await bounded(held,'answer release',holdMs);await route.fulfill({response});}
  catch{await route.abort().catch(()=>{});}
  finally{try{await page.unroute(url,handler);}finally{doneResolve();}}
 };
 await page.route(url,handler);
 return {ready:()=>bounded(ready,'answer fetched'),settled:()=>bounded(done,'answer settled'),async deliver(){release();await bounded(done,'answer delivery');}};
}

try{
 await page.goto(assistantURL);await page.locator('#login-user').fill(process.env.CONWAY_TEST_USERNAME);await page.locator('#login-pass').fill(process.env.CONWAY_TEST_PASSWORD);await page.locator('#signin-form button[type=submit]').click();
 await page.locator('#announcements-overlay').waitFor({state:'visible'});await chooseUpdate(page,'Ask about your saved plan');assert.equal(await page.locator('[data-announcement-action="planning-assistant-v1"]').count(),1);await readAllUpdates(page);await page.locator('[data-announcement-close]').click();
 await checkPortfolioForecast(page,base,plan,process.env.CONWAY_TEST_SNAPSHOT_ID,holdAnswer);await page.locator('#view-assistant').click();
 const ask=page.locator('[data-assistant-ask]'),status=page.locator('[data-assistant-status]'),answer=page.locator('[data-assistant-answer]');
 await page.waitForFunction(()=>document.querySelector('[data-assistant-ask]')&&!document.querySelector('[data-assistant-ask]').disabled);
 const saveNotice=await page.locator('#plan-save-status').textContent();
 await page.locator('#assistant-initiative').selectOption('Beacon');await ask.focus();await page.keyboard.press('Enter');await answer.getByRole('heading',{name:'Beacon',exact:true}).waitFor();assert.match(await answer.textContent(),/Modeled start/);assert.match(await answer.textContent(),/Saved plan/);
 await answer.getByRole('link',{name:'Open timeline'}).click();await page.locator('#view-timeline[aria-pressed="true"]').waitFor();assert.equal(new URL(page.url()).searchParams.get('plan'),plan);
 await page.locator('#view-assistant').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);
 await page.locator('#assistant-task').selectOption('question');await page.locator('#assistant-question').fill('Explain Beacon');await ask.click();assert.equal(await answer.textContent(),'','consent is required');await page.locator('#assistant-consent').check();await ask.click();await answer.getByRole('heading',{name:'Beacon',exact:true}).waitFor();assert.match(await answer.textContent(),/Modeled start/);
 await page.locator('#assistant-task').selectOption('changes');await ask.click();await answer.getByRole('heading',{name:/Comparison with active agreement/}).waitFor();assert.match(await answer.textContent(),/Initial agreement/);
 await page.locator('#assistant-task').selectOption('review');await page.locator('#assistant-snapshot').selectOption(process.env.CONWAY_TEST_SNAPSHOT_ID);await ask.click();await answer.getByRole('heading',{name:/Review agenda/}).waitFor();assert.match(await answer.textContent(),/Atlas observed evidence/);
 await page.locator('#assistant-date').fill('2026-09-14');await page.locator('#assistant-zone').fill('UTC');await page.locator('#assistant-initiative').selectOption('Beacon');await ask.click();await answer.getByRole('heading',{name:/Review agenda/}).waitFor();
 await answer.getByRole('link',{name:'Open execution review'}).first().click();await page.locator('#weekly-review-date').waitFor();assert.equal(await page.locator('#weekly-review-date').inputValue(),'2026-09-14');assert.equal(await page.locator('#weekly-review-timezone').inputValue(),'UTC');assert.equal(await page.locator('#weekly-review-initiative').inputValue(),'Beacon');
 await page.locator('#view-assistant').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await page.locator('#assistant-task').selectOption('review');
 await page.locator('#assistant-snapshot').selectOption('manual');await ask.click();await answer.getByRole('heading',{name:/Review agenda/}).waitFor();assert.match(await answer.textContent(),/Manual review/);
 let failed=false;const fail=async route=>{if(!failed&&route.request().method()==='POST'){failed=true;await route.fulfill({status:503,body:'Temporary fixture outage'});}else await route.continue();};await page.route(endpoint,fail);await ask.click();await status.getByText(/Temporary fixture outage/).waitFor();assert.equal(await page.locator('#assistant-task').inputValue(),'review');assert.equal(await page.locator('#assistant-snapshot').inputValue(),'manual');await page.unroute(endpoint,fail);await ask.click();await answer.getByRole('heading',{name:/Review agenda/}).waitFor();
 await page.locator('#assistant-task').selectOption('schedule');const pending=await holdAnswer();await ask.click();await pending.ready();await page.locator('#assistant-task').selectOption('changes');await pending.deliver();assert.equal(await answer.textContent(),'');await ask.click();await answer.getByRole('heading',{name:/Comparison with active agreement/}).waitFor();
 const leave=await holdAnswer();await ask.click();await leave.ready();await page.locator('#view-execution').click();await leave.deliver();assert.equal(await page.locator('[data-assistant-answer]').count(),0);assert.equal(new URL(page.url()).searchParams.get('planView'),'execution');
 await page.locator('#view-assistant').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await ask.click();await answer.getByRole('heading',{name:/initiative/}).first().waitFor();
 const home=await holdAnswer();await ask.click();await home.ready();await page.locator('#home-tab').click();await home.deliver();assert.equal(await answer.textContent(),'');await page.locator('#plan-btn').click();await page.locator('.tab[data-view="plan"]').first().click();await ask.click();await answer.getByRole('heading',{name:/initiative/}).first().waitFor();
 const loadingHome=await holdAnswer(base+'/api/snapshots','GET');await page.locator('[data-assistant-refresh]').click();await loadingHome.ready();await page.locator('#home-tab').click();await loadingHome.deliver();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await page.locator('#plan-btn').click();await page.locator('.tab[data-view="plan"]').first().click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);
 const catalog=await holdAnswer(base+'/api/snapshots','GET');await page.locator('[data-assistant-refresh]').click();await catalog.ready();await page.locator('#assistant-task').selectOption('review');await page.locator('#assistant-initiative').selectOption('Beacon');await catalog.deliver();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await page.locator('#assistant-snapshot').selectOption(process.env.CONWAY_TEST_SNAPSHOT_ID);assert.equal(await page.locator('#assistant-task').inputValue(),'review');
 const configuration=await holdAnswer(endpoint,'GET');await page.locator('[data-assistant-refresh]').click();await configuration.ready();await page.locator('#assistant-task').selectOption('changes');await configuration.deliver();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);assert.equal(await page.locator('#assistant-task').inputValue(),'changes');await ask.click();await answer.getByRole('heading',{name:/Comparison with active agreement/}).waitFor();
 assert.equal(await page.locator('#plan-save-status').textContent(),saveNotice,'read-only requests preserve the save notice');
 await page.locator('#assistant-team').selectOption('');await page.locator('#assistant-initiative').selectOption('');await page.locator('[data-assistant-refresh]').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);assert.equal(await page.locator('#assistant-team').inputValue(),'');assert.equal(await page.locator('#assistant-initiative').inputValue(),'');await ask.click();await answer.getByRole('heading',{name:/Comparison with active agreement/}).waitFor();
 // A held read must remove its interception even when no caller releases it.
 const sentinel=async route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({marker:'after-timeout'})});
 await page.route(endpoint,sentinel);
 try{
  const timed=await holdAnswer(endpoint,'GET',25);
  const fetchContext=()=>page.evaluate(async url=>{try{const r=await fetch(url,{headers:{Authorization:'Bearer '+localStorage.getItem('conway_token')}});return await r.json();}catch{return {aborted:true};}},endpoint);
  assert.deepEqual(await fetchContext(),{aborted:true});await timed.settled();assert.deepEqual(await fetchContext(),{marker:'after-timeout'});
 }finally{await page.unroute(endpoint,sentinel);}
 // Retain an explicit source omitted by the catalog, and an intentional clearing.
 const omitted=async route=>route.fulfill({status:200,contentType:'application/json',body:'[]'});
 await page.route(base+'/api/snapshots',omitted);
 await page.goto(assistantURL+'&executionSnapshot='+process.env.CONWAY_TEST_SNAPSHOT_ID);
 await page.waitForFunction(()=>{const b=document.querySelector('[data-assistant-ask]');return b&&!b.disabled;});
 assert.equal(await page.locator('#assistant-snapshot').inputValue(),process.env.CONWAY_TEST_SNAPSHOT_ID);
 await page.locator('#assistant-task').selectOption('review');await ask.click();await answer.getByRole('heading',{name:/Review agenda/}).waitFor();assert.match(await answer.textContent(),/Atlas observed evidence/);
 await page.locator('#assistant-snapshot').selectOption('');await page.locator('[data-assistant-refresh]').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);assert.equal(await page.locator('#assistant-snapshot').inputValue(),'');await ask.click();assert.equal(await answer.textContent(),'');
 await page.goto(assistantURL+'&executionSnapshot=missing-capture');await page.waitForFunction(()=>{const b=document.querySelector('[data-assistant-ask]');return b&&!b.disabled;});assert.equal(await page.locator('#assistant-snapshot').inputValue(),'missing-capture');await page.locator('#assistant-task').selectOption('review');await ask.click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);assert.equal(await answer.textContent(),'');assert.match(await status.textContent(),/snapshot|access|not found/i);
 await page.unroute(base+'/api/snapshots',omitted);
 // A completed assumption save must refresh the assistant without changing view.
 await page.locator('#view-order').click();await page.locator('#sched-open').click();await page.locator('#sched-period-start').fill('2026-10-05');
 const saving=await holdAnswer(base+'/api/plan/'+plan+'/scheduling','PATCH');await page.locator('#sched-save').click();await saving.ready();await page.keyboard.press('Escape');await page.locator('#view-assistant').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await page.locator('#assistant-task').selectOption('question');await page.locator('#assistant-question').fill('Explain Beacon');await saving.deliver();await page.waitForFunction(()=>{const b=document.querySelector('[data-assistant-ask]');return b&&!b.disabled;});assert.equal(new URL(page.url()).searchParams.get('planView'),'assistant');assert.equal(await page.locator('#assistant-question').inputValue(),'Explain Beacon');await page.locator('#assistant-consent').check();await ask.click();await answer.getByRole('heading',{name:'Beacon',exact:true}).waitFor();assert.match(await answer.textContent(),/2026-10-05/);
 // Undo also completes against its saved inputs while preserving the active panel.
 await page.locator('#view-timeline').click();await page.locator('[data-select-init]').first().click();const form=page.locator('.tl-precise-edit'),start=form.locator('[name=startWeek]').first();const original=await start.inputValue();await start.fill(String(Number(original)+1));await form.locator('button[type=submit]').click();await page.locator('#tl-undo:not([disabled])').waitFor();
 const undo=await holdAnswer(base+'/api/plan/'+plan+'/initiatives','PATCH');await page.locator('#tl-undo').click();await undo.ready();await page.locator('#view-assistant').click();await undo.deliver();await page.waitForFunction(()=>{const b=document.querySelector('[data-assistant-ask]');return b&&!b.disabled;});assert.equal(new URL(page.url()).searchParams.get('planView'),'assistant');await page.locator('#view-timeline').click();await page.waitForFunction(value=>document.querySelector('.tl-precise-edit [name=startWeek]')?.value===value,original);await page.locator('#view-assistant').click();await page.waitForFunction(()=>!document.querySelector('[data-assistant-ask]').disabled);await ask.click();await answer.getByRole('heading',{name:/initiative/}).first().waitFor();
 for(const [source,subpath] of [['order','schedule'],['network','simulate']]){
  const drawing=await holdAnswer(base+'/api/plan/'+plan+'/'+subpath,'POST');await page.goto(base+'?view=plan&plan='+plan+'&planView='+source);await drawing.ready();await page.locator('#view-assistant').click();await drawing.deliver();await page.waitForFunction(()=>{const b=document.querySelector('[data-assistant-ask]');return b&&!b.disabled;});assert.equal(new URL(page.url()).searchParams.get('planView'),'assistant');
 }
 await ask.click();await answer.getByRole('heading',{name:/initiative/}).first().waitFor();
 for(const theme of ['light','dark']){await page.evaluate(t=>document.documentElement.dataset.bsTheme=t,theme);await page.setViewportSize({width:360,height:800});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'assistant fits mobile');await page.screenshot({path:join(tmpdir(),'conway-assistant-'+theme+'.png'),fullPage:true});}
 const docs=await page.request.get(base+'/docs.html');assert.match(await docs.text(),/id="planning-assistant"/);
 const token=await page.evaluate(()=>localStorage.getItem('conway_token'));const reviews=await page.request.get(base+'/api/plan/'+plan+'/reviews',{headers:{Authorization:'Bearer '+token}});assert.deepEqual((await reviews.json()).reviews,[]);
 assert.deepEqual(errors,[]);console.log(JSON.stringify({schedule:true,agreement:true,observedAndManualReview:true,retry:true,staleScope:true,viewOwnership:true,mobile:true,readOnly:true}));
}finally{for(const release of releases)release();await browser.close();}
