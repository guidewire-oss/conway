import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

export async function checkForecastRegistrations(page,base,holdAnswer){
 const plan=process.env.CONWAY_TEST_EVALUATION_PLAN,ref=process.env.CONWAY_TEST_EVALUATION_REFERENCE,training=process.env.CONWAY_TEST_EVALUATION_TRAINING,later=process.env.CONWAY_TEST_EVALUATION_LATER;
 const endpoint=base+'/api/plan/'+plan+'/predictions/'+ref+'/registrations';
 await page.goto(base+'?view=plan&plan='+plan+'&planView=forecast');
 await page.evaluate(()=>{window.registrationConsumed=0;const fetch=window.fetch.bind(window);window.fetch=async(...args)=>{const r=await fetch(...args);if(String(args[0]).includes('/registrations')){const json=r.json.bind(r);r.json=async()=>{const value=await json();setTimeout(()=>{window.registrationConsumed++;},0);return value;};}return r;};});
 const consume=async held=>{const before=await page.evaluate(()=>window.registrationConsumed);await held.deliver();await page.waitForFunction(n=>window.registrationConsumed>n,before);};
 await page.getByText('Prediction history',{exact:true}).click();await page.locator('[data-prediction-open="'+ref+'"]').click();
 const initialList=await holdAnswer(endpoint,'GET');
 await page.getByText('Registered models',{exact:true}).click();await initialList.ready();
 const status=page.locator('[data-registration-status]'),listStatus=page.locator('[data-registration-list-status]');
 await page.locator('#registration-name').fill('Draft while loading');await consume(initialList);
 assert.equal(await page.locator('[data-registration-list] article').count(),1);
 assert.equal(await page.locator('#registration-name').inputValue(),'Draft while loading');
 assert.match(await status.textContent(),/Registration details changed/);
 for(const letter of ['界','\u{10400}']){
  await page.locator('#registration-name').fill(letter.repeat(120));assert.equal(await page.locator('#registration-name').evaluate(n=>n.checkValidity()),true);
  await page.locator('#registration-name').fill(letter.repeat(121));assert.equal(await page.locator('#registration-name').evaluate(n=>n.checkValidity()),false);
 }

 await page.locator('#registration-name').fill('Autumn probability baseline');await page.locator('#registration-capture').selectOption(training);
 // The first save reaches the server but its response is lost. Retry must recover one row.
 let lost=false;const lose=async route=>{if(route.request().method()==='POST'&&!lost){lost=true;const response=await route.fetch();assert.equal(response.status(),200);await route.abort();}else await route.continue();};
 const oldList=await holdAnswer(endpoint,'GET');await page.locator('[data-registration-refresh]').click();await oldList.ready();
 await page.route(endpoint,lose);await page.locator('[data-register-model]').click();await status.getByText(/Retry unchanged/).waitFor();await page.unroute(endpoint,lose);
 await page.locator('[data-register-model]').focus();await page.keyboard.press('Enter');await status.getByText(/Model registered/).waitFor();await listStatus.getByText(/Choose the later/).waitFor();await consume(oldList);assert.match(await status.textContent(),/Model registered/);
 assert.equal(await page.locator('[data-registration-list] article').count(),2);
 assert.match(await page.locator('[data-registration-list]').textContent(),/66.7%/);
 await page.locator('[data-registration-refresh]').click();await listStatus.getByText(/Choose the later/).waitFor();assert.equal(await page.locator('[data-registration-list] article').count(),2);
 await page.locator('[data-registration-list] article').filter({hasText:'Autumn probability baseline'}).locator('[data-registration-assess]').click();await status.getByText(/Choose the later evidence/).waitFor();assert.equal(await page.locator('#prediction-outcome-capture').evaluate(el=>el===document.activeElement),true);
 await page.locator('#prediction-outcome-capture').selectOption(later);await page.locator('[data-registration-list] article').filter({hasText:'Autumn probability baseline'}).locator('[data-registration-assess]').click();await status.getByText(/started after registration/).waitFor();
 await page.locator('[data-registration-list] article').filter({hasText:'Earlier registered baseline'}).locator('[data-registration-assess]').click();
 const result=page.getByRole('region',{name:'Registered model assessment'});await result.waitFor();assert.match(await result.textContent(),/0.111/);assert.match(await result.textContent(),/registered before the future predictions/);
 // Same mounted result target: changed selection must suppress a consumed old response.
 await page.evaluate(()=>{window.retainedRegistrationHost=document.querySelector('[data-forecast-registrations]');});
 const pending=await holdAnswer(endpoint+'/earlier-registered-model/assessment');
 await page.locator('[data-registration-assess="earlier-registered-model"]').click();await pending.ready();
 await page.locator('#prediction-outcome-capture').selectOption(training);await status.getByText(/Evidence selection changed/).waitFor();
 await consume(pending);assert.equal(await page.evaluate(()=>window.retainedRegistrationHost===document.querySelector('[data-forecast-registrations]')&&window.retainedRegistrationHost.isConnected),true);
 assert.equal(await page.locator('[data-registration-result]').textContent(),'');assert.match(await status.textContent(),/Evidence selection changed/);
 await page.locator('#prediction-outcome-capture').selectOption(later);await page.locator('[data-registration-assess="earlier-registered-model"]').click();await result.waitFor();
 for(const theme of ['light','dark']){await page.evaluate(t=>document.documentElement.dataset.bsTheme=t,theme);await page.setViewportSize({width:360,height:800});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-registration-'+theme+'.png'),fullPage:true});}
 await page.setViewportSize({width:1280,height:960});
 // Removing the chosen later capture also removes an already-rendered result.
 await page.route(base+'/api/snapshots',async route=>{const response=await route.fetch();await route.fulfill({json:(await response.json()).filter(s=>s.id!==later)});});
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('[data-prediction-assessment-status]').getByText(/no longer available/).waitFor();
 assert.equal(await page.locator('#prediction-outcome-capture').inputValue(),'');assert.equal(await page.locator('[data-registration-result]').textContent(),'');
 await page.unroute(base+'/api/snapshots');
 // Late list responses must not replace another prediction's live registration list.
 const held=await holdAnswer(endpoint,'GET');await page.locator('[data-registration-refresh]').click();await held.ready();
 await page.locator('[data-prediction-open="evaluation-test"]').first().click();await page.locator('[data-prediction-title]').getByText('Atlas release prediction',{exact:true}).waitFor();
 await page.getByText('Registered models',{exact:true}).click();await listStatus.getByText(/No registered models/).waitFor();await consume(held);
 assert.equal(await page.locator('[data-registration-list] article').count(),0);assert.equal(await page.locator('[data-registration-list]').count(),1);
 const docs=await page.request.get(base+'/docs.html');const guide=await docs.text();assert.match(guide,/id="forecast-registration"/);assert.doesNotMatch(guide,/Prospective model registration and subsequent independent evaluation remain future work/);
}
