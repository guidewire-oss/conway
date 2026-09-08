// Real rendered UI acceptance, launched by server/bootstrap_browser_test.go.
import assert from 'node:assert/strict';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const channel = process.env.PLAYWRIGHT_BROWSER_CHANNEL;
const browser = await chromium.launch({headless:true, ...(channel ? {channel} : {})});
const page = await browser.newPage({viewport:{width:1280,height:960}});
const errors = [];
page.on('pageerror', e => errors.push(e.message));
page.on('console', message => { if(message.type()==='error')errors.push('Console: '+message.text()+' '+message.location().url); });
page.on('requestfailed', request => {
  // Intentional navigation tears down fixture requests; other transport failures matter.
  if(request.failure()?.errorText !== 'net::ERR_ABORTED')errors.push('Request: '+request.url()+' '+request.failure()?.errorText);
});
try {
  // The minimal acceptance host has no document icon; avoid an incidental browser 404.
  await page.route('**/favicon.ico',route=>route.fulfill({status:204}));
  await page.goto(process.env.CONWAY_TEST_BASE_URL+'/bootstrap-acceptance');
  await page.evaluate(async () => {
    const {initForms} = await import('/js/forms.js');
    const {mountReadyQueue} = await import('/js/readyqueueui.js');
    const {executionEvidenceHTML} = await import('/js/executionui.js');
    const {timelineControlsHTML,timelineRowHTML,timelineInspectorHTML,podLensHTML} = await import('/js/timeline.js');
    const {baselineListHTML,baselineChipHTML} = await import('/js/baseline.js');
    const {initScoreboard} = await import('/js/scoreboard.js');
    const {orderingBadge,verdictBadgeHTML,schedulingFormHTML,orderHeaderHTML} = await import('/js/order.js');
    const {initHome} = await import('/js/home.js');
    initForms();
    const commitments=document.createElement('div');commitments.id='commitment-controls';
    commitments.innerHTML=orderHeaderHTML({initiatives:[],podWeeks:[]});
    document.querySelector('main').prepend(commitments);
    const item = {initiative:'Atlas acceptance checkpoint',kind:'milestone',state:'ready',canRelease:true,
      plannedStartWeek:0,plannedFinishWeek:0,reasons:[{message:'Acceptance evidence '+ 'reference'.repeat(20)}],checklist:[
        {key:'scope_ready',label:'Scope ready'},
        {key:'dependencies_accepted',label:'Dependencies accepted'},
        {key:'team_available',label:'Team available'}]};
    const context = {team:'Team A',asOfWeek:0,periodStart:'2026-09-07',acceptedOrdering:'stated',basis:'Accepted plan and current evidence'};
    window.acceptedContexts = [];
    await mountReadyQueue(document.querySelector('#ready'), {
      plan:{id:'bootstrap-fixture',horizonWeeks:26,teams:[{name:'Team A'}]},
      request: async () => ({ok:true,json:async()=>({context,items:[item],counts:{total:1,ready:1,waiting:0,deferred:0}})}),
      onContext:(...args)=>window.acceptedContexts.push(args),
      onInspect:()=>{},onReview:()=>{}
    });
    document.querySelector('#evidence').innerHTML = executionEvidenceHTML({coverage:{tracked:1,total:1},
      initiatives:[{name:'Atlas',status:'unknown',epicKeys:['PROJ-1'],suggestions:[{key:'PROJ-2',summary:'Beacon follow-up'}],
        slices:[{pod:'Team A',confidence:'unknown',gaps:[]}]}]});
    document.querySelector('#timeline').innerHTML = timelineControlsHTML({lens:'pod',spans:[{id:'all',label:'Whole period'},{id:'short',label:'Next weeks'}],spanSel:'all',initiativeFilter:'Atlas',teamFilter:'Team A'});
    const badges=orderingBadge()+orderingBadge({acceptedOrdering:'engine'})+['on-time','at-risk','late','no-date','structurally-infeasible','unschedulable','beyond-horizon'].map(verdict=>verdictBadgeHTML({verdict,weeksLate:2})).join('');
    document.querySelector('#badge-examples').innerHTML = ['', 'card p-3', 'bg-body-tertiary p-3'].map(surface=>'<div class="d-flex flex-wrap gap-2 '+surface+'">'+badges+'</div>').join('');
    const sizing=document.createElement('div');sizing.id='framework-sizes';sizing.className='d-flex align-items-center gap-2';
    sizing.innerHTML=['btn-sm','','btn-lg'].map(size=>'<button class="btn btn-secondary '+size+'">Action</button>').join('');
    document.querySelector('main').append(sizing);
    const row=document.createElement('div');row.id='timeline-label-example';
    row.innerHTML=timelineRowHTML({name:'Atlas unavailable',verdict:'unschedulable',slices:[]});
    document.querySelector('main').append(row);
    const baselines=document.createElement('div');baselines.id='baseline-examples';baselines.className='table-responsive';
    baselines.innerHTML=baselineListHTML([{id:'baseline-a',name:'Atlas agreement',active:true},{id:'baseline-b',name:'Beacon agreement'}]);
    document.querySelector('main').append(baselines);
    const chip=document.createElement('div');chip.id='agreement-chip-example';chip.innerHTML=baselineChipHTML([{id:'atlas',name:'Atlas delivery agreement with a long descriptive name',active:true,diverged:true}]);document.querySelector('main').append(chip);
    const inspectors=document.createElement('div');inspectors.id='inspector-examples';
    inspectors.innerHTML=timelineInspectorHTML(null,{})+timelineInspectorHTML({name:'Atlas',verdict:'unschedulable',slices:[]},{});
    document.querySelector('main').append(inspectors);
    const exceptions=document.createElement('div');exceptions.id='timeline-exceptions';
    exceptions.innerHTML=podLensHTML({horizonWeeks:4,initiatives:[{name:'Atlas held',verdict:'unschedulable',slices:[]}],podWeeks:[{pod:'Atlas',tracks:1,weeks:[],slices:[{initiative:'Beacon later',startWeek:6,finishWeek:8,lane:0,lanesUsed:1}]}]}, {planInitiatives:[{name:'Atlas held',work:{Atlas:{inPath:true}}}]});
    document.querySelector('main').append(exceptions);

    const shell=new DOMParser().parseFromString(await (await fetch('/index.html')).text(),'text/html');
    const help=document.createElement('div');help.id='legacy-help-example';
    help.append(shell.querySelector('#view-scoreboard h2 .help'));
    document.querySelector('main').append(help);
    const calendar=document.createElement('div');calendar.innerHTML=schedulingFormHTML({calendars:[{kind:'change-freeze',scope:'org',fromDate:'2026-09-07',toDate:'2026-09-14',effect:'block-start'}]});
    document.querySelector('main').append(calendar.querySelector('.cal-wins'));
    const fever=shell.querySelector('#fever-count').parentElement;
    fever.querySelector('#fever-loading').hidden=false;
    document.querySelector('main').append(fever);
    const home=document.createElement('div');home.id='home-body';document.querySelector('main').append(home);
    await initHome({pods:[{name:'Atlas',location:'Central'}],stats:{Atlas:{wip:3,load:0.5,rho0:0.5}},edges:[]});
    const scoreboard=document.createElement('div');scoreboard.className='table-responsive';
    scoreboard.innerHTML='<table id="score-table" class="table"></table>';
    document.querySelector('main').append(scoreboard);
    const stats={wip:3,throughputWk:1,p50:5,load:0.5,rho0:0.5,sigma:0.5};
    initScoreboard({edges:[],overlap:{},pods:['Atlas','Beacon'].map(name=>({name,location:'Central',devCount:4,streams:2})),stats:{Atlas:{...stats,p85:10},Beacon:{...stats,p85:20}}});
    new bootstrap.Tooltip(document.body,{selector:'[data-bs-toggle="tooltip"], [data-tip], .help',title:el=>el.dataset.bsTitle??el.dataset.tip??'',trigger:'hover focus',placement:'bottom'});
  });
  await page.locator('#ready-search.form-control').waitFor();
  // specs/033-consistent-plan-controls-and-samples.md:35
  for (const theme of ['light','dark']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const peerSizes=await page.locator('#ord-optimize,#sched-open,#tl-open').evaluateAll(nodes=>nodes.map(node=>({height:node.getBoundingClientRect().height,font:getComputedStyle(node).fontSize})));
    assert.equal(peerSizes.length,3);
    assert.ok(peerSizes.every(size=>Math.abs(size.height-peerSizes[0].height)<=1 && size.font===peerSizes[0].font),'Commitment actions share framework sizing: '+JSON.stringify(peerSizes));
    const filterGeometry=await page.locator('#tl-initiative-filter,#tl-team-filter,#tl-fullscreen').evaluateAll(nodes=>nodes.map(node=>({height:node.getBoundingClientRect().height,bottom:node.getBoundingClientRect().bottom,font:getComputedStyle(node).fontSize})));
    assert.equal(filterGeometry.length,3);
    assert.ok(filterGeometry.every(size=>Math.abs(size.height-filterGeometry[0].height)<=1 && Math.abs(size.bottom-filterGeometry[0].bottom)<=1 && size.font===filterGeometry[0].font),'Timeline peers align and share framework sizing: '+JSON.stringify(filterGeometry));
  }
  const calendarWidth=await page.locator('.cal-win').evaluate(row=>({width:row.getBoundingClientRect().width,selects:[...row.querySelectorAll('select')].map(select=>select.getBoundingClientRect().width)}));
  assert.ok(calendarWidth.selects.every(width=>width<calendarWidth.width/2),'Calendar selectors leave room for the other window fields: '+JSON.stringify(calendarWidth));
  const calendarAffordances=await page.locator('.cal-win select').evaluateAll(selects=>selects.map(select=>{const style=getComputedStyle(select);return {arrow:style.backgroundImage,left:parseFloat(style.paddingLeft),right:parseFloat(style.paddingRight)};}));
  const deleteSize=await page.locator('.cal-del').boundingBox();
  assert.ok(deleteSize.width>=24 && deleteSize.height>=24 && deleteSize.height<=32,'Calendar delete remains compact with a usable target: '+JSON.stringify(deleteSize));
  assert.ok(calendarAffordances.every(select=>select.arrow!=='none' && select.right>select.left),'Calendar selectors retain a visible arrow and its reserved text spacing: '+JSON.stringify(calendarAffordances));
  const feverGeometry=await page.locator('#fever-count').evaluate(select=>{const field=select.getBoundingClientRect(),status=document.querySelector('#fever-loading').getBoundingClientRect();return {width:field.width,top:field.top,bottom:field.bottom,statusTop:status.top,statusBottom:status.bottom};});
  assert.ok(feverGeometry.width<120,'Fever sample count stays compact');
  assert.ok(feverGeometry.top<feverGeometry.statusBottom && feverGeometry.statusTop<feverGeometry.bottom,'Fever loading feedback stays alongside its selector');
  // per specs/011-bootstrap-adoption-debt.md:73
  const buttonSizes=await page.locator('#framework-sizes button').evaluateAll(buttons=>buttons.map(button=>({font:parseFloat(getComputedStyle(button).fontSize),height:button.getBoundingClientRect().height})));
  assert.ok(buttonSizes[0].font<buttonSizes[1].font && buttonSizes[1].font<buttonSizes[2].font,'Bootstrap small, default and large button text sizes remain distinct: '+JSON.stringify(buttonSizes));
  assert.ok(buttonSizes[0].height<buttonSizes[1].height && buttonSizes[1].height<buttonSizes[2].height,'Bootstrap size modifiers preserve distinct control heights');
  const labelGeometry=await page.locator('#timeline-label-example .tl-label').evaluate(label=>{const style=getComputedStyle(label);return {align:style.textAlign,padding:[style.paddingTop,style.paddingRight,style.paddingBottom,style.paddingLeft].map(parseFloat)};});
  assert.equal(labelGeometry.align,'left','Timeline labels stay aligned with their team rows');
  assert.deepEqual(labelGeometry.padding,[0,10,0,0],'Timeline labels keep their ten-pixel track gap without generic action-button padding');
  const timelineLabel=page.locator('#timeline-label-example .tl-label');
  await timelineLabel.focus();await page.keyboard.press('Tab');await page.keyboard.press('Shift+Tab');
  await timelineLabel.evaluate(el=>Promise.all(el.getAnimations().map(animation=>animation.finished.catch(()=>{}))));
  const labelFocus=await timelineLabel.evaluate(el=>({active:el===document.activeElement,outline:getComputedStyle(el).outlineStyle,shadow:getComputedStyle(el).boxShadow}));
  assert.equal(labelFocus.active,true);assert.notEqual(labelFocus.outline,'none');assert.equal(labelFocus.shadow,'none','Timeline labels retain one domain focus outline');
  await timelineLabel.evaluate(el=>el.blur());
  const pngDownload=page.waitForEvent('download',{timeout:15000}).catch(()=>null);
  const exported=await page.evaluate(async()=>{
    const {exportBlockPNG}=await import('/js/exportpng.js');
    const serialize=XMLSerializer.prototype.serializeToString;
    XMLSerializer.prototype.serializeToString=function(node){const text=serialize.call(this,node);window.exportedTimelineMarkup=text;return text;};
    try{return await exportBlockPNG(document.querySelector('#timeline-label-example'),'timeline-acceptance.png');}
    finally{XMLSerializer.prototype.serializeToString=serialize;}
  });
  assert.equal(exported,true,'The timeline produces a PNG artifact');
  const png=await pngDownload;
  assert.ok(png,'PNG export offers a download');
  assert.equal(png.suggestedFilename(),'timeline-acceptance.png');
  const exportLabels=await page.evaluate(()=>{
    const doc=new DOMParser().parseFromString(window.exportedTimelineMarkup,'application/xml');
    return {buttons:doc.querySelectorAll('button, .btn').length,names:[...doc.querySelectorAll('.tl-label')].map(el=>el.textContent)};
  });
  assert.equal(exportLabels.buttons,0,'Meeting exports contain plain initiative labels rather than interactive button chrome');
  assert.deepEqual(exportLabels.names,['Atlas unavailable'],'Export keeps the initiative evidence');
  const legacyHelp=page.locator('#legacy-help-example .help');
  assert.equal(await legacyHelp.evaluate(el=>el instanceof HTMLButtonElement),true,'Legacy metric help remains a native keyboard action');
  assert.equal(await legacyHelp.evaluate(el=>el.classList.contains('btn')),true,'Metric help adopts the same Bootstrap action primitive');
  const helpType=await legacyHelp.evaluate(el=>{const style=getComputedStyle(el);return {font:parseFloat(style.fontSize),line:parseFloat(style.lineHeight)};});
  assert.ok(helpType.line<=helpType.font+0.1,'Compact help uses a tight line box: '+JSON.stringify(helpType));
  assert.ok((await legacyHelp.getAttribute('aria-label') || '').trim().length>1,'Metric help has an explanatory accessible name');
  await legacyHelp.focus();
  assert.equal(await legacyHelp.evaluate(el=>document.activeElement===el),true,'Metric help receives keyboard focus');
  // per specs/011-bootstrap-adoption-debt.md:83
  const scoreboardRows=()=>page.locator('#score-table tbody tr td:first-child').allTextContents();
  const originalRows=await scoreboardRows();
  assert.deepEqual(originalRows,['Beacon','Atlas'],'The scoreboard starts with descending cycle P85');
  const cycleHelp=page.locator('#score-table th[data-i="7"] .help');
  await cycleHelp.focus();
  await page.waitForFunction(()=>!!document.querySelector('#score-table th[data-i="7"] .help')?.getAttribute('aria-describedby'));
  const tooltipID=await cycleHelp.getAttribute('aria-describedby');
  assert.match(await page.locator('#'+tooltipID).textContent(),/85th percentile/,'Keyboard focus exposes the real Bootstrap help tooltip');
  await page.keyboard.press('Enter');
  assert.deepEqual(await scoreboardRows(),originalRows,'Activating column help does not change the selected sort');
  assert.equal(await cycleHelp.evaluate(el=>document.activeElement===el),true,'Help activation preserves focus instead of replacing the header');
  await page.locator('#score-table th[data-i="7"]').click({position:{x:5,y:5}});
  assert.deepEqual(await scoreboardRows(),['Atlas','Beacon'],'The surrounding column header still changes the sort direction');
  const comparisonGeometry=await page.locator('#baseline-examples .bl-compare').first().evaluate(button=>{
    const buttonRect=button.getBoundingClientRect(),selectRect=button.parentElement.querySelector('select').getBoundingClientRect();
    return {buttonTop:buttonRect.top,buttonBottom:buttonRect.bottom,selectTop:selectRect.top,selectBottom:selectRect.bottom};
  });
  assert.ok(comparisonGeometry.buttonTop<comparisonGeometry.selectBottom && comparisonGeometry.selectTop<comparisonGeometry.buttonBottom,'Saved-agreement compare controls stay on the same desktop action row: '+JSON.stringify(comparisonGeometry));
  const baselineActions=await page.locator('#baseline-examples .bl-activate,#baseline-examples .bl-compare,#baseline-examples .bl-delete').evaluateAll(buttons=>buttons.map(button=>button.getBoundingClientRect().height));
  assert.equal(baselineActions.length,5,'Two agreement rows offer compare/delete and one offers activation');
  assert.ok(baselineActions.every(height=>height>=24 && height<=32),'Agreement history actions match compact controls: '+JSON.stringify(baselineActions));
  const inspectorPadding=[];
  for(const width of [1280,360]) {
    await page.setViewportSize({width,height:960});
    inspectorPadding.push(await page.locator('#inspector-examples .tl-inspector').evaluateAll(elements=>elements.map(el=>parseFloat(getComputedStyle(el).paddingLeft))));
  }
  assert.ok(inspectorPadding[1].every((padding,index)=>padding<inspectorPadding[0][index] && padding>=8),'Both empty and selected initiative inspectors compact their padding on mobile');
  // per specs/011-bootstrap-adoption-debt.md:67
  const group = page.getByRole('group',{name:'Timeline grouping',exact:true});
  assert.equal(await group.locator('button:not(.btn)').count(),0,'Every grouping option must adopt the Bootstrap button primitive');
  assert.equal(await group.locator('button.active[aria-pressed="true"]').count(),1,'Visible active state must agree with the announced selection');
  assert.equal(await group.locator('button:not(.active)[aria-pressed="false"]').count(),1);
  assert.equal(await page.locator('#ready button:not(.btn), #evidence button:not(.btn)').count(),0,'Operational actions must use Bootstrap buttons');
  assert.equal(await page.locator('.ready-item.card').count(),1,'Ready work uses the shared Bootstrap card primitive');
  assert.equal(await page.locator('.execution-initiative.card').count(),1,'Execution evidence uses the same card primitive');
  assert.equal(await page.locator('#ready .badge').count()>0,true,'Queue counts use Bootstrap badges');
  assert.equal(await page.locator('#evidence .badge').count()>0,true,'Evidence status uses Bootstrap badges');
  // Native details remain keyboard-operable without replacing the disclosure.
  const summary = page.locator('.ready-decision > summary');
  await summary.focus(); await page.keyboard.press('Enter');
  const decision = page.locator('[data-ready-decision]');
  await decision.waitFor({state:'visible'});
  await decision.locator('[name=decision]').selectOption('defer');
  await decision.locator('[name=owner]').fill('Team lead');
  await decision.locator('[name=evidence]').fill('Wait for acceptance evidence');
  await page.locator('.ready-checklist > summary').click();
  await page.locator('[name=scope_ready_checked]').check();
  await page.locator('[name=scope_ready_owner]').fill('Delivery manager');
  assert.equal(await page.locator('[name=scope_ready_checked].form-check-input').count(),1);
  assert.equal(await decision.locator('[name=decision].form-select').count(),1);
  assert.equal(await decision.locator('[name=evidence].form-control').count(),1);
  // One-at-a-time insertion exercises the observer's added-node path.
  await page.evaluate(()=>{
    const dynamic = document.querySelector('#dynamic');
    const input = document.createElement('input'); input.id='late-input'; input.value='Existing draft'; dynamic.append(input);
  });
  await page.locator('#late-input.form-control').waitFor();
  const measureContrast = locator => locator.evaluateAll(badges=>{
      const canvas=document.createElement('canvas');canvas.width=canvas.height=1;
      const context=canvas.getContext('2d',{willReadFrequently:true});
      const rgba=color=>{
        context.clearRect(0,0,1,1);context.fillStyle=color;context.fillRect(0,0,1,1);
        const pixel=[...context.getImageData(0,0,1,1).data];return [...pixel.slice(0,3),pixel[3]/255];
      };
      const composite=(front,back)=>front.slice(0,3).map((value,index)=>value*front[3]+back[index]*(1-front[3]));
      const luminance=color=>color.map(value=>value/255).map(value=>value<=0.04045?value/12.92:((value+0.055)/1.055)**2.4).reduce((sum,value,index)=>sum+value*[0.2126,0.7152,0.0722][index],0);
      return badges.map(badge=>{
        const ancestors=[];for(let node=badge;node;node=node.parentElement)ancestors.unshift(node);
        const background=ancestors.reduce((color,node)=>composite(rgba(getComputedStyle(node).backgroundColor),color),[255,255,255]);
        const foreground=composite(rgba(getComputedStyle(badge).color),background);
        const light=luminance(foreground),dark=luminance(background);
        return {label:badge.textContent,foreground,background,ratio:(Math.max(light,dark)+0.05)/(Math.min(light,dark)+0.05)};
      });
    });
  const colors = [];
  const neutralSurfaces=[];
  for (const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const cardTheme=await page.locator('.ready-item').evaluate(card=>{
      const background=getComputedStyle(card).backgroundColor;
      const canvas=document.createElement('canvas');canvas.width=canvas.height=1;
      const context=canvas.getContext('2d');context.fillStyle=background;context.fillRect(0,0,1,1);
      const alpha=context.getImageData(0,0,1,1).data[3];
      const ancestor=card.parentElement;
      const prior=ancestor.style.getPropertyValue('--bs-card-bg');
      ancestor.style.setProperty('--bs-card-bg','rgb(24, 45, 68)');
      const inherited=getComputedStyle(card).backgroundColor;
      if(prior)ancestor.style.setProperty('--bs-card-bg',prior);else ancestor.style.removeProperty('--bs-card-bg');
      return {background,alpha,inherited};
    });
    assert.equal(cardTheme.alpha,255,'Real operational cards stay opaque in '+theme+': '+cardTheme.background);
    assert.equal(cardTheme.inherited,'rgb(24, 45, 68)','A themed ancestor can supply the Bootstrap card surface in '+theme);
    const badgeContrast = await measureContrast(page.locator('#badge-examples .badge'));
    assert.equal(badgeContrast.length,27,'Both orderings and all seven verdicts render on body, card and raised surfaces');
    for(const badge of badgeContrast) assert.ok(badge.ratio>=4.5,theme+' '+badge.label+' badge contrast '+badge.ratio.toFixed(2)+' must reach 4.5:1 '+JSON.stringify(badge));
    const neutral=await measureContrast(page.locator('#evidence h4 .badge,#ready .ready-item-heading .badge'));
    assert.equal(neutral.length,2);for(const badge of neutral)assert.ok(badge.ratio>=4.5,theme+' neutral workflow badge contrast '+JSON.stringify(badge));
    neutralSurfaces.push(neutral.map(badge=>badge.background));
    const infoTokens=await page.evaluate(()=>{const style=getComputedStyle(document.documentElement);return ['text-emphasis','bg-subtle','border-subtle'].map(suffix=>[style.getPropertyValue('--bs-info-'+suffix).trim(),style.getPropertyValue('--bs-primary-'+suffix).trim()]);});
    assert.ok(infoTokens.every(([info,primary])=>info===primary),'Info companion tokens follow their primary theme alias in '+theme);
    const homeAlerts=page.locator('#home-body .home-alert');
    assert.equal(await homeAlerts.count(),2,'The real home renderer supplies warning and healthy-load alerts');
    for(let index=0;index<await homeAlerts.count();index++) {
      const alert=homeAlerts.nth(index);
      await page.mouse.move(0,0);await alert.evaluate(el=>el.blur());
      for(const state of ['normal','hover','active']) {
        if(state==='hover')await alert.hover();
        if(state==='active')await page.mouse.down();
        try {
          await alert.evaluate(el=>Promise.all(el.getAnimations().map(animation=>animation.finished.catch(()=>{}))));
          const [contrast]=await measureContrast(alert.locator('b'));
          console.log(JSON.stringify({theme,state,homeAlertContrast:contrast}));
          assert.ok(contrast.ratio>=4.5,theme+' home alert '+state+' text contrast '+contrast.ratio.toFixed(2)+' must reach 4.5:1 '+JSON.stringify(contrast));
        } finally {if(state==='active')await page.mouse.up();}
      }
    }
    for (const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,theme+' '+width+' has no page-level horizontal overflow');
      const homeHints=await page.locator('#home-body .home-alert .hint,#home-body .home-act .hint').evaluateAll(hints=>hints.map(el=>({text:el.textContent,whiteSpace:getComputedStyle(el).whiteSpace,scroll:el.scrollWidth,client:el.clientWidth})));
      assert.ok(homeHints.length>0 && homeHints.every(hint=>hint.whiteSpace==='normal' && hint.scroll<=hint.client),'Home subtitles wrap within their cards: '+JSON.stringify(homeHints));

      assert.equal(await decision.locator('[name=decision]').inputValue(),'defer');
      assert.equal(await decision.locator('[name=owner]').inputValue(),'Team lead');
      assert.equal(await decision.locator('[name=evidence]').inputValue(),'Wait for acceptance evidence');
      assert.equal(await page.locator('[name=scope_ready_checked]').isChecked(),true);
      assert.equal(await page.locator('#late-input').inputValue(),'Existing draft');
      assert.equal(await page.locator('#tl-initiative-filter').inputValue(),'Atlas');
      assert.equal(await page.locator('#tl-team-filter').inputValue(),'Team A');
      for(const name of ['Hide teams without matching work','Show other work']) {
        const checkbox=page.getByRole('checkbox',{name,exact:true});
        const placement=await checkbox.evaluate(input=>({labels:input.labels.length,margin:parseFloat(getComputedStyle(input).marginLeft),float:getComputedStyle(input).cssFloat,left:input.getBoundingClientRect().left,labelLeft:input.closest('label').getBoundingClientRect().left}));
        assert.equal(placement.labels,1);assert.equal(placement.float,'none');assert.ok(placement.margin>=0 && placement.left>=placement.labelLeft,'Timeline checkbox remains inside its associated label: '+JSON.stringify(placement));
      }
      const exceptionActions=await page.locator('#timeline-exceptions .tl-unplaced-list button,#timeline-exceptions .tl-outside-list button').evaluateAll(buttons=>buttons.map(button=>({name:button.dataset.selectInit,height:button.getBoundingClientRect().height})));
      assert.deepEqual(exceptionActions.map(action=>action.name).sort(),['Atlas held','Beacon later']);
      assert.ok(exceptionActions.every(action=>action.height>=24 && action.height<=32),'Timeline exception actions stay compact and usable');
      const chip=await page.locator('#bl-chip').evaluate(el=>({whiteSpace:getComputedStyle(el).whiteSpace,width:el.getBoundingClientRect().width,parent:el.parentElement.getBoundingClientRect().width,scroll:el.scrollWidth,client:el.clientWidth}));
      assert.equal(chip.whiteSpace,'normal');assert.ok(chip.width<=chip.parent && chip.scroll<=chip.client,'Long agreement status wraps within the available header width: '+JSON.stringify(chip));
      const owner = decision.locator('[name=owner]'); await owner.focus();
      assert.equal(await owner.evaluate(el=>document.activeElement===el),true);
      assert.equal(await owner.evaluate(el=>getComputedStyle(el).boxShadow!=='none'),true,'Bootstrap focus indicator remains visible in '+theme);
      await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-bootstrap-'+theme+'-'+width+'.png'),fullPage:true});
    }
    // per specs/011-bootstrap-adoption-debt.md:80
    const primary=decision.locator('button.btn-primary');
    await page.mouse.move(0,0); await primary.evaluate(el=>el.blur());
    const settled=()=>primary.evaluate(el=>Promise.all(el.getAnimations().map(animation=>animation.finished.catch(()=>{}))));
    for(const state of ['normal','hover']) {
      if(state==='hover') await primary.hover();
      await settled();
      const [result]=await measureContrast(primary);
      assert.ok(result.ratio>=4.5,theme+' primary '+state+' contrast '+result.ratio.toFixed(2)+' must reach 4.5:1 '+JSON.stringify(result));
    }
    await page.mouse.move(0,0); await primary.focus();
    await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab'); await settled();
    const focus=await primary.evaluate(el=>({active:document.activeElement===el,shadow:getComputedStyle(el).boxShadow}));
    assert.equal(focus.active,true,'Primary action receives keyboard focus in '+theme);
    assert.notEqual(focus.shadow,'none','Primary action renders its focus shadow in '+theme);
    colors.push(await page.locator('.ready-item').evaluate(el=>getComputedStyle(el).backgroundColor));
  }
  assert.notEqual(colors[0],colors[1],'Bootstrap cards must respond to both theme modes');
  assert.notDeepEqual(neutralSurfaces[0],neutralSurfaces[1],'Operational metadata responds to both theme modes');
  assert.deepEqual(await page.evaluate(()=>window.acceptedContexts),[['Team A',0]],'Theme and viewport changes preserve the accepted queue context');
  // per specs/012-in-app-usage-guide.md:228
  await page.goto(process.env.CONWAY_TEST_BASE_URL+'/docs.html');
  await page.setViewportSize({width:360,height:800});
  const search = page.getByRole('searchbox',{name:'Search this guide'});
  const contents = page.locator('#contents-toggle');
  const results = page.getByRole('region',{name:'Search results'});
  await page.locator('html.reader-ready').waitFor();
  await page.setViewportSize({width:1280,height:960});
  for(const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const shading=await page.locator('main table').first().evaluate(table=>({header:getComputedStyle(table.querySelector('thead th')).backgroundColor,body:getComputedStyle(table.querySelector('tbody td')).backgroundColor}));
    assert.notEqual(shading.header,shading.body,'Guide table headings remain visually distinct in '+theme);
    const desktopPadding=await page.locator('main table').first().locator('td').first().evaluate(cell=>{const style=getComputedStyle(cell);return [style.paddingTop,style.paddingRight,style.paddingBottom,style.paddingLeft].map(parseFloat);});
    assert.deepEqual(desktopPadding,[12,12,12,12],'Desktop guide tables preserve readable twelve-pixel cell padding');
  }
  await page.setViewportSize({width:360,height:800});
  assert.equal(await page.locator('.input-group #manual-search.form-control').count(),1,'Reader search uses a Bootstrap input group');
  assert.equal(await page.locator('#contents-toggle.btn').count(),1);
  assert.equal(await page.locator('#search-clear.btn').count(),1);
  // per specs/012-in-app-usage-guide.md:233
  await page.evaluate(()=>document.documentElement.dataset.bsTheme='dark');
  await page.emulateMedia({media:'print'});
  const printCells = await page.locator('main table').first().locator('th,td').evaluateAll(cells=>cells.map(cell=>{
    const style=getComputedStyle(cell);
    return {color:style.color,background:style.backgroundColor};
  }));
  assert.ok(printCells.length>0,'Print acceptance includes actual guide table cells');
  for (const cell of printCells) {
    const channels = value => {
      const components=value.match(/[\d.]+/g) || [];
      assert.ok(components.length>=3,'Printed table color must resolve to RGB channels: '+value);
      return components.slice(0,3).map(Number);
    };
    assert.ok(channels(cell.color).every(value=>value<=80),'Printed table text stays dark: '+cell.color);
    assert.ok(channels(cell.background).every(value=>value>=220),'Printed table background stays light: '+cell.background);
  }
  await page.emulateMedia({media:'screen'});
  for (const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const mobileLabels=await page.locator('main table').first().evaluate(table=>{const cells=table.querySelectorAll('tbody tr:first-child td');return {label:getComputedStyle(cells[0]).backgroundColor,content:getComputedStyle(cells[1]).backgroundColor};});
    assert.notEqual(mobileLabels.label,mobileLabels.content,'Mobile reference rows distinguish their label from content in '+theme);
    const mobilePadding=await page.locator('main table').first().locator('td').first().evaluate(cell=>{const style=getComputedStyle(cell);return [style.paddingTop,style.paddingRight,style.paddingBottom,style.paddingLeft].map(parseFloat);});
    assert.deepEqual(mobilePadding,[10,14,10,14],'Mobile guide cards preserve readable vertical and horizontal padding');
    await search.fill('critical chain');
    await results.waitFor({state:'visible'});
    assert.ok(await results.locator('a').count()>0,'Guide search still finds its planning concepts');
    await page.keyboard.press('ArrowDown');
    assert.equal(await results.locator('a').first().evaluate(el=>document.activeElement===el),true,'ArrowDown reaches a search result');
    await page.keyboard.press('Escape');
    assert.equal(await results.isVisible(),false,'Escape dismisses guide search');
    assert.equal(await search.evaluate(el=>document.activeElement===el),true,'Search dismissal returns focus');
    await page.getByRole('button',{name:'Clear search',exact:true}).click();
    assert.equal(await search.inputValue(),'');
    assert.equal(await search.evaluate(el=>document.activeElement===el),true);
    await contents.focus(); await page.keyboard.press('Enter');
    assert.equal(await contents.getAttribute('aria-expanded'),'true');
    await page.locator('#manual-contents').waitFor({state:'visible'});
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Mobile guide contents fit in '+theme);
    await page.keyboard.press('Escape');
    assert.equal(await contents.getAttribute('aria-expanded'),'false');
    assert.equal(await contents.evaluate(el=>document.activeElement===el),true);
    await contents.click();
    await page.locator('#manual-contents a[href="#foundations"]').click();
    assert.equal(new URL(page.url()).hash,'#foundations');
    await page.waitForFunction(()=>{const top=document.querySelector('#foundations').getBoundingClientRect().top;return top>=0 && top<300;});
    assert.equal(await page.locator('#foundations').evaluate(el=>document.activeElement===el),true,'Contents selection focuses the requested section');
    assert.equal(await contents.getAttribute('aria-expanded'),'false');
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Guide content fits in '+theme);
    await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-bootstrap-guide-'+theme+'-360.png')});
  }
  await page.route('**/api/plan',route=>route.fulfill({json:[{id:'atlas-plan',name:'Atlas plan',estimateModel:'effort',baselineCount:2}]}));
  await page.goto(process.env.CONWAY_TEST_BASE_URL+'/bootstrap-acceptance');
  await page.evaluate(async()=>{
    const root=document.createElement('div');root.id='plan-root';document.querySelector('main').append(root);
    const {initPlanUI,restorePlanLocation}=await import('/js/planui.js');
    initPlanUI();await restorePlanLocation({view:'plan'});
    const shell=new DOMParser().parseFromString(await (await fetch('/index.html')).text(),'text/html');
    const host=shell.querySelector('#measure-context');document.querySelector('main').append(host);
    const {mountMeasureContext}=await import('/js/measure-context.js');
    window.measureContext=mountMeasureContext(host,{state:{},view:'plan',request:async()=>({ok:true,json:async()=>[]})});
    await window.measureContext.ready;
    const {helpButton,term}=await import('/js/terms.js');
    const gaps=document.createElement('div');gaps.id='help-gap-fixture';
    gaps.innerHTML=['Before'+helpButton('Context','context'),term('wip','WIP'),'Before'+term('wip')].map(markup=>'<span class="d-inline-block me-3">'+markup+'</span>').join('');document.querySelector('main').append(gaps);
    const help=document.createElement('div');help.id='fallback-help';help.innerHTML=helpButton('Use "accepted" & evidence <only>; literal &amp;','acceptance');document.querySelector('main').append(help);
  });
  assert.equal(await page.locator('#fallback-help button').getAttribute('title'),'Use "accepted" & evidence <only>; literal &amp;','Shared help retains an escaped native explanation before tooltip initialization');
  const helpGaps=await page.locator('#help-gap-fixture > span').evaluateAll(spans=>spans.map(span=>{
    const text=document.createRange();text.selectNodeContents(span.firstChild);const button=span.querySelector('button');
    return {gap:button.getBoundingClientRect().left-text.getBoundingClientRect().right,margin:parseFloat(getComputedStyle(button).marginLeft)};
  }));
  assert.equal(helpGaps.length,3);assert.ok(helpGaps.every(gap=>Math.abs(gap.gap-gap.margin)<1),'Bootstrap margin supplies the only contextual/glossary gap: '+JSON.stringify(helpGaps));
  await page.evaluate(()=>{window.acceptanceTooltip=new bootstrap.Tooltip(document.querySelector('#fallback-help button'),{trigger:'manual',animation:false});window.acceptanceTooltip.show();});
  assert.equal(await page.locator('.tooltip-inner').textContent(),'Use "accepted" & evidence <only>; literal &amp;','Bootstrap tooltip displays decoded attribute text without losing literal entity text');
  await page.evaluate(()=>{window.acceptanceTooltip.dispose();delete window.acceptanceTooltip;});
  for(const view of ['plan','network','game','scoreboard','home']) {
    await page.evaluate(view=>window.measureContext.setView(view),view);
    assert.equal(await page.locator('#measure-context').isVisible(),['network','scoreboard','home'].includes(view),'Measure source card visibility follows its owning view: '+view);
  }
  const planSurfaces=[];
  for(const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const badges=await measureContrast(page.locator('#plan-root .badge'));
    assert.equal(badges.length,2,'The real plan list renders estimate and agreement metadata');
    for(const badge of badges)assert.ok(badge.ratio>=4.5,theme+' plan metadata contrast: '+JSON.stringify(badge));
    planSurfaces.push(badges[0].background);
  }
  assert.notDeepEqual(planSurfaces[0],planSurfaces[1],'Plan metadata surfaces follow the active theme');
  const setupPlan={id:'atlas-plan',name:'Atlas plan',horizonWeeks:26,capacityLoss:0,teams:[],initiatives:[]};
  const savedSettings=[];
  await page.route('**/api/plan/atlas-plan',async route=>{
    if(route.request().method()==='PATCH') {const update=route.request().postDataJSON();savedSettings.push(update);Object.assign(setupPlan,update);}
    await route.fulfill({json:setupPlan});
  });
  await page.route('**/api/plan/atlas-plan/baseline',route=>route.fulfill({json:{baselines:[]}}));
  await page.route('**/api/rosters',route=>route.fulfill({json:[]}));
  await page.evaluate(async()=>{const {restorePlanLocation}=await import('/js/planui.js');await restorePlanLocation({view:'plan',plan:'atlas-plan'});});
  for(const theme of ['light','dark']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    for(const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      const settings=await page.locator('#plan-horizon,#plan-loss,#plan-save').evaluateAll(controls=>controls.map(el=>{const r=el.getBoundingClientRect();return {left:r.left,right:r.right,width:r.width,height:r.height};}));
      assert.equal(settings.length,3);
      assert.ok(settings.every(control=>control.left>=0 && control.right<=width && control.width>=44 && control.height>=24),'New plan settings remain usable inside '+theme+' '+width+': '+JSON.stringify(settings));
    }
  }
  await page.locator('#plan-horizon').fill('39');await page.locator('#plan-loss').fill('15');
  await page.locator('#plan-save').focus();await page.keyboard.press('Enter');
  await page.waitForFunction(()=>document.querySelector('.plan-setup summary')?.textContent.includes('15% capacity loss'));
  assert.deepEqual(savedSettings,[{horizonWeeks:39,capacityLoss:0.15}],'Responsive settings retain the existing save workflow');
  assert.equal(await page.locator('#plan-horizon').inputValue(),'39');
  assert.equal(await page.locator('#plan-loss').inputValue(),'15');
  setupPlan.teams=[{name:'Atlas',tracks:2},{name:'Beacon',tracks:2}];
  setupPlan.initiatives=[{name:'Delivery checkpoint',work:{Atlas:{weeks:2,inPath:true}}}];
  const simulation={loads:[],initiatives:[],constraints:0,fitting:1,total:1,medianLeadWeeks:2};
  await page.route('**/api/plan/atlas-plan/simulate',route=>route.fulfill({json:{before:simulation,after:simulation}}));
  await page.route('**/api/plan/atlas-plan/sites',route=>route.fulfill({json:{sites:[]}}));
  await page.evaluate(async()=>{const {restorePlanLocation}=await import('/js/planui.js');await restorePlanLocation({view:'plan',plan:'atlas-plan',planView:'network'});});
  await page.locator('#lev-type').waitFor();
  for(const width of [1280,360]) {
    await page.setViewportSize({width,height:960});
    for(const type of ['addCapacity','unpair','descope','defer','reduceWip','reassign','dropPod']) {
      await page.locator('#lev-type').selectOption(type);
      const controls=await page.locator('#lev-type,#lev-target select,#lev-target input,#lev-add').evaluateAll(els=>els.map(el=>{const r=el.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom,width:r.width};}));
      assert.ok(controls.every(c=>c.left>=0 && c.right<=width && c.width>=40),'Lever '+type+' controls fit '+width+': '+JSON.stringify(controls));
      if(width===1280)assert.ok(Math.max(...controls.map(c=>c.top))<Math.min(...controls.map(c=>c.bottom)),'Lever '+type+' controls share a desktop row');
    }
  }
  await page.unroute('**/api/plan/atlas-plan/simulate');await page.unroute('**/api/plan/atlas-plan/sites');
  await page.unroute('**/api/plan/atlas-plan');await page.unroute('**/api/plan/atlas-plan/baseline');await page.unroute('**/api/rosters');
  await page.unroute('**/api/plan');
  const {checkMeasureBootstrap}=await import('./measure-bootstrap.mjs');
  await checkMeasureBootstrap(page);
  await page.goto(process.env.CONWAY_TEST_BASE_URL+'/bootstrap-acceptance');
  const {checkGameBootstrap}=await import('./game-bootstrap.mjs');
  await checkGameBootstrap(page);
  assert.deepEqual(errors,[]);
  console.log(JSON.stringify({bootstrapControls:true,dynamicInsertion:true,selectedGroups:true,nativeDisclosureKeyboard:true,draftsRetained:true,guideSearchAndContents:true,readablePrint:true,badgeContrast:true,themes:['dark','light'],mobileOverflow:false,pageErrors:errors}));
} catch (error) {
  try { console.error(await page.locator('body').innerText({timeout:2000})); }
  catch (diagnosticError) { console.error('Body diagnostic unavailable:',diagnosticError.message); }
  try { await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-bootstrap-failure.png'),fullPage:true,timeout:2000}); }
  catch (diagnosticError) { console.error('Screenshot diagnostic unavailable:',diagnosticError.message); }
  throw error;
} finally { await browser.close(); }
