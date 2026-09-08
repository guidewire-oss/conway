import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

export async function checkPredictionEvaluation(page,base,holdAnswer){
 const plan=process.env.CONWAY_TEST_EVALUATION_PLAN,reference=process.env.CONWAY_TEST_EVALUATION_REFERENCE,training=process.env.CONWAY_TEST_EVALUATION_TRAINING,later=process.env.CONWAY_TEST_EVALUATION_LATER;
 const url=base+'?view=plan&plan='+plan+'&planView=forecast',endpoint=base+'/api/plan/'+plan+'/predictions/'+reference+'/evaluation';
 await page.setViewportSize({width:1280,height:960});await page.goto(url);
 // Signal in the next task, after JSON consumers and their microtasks have run.
 await page.evaluate(()=>{
  window.evaluationConsumed=0;const fetch=window.fetch.bind(window);
  window.fetch=async(...args)=>{const response=await fetch(...args);if(String(args[0]).endsWith('/evaluation')){const json=response.json.bind(response);response.json=async()=>{const value=await json();setTimeout(()=>{window.evaluationConsumed++;},0);return value;};}return response;};
 });
 const consume=async held=>{const before=await page.evaluate(()=>window.evaluationConsumed);await held.deliver();await page.waitForFunction(previous=>window.evaluationConsumed>previous,before);};
 await page.getByText('Prediction history',{exact:true}).click();await page.locator('[data-prediction-open="'+reference+'"]').click();await page.locator('[data-prediction-title]').waitFor();
 await page.locator('#prediction-outcome-capture').selectOption(later);
 await page.getByText('Test a probability model',{exact:true}).click();
 await page.locator('[data-prediction-evaluate]').click();await page.locator('[data-prediction-assessment-status][role=alert]').getByText(/Choose an earlier training capture/).waitFor();
 assert.equal(await page.locator('#prediction-training-capture').evaluate(el=>el===document.activeElement),true);
 await page.locator('#prediction-training-capture').selectOption(later);await page.locator('[data-prediction-evaluate]').click();await page.locator('[data-prediction-assessment-status][role=alert]').getByText(/finish before/).waitFor();
 await page.locator('#prediction-training-capture').selectOption(training);
 await page.route(endpoint,route=>route.fulfill({status:503,body:'Model evaluation temporarily unavailable.'}));
 await page.locator('[data-prediction-evaluate]').click();await page.locator('[data-prediction-assessment-status][role=alert]').waitFor();assert.equal(await page.locator('#prediction-training-capture').inputValue(),training);assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');await page.unroute(endpoint);
 await page.locator('[data-prediction-evaluate]').focus();await page.keyboard.press('Enter');
 const result=page.getByRole('region',{name:'Forecast model evaluation'});await result.waitFor();
 assert.match(await result.textContent(),/66.7%/);assert.match(await result.textContent(),/100.0%/);assert.match(await result.textContent(),/0.111/);assert.match(await result.textContent(),/not a certified confidence/);
 await result.getByText('Test evidence: inspect 1 entries',{exact:true}).click();await result.getByRole('button',{name:'Open Atlas release prediction'}).waitFor();
 for(const theme of ['light','dark']){
  await page.evaluate(value=>document.documentElement.dataset.bsTheme=value,theme);await page.setViewportSize({width:360,height:800});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-evaluation-'+theme+'-mobile.png'),fullPage:true});
 }
 await page.setViewportSize({width:1280,height:960});
 const held=await holdAnswer(endpoint);await page.locator('[data-prediction-evaluate]').click();await held.ready();assert.match(await page.locator('[data-prediction-assessment-status]').textContent(),/Fitting earlier evidence/);
 await page.locator('#prediction-training-capture').selectOption(later);await consume(held);assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');
 await page.locator('#prediction-training-capture').selectOption(training);
 const removed=await holdAnswer(endpoint);await page.locator('[data-prediction-evaluate]').click();await removed.ready();
 await page.route(base+'/api/snapshots',async route=>{const r=await route.fetch();await route.fulfill({json:(await r.json()).filter(s=>s.id!==training)});});
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('[data-prediction-assessment-status]').getByText(/no longer available/).waitFor();await consume(removed);assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');await page.unroute(base+'/api/snapshots');
 await page.locator('[data-prediction-detail] [data-prediction-sources]').click();await page.locator('#prediction-training-capture').selectOption(training);
 await page.locator('[data-prediction-evaluate]').click();await result.waitFor();
 await result.getByText('Test evidence: inspect 1 entries',{exact:true}).click();await result.getByRole('button',{name:'Open Atlas release prediction'}).click();await page.locator('[data-prediction-title]').getByText('Atlas release prediction',{exact:true}).waitFor();
 assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'');
 await page.locator('#prediction-outcome-capture').selectOption(later);await page.getByText('Test a probability model',{exact:true}).click();await page.locator('#prediction-training-capture').selectOption(training);
 const leaving=await holdAnswer(base+'/api/plan/'+plan+'/predictions/evaluation-test/evaluation');await page.locator('[data-prediction-evaluate]').click();await leaving.ready();
 await page.evaluate(()=>{window.retainedEvaluationHistory=document.querySelector('[data-prediction-history]');});
 await page.locator('.tab[data-view="home"]').click();
 await page.locator('#plan-btn').click();await page.locator('.tab[data-view="plan"]').click();
 assert.equal(await page.evaluate(()=>window.retainedEvaluationHistory.isConnected&&window.retainedEvaluationHistory===document.querySelector('[data-prediction-history]')),true,'Home navigation retains the forecast panel');
 // Reopen a different record before delivery so a stale render has a live target.
 await page.locator('[data-prediction-list] [data-prediction-open="'+reference+'"]').click();await page.locator('[data-prediction-title]').getByText('Beacon prediction',{exact:true}).waitFor();
 const freshStatus=await page.locator('[data-prediction-assessment-status]').textContent();
 await consume(leaving);
 assert.equal(await page.locator('[data-prediction-title]').textContent(),'Beacon prediction');
 assert.equal(await page.locator('[data-prediction-assessment]').count(),1);
 assert.equal(await page.locator('[data-prediction-assessment]').textContent(),'','The old evaluation cannot populate the reopened prediction');
 assert.equal(await page.locator('[data-prediction-assessment-status]').textContent(),freshStatus);
 await page.evaluate(()=>{delete window.retainedEvaluationHistory;});
 const docs=await page.request.get(base+'/docs.html');assert.match(await docs.text(),/id="forecast-evaluation"/);
}
