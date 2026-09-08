import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {inflateRawSync} from 'node:zlib';
import {join} from 'node:path';
import {tmpdir} from 'node:os';
import {chooseUpdate} from './announcement-navigation.mjs';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_BROWSER_CHANNEL?{channel:process.env.PLAYWRIGHT_BROWSER_CHANNEL}:{})});
const page=await browser.newPage({viewport:{width:1440,height:960}});
page.setDefaultTimeout(10000);
const base=process.env.CONWAY_TEST_BASE_URL, plan=process.env.CONWAY_TEST_PLAN_ID;
const endpoint=base+'/api/plan/'+plan+'/sample/initiatives.xlsx';
const errors=[];page.on('pageerror',e=>errors.push(e.message));
// Read the actual ZIP central directory, including files with data descriptors.
function sheetXML(bytes) {
  for(let p=0;p+46<bytes.length;p++) {
    if(bytes.readUInt32LE(p)!==0x02014b50)continue;
    const nameLen=bytes.readUInt16LE(p+28),name=bytes.subarray(p+46,p+46+nameLen).toString();
    if(name!=='xl/worksheets/sheet1.xml')continue;
    const offset=bytes.readUInt32LE(p+42),start=offset+30+bytes.readUInt16LE(offset+26)+bytes.readUInt16LE(offset+28);
    const compressed=bytes.subarray(start,start+bytes.readUInt32LE(p+20));
    return (bytes.readUInt16LE(p+10)===8?inflateRawSync(compressed):compressed).toString();
  }
  throw new Error('No worksheet in downloaded sample');
}
async function download() {
  const pending=page.waitForEvent('download');await page.locator('#plan-init-sample').click();
  const file=await pending;assert.equal(await file.failure(),null);
  assert.equal(file.suggestedFilename(),'conway-sample-initiatives.xlsx');
  const path=await file.path();return {path,xml:sheetXML(await readFile(path))};
}
try {
  await page.goto(base+'?view=plan&plan='+plan);
  await page.locator('#login-user').fill(process.env.CONWAY_TEST_USERNAME);
  await page.locator('#login-pass').fill(process.env.CONWAY_TEST_PASSWORD);
  await page.locator('#signin-form button[type=submit]').click();
  await page.locator('#announcements-overlay').waitFor({state:'visible'});
  await chooseUpdate(page,'Start a workbook with your own teams');
  await page.locator('[data-announcement-action="roster-initiative-sample-v1"]').click();
  await page.locator('#announcements-overlay').waitFor({state:'hidden'});
  await page.locator('#plan-roster-sel').waitFor();
  const demo=await download();assert.doesNotMatch(demo.xml,/Example initiative - replace before importing/);
  await page.locator('#plan-roster-sel').selectOption({label:'Atlas roster (1 pods)'});
  await page.waitForFunction(()=>document.querySelector('#plan-roster-sel')?.disabled===false && document.querySelector('.plan-setup')?.textContent.includes("plan's 1 attached teams"));
  const atlas=await download();assert.match(atlas.xml,/Atlas Sequence/);assert.doesNotMatch(atlas.xml,/Beacon Sequence/);
  // A refused change restores the previous selection and leaves download retryable.
  await page.route('**/api/plan/*/roster',route=>route.fulfill({status:503,body:'Temporary failure'}),{times:1});
  await page.locator('#plan-roster-sel').selectOption({label:'Beacon roster (1 pods)'});
  await page.getByText('The roster update response could not be confirmed.',{exact:false}).waitFor();
  assert.match(await page.locator('#plan-roster-sel option:checked').textContent(),/Atlas/);
  assert.match((await download()).xml,/Atlas Sequence/);
  // A save can commit even when its response is lost; reconcile the saved selection.
  await page.route('**/api/plan/*/roster',async route=>{await route.fetch();await route.abort();},{times:1});
  await page.locator('#plan-roster-sel').selectOption({label:'Beacon roster (1 pods)'});
  await page.getByText('The roster update response could not be confirmed.',{exact:false}).waitFor();
  assert.match(await page.locator('#plan-roster-sel option:checked').textContent(),/Beacon/);
  await page.waitForFunction(()=>!document.querySelector('#plan-roster-sel')?.disabled && !document.querySelector('#plan-uploading'));
  await page.route(endpoint,route=>route.fulfill({status:503,body:'Temporary failure'}),{times:1});
  await page.locator('#plan-init-sample').click();await page.getByText('Could not download the sample.',{exact:false}).waitFor();
  const beacon=await download();assert.match(beacon.xml,/Beacon Sequence/);assert.doesNotMatch(beacon.xml,/Atlas Sequence/);
  await page.locator('input[data-kind="initiatives"]').setInputFiles({name:'initiatives.xlsx',mimeType:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',buffer:await readFile(beacon.path)});
  await page.locator('#plan-strict-deps').check();
  await page.locator('#plan-init-preview').click();
  await page.locator('#sched-cancel').click();
  await page.locator('#plan-draft-save').click();
  await page.locator('#plan-draft-save').waitFor({state:'detached'});
  await page.locator('#sched-cancel').click();
  await page.locator('#view-timeline').waitFor();assert.equal(await page.locator('#unknown-fix').count(),0);
  await page.locator('#view-timeline').click();await page.locator('#tl-fullscreen').waitFor();
  for(const lens of ['tl-by-initiative','tl-by-pod']) {
    await page.locator('#'+lens).click();
    for(const theme of ['light','dark']) {
      await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
      const geometry=await page.locator('#tl-fullscreen,#tl-initiative-filter,#tl-team-filter').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {height:r.height,bottom:r.bottom};}));
      assert.ok(geometry.every(g=>Math.abs(g.height-geometry[0].height)<=1 && Math.abs(g.bottom-geometry[0].bottom)<=1),JSON.stringify(geometry));
    }
  }
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-plan-controls-desktop.png'),fullPage:true});
  await page.setViewportSize({width:390,height:844});
  const overflow=await page.locator('.tl-controls').evaluate(el=>el.scrollWidth>el.clientWidth);
  assert.equal(overflow,false,'Timeline toolbar fits a narrow screen');
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-plan-controls-mobile.png'),fullPage:true});
  await page.setViewportSize({width:1440,height:960});
  await page.locator('.plan-setup summary').click();
  let releaseRoster;
  const heldRoster=new Promise(resolve=>{releaseRoster=resolve;});
  const releaseTimer=setTimeout(()=>releaseRoster(),10000);
  await page.route('**/api/plan/*/roster',async route=>{await heldRoster;await route.continue();},{times:1});
  try {
    page.once('dialog',dialog=>dialog.accept());
    const requested=page.waitForRequest(request=>request.url().endsWith('/roster') && request.method()==='POST');
    await page.locator('#plan-roster-sel').selectOption({label:'Atlas roster (1 pods)'});await requested;
    assert.equal(await page.locator('#plan-init-sample').isDisabled(),true);
    await page.locator('#view-order').click();await page.locator('#sched-open').waitFor();
    await page.locator('.plan-setup summary').click();
    await page.locator('#plan-roster-sel').waitFor();
    assert.equal(await page.locator('#plan-roster-sel').isDisabled(),true,'View rendering preserves the pending roster lock');
    assert.equal(await page.locator('#plan-init-sample').isDisabled(),true,'View rendering cannot enable a stale sample');
  } finally {clearTimeout(releaseTimer);releaseRoster();}
  await page.waitForFunction(()=>document.querySelector('#plan-roster-sel option:checked')?.textContent.includes('Atlas') && !document.querySelector('#plan-roster-sel')?.disabled);
  assert.deepEqual(errors,[]);console.log('Plan samples: demo fallback, roster switch, failures, import and responsive controls passed');
} catch(error) {
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-plan-samples-failure.png'),fullPage:true});
  throw error;
} finally { await browser.close(); }
