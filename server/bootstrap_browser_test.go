package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per specs/011-bootstrap-adoption-debt.md:58
// per specs/011-bootstrap-adoption-debt.md:67
// per specs/011-bootstrap-adoption-debt.md:73
// per specs/011-bootstrap-adoption-debt.md:77
// per specs/011-bootstrap-adoption-debt.md:81
var _ = Describe("linked features browser Bootstrap adoption", Label("browser"), func() {
	// per specs/011-bootstrap-adoption-debt.md:73
	// per specs/011-bootstrap-adoption-debt.md:77
	// per specs/011-bootstrap-adoption-debt.md:81
	// per specs/012-in-app-usage-guide.md:228
	It("retains operational drafts and accessible controls through dynamic rendering and theme changes", func() {
		if os.Getenv("CONWAY_TEST_BROWSER") != "1" {
			Skip("Set CONWAY_TEST_BROWSER=1 with Playwright; no database is required.")
		}
		app, err := filepath.Abs("../app")
		Expect(err).NotTo(HaveOccurred())
		mux := http.NewServeMux()
		mux.HandleFunc("/bootstrap-acceptance", func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, writeErr := w.Write([]byte(`<!doctype html><html lang="en" data-bs-theme="dark"><head>
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/vendor/bootstrap/bootstrap.min.css">
<link rel="stylesheet" href="/css/conway.css"><link rel="stylesheet" href="/css/style.css">
<link rel="stylesheet" href="/css/planning-ux.css"><link rel="stylesheet" href="/css/execution.css">
<link rel="stylesheet" href="/css/readyqueue.css"></head><body>
<main class="container-fluid p-3"><div id="ready"></div><div id="evidence" class="execution-review"></div><div id="timeline"></div><div id="dynamic"></div><div id="badge-examples" class="d-flex flex-column gap-2"></div></main>
<script src="/vendor/d3.min.js"></script><script src="/vendor/bootstrap/bootstrap.bundle.min.js"></script></body></html>`))
			Expect(writeErr).NotTo(HaveOccurred())
		})
		mux.Handle("/", http.FileServer(http.Dir(app)))
		host := httptest.NewServer(mux)
		DeferCleanup(host.Close)
		node := os.Getenv("CONWAY_TEST_NODE")
		if node == "" {
			node = "node"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", bootstrapAdoptionBrowser)
		cmd.Env = append(os.Environ(), "CONWAY_TEST_BASE_URL="+host.URL)
		output, err := cmd.CombinedOutput()
		GinkgoWriter.Printf("%s", output)
		Expect(ctx.Err()).NotTo(HaveOccurred(), "Bootstrap browser workload exceeded its three-minute execution limit; inspect browser output before attributing this to a product assertion.")
		Expect(err).NotTo(HaveOccurred(), "%s", output)
	})
})

// The browser executes real view modules and their shared adoption entry point.
// Only the ready queue response is a fixture; assertions concern rendered user
// controls and retained values, not snapshots of implementation source text.
const bootstrapAdoptionBrowser = `
import assert from 'node:assert/strict';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const channel = process.env.PLAYWRIGHT_BROWSER_CHANNEL;
const browser = await chromium.launch({headless:true, ...(channel ? {channel} : {})});
const page = await browser.newPage({viewport:{width:1280,height:960}});
const errors = [];
page.on('pageerror', e => errors.push(e.message));
try {
  await page.goto(process.env.CONWAY_TEST_BASE_URL+'/bootstrap-acceptance');
  await page.evaluate(async () => {
    const {initForms} = await import('/js/forms.js');
    const {mountReadyQueue} = await import('/js/readyqueueui.js');
    const {executionEvidenceHTML} = await import('/js/executionui.js');
    const {timelineControlsHTML,timelineRowHTML} = await import('/js/timeline.js');
    const {baselineListHTML} = await import('/js/baseline.js');
    const {initScoreboard} = await import('/js/scoreboard.js');
    const {orderingBadge,verdictBadgeHTML} = await import('/js/order.js');
    initForms();
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
    const badges=orderingBadge()+orderingBadge({acceptedOrdering:'engine'})+['on-time','at-risk','late'].map(verdict=>verdictBadgeHTML({verdict,weeksLate:2})).join('');
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
    const shell=new DOMParser().parseFromString(await (await fetch('/index.html')).text(),'text/html');
    const help=document.createElement('div');help.id='legacy-help-example';
    help.append(shell.querySelector('#view-scoreboard h2 .help'));
    document.querySelector('main').append(help);
    const scoreboard=document.createElement('div');scoreboard.className='table-responsive';
    scoreboard.innerHTML='<table id="score-table" class="table"></table>';
    document.querySelector('main').append(scoreboard);
    const stats={wip:3,throughputWk:1,p50:5,load:0.5,rho0:0.5,sigma:0.5};
    initScoreboard({edges:[],overlap:{},pods:['Atlas','Beacon'].map(name=>({name,location:'Central',devCount:4,streams:2})),stats:{Atlas:{...stats,p85:10},Beacon:{...stats,p85:20}}});
    new bootstrap.Tooltip(document.body,{selector:'[data-bs-toggle="tooltip"], [data-tip], .help',title:el=>el.dataset.bsTitle??el.dataset.tip??'',trigger:'hover focus',placement:'bottom'});
  });
  await page.locator('#ready-search.form-control').waitFor();
  // per specs/011-bootstrap-adoption-debt.md:73
  const buttonSizes=await page.locator('#framework-sizes button').evaluateAll(buttons=>buttons.map(button=>({font:parseFloat(getComputedStyle(button).fontSize),height:button.getBoundingClientRect().height})));
  assert.ok(buttonSizes[0].font<buttonSizes[1].font && buttonSizes[1].font<buttonSizes[2].font,'Bootstrap small, default and large button text sizes remain distinct: '+JSON.stringify(buttonSizes));
  assert.ok(buttonSizes[0].height<buttonSizes[1].height && buttonSizes[1].height<buttonSizes[2].height,'Bootstrap size modifiers preserve distinct control heights');
  const labelGeometry=await page.locator('#timeline-label-example .tl-label').evaluate(label=>{const style=getComputedStyle(label);return {align:style.textAlign,padding:[style.paddingTop,style.paddingRight,style.paddingBottom,style.paddingLeft].map(parseFloat)};});
  assert.equal(labelGeometry.align,'left','Timeline labels stay aligned with their team rows');
  assert.deepEqual(labelGeometry.padding,[0,10,0,0],'Timeline labels keep their ten-pixel track gap without generic action-button padding');
  const legacyHelp=page.locator('#legacy-help-example .help');
  assert.equal(await legacyHelp.evaluate(el=>el instanceof HTMLButtonElement),true,'Legacy metric help remains a native keyboard action');
  assert.equal(await legacyHelp.evaluate(el=>el.classList.contains('btn')),true,'Metric help adopts the same Bootstrap action primitive');
  assert.ok((await legacyHelp.getAttribute('aria-label') || '').trim().length>1,'Metric help has an explanatory accessible name');
  await legacyHelp.focus();
  assert.equal(await legacyHelp.evaluate(el=>document.activeElement===el),true,'Metric help receives keyboard focus');
  // per specs/011-bootstrap-adoption-debt.md:82
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
    const button = document.createElement('button'); button.id='late-button'; button.className='btn btn-secondary'; button.textContent='Later action'; button.disabled=true; dynamic.append(button);
  });
  await page.locator('#late-input.form-control').waitFor();
  assert.equal(await page.locator('#late-button').isDisabled(),true,'Adoption must preserve disabled actions');
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
    assert.equal(badgeContrast.length,15,'Ordering and verdict badges render on body, card and raised surfaces');
    for(const badge of badgeContrast) assert.ok(badge.ratio>=4.5,theme+' '+badge.label+' badge contrast '+badge.ratio.toFixed(2)+' must reach 4.5:1 '+JSON.stringify(badge));
    for (const width of [1280,360]) {
      await page.setViewportSize({width,height:960});
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,theme+' '+width+' has no page-level horizontal overflow');
      assert.equal(await decision.locator('[name=decision]').inputValue(),'defer');
      assert.equal(await decision.locator('[name=owner]').inputValue(),'Team lead');
      assert.equal(await decision.locator('[name=evidence]').inputValue(),'Wait for acceptance evidence');
      assert.equal(await page.locator('[name=scope_ready_checked]').isChecked(),true);
      assert.equal(await page.locator('#late-input').inputValue(),'Existing draft');
      assert.equal(await page.locator('#tl-initiative-filter').inputValue(),'Atlas');
      assert.equal(await page.locator('#tl-team-filter').inputValue(),'Team A');
      const owner = decision.locator('[name=owner]'); await owner.focus();
      assert.equal(await owner.evaluate(el=>document.activeElement===el),true);
      assert.equal(await owner.evaluate(el=>getComputedStyle(el).boxShadow!=='none'),true,'Bootstrap focus indicator remains visible in '+theme);
      await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-bootstrap-'+theme+'-'+width+'.png'),fullPage:true});
    }
    // per specs/011-bootstrap-adoption-debt.md:79
    const primary=decision.locator('button.btn-primary');
    await page.mouse.move(0,0); await primary.evaluate(el=>el.blur());
    const settled=()=>primary.evaluate(el=>Promise.all(el.getAnimations().map(animation=>animation.finished.catch(()=>{}))));
    for(const state of ['normal','hover','focus']) {
      if(state==='hover') await primary.hover();
      if(state==='focus') { await page.mouse.move(0,0); await primary.focus(); }
      await settled();
      const [result]=await measureContrast(primary);
      assert.ok(result.ratio>=4.5,theme+' primary '+state+' contrast '+result.ratio.toFixed(2)+' must reach 4.5:1 '+JSON.stringify(result));
    }
    colors.push(await page.locator('.ready-item').evaluate(el=>getComputedStyle(el).backgroundColor));
  }
  assert.notEqual(colors[0],colors[1],'Bootstrap cards must respond to both theme modes');
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
    const channels = value => value.match(/[\d.]+/g).slice(0,3).map(Number);
    assert.ok(channels(cell.color).every(value=>value<=80),'Printed table text stays dark: '+cell.color);
    assert.ok(channels(cell.background).every(value=>value>=220),'Printed table background stays light: '+cell.background);
  }
  await page.emulateMedia({media:'screen'});
  for (const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    const mobileLabels=await page.locator('main table').first().evaluate(table=>{const cells=table.querySelectorAll('tbody tr:first-child td');return {label:getComputedStyle(cells[0]).backgroundColor,content:getComputedStyle(cells[1]).backgroundColor};});
    assert.notEqual(mobileLabels.label,mobileLabels.content,'Mobile reference rows distinguish their label from content in '+theme);
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
  const {checkGameBootstrap}=await import(pathToFileURL(resolve('../tests/browser/game-bootstrap.mjs')).href);
  await checkGameBootstrap(page);
  assert.deepEqual(errors,[]);
  console.log(JSON.stringify({bootstrapControls:true,dynamicInsertion:true,selectedGroups:true,nativeDisclosureKeyboard:true,draftsRetained:true,guideSearchAndContents:true,readablePrint:true,badgeContrast:true,themes:['dark','light'],mobileOverflow:false,pageErrors:errors}));
} catch (error) {
  console.error(await page.locator('body').innerText());
  await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-bootstrap-failure.png'),fullPage:true});
  throw error;
} finally { await browser.close(); }
`
