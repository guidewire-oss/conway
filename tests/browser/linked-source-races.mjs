import assert from 'node:assert/strict';

// Only the test browser serves this fixture and intercepts its generic API.
// The production module and modal lifecycle run unchanged in an actual DOM.
export async function checkLinkedSourceRaces(browser,base){
  for(const operation of ['check','link','apply']){
    const page=await browser.newPage();
    let release;
    try{
      const fixtureURL=base+'/__linked_source_race_test__';
      await page.route(fixtureURL,route=>route.fulfill({contentType:'text/html',body:'<!doctype html><html><head><link rel="stylesheet" href="/vendor/bootstrap/bootstrap.min.css"></head><body><script src="/vendor/bootstrap/bootstrap.bundle.min.js"></script></body></html>'}));
      const source={id:'source-a',kind:'teams',status:'active',mode:'review',range:'Teams',spreadsheetUrl:'https://docs.google.com/spreadsheets/d/generic-sheet-id/edit',pollMinutes:15,latestVersionId:'version-a'};
      const version={id:'version-a',valid:true,count:1,rows:[['Name','Tracks'],['Team A','3']]};
      let holding=true,started;
      const pending=new Promise(resolve=>{started=resolve;});
      const held=new Promise(resolve=>{release=resolve;});
      await page.route('**/api/plan/generic-plan/sources**',async route=>{
        const request=route.request(),path=new URL(request.url()).pathname;
        let body;
        if(request.method()==='POST'){
          if(holding){started();await held;}
          body={source,applied:true};
        } else if(path.endsWith('/versions/version-a')){
          body={version,preview:{count:1,teams:[{name:'Team A',tracks:3}],errors:[]},applyable:true,planFingerprint:'current-plan'};
        } else if(path.endsWith('/versions'))body={versions:[version]};
        else body={configured:true,sources:[source]};
        await route.fulfill({json:body});
      });
      await page.goto(fixtureURL);
      await page.evaluate(async()=>{
        const {openLinkedSheets}=await import('/js/linksheets.js');
        window.appliedCallbacks=0;
        await openLinkedSheets('generic-plan',async()=>{window.appliedCallbacks++;});
      });
      const overlay=page.locator('#linked-sheets-overlay');
      async function startWrite(){
        if(operation==='check')await overlay.locator('[data-check]').click();
        else if(operation==='link'){
          await overlay.locator('summary').click();
          const form=overlay.locator('[data-link]');
          await form.locator('[name=spreadsheetUrl]').fill(source.spreadsheetUrl);
          await form.locator('[name=range]').fill('Teams');
          await form.locator('button[type=submit]').click();
        } else {
          await overlay.locator('[data-history]').click();
          await overlay.locator('[data-version]').click();
          await overlay.locator('[data-apply]').click();
        }
      }
      await startWrite();await pending;
      if(operation==='apply')await overlay.locator('[data-back]').click();
      else await overlay.locator('[data-history]').click();
      await overlay.locator('[data-version]').waitFor();
      // A newer preview is also a destination: test both history and preview.
      if(operation==='link'){
        await overlay.locator('[data-version]').click();
        await overlay.locator('[data-apply]').waitFor();
      }
      const destination=await overlay.locator('h2').textContent();
      release();await page.waitForLoadState('networkidle');
      assert.equal(await page.evaluate(()=>window.appliedCallbacks),0,`${operation}: a stale write must not refresh the surrounding plan`);
      assert.equal(await overlay.locator('h2').textContent(),destination,`${operation}: keep the newer destination`);
      // A current write still refreshes its owner, proving callback suppression
      // is tied to navigation and does not disable successful write feedback.
      holding=false;
      if(operation==='link')await overlay.locator('[data-back]').click();
      await overlay.locator('[data-back]').click();
      await startWrite();
      await page.waitForFunction(()=>window.appliedCallbacks===1);
      await overlay.locator('[data-source]').waitFor();
    }finally{release?.();await page.close();}
  }
}
