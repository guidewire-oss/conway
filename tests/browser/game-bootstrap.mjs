import assert from 'node:assert/strict';

// specs/011-bootstrap-adoption-debt.md:73 — exercise the actual game renderer,
// using sanitized response fixtures rather than reproducing its button markup.
export async function checkGameBootstrap(page) {
  const base = new URL(page.url()).origin;
  const config = {serverGame:true, gameOpen:true, rounds:4, ap:5, openRound:1};
  const game = {
    round:1, totalRounds:4, over:false, apPerRound:5, apLeft:5,
    score:{total:50}, history:[], levers:[], movesThisRound:[], edges:[],
    pods:[{name:'Atlas', location:'Central', wip:5, rho:0.8, morale:0.8,
      interrupt:1, ktlo:1, readiness:0.8, hygiene:0.8, pairing:true}],
  };
  const staged = [], submitted = [];
  const mainRoute = route => route.fulfill({contentType:'text/javascript',body:''});
  const apiRoute = async route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    let body;
    if(path === '/api/config') body = config;
    else if(['/api/games','/api/plan','/api/snapshots'].includes(path)) body = [];
    else if(path === '/api/game') body = game;
    else if(path === '/api/game/stage' && request.method() === 'POST') {
      const move = request.postDataJSON(); staged.push(move);
      game.movesThisRound = [move]; game.apLeft = 4;
      body = {ok:true,view:game};
    } else if(path === '/api/game/submit' && request.method() === 'POST') {
      submitted.push(structuredClone(game.movesThisRound));
      game.round = 2; game.movesThisRound = [];
      body = {view:game,report:{round:1,headline:'Planned work submitted',scoreDelta:0,
        event:'Normal operations',valueDelivered:1,costFn:1,commitmentsHit:0,narrative:[]}};
    } else {
      await route.fulfill({status:404,json:{error:'Unexpected game fixture request'}});
      return;
    }
    await route.fulfill({json:body});
  };
  const expectedConfirmation="Submit this round? Your planned moves lock in and can't be changed.";
  const dialogs=[];let submitting=false;
  const dialogHandler = async dialog => {
    dialogs.push({type:dialog.type(),message:dialog.message()});
    if(submitting && dialogs.length===1 && dialog.type()==='confirm' && dialog.message()===expectedConfirmation)await dialog.accept();
    else await dialog.dismiss();
  };
  await page.route('**/js/main.js',mainRoute);
  await page.route('**/api/**',apiRoute);
  page.on('dialog',dialogHandler);
  try {
    await page.goto(base+'/index.html');
    await page.evaluate(async () => {
      document.querySelectorAll('.view').forEach(view=>view.classList.remove('active'));
      document.querySelector('#view-game').classList.add('active');
      document.querySelector('#game-net').hidden = true;
      document.querySelector('#game-pods').hidden = false;
      const {initGameUI} = await import('/js/gameui.js');
      await initGameUI();
    });
    const leverButtons = page.locator('#game-levers [data-do]');
    assert.equal(await page.locator('#game-setup').isVisible(),false,'Starting the game hides the setup card in the rendered layout');
    assert.equal(await page.locator('#measure-context').isVisible(),false,'The initial hidden Measure card does not appear over the game');
    for(const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      const geometry=await page.locator('#lv-freeze-pod').evaluate(select=>{
        const field=select.getBoundingClientRect(),input=document.querySelector('#lv-freeze-n').getBoundingClientRect();
        return {width:field.width,top:field.top,bottom:field.bottom,inputTop:input.top,inputBottom:input.bottom};
      });
      assert.ok(geometry.width<200 && geometry.top<geometry.inputBottom && geometry.bottom>geometry.inputTop,'Freeze selector and count remain compact on one row at '+width+': '+JSON.stringify(geometry));
    }
    assert.equal(await leverButtons.count(),11,'All offered game lever actions are rendered');
    assert.equal(await leverButtons.evaluateAll(buttons=>buttons.every(button=>button.classList.contains('btn'))),true,'Lever actions adopt Bootstrap');
    assert.equal(await page.locator('#game-submit.btn.btn-primary').count(),1,'Round submission adopts the primary action primitive');
    const add = page.locator('[data-do="freeze"]');
    await add.focus(); await page.keyboard.press('Enter');
    await page.waitForFunction(()=>document.querySelector('#game-moves')?.textContent.includes('freeze Atlas'));
    assert.deepEqual(staged,[{lever:'freeze',pod:'Atlas',n:5}],'Keyboard staging submits the selected lever and preserves it in the rendered draft');
    assert.equal(await page.locator('#game-levers [data-do]:not(.btn)').count(),0,'Rerendered lever actions retain Bootstrap');
    submitting=true;
    await page.locator('#game-submit').focus(); await page.keyboard.press('Enter');
    await page.locator('#resolve-continue.btn.btn-primary').waitFor();
    submitting=false;
    assert.deepEqual(dialogs,[{type:'confirm',message:expectedConfirmation}],'Submission asks for exactly the expected confirmation');
    assert.deepEqual(submitted,[[{lever:'freeze',pod:'Atlas',n:5}]],'Submission uses the staged move exactly once');
    await page.locator('#resolve-continue').focus(); await page.keyboard.press('Enter');
    await page.locator('#resolve-overlay').waitFor({state:'hidden'});
    assert.equal(await page.locator('#game-levers.card').count(),1,'The containing game panel owns the card surface');
    assert.equal(await page.locator('#game-levers .card').count(),0,'The between-rounds notice does not duplicate its containing panel');
    config.gameOpen = false;
    // Allow several real poll cycles on loaded CI workers; do not bypass detection.
    await page.locator('#halt-ok.btn.btn-primary').waitFor({timeout:30000});
    await page.locator('#halt-ok').focus(); await page.keyboard.press('Enter');
    await page.locator('#halt-overlay').waitFor({state:'hidden'});
    assert.equal(await page.locator('#game-levers > .halt-card').count(),1,'The paused-game explanation remains visible');
    assert.equal(await page.locator('#game-levers .card').count(),0,'The paused-game notice does not duplicate its containing panel');
    assert.equal(await page.locator('#game-submit').count(),0,'The pause transition removes round submission controls');
    await page.evaluate(async()=>{const {openGames}=await import('/js/gamesui.js');await openGames();});
    await page.locator('#games-overlay').waitFor({state:'visible'});
    for(const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      const fields=await page.locator('#g-rounds,#g-ap,#g-timer').evaluateAll(inputs=>inputs.map(input=>{
        const field=input.getBoundingClientRect(),label=input.closest('label');
        const text=document.createRange();text.selectNodeContents(label.firstChild);
        const caption=text.getBoundingClientRect();
        return {width:field.width,top:field.top,bottom:field.bottom,captionTop:caption.top,captionBottom:caption.bottom};
      }));
      assert.ok(fields.every(field=>field.width<100 && field.top<field.captionBottom && field.bottom>field.captionTop),'Game numeric fields remain compact beside their labels at '+width+': '+JSON.stringify(fields));
      const name=await page.locator('#g-name').boundingBox();
      assert.ok(name.x>=0 && name.x+name.width<=width && name.width<400,'Game name fits within the viewport');
    }
    await page.locator('#games-close').click();
    await page.locator('#games-overlay').waitFor({state:'hidden'});
    assert.equal(await page.locator('#game-levers > .halt-card').isVisible(),true,'Closing Games returns to the paused-game explanation');
    assert.deepEqual(dialogs,[{type:'confirm',message:expectedConfirmation}],'No unexpected alert or confirmation is silently accepted');
  } finally {
    // Navigation tears down the real game poll before fixture routes disappear.
    await page.goto('about:blank');
    page.off('dialog',dialogHandler);
    await page.unroute('**/js/main.js',mainRoute);
    await page.unroute('**/api/**',apiRoute);
  }
}
