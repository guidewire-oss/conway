import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

// specs/028-portfolio-forecasts.md:259: cleanup must not hide the original cause.
export async function withPredictionCleanup(journey,...cleanups){
 const errors=[];
 try{await journey();}catch(error){errors.push(error);}
 finally{for(const cleanup of cleanups){try{await cleanup();}catch(error){errors.push(error);}}}
 if(errors.length===1)throw errors[0];
 if(errors.length>1)throw new AggregateError(errors,'Multiple failures during prediction test execution');
}

export async function checkPredictionHistory(page,base,plan,snapshot,holdAnswer){
 const token=await page.evaluate(()=>localStorage.getItem('conway_token'));
 const headers={Authorization:'Bearer '+token};
 const scheduleResponse=await page.request.get(base+'/api/plan/'+plan,{headers});assert.equal(scheduleResponse.status(),200);const original=(await scheduleResponse.json()).scheduling;
 const period=new Date();period.setUTCDate(period.getUTCDate()+7);const periodStart=period.toISOString().slice(0,10);
 await withPredictionCleanup(async()=>{
 const changed=await page.request.patch(base+'/api/plan/'+plan+'/scheduling',{headers,data:{...original,periodStart}});assert.equal(changed.status(),200);
 await page.locator('[data-forecast-run]').click();await page.locator('[data-forecast-result]').getByRole('table').waitFor();
 await page.getByText('Record this prediction',{exact:true}).click();
 await page.locator('#prediction-capture').selectOption(snapshot);
 await page.locator('#prediction-name').fill('September planning check');
 // Exercise the capabilities of plain HTTP, and recovery when randomness is unavailable.
 await page.evaluate(()=>{window.predictionRandomValues=crypto.getRandomValues;Object.defineProperty(crypto,'randomUUID',{configurable:true,value:undefined});Object.defineProperty(crypto,'getRandomValues',{configurable:true,value:undefined});});
 await page.locator('[data-prediction-record]').click();await page.locator('[data-prediction-save-status][role=alert]').getByText(/Secure randomness is unavailable/).waitFor();
 assert.equal(await page.locator('[data-prediction-record]').isEnabled(),true);
 await page.evaluate(()=>{Object.defineProperty(crypto,'getRandomValues',{configurable:true,value:window.predictionRandomValues});delete window.predictionRandomValues;});
 const endpoint=base+'/api/plan/'+plan+'/predictions';let recorded;
 const lostResponse=async route=>{if(route.request().method()!=='POST'){await route.continue();return;}const response=await route.fetch();assert.equal(response.status(),200);recorded=await response.json();await route.fulfill({status:503,body:'Fixture response lost after save'});};
 await page.route(endpoint,lostResponse);await page.locator('[data-prediction-record]').click();
 await page.locator('[data-prediction-save-status][role=alert]').waitFor();
 assert.equal(await page.locator('#prediction-name').inputValue(),'September planning check');
 await page.unroute(endpoint,lostResponse);await page.locator('[data-prediction-record]').click();
 await page.locator('[data-prediction-title]').waitFor();assert.equal(await page.locator('[data-prediction-open]').count(),1,'Retry returns the original prediction');
 await page.reload();await page.locator('#view-forecast[aria-pressed=true]').waitFor();
 await page.route(base+'/api/snapshots',route=>route.fulfill({status:503,body:'Capture catalog temporarily unavailable'}));
 await page.getByText('Prediction history',{exact:true}).click();await page.locator('[data-prediction-open]').click();await page.locator('[data-prediction-title]').waitFor();
 await page.locator('[data-prediction-capture-status][role=alert]').waitFor();assert.match(await page.locator('[data-prediction-capture-status]').textContent(),/Refresh captures/);
 await page.unroute(base+'/api/snapshots');await page.locator('[data-prediction-detail] [data-prediction-sources]').click();
 assert.match(await page.locator('[data-prediction-title]').textContent(),/September planning check/);
 await page.locator('#prediction-outcome-capture').selectOption(snapshot);await page.locator('[data-prediction-assess]').click();
 await page.locator('[data-prediction-assessment]').getByRole('table').waitFor();assert.match(await page.locator('[data-prediction-assessment]').textContent(),/Choose a capture started after/);
 const repeatedResponse=await page.request.post(endpoint,{headers,data:{id:'history-validation-repeat',name:'Repeated planning check',fingerprint:recorded.forecast.fingerprint,settings:recorded.forecast.settings,snapshotId:snapshot}});assert.equal(repeatedResponse.status(),200);const repeated=await repeatedResponse.json();
 await page.waitForFunction(issued=>Math.floor(Date.now()/1000)>issued,Math.max(recorded.issuedAt,repeated.issuedAt));
 const laterResponse=await page.request.post(base+'/__prediction-outcome-fixture');assert.equal(laterResponse.status(),200);const later=await laterResponse.json();
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('#prediction-outcome-capture').selectOption(later.id);
 await page.locator('[data-prediction-assess]').click();await page.locator('[data-prediction-assessment]').getByText(/of 1 eligible completed/).waitFor();
 assert.match(await page.locator('[data-prediction-assessment]').textContent(),/Before envelope/);
 await page.setViewportSize({width:360,height:800});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-prediction-history-mobile.png'),fullPage:true});
 await page.setViewportSize({width:1280,height:960});
 const validationURL=endpoint+'/'+recorded.id+'/validation';
 await page.route(validationURL,route=>route.fulfill({status:503,body:'History temporarily unavailable. Retry.'}));
 await page.locator('[data-prediction-validate]').click();await page.locator('[data-prediction-assessment-status][role=alert]').waitFor();assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');assert.equal(await page.locator('#prediction-outcome-capture').inputValue(),later.id);
 await page.unroute(validationURL);await page.locator('[data-prediction-validate]').click();
 const validation=page.getByRole('region',{name:'History validation'});await validation.waitFor();assert.match(await validation.textContent(),/1 repeated entries/);assert.match(await validation.textContent(),/0 within \/ 1 completed/);assert.match(await validation.textContent(),/not independent samples/);
 await validation.locator('summary').click();await validation.getByRole('button',{name:'Open representative prediction'}).waitFor();
 await page.setViewportSize({width:360,height:800});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-validation-mobile.png'),fullPage:true});await page.setViewportSize({width:1280,height:960});
 const validationHeld=await holdAnswer(validationURL);await page.locator('[data-prediction-validate]').click();await validationHeld.ready();assert.equal(await page.locator('[data-prediction-assessment-status]').textContent(),'Validating comparable prediction history…');
 await page.locator('#prediction-outcome-capture').selectOption(snapshot);await validationHeld.deliver();assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');
 await page.locator('#prediction-outcome-capture').selectOption(later.id);await page.locator('[data-prediction-validate]').click();await validation.waitFor();await validation.locator('summary').click();
 await validation.getByRole('button',{name:'Open representative prediction'}).click();await page.locator('[data-prediction-title]').waitFor();assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');
 await page.locator('#prediction-outcome-capture').selectOption(later.id);
 const assessmentURL=endpoint+'/'+recorded.id+'/assessment';
 const refreshHeld=await holdAnswer(assessmentURL);await page.locator('[data-prediction-assess]').click();await refreshHeld.ready();assert.equal(await page.locator('[data-prediction-assessment-status]').textContent(),'Comparing captured outcomes with saved dates…');
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('[data-prediction-capture-status]').getByText(/Managed captures refreshed/).waitFor();assert.equal(await page.locator('#prediction-outcome-capture').inputValue(),later.id);await refreshHeld.deliver();await page.locator('[data-prediction-assessment]').getByRole('table').waitFor();
 const removed=await holdAnswer(assessmentURL);await page.locator('[data-prediction-assess]').click();await removed.ready();
 await page.route(base+'/api/snapshots',async route=>{const r=await route.fetch();await route.fulfill({json:(await r.json()).filter(s=>s.id!==later.id)});});
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('[data-prediction-assessment-status]').getByText(/no longer available/).waitFor();await removed.deliver();assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');await page.unroute(base+'/api/snapshots');
 await page.locator('#prediction-outcome-capture').selectOption(snapshot);
 const held=await holdAnswer(assessmentURL);await page.locator('[data-prediction-assess]').click();await held.ready();
 await page.locator('#prediction-outcome-capture').selectOption('');await held.deliver();assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');
 const pending=await holdAnswer(endpoint+'/'+recorded.id,'GET');await page.locator('[data-prediction-list] [data-prediction-open="'+recorded.id+'"]').click();await pending.ready();
 await page.locator('#view-timeline').click();await pending.deliver();assert.equal(await page.locator('[data-prediction-detail]').count(),0);
 await page.locator('#view-forecast').click();await page.locator('[data-forecast-run]').click();await page.locator('[data-forecast-result]').getByRole('table').waitFor();
 await page.getByText('Record this prediction',{exact:true}).click();await page.locator('#prediction-capture').selectOption(snapshot);await page.locator('#prediction-name').fill('Second planning check');
 const saveResponse=await holdAnswer(endpoint,'POST');await page.locator('[data-prediction-record]').click();await saveResponse.ready();
 await page.getByText('Prediction history',{exact:true}).click();await page.locator('[data-prediction-open="'+recorded.id+'"]').click();await page.locator('[data-prediction-title]').getByText('September planning check',{exact:true}).waitFor();await saveResponse.deliver();
 await page.locator('[data-prediction-record]:enabled').waitFor();assert.equal(await page.locator('[data-prediction-title]').textContent(),'September planning check','A late save respects the newer selected prediction');
 await page.locator('#prediction-name').fill('Third planning check');
 const savingHistory=await holdAnswer(endpoint,'GET');await page.locator('[data-prediction-record]').click();await savingHistory.ready();
 await page.locator('#forecast-upper').fill('2');await savingHistory.deliver();
 await page.locator('[data-forecast-run]').click();await page.locator('[data-forecast-result]').getByRole('table').waitFor();assert.equal(await page.locator('[data-prediction-title]').count(),0,'An abandoned save continuation cannot reopen its detail');
 },async()=>{
  const restored=await page.request.patch(base+'/api/plan/'+plan+'/scheduling',{headers,data:original});assert.equal(restored.status(),200);
 },async()=>{
  await page.evaluate(()=>{delete crypto.randomUUID;delete crypto.getRandomValues;delete window.predictionRandomValues;});
 });
 await page.locator('#view-forecast').click();await page.locator('[data-forecast-run]').click();await page.locator('[data-forecast-result]').getByRole('table').waitFor();
}
