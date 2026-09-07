import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

export async function checkPortfolioForecast(page,base,plan,snapshot,holdAnswer) {
 await page.locator('#view-forecast').click();
 const run=page.locator('[data-forecast-run]'),result=page.locator('[data-forecast-result]');
 await run.focus();await page.keyboard.press('Enter');await result.getByRole('table').waitFor();
 assert.match(await result.textContent(),/Not calibrated/);assert.match(await result.textContent(),/Beacon/);
 assert.equal(await result.getByRole('columnheader').count(),4);
 await page.locator('#forecast-upper').fill('1.8');assert.equal(await result.textContent(),'');
 await run.click();await result.getByRole('table').waitFor();assert.match(await result.textContent(),/1.8×/);
 const endpoint=base+'/api/plan/'+plan+'/forecast';
 await page.route(endpoint,route=>route.fulfill({status:503,body:'Forecast service unavailable'}));
 await run.click();await page.locator('[data-forecast-status][role=alert]').waitFor();assert.match(await page.locator('[data-forecast-status]').textContent(),/retry/i);
 assert.equal(await result.textContent(),'');await page.unroute(endpoint);await run.click();await result.getByRole('table').waitFor();
 await page.getByText('Check historical evidence',{exact:true}).click();await page.waitForFunction(()=>document.querySelector('#forecast-snapshot option')?.nextElementSibling);
 await page.locator('#forecast-snapshot').selectOption(snapshot);await page.locator('[data-forecast-evidence]').getByText(/These ratios compare inferred/).waitFor();
 assert.match(await page.locator('[data-forecast-evidence]').textContent(),/No eligible completed-work samples/);
 await page.setViewportSize({width:360,height:800});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-forecast-mobile.png'),fullPage:true});
 await page.setViewportSize({width:1280,height:960});
 let held=await holdAnswer(endpoint);
 await run.click();await held.ready();await page.locator('#forecast-upper').fill('2');await held.deliver();
 assert.equal(await result.textContent(),'','Late results cannot survive a settings change');
 held=await holdAnswer(endpoint);
 await run.click();await held.ready();await page.locator('.tab[data-view="home"]').click();
 await page.locator('#plan-btn').click();await page.locator('.tab[data-view="plan"]').click();await held.deliver();
 assert.equal(await result.textContent(),'','Leaving and returning before response completion invalidates it');
 held=await holdAnswer(endpoint);
 await run.click();await held.ready();await page.locator('#view-timeline').click();await held.deliver();
 await page.locator('#tl-initiative-filter').waitFor();assert.equal(await page.locator('[data-forecast-result]').count(),0);
 await page.locator('#view-forecast').click();await run.click();await result.getByRole('table').waitFor();
 await result.getByRole('link',{name:'Beacon',exact:true}).click();await page.locator('#view-timeline[aria-pressed=true]').waitFor();
 assert.equal(new URL(page.url()).searchParams.get('plan'),plan);
 assert.equal(new URL(page.url()).searchParams.get('selected'),'Beacon');
}
