import assert from 'node:assert/strict';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

// specs/034-readable-scrollable-timelines.md:31
export async function checkTimelineViewport(page) {
  await page.evaluate(async () => {
    const {podLensHTML, portfolioTimelineHTML} = await import('/js/timeline.js');
    const {attachTimelineViewport} = await import('/js/timeline-viewport.js');
    const slices = [
      {initiative:'Atlas platform reliability and credential renewal',pod:'Atlas',startWeek:0,finishWeek:4,lanesUsed:2},
      {initiative:'Beacon delivery',pod:'Atlas',startWeek:4,finishWeek:7,lanesUsed:1},
      {initiative:'Cedar late checkpoint',pod:'Atlas',startWeek:150,finishWeek:154,lanesUsed:1},
      {initiative:'Delta handoff',pod:'Atlas',startWeek:53,finishWeek:53,lanesUsed:1},
    ];
    const schedule = {horizonWeeks:26,periodStart:'2026-09-07',podWeeks:[{pod:'Atlas',tracks:5,weeks:[],slices}],
      initiatives:slices.map((s,index)=>({name:s.initiative,proposedRank:index+1,startWeek:s.startWeek,rawFinishWeek:s.finishWeek,commitWeek:s.finishWeek+1,bufferWeeks:1,targetWeek:s.finishWeek,slices:[s,{...s,pod:'Beacon'}]}))};
    document.querySelector('main').innerHTML = '<div id="timeline-test"></div>';
    window.timelinePresentation = {};
    window.paintTimeline = lens => {
      window.disposeTimelineTest?.();
      const root=document.querySelector('#timeline-test');
      root.innerHTML=(lens==='pod'?podLensHTML:portfolioTimelineHTML)(schedule,{span:154,todayWeek:2,expand:slices[0].initiative});
      window.disposeTimelineTest=attachTimelineViewport(root,window.timelinePresentation);
    };
    window.paintTimeline('pod');
  });
  const geometry = async () => page.locator('.tl-plot').evaluate(plot => {
    const axis=plot.querySelector('.tl-axis').getBoundingClientRect();
    const tracks=[...plot.querySelectorAll('.tl-track')].map(el=>el.getBoundingClientRect());
    const bars=[...plot.querySelectorAll('.tl-bar')].map(el=>({start:Number(el.style.left.slice(0,-1)),width:Number(el.style.width.slice(0,-1)),rect:el.getBoundingClientRect(),track:el.closest('.tl-track').getBoundingClientRect()}));
    return {axis,tracks,bars,scroll:plot.querySelector('.tl-scroll').scrollLeft};
  });
  const aligned = async () => {
    const {axis,tracks,bars}=await geometry();
    assert.ok(tracks.length>=3);
    for(const track of tracks){assert.ok(Math.abs(track.x-axis.x)<1);assert.ok(Math.abs(track.width-axis.width)<1,'Idle and busy tracks use the full shared time width');}
    for(const bar of bars){assert.ok(Math.abs(bar.rect.x-(bar.track.x+bar.start*bar.track.width/100))<1);assert.ok(Math.abs(bar.rect.width-bar.width*bar.track.width/100)<1,'Bars must not exaggerate duration');}
  };
  await page.setViewportSize({width:1280,height:960});
  await aligned();
  const scroll=page.locator('.tl-scroll'), separator=page.getByRole('separator',{name:'Resize timeline labels'});
  assert.ok(await scroll.evaluate(el=>el.scrollWidth>el.clientWidth*2),'154 weeks have a readable scrollable canvas');
  const busyWidths=await page.locator('.tl-lane .tl-track').evaluateAll(els=>els.map(el=>el.getBoundingClientRect().width));
  assert.equal(busyWidths.length,6,'Five physical tracks and a separate checkpoint row remain visible');
  const checkpoint=page.locator('.tl-checkpoints .tl-checkpoint');
  assert.equal(await checkpoint.count(),1);
  assert.match(await checkpoint.getAttribute('aria-label'),/week 53, no track time/);
  await separator.scrollIntoViewIfNeeded();
  const initial=Number(await separator.getAttribute('aria-valuenow'));
  const box=await separator.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+10);await page.mouse.down();
  await page.mouse.move(box.x+box.width/2+180,box.y+10,{steps:8});await page.mouse.up();
  assert.equal(Number(await separator.getAttribute('aria-valuenow')),initial+180);
  await aligned();
  await separator.focus();await page.keyboard.press('ArrowRight');
  assert.equal(Number(await separator.getAttribute('aria-valuenow')),initial+196);
  await separator.focus();await page.keyboard.press('Home');
  assert.equal(Number(await separator.getAttribute('aria-valuenow')),160);
  await page.keyboard.press('ArrowRight');
  const savedWidth=Number(await separator.getAttribute('aria-valuenow'));
  await scroll.evaluate(el=>{el.scrollLeft=el.scrollWidth;});
  await page.waitForFunction(()=>window.timelinePresentation.scroll?.Atlas>0);
  const visible=await page.locator('.tl-lane').first().evaluate(row=>{
    const label=row.querySelector('.hint').getBoundingClientRect(),viewport=row.closest('.tl-scroll').getBoundingClientRect();
    return label.left>=viewport.left-1 && label.right<=viewport.right;
  });
  assert.equal(visible,true,'Labels remain visible at the far right');
  await aligned();
  const rightEdge=await page.locator('[data-initiative="Cedar late checkpoint"]').evaluate(el=>({right:el.getBoundingClientRect().right,viewport:el.closest('.tl-scroll').getBoundingClientRect().right}));
  assert.ok(Math.abs(rightEdge.right-rightEdge.viewport)<1,'The final scheduled week is reachable');
  const download=page.waitForEvent('download');
  assert.equal(await page.evaluate(async()=>{
    const {exportBlockPNG}=await import('/js/exportpng.js');
    const serialize=XMLSerializer.prototype.serializeToString;
    XMLSerializer.prototype.serializeToString=function(node){const text=serialize.call(this,node);window.timelineExport=text;return text;};
    try{return await exportBlockPNG(document.querySelector('.tl-pod'),'timeline-full-span.png');}
    finally{XMLSerializer.prototype.serializeToString=serialize;}
  }),true);
  const file=await download;
  const path=join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-timeline-full-span.png');
  await file.saveAs(path);
  const {readFile}=await import('node:fs/promises');const png=await readFile(path);
  assert.ok(png.readUInt32BE(16)>154*24*2,'PNG includes off-screen weeks at export scale');
  const pixels=await page.evaluate(async b64=>{
    const doc=new DOMParser().parseFromString(window.timelineExport,'application/xml');
    const marker=doc.querySelector('.tl-checkpoint');
    const img=new Image();img.src='data:image/png;base64,'+b64;await img.decode();
    const canvas=document.createElement('canvas');canvas.width=img.width;canvas.height=img.height;
    const ctx=canvas.getContext('2d');ctx.drawImage(img,0,0);
    const [red,green,blue]=getComputedStyle(document.querySelector('.tl-bar')).backgroundColor.match(/\d+/g).map(Number);
    const data=ctx.getImageData(Math.floor(img.width*.93),0,Math.floor(img.width*.06),img.height).data;
    let count=0;for(let i=0;i<data.length;i+=4)if(Math.abs(data[i]-red)<5&&Math.abs(data[i+1]-green)<5&&Math.abs(data[i+2]-blue)<5)count++;
    return {count,checkpoint:marker?.getAttribute('style'),title:marker?.getAttribute('title')};
  },png.toString('base64'));
  assert.ok(pixels.count>20,'Export contains the late-week work bar, not just a wide empty canvas');
  assert.equal(pixels.checkpoint,'left:34.42%');assert.match(pixels.title,/checkpoint w53/);
  const savedScroll=(await geometry()).scroll;
  await page.evaluate(()=>window.paintTimeline('pod'));
  assert.equal((await geometry()).scroll,savedScroll,'Repainting selection retains its exact scroll position');
  assert.equal(Number(await separator.getAttribute('aria-valuenow')),savedWidth);
  await page.evaluate(()=>window.paintTimeline('initiative'));
  assert.equal(Number(await separator.getAttribute('aria-valuenow')),savedWidth,'Non-default width survives lens changes');
  await aligned();
  const vertical=await page.locator('.tl-row').first().evaluate(row=>({label:row.querySelector('.tl-label').getBoundingClientRect().top,bar:row.querySelector('.tl-track > .tl-bar').getBoundingClientRect().top}));
  assert.ok(Math.abs(vertical.label-vertical.bar)<4,'Expanded initiative name aligns with its own work, not the middle of the subrows');
  const label=page.locator('.tl-label').first();
  const labelWidth=await label.evaluate(el=>el.getBoundingClientRect().width);
  await separator.focus();await page.keyboard.press('End');
  assert.ok(await label.evaluate(el=>el.getBoundingClientRect().width)>labelWidth);
  await aligned();
  for(const theme of ['dark','light']) {
    await page.evaluate(theme=>document.documentElement.dataset.bsTheme=theme,theme);
    await page.screenshot({path:join(process.env.CONWAY_TEST_ARTIFACT_DIR||tmpdir(),'conway-timeline-'+theme+'.png'),fullPage:true});
    await page.setViewportSize({width:360,height:800});
    await page.waitForFunction(()=>Number(document.querySelector('.tl-label-resizer').getAttribute('aria-valuenow'))<=200);
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Mobile timeline scroll stays inside the chart');
    await aligned();
    await page.setViewportSize({width:1280,height:960});
  }
  await page.evaluate(()=>window.disposeTimelineTest());
  console.log('Timeline geometry: busy/idle lanes, expanded rows, 154-week scrolling, pointer/keyboard labels, themes, mobile and full-span PNG passed');
}
