import assert from 'node:assert/strict';

// A browser-only fixture isolates the DOM failure path. Production has no
// corresponding route; the main acceptance flow uses real authenticated APIs.
export async function checkAnnouncementRecovery(browser, base) {
  const page=await browser.newPage();
  try {
    const url=base+'/__announcement_dom_test__';
    await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><link rel="stylesheet" href="/vendor/bootstrap/bootstrap.min.css"></head><body><button id="replay">What's new</button><button id="help-btn">Help</button><button id="docs-btn">Guide</button><script src="/vendor/bootstrap/bootstrap.bundle.min.js"></script></body></html>`}));
    await page.goto(url);
    await page.evaluate(async()=>{
      const {mountAnnouncements}=await import('/js/announcements.js');
      window.fixture={fail:true,failAck:false,ackAttempts:0,acknowledged:0,opened:0};
      const feature={id:'guide-navigation-v1',title:'Guide',description:'Find help',action:{type:'menu',target:'docs-btn',parent:'help-btn'},announced:false,visited:false};
      const request=async(_url,options)=> {
        if(window.fixture.fail) return {ok:false};
        if(options){window.fixture.ackAttempts++;if(window.fixture.failAck)return {ok:false};const {kind}=JSON.parse(options.body);feature[kind]=true;window.fixture.acknowledged++;return {ok:true,json:async()=>({...feature})};}
        return {ok:true,json:async()=>({features:[{...feature}]})};
      };
      window.controller=mountAnnouncements({request,getIdentity:()=> 'generic-account',replayButton:document.getElementById('replay'),onAction:()=>{window.fixture.opened++;return true;}});
      await window.controller.ready;
    });
    await page.locator('#replay').click();
    const modal=page.locator('#announcements-overlay');
    await modal.waitFor({state:'visible'});
    await modal.locator('[data-announcement-error]').waitFor({state:'visible'});
    assert.match(await modal.locator('[data-announcement-error]').textContent(),/could not be loaded/i);
    assert.doesNotMatch(await modal.locator('[data-announcement-content]').textContent(),/No feature announcements are available/);
    await modal.locator('[data-announcement-retry]').waitFor({state:'visible'});
    await page.evaluate(()=>{window.fixture.fail=false;});
    await modal.locator('[data-announcement-retry]').click();
    await modal.locator('[data-announcement-action]').waitFor({state:'visible'});
    await page.waitForFunction(()=>window.controller.state().features[0]?.announced);
    assert.equal(await page.evaluate(()=>window.fixture.acknowledged),1);
    assert.equal(await modal.getAttribute('role'),'dialog');
    assert.equal(await modal.getAttribute('aria-modal'),'true');
    const close=modal.locator('[data-announcement-close]');await close.focus();await page.keyboard.press('Shift+Tab');
    assert.equal(await modal.locator('[data-announcement-action]').evaluate(el=>el===document.activeElement),true);
    await page.keyboard.press('Escape');await modal.waitFor({state:'hidden'});
    assert.equal(await page.locator('#replay').evaluate(el=>el===document.activeElement),true);
    assert.equal(await page.evaluate(()=>window.fixture.opened),0);
    assert.equal(await page.locator('#docs-btn [data-announcement-indicator]').count(),1);
    await page.locator('#replay').click();await modal.waitFor({state:'visible'});
    await page.evaluate(()=>{window.fixture.failAck=true;});
    await modal.locator('[data-announcement-action]').click();await modal.waitFor({state:'hidden'});
    await page.waitForFunction(()=>window.controller.state().error.includes('could not be saved'));
    assert.equal(await page.evaluate(()=>window.controller.state().features[0].visited),false);
    assert.equal(await page.locator('#docs-btn [data-announcement-indicator]').count(),1);
    assert.equal(await page.evaluate(()=>window.fixture.ackAttempts),2);
    assert.equal(await page.evaluate(()=>window.fixture.acknowledged),1);
    await page.locator('#replay').click();await modal.waitFor({state:'visible'});
    await modal.locator('[data-announcement-retry]').waitFor({state:'visible'});
    assert.match(await modal.locator('[data-announcement-retry]').textContent(),/saving announcement history/i);
    assert.match(await modal.locator('[data-announcement-error]').textContent(),/could not be saved/i);
    await page.evaluate(()=>{window.fixture.failAck=false;});
    await modal.locator('[data-announcement-retry]').click();
    await page.waitForFunction(()=>window.controller.state().features[0]?.visited);
    await modal.locator('[data-announcement-error]').waitFor({state:'hidden'});
    await modal.locator('[data-announcement-retry]').waitFor({state:'hidden'});
    assert.equal(await page.evaluate(()=>window.controller.state().error),'');
    assert.equal(await page.evaluate(()=>window.fixture.ackAttempts),3);
    assert.equal(await page.evaluate(()=>window.fixture.acknowledged),2);
    assert.equal(await page.locator('#docs-btn [data-announcement-indicator]').count(),0);
    assert.equal(await page.evaluate(()=>window.fixture.opened),1);
  } finally {await page.close();}
}
