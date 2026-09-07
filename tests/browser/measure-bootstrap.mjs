import assert from 'node:assert/strict';

// specs/011-bootstrap-adoption-debt.md:181 — use real Measure renderers and
// sanitized API responses to exercise compact controls and responsive evidence.
export async function checkMeasureBootstrap(page) {
  const base=new URL(page.url()).origin;
  const mainRoute=route=>route.fulfill({contentType:'text/javascript',body:''});
  const apiRoute=async route=>{
    const path=new URL(route.request().url()).pathname;
    const responses={
      '/api/config':{},
      '/api/me':{username:'acceptance-admin',roles:['admin']},
      '/api/admin/analytics':{activeThisWeek:0,activeLastWeek:0,totalEvents:0,from:'2026-08-08T00:00:00Z',to:'2026-09-07T00:00:00Z'},
      '/api/rosters':[{id:'atlas-roster',name:'Atlas roster',mine:true,podCount:1,public:false}],
      '/api/jira/status':{connected:true},
      '/api/jira/projects':[{key:'PROJ',name:'Atlas delivery'}],
      '/api/snapshots/baseline/epic-stats':{missing:1,known:2,overdue:0,noDue:1},
      '/api/snapshots/baseline/unassoc-epics':[],
      '/api/snapshots/baseline/hygiene-counts':{unsized:2,stale:1,unassigned:0,nooutcome:1},
      '/api/snapshots/baseline/wip-summary':{},
      '/api/snapshots/baseline/epic/PROJ-1':{epic:'PROJ-1',hasOutcome:true,tasks:[{key:'PROJ-2',pod:'Atlas',points:3,status:'Open',blockedBy:[]}]},
    };
    if(Object.hasOwn(responses,path))await route.fulfill({json:responses[path]});
    else await route.fulfill({status:404,json:{error:'Unexpected Measure fixture request'}});
  };
  await page.route('**/js/main.js',mainRoute);
  await page.route('**/api/**',apiRoute);
  try {
    await page.goto(base+'/index.html?testtoken=acceptance-fixture');
    const staticHelp=await page.locator('button.help[data-tip]').evaluateAll(buttons=>buttons.map(button=>({tip:button.dataset.tip,title:button.getAttribute('title')})));
    assert.ok(staticHelp.length>=6 && staticHelp.every(help=>help.tip===help.title),'Every static help explanation retains its native fallback');
    await page.evaluate(async()=>{const {initAuth}=await import('/js/auth.js');await initAuth();});
    assert.ok(await page.locator('#auth-logout').evaluate(el=>parseFloat(getComputedStyle(el).fontSize)<=14),'Sign out remains compact in the identity chip');
    await page.locator('#usage-btn').click();
    const closeUsage=page.getByRole('button',{name:'Close usage analytics',exact:true});await closeUsage.waitFor();
    await page.waitForFunction(()=>document.querySelector('#usage-foot')?.textContent.includes('2026-08-08'));
    await closeUsage.focus();await page.keyboard.press('Enter');
    assert.equal(await page.locator('#usage-overlay').isVisible(),false,'The named analytics close action works by keyboard');
    await page.evaluate(async()=>{
      window.measureFixture={pods:[{name:'Atlas',location:'Central'}],stats:{Atlas:{mu:1,sigma:0.2,rho0:0.3}},hygiene:{},edges:[],overlap:{Atlas:{Atlas:1}}};
      window.showMeasureView=view=>document.querySelectorAll('.view').forEach(el=>el.classList.toggle('active',el.id==='view-'+view));
      window.showMeasureView('hygiene');
      const {initHygiene}=await import('/js/hygiene.js');initHygiene(window.measureFixture);
      const {initGuide}=await import('/js/guide.js');initGuide(window.measureFixture);
    });
    await page.waitForFunction(()=>document.querySelectorAll('#hygiene-cards .stat').length===7);
    for(const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      const boxes=await page.locator('#hygiene-cards .stat').evaluateAll(cards=>cards.map(card=>{const r=card.getBoundingClientRect();return {left:r.left,right:r.right,width:r.width,top:r.top};}));
      assert.ok(boxes.every(box=>box.width>=180 && box.left>=0 && box.right<=width),'Quality metrics remain readable and inside the viewport at '+width+': '+JSON.stringify(boxes));
      if(width===360)assert.equal(new Set(boxes.map(box=>box.top)).size,7,'Quality metrics stack on mobile');
    }
    await page.evaluate(async()=>{
      window.showMeasureView('simulator');
      const {initSimulator}=await import('/js/simulator.js');initSimulator(window.measureFixture);
    });
    const taskActions=await page.locator('#task-table .del').evaluateAll(buttons=>buttons.map(button=>button.getBoundingClientRect().height));
    assert.equal(taskActions.length,7);assert.ok(taskActions.every(height=>height>=24 && height<=32),'Task removal actions retain compact usable targets');
    for(const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      const importGeometry=await page.locator('#epic-key').evaluate(input=>{
        const field=input.getBoundingClientRect(),button=document.querySelector('#import-epic').getBoundingClientRect();
        return {fieldWidth:field.width,buttonWidth:button.width,top:field.top,bottom:field.bottom,buttonTop:button.top,buttonBottom:button.bottom,right:button.right};
      });
      assert.ok(importGeometry.fieldWidth<250 && importGeometry.buttonWidth>=80 && importGeometry.right<=width,'Epic import retains a compact key and usable action at '+width+': '+JSON.stringify(importGeometry));
      assert.ok(importGeometry.top<importGeometry.buttonBottom && importGeometry.bottom>importGeometry.buttonTop,'Epic key and import action share a row at '+width);
      const cards=await page.locator('#stat-cards .stat').evaluateAll(elements=>elements.map(el=>{const r=el.getBoundingClientRect();return {width:r.width,right:r.right};}));
      assert.equal(cards.length,3,'The simulator renders all three forecast percentiles');
      assert.ok(cards.every(card=>card.width>=180 && card.right<=width),'Forecast cards remain readable at '+width);
    }
    await page.locator('#epic-key').fill('PROJ-1');await page.locator('#import-epic').click();
    const help=page.getByRole('button',{name:'Explain the full-kit check',exact:true});await help.waitFor();
    assert.match(await help.getAttribute('data-bs-title'),/Machine-checkable half/,'Imported full-kit guidance uses the shared Bootstrap tooltip');
    assert.equal(await help.getAttribute('title'),await help.getAttribute('data-bs-title'),'Full-kit guidance retains native fallback');
    await page.locator('#kit-tmpl-btn').click();
    assert.equal(await page.locator('#kit-template').isVisible(),true,'The existing full-kit template action remains usable');
    await page.setViewportSize({width:1280,height:960});
    for(const theme of ['light','dark']) {
      await page.evaluate(theme=>{
        document.documentElement.dataset.bsTheme=theme;
        document.querySelector('#explore-menu [data-view="simulator"]').classList.add('active');
      },theme);
      await page.locator('#explore-btn').click();
      const item=page.locator('#explore-menu [data-view="simulator"]');
      const active=await item.evaluate(el=>{
        const style=getComputedStyle(el),probe=document.createElement('span');
        probe.style.backgroundColor='var(--bs-primary-bg-subtle)';el.append(probe);
        const expected=getComputedStyle(probe).backgroundColor;probe.remove();
        return {background:style.backgroundColor,expected};
      });
      assert.equal(active.background,active.expected,'Selected navigation uses the primary theme surface in '+theme);
      await page.keyboard.press('Escape');
    }
    await page.evaluate(async()=>{const {openRosters}=await import('/js/rostersui.js');await openRosters();});
    await page.locator('.ros-pub').waitFor();
    const actions=await page.locator('.ros-pub,.ros-edit,.ros-del').evaluateAll(buttons=>buttons.map(button=>button.getBoundingClientRect().height));
    assert.equal(actions.length,3);assert.ok(actions.every(height=>height>=24 && height<=32),'Roster row actions retain compact usable heights');
    await page.locator('#rosters-close').click();await page.locator('#rosters-overlay').waitFor({state:'hidden'});
    await page.evaluate(async()=>{const {openImport}=await import('/js/importui.js');await openImport();});
    const checkbox=page.locator('.imp-proj input');await checkbox.waitFor();
    const placement=await checkbox.evaluate(input=>{const box=input.getBoundingClientRect(),label=input.closest('label').getBoundingClientRect(),style=getComputedStyle(input);return {left:box.left,labelLeft:label.left,margin:parseFloat(style.marginLeft),float:style.cssFloat};});
    assert.ok(placement.left>=placement.labelLeft && placement.margin>=0,'Project checkbox stays inside its label: '+JSON.stringify(placement));
    assert.equal(placement.float,'none','Standalone Bootstrap checkbox has no float offset');
    await checkbox.focus();await page.keyboard.press('Space');assert.equal(await checkbox.isChecked(),true,'Project selection remains keyboard-operable');
    await page.locator('#imp-close').click();await page.locator('#import-overlay').waitFor({state:'hidden'});
    await page.locator('#help-btn').click();await page.locator('#guide-btn').click();
    const roles=page.getByRole('radiogroup',{name:'Guidance role'});await roles.waitFor();
    const planner=roles.getByRole('radio',{name:'Planning Manager',exact:true});
    await page.locator('label[for="guide-persona-planner"]').click();await planner.focus();await page.keyboard.press('ArrowRight');
    const executive=roles.getByRole('radio',{name:'Executive / VP',exact:true});
    assert.equal(await executive.isChecked(),true,'Arrow navigation selects exactly one guidance role');
    assert.equal(await roles.locator('input:checked').count(),1);
    assert.equal(await executive.evaluate(el=>document.activeElement===el),true,'Changing role preserves keyboard focus after content refresh');
    assert.match(await page.locator('.guide-intro').textContent(),/manage the system/);
    await page.locator('#guide-close').click();await page.locator('#guide-overlay').waitFor({state:'hidden'});
    await page.locator('#help-btn').click();await page.locator('#guide-btn').click();
    assert.equal(await roles.getByRole('radio',{name:'Executive / VP',exact:true}).isChecked(),true,'Reopening guidance retains the selected role');
    await page.locator('#guide-close').click();await page.locator('#guide-overlay').waitFor({state:'hidden'});
  } finally {
    await page.goto('about:blank');
    await page.unroute('**/js/main.js',mainRoute);await page.unroute('**/api/**',apiRoute);
  }
}
