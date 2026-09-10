import assert from 'node:assert/strict';

// specs/015-baselines-drawer.md:218
export async function checkBaselineComparison(page, base, plan) {
  await page.locator('#bl-chip').click();
  const drawer=page.locator('.bl-drawer-overlay');
  const row=name=>drawer.locator('.bl-table tbody tr').filter({has:page.locator('td:first-child').filter({hasText:name})});
  await drawer.locator('#bl-drawer-name').fill('Atlas original');
  await drawer.locator('#bl-save').click();
  await row('Atlas original').waitFor();
  await drawer.locator('#bl-drawer-name').fill('Beacon revision');
  await drawer.locator('#bl-save').click();
  await row('Beacon revision').waitFor();
  const original=row('Atlas original');
  await drawer.locator('#bl-drawer-name').fill('Unfinished agreement');
  await original.locator('.bl-compare').click();
  await drawer.locator('.bl-compare-card').waitFor();
  assert.match(await drawer.locator('.bl-compare-card').textContent(),/Nothing has moved since Atlas original/);
  assert.equal(await drawer.locator('#bl-drawer-name').inputValue(),'Unfinished agreement');
  await original.locator('.bl-compare').click();
  await drawer.locator('.bl-compare-card').waitFor({state:'detached'});
  await original.locator('.bl-vs-sel').selectOption({label:'Beacon revision'});
  await drawer.locator('.bl-compare-card').waitFor();
  assert.match(await drawer.locator('.bl-compare-card').textContent(),/Nothing moved between Atlas original and Beacon revision/);
  await original.locator('.bl-compare').click();
  await drawer.getByText('Nothing has moved since Atlas original').waitFor();
  await original.locator('.bl-compare').click();
  const id=await original.locator('.bl-compare').getAttribute('data-id');
  const endpoint=base+'/api/plan/'+plan+'/baseline/'+id+'/compare';
  const otherID=await original.locator('.bl-vs-sel option').filter({hasText:'Beacon revision'}).getAttribute('value');
  const pairEndpoint=base+'/api/plan/'+plan+'/baseline/'+id+'/compare-to/'+otherID;
  // A history repaint keeps the same drawer session and pending comparison.
  let releaseRefresh;
  const heldRefresh=new Promise(resolve=>{releaseRefresh=resolve;});
  await page.route(endpoint,async route=>{const response=await route.fetch();await heldRefresh;await route.fulfill({response});},{times:1});
  try {
    await original.locator('.bl-compare').click();
    await drawer.getByText('Comparing baselines…').waitFor();
    await row('Atlas original').locator('.bl-activate').click();
    await row('Atlas original').locator('.bl-activate').waitFor({state:'detached'});
    await drawer.getByText('Comparing baselines…').waitFor();
    assert.equal(await drawer.locator('#bl-drawer-name').inputValue(),'Unfinished agreement');
    releaseRefresh();
    await drawer.getByText('Nothing has moved since Atlas original').waitFor();
  } finally {releaseRefresh();}
  await original.locator('.bl-compare').click();
  let releasePair;
  const heldPair=new Promise(resolve=>{releasePair=resolve;});
  await page.route(pairEndpoint,async route=>{const response=await route.fetch();await heldPair;await route.fulfill({response});},{times:1});
  try {
    await original.locator('.bl-vs-sel').selectOption({label:'Beacon revision'});
    await drawer.getByText('Comparing baselines…').waitFor();
    await original.locator('.bl-compare').click();
    await drawer.getByText('Nothing has moved since Atlas original').waitFor();
    const pairFinished=page.waitForResponse(response=>response.url()===pairEndpoint);
    releasePair();await pairFinished;await page.waitForLoadState('networkidle');
    assert.match(await drawer.locator('.bl-compare-card').textContent(),/Nothing has moved since Atlas original/,'A late pairwise response cannot replace the newer live comparison');
  } finally {releasePair();}
  await original.locator('.bl-compare').click();
  await page.route(endpoint,route=>route.fulfill({status:503,body:'Comparison temporarily unavailable'}),{times:1});
  await original.locator('.bl-compare').click();
  await drawer.getByText(/Comparison temporarily unavailable/).waitFor();
  await original.locator('.bl-compare').click();
  await drawer.getByText('Nothing has moved since Atlas original').waitFor();
  assert.equal(await drawer.locator('#bl-drawer-name').inputValue(),'Unfinished agreement');
  await original.locator('.bl-compare').click();
  let release;
  const held=new Promise(resolve=>{release=resolve;});
  await page.route(endpoint,async route=>{const response=await route.fetch();await held;await route.fulfill({response});},{times:1});
  try {
    await original.locator('.bl-compare').click();
    await drawer.getByText('Comparing baselines…').waitFor();
    await drawer.locator('.bl-drawer-close').click();
    await page.locator('#bl-chip').click();
    const finished=page.waitForResponse(response=>response.url()===endpoint);
    release();await finished;
    await page.waitForLoadState('networkidle');
    assert.equal(await drawer.locator('.bl-compare-card').count(),0,'A closed comparison cannot populate a reopened drawer');
    await original.locator('.bl-compare').click();
    await drawer.getByText('Nothing has moved since Atlas original').waitFor();
  } finally {release();}
  await drawer.locator('.bl-drawer-close').click();
  await page.locator('#view-timeline').click();
  await page.locator('[data-select-init]').first().click();
  const start=page.locator('.tl-precise-edit [name=startWeek]').first();
  await start.fill(String(Number(await start.inputValue())+1));
  await page.locator('.tl-precise-edit button[type=submit]').click();
  await page.locator('#tl-undo:not([disabled])').waitFor();
  await page.locator('#bl-chip').click();
  assert.equal(await drawer.locator('.bl-compare-card').count(),0,'Working-order changes invalidate completed live deltas');
  await drawer.getByText('Working plan changed.',{exact:false}).waitFor();
  const fresh=page.waitForResponse(endpoint);
  await original.locator('.bl-compare').click();await fresh;
  await drawer.locator('.bl-compare-card').waitFor();
  await original.locator('.bl-vs-sel').selectOption({label:'Beacon revision'});
  await drawer.getByText('Nothing moved between Atlas original and Beacon revision').waitFor();
  await drawer.locator('.bl-drawer-close').click();
  await page.locator('#tl-undo').click();
  await page.locator('#tl-undo[disabled]').waitFor();
  await page.locator('#bl-chip').click();
  await drawer.getByText('Nothing moved between Atlas original and Beacon revision').waitFor();
  await drawer.locator('.bl-drawer-close').click();
  await page.locator('#view-order').click();
  console.log('Baseline drawer: current/pairwise comparison, working-order invalidation, same-drawer refresh, dismissal, draft preservation and retry passed');
}
