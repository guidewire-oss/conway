// Run only against an isolated test server: this creates and deletes its own demo plans.
import {tmpdir} from 'node:os';
import {join} from 'node:path';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL=process.env.CONWAY_TEST_BASE_URL;
const username=process.env.CONWAY_TEST_USERNAME;
const password=process.env.CONWAY_TEST_PASSWORD;
if(!baseURL || !username || !password) throw new Error('Set CONWAY_TEST_BASE_URL, CONWAY_TEST_USERNAME and CONWAY_TEST_PASSWORD for an isolated test server.');
const artifacts=process.env.CONWAY_TEST_ARTIFACT_DIR || tmpdir();
const createdPlans=[];
import assert from 'node:assert/strict';
const browser=await chromium.launch({channel:'chrome',headless:true});
const page=await browser.newPage({viewport:{width:1440,height:1000}});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
let primaryFailure;
try {
 await page.goto(baseURL);
 await page.locator('#login-user').fill(username); await page.locator('#login-pass').fill(password);
 const catalogResponse=page.waitForResponse(r=>r.url().endsWith('/api/announcements')&&r.request().method()==='GET');
 await page.locator('#signin-form button[type=submit]').click();
 const catalog=await catalogResponse;
 if(catalog.ok() && (await catalog.json()).features.some(f=>!f.announced)) {
  const intro=page.locator('#announcements-overlay'); await intro.waitFor({state:'visible'});
  await page.waitForFunction(async()=>{const r=await fetch('/api/announcements',{headers:{Authorization:'Bearer '+localStorage.getItem('conway_token')}});return r.ok&&(await r.json()).features.every(f=>f.announced);});
  await intro.locator('[data-announcement-close]').click(); await intro.waitFor({state:'hidden'});
 }
 await page.locator('#plan-btn').waitFor(); await page.locator('#plan-btn').click(); await page.locator('.tab[data-view="plan"]').click();
 await page.locator('#plan-demo').click();
 await page.locator('#view-order.active').waitFor(); await page.locator('.ord-table').waitFor();
 const planURL=page.url(); const planID=new URL(planURL).searchParams.get('plan'); assert(planID); createdPlans.push(planID);
 assert.equal(await page.locator('.plan-setup').getAttribute('open'),null);
 await page.screenshot({path:join(artifacts,'conway-order-desktop.png'),fullPage:true});
 const sched=page.locator('#sched-dialog'); if(await sched.isVisible()) { await page.locator('#sched-wip-model').selectOption('strict'); await page.locator('#sched-save').click(); }
 await page.locator('#view-timeline').click(); await page.locator('#tl-initiative-filter').waitFor();
 await page.locator('[data-select-init]').first().click();
 await page.locator('.tl-precise-edit').waitFor();
 const name=await page.locator('.tl-precise-edit').getAttribute('data-init');
 await page.locator('#tl-initiative-filter').fill(name); await page.waitForTimeout(250);
 await page.locator('#tl-by-pod').click(); assert.equal(await page.locator('#tl-initiative-filter').inputValue(),name);
 await page.reload(); await page.locator('.tl-precise-edit').waitFor(); assert.equal(await page.locator('#tl-initiative-filter').inputValue(),name);
 assert.equal(await page.locator('#announcements-overlay').isVisible(),false,'introduction must not repeat after reload');
 const form=page.locator('.tl-precise-edit'); const start=form.locator('[name=startWeek]').first();
 const original=await start.inputValue(); await start.fill(String(Number(original)+1));
 await form.locator('button[type=submit]').click(); await page.locator('#tl-undo:not([disabled])').waitFor();
 await page.locator('#tl-undo').click();
 await page.waitForFunction(value=>document.querySelector('.tl-precise-edit [name=startWeek]')?.value===value,original);
 assert.equal(await page.locator('.tl-precise-edit [name=startWeek]').first().inputValue(),original,'undo must restore the original start week');
 await page.locator('#view-order').click(); await page.locator('#bl-chip').click();
 await page.locator('#bl-save').click(); assert.match(await page.locator('.bl-drawer-error').textContent(),/name/i);
 await page.locator('#bl-drawer-name').fill('UX review agreement'); await page.locator('#bl-save').click();
 await page.locator('.bl-table tbody tr').waitFor(); assert.match(await page.locator('.bl-table').textContent(),/UX review agreement/); await page.keyboard.press('Escape');
 await page.locator('#view-execution').click(); await page.locator('.execution-evidence').waitFor();
 await page.screenshot({path:join(artifacts,'conway-execution-desktop.png'),fullPage:true});
 await page.locator('#plan-scenario').click(); await page.locator('#scenario-form input').fill('Atlas review scenario');
 await page.locator('#scenario-form button').click(); await page.waitForURL(u=>u.searchParams.get('plan') !== planID);
 createdPlans.push(new URL(page.url()).searchParams.get('plan'));
 await page.locator('#bl-chip').waitFor(); assert.match(await page.locator('#bl-chip').textContent(),/none/i);
 await page.setViewportSize({width:360,height:800}); await page.screenshot({path:join(artifacts,'conway-plan-360.png'),fullPage:true});
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth); assert.equal(overflow,false);
 assert.deepEqual(errors,[]); console.log(JSON.stringify({defaultOrder:true,setupCollapsed:true,routeRestored:true,filterPreserved:true,preciseEditUndo:true,baselineValidation:true,executionEvidence:true,scenarioIndependent:true,mobileOverflow:overflow,pageErrors:errors}));
} catch(e) {
 primaryFailure=e;
 try {console.log(await page.locator('body').innerText());console.log({errors});await page.screenshot({path:join(artifacts,'conway-plan-failure.png'),fullPage:true});}catch(diagnosticError){console.error('Could not capture failure diagnostics:',diagnosticError);}
 throw e;
} finally {
 const cleanupErrors=[];
 try {
  for(const id of createdPlans) {
   try {await page.evaluate(async id=>{const token=localStorage.getItem('conway_token');const r=await fetch('/api/plan/'+encodeURIComponent(id),{method:'DELETE',headers:{Authorization:'Bearer '+token}});if(!r.ok)throw new Error('Could not clean up the browser-created plan '+id);},id);}catch(error){cleanupErrors.push(error);}
  }
 } finally {try{await browser.close();}catch(error){cleanupErrors.push(error);}}
 if(cleanupErrors.length){if(primaryFailure)console.error('Additional cleanup failures:',cleanupErrors);else throw new AggregateError(cleanupErrors,'Browser workflow cleanup failed');}
}
