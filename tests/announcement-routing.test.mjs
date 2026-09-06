import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
import {readRoute} from '../app/js/navigation.js';
const main=readFileSync(new URL('../app/js/main.js',import.meta.url),'utf8');
const plan=readFileSync(new URL('../app/js/planui.js',import.meta.url),'utf8');
function sourceSection(source,startMarker,endMarker){
  const start=source.indexOf(startMarker),end=source.indexOf(endMarker);
  assert.ok(start>=0,`Missing source start marker: ${startMarker}`);
  assert.ok(end>=0,`Missing source end marker: ${endMarker}`);
  assert.ok(end>start,`Source markers are out of order: ${startMarker} / ${endMarker}`);
  return source.slice(start,end);
}

function announcementsHarness(){
  const listeners=new Map(),visits=[],destinations=[];let identity='account-a',features=[],finish,options,disposals=0,refreshes=0;
  const ready=new Promise(resolve=>{finish=resolve;});
  const controller={ready,state:()=>({features}),visit:target=>visits.push(target),dispose:()=>disposals++,refreshIndicators:()=>refreshes++};
  const scope=vm.createContext({queueMicrotask,authMode:()=> 'auth',authUser:()=>identity,authToken:()=> 'token',authGameID:()=> '',authFetch(){},openDocs:()=>destinations.push('guide'),openPlanDestination:async target=>{destinations.push(target);return false;},
    window:{addEventListener:(name,fn)=>listeners.set(name,fn)},document:{addEventListener(){},getElementById:()=>({}),querySelector:()=>null},mountAnnouncements:value=>{options=value;return controller;}});
  vm.runInContext(sourceSection(main,'let measureContext =','export const state'),scope);
  const tail=sourceSection(main,'  // specs/022-feature-announcements','async function restoreWorkspace()');
  vm.runInContext('async function mountNews(){\n'+tail,scope);
  return {scope,visits,destinations,listeners,controller,setIdentity:value=>{identity=value;},setFeatures:value=>{features=value;},finish,options:()=>options,counters:()=>({disposals,refreshes})};
}
const guide={id:'guide',action:{target:'docs-btn'}};

test('view navigation keeps aria-current aligned with the active destination',()=>{
  const handlers=new Map(),routes=[];
  const tabs=['home','network','plan','game'].map(view=>{
    const attributes=new Map(view==='home'?[['aria-current','page']]:[]);
    const tab={id:view+'-tab',dataset:{view},active:view==='home',attributes,
      classList:{toggle:(_name,value)=>{tab.active=value;}},
      setAttribute:(key,value)=>attributes.set(key,value),removeAttribute:key=>attributes.delete(key),
      addEventListener:(_event,fn)=>handlers.set(view,fn)};
    return tab;
  });
  const views=tabs.map(tab=>{const view={id:'view-'+tab.dataset.view,active:tab.active};view.classList={toggle:(_name,value)=>{view.active=value;}};return view;});
  const scope=vm.createContext({document:{querySelectorAll:selector=>selector==='.tab[data-view]'?tabs:views},syncMeasureContext(){},writeRoute:route=>routes.push(route.view)});
  vm.runInContext(sourceSection(main,"document.querySelectorAll('.tab[data-view]').forEach((b)",'// Explore ▾ dropdown'),scope);
  for(const destination of ['network','plan','game','home']) {
    handlers.get(destination)();
    assert.deepEqual(tabs.filter(tab=>tab.attributes.has('aria-current')).map(tab=>tab.dataset.view),[destination]);
    assert.deepEqual(tabs.filter(tab=>tab.active).map(tab=>tab.dataset.view),[destination]);
    assert.deepEqual(views.filter(view=>view.active).map(view=>view.id),['view-'+destination]);
  }
  assert.deepEqual(routes,['network','plan','game','home']);
});

test('an actual feature opened before catalog readiness is acknowledged after loading',async()=>{
  const h=announcementsHarness();h.listeners.get('conway:feature-opened')({detail:{action:'guide'}});
  const mounted=h.scope.mountNews();assert.deepEqual(h.visits,[]);
  h.setFeatures([guide]);h.finish();await mounted;assert.deepEqual(h.visits,['docs-btn']);
});

test('queued feature visits are discarded when the authenticated account changes',async()=>{
  const h=announcementsHarness();h.listeners.get('conway:feature-opened')({detail:{action:'guide'}});
  h.setIdentity('account-b');const mounted=h.scope.mountNews();h.setFeatures([guide]);h.finish();await mounted;assert.deepEqual(h.visits,[]);
});

test('catalog recovery flushes earlier visits through the state-change callback',async()=>{
  const h=announcementsHarness();const mounted=h.scope.mountNews();h.listeners.get('conway:feature-opened')({detail:{action:'guide'}});h.finish();await mounted;assert.deepEqual(h.visits,[]);
  h.setFeatures([guide]);h.options().onStateChange();await Promise.resolve();assert.deepEqual(h.visits,['docs-btn']);
});

test('BFCache page lifecycle preserves the controller and refreshes indicators on return',async()=>{
  const h=announcementsHarness();const mounted=h.scope.mountNews();h.finish();await mounted;
  h.listeners.get('pagehide')({persisted:true});assert.equal(h.counters().disposals,0);
  h.listeners.get('pageshow')({persisted:true});assert.equal(h.counters().refreshes,1);
  h.listeners.get('pagehide')({persisted:false});assert.equal(h.counters().disposals,1);
});

test('announcement actions delegate plan destinations instead of requiring rendered buttons',async()=>{
  const h=announcementsHarness();const mounted=h.scope.mountNews();h.finish();await mounted;
  assert.equal(await h.options().onAction({target:'plan-linked-sheets'}),false);
  assert.equal(await h.options().onAction({target:'view-execution'}),false);
  assert.deepEqual(h.destinations,['linked-sheets','execution']);assert.deepEqual(h.visits,[]);
});

test('plan destinations remain pending through selection and support cancellation',async()=>{
  let opens=0,clicks=0,cancel;const fixtureRoot={querySelector:selector=>selector==='[data-cancel-destination]'?{addEventListener:(_name,fn)=>{cancel=fn;}}:{remove(){}}};
  const scope=vm.createContext({fixtureRoot,document:{querySelector:()=>({click:()=>clicks++})},esc:String,showLinkedSheets:async()=>{opens++;return true;}});
  vm.runInContext(sourceSection(plan,'let root, current','// specs/017-planning-and-execution-usability.md:86').replace(/export /g,''),scope);
  vm.runInContext('root=fixtureRoot;',scope);
  assert.equal(await scope.openPlanDestination('linked-sheets'),false);assert.match(scope.pendingDestinationHTML(),/Choose a plan/);assert.match(scope.pendingDestinationHTML(),/data-cancel-destination/);assert.equal(opens,0);
  vm.runInContext('current={id:"generic-plan"};',scope);assert.equal(await scope.resumePlanDestination(),true);assert.equal(opens,1);assert.equal(scope.pendingDestinationHTML(),'');
  vm.runInContext('current=null;',scope);await scope.openPlanDestination('linked-sheets');scope.wirePendingDestination();cancel();vm.runInContext('current={id:"generic-plan"};',scope);
  assert.equal(await scope.resumePlanDestination(),false);assert.equal(opens,1);assert.equal(scope.pendingDestinationHTML(),'');assert.equal(clicks,2);
});

test('network route restoration selects the saved observe or what-if lens',async()=>{
  for(const lens of ['observe','what-if']){
    const clicked=[];const scope=vm.createContext({location:{href:'https://example.test/?view=network&networkLens='+lens},readRoute,restoringRoute:async fn=>fn(),authMode:()=> 'auth',isStaff:()=>true,hasRole:()=>true,document:{querySelector:selector=>({click:()=>clicked.push(selector)})},restorePlanLocation(){}});
    vm.runInContext(sourceSection(main,'async function restoreWorkspace()','// Rosters, Import, and Snapshots'),scope);await scope.restoreWorkspace();
    assert.equal(clicked[0],lens==='what-if'?'#net-plan':'#net-observe');
  }
});

test('source extraction rejects missing or reordered markers explicitly',()=>{
  assert.throws(()=>sourceSection('start middle end','absent','end'),/Missing source start marker/);
  assert.throws(()=>sourceSection('start middle end','start','absent'),/Missing source end marker/);
  assert.throws(()=>sourceSection('end middle start','start','end'),/out of order/);
});

test('what-if route restoration respects manager, facilitator, player and static dev access',async()=>{
  for(const [role,mode,staff,manager,expected] of [
    ['manager','auth',true,true,'#net-plan'],
    ['facilitator','auth',true,false,'#net-observe'],
    ['player','auth',false,false,'.tab[data-view="game"]'],
    ['dev','none',false,false,'#net-plan'],
  ]){
    const clicked=[];
    const scope=vm.createContext({location:{href:'https://example.test/?view=network&networkLens=what-if'},readRoute,restoringRoute:async fn=>fn(),authMode:()=>mode,isStaff:()=>staff,hasRole:()=>manager,document:{querySelector:selector=>({click:()=>clicked.push(selector)})}});
    vm.runInContext(sourceSection(main,'async function restoreWorkspace()','// Rosters, Import, and Snapshots'),scope);
    await scope.restoreWorkspace();assert.deepEqual(clicked,[expected],role);
  }
});

test('a destination requested inside an unsaved draft displays a notice with working Cancel',async()=>{
  let notice='',html='',cancel,opens=0;
  const fixtureRoot={querySelector(selector){
    if(selector==='[data-pending-destination]')return html?{remove(){html='';}}:null;
    if(selector==='[data-cancel-destination]')return html?{addEventListener(_name,fn){cancel=fn;}}:null;
    if(selector==='.plan-head')return {insertAdjacentHTML(_position,value){html=value;}};
    return null;
  }};
  const scope=vm.createContext({fixtureRoot,esc:String,planNotice:value=>{notice=value;},document:{querySelector:()=>({click(){}})},showLinkedSheets:async()=>{opens++;return true;}});
  vm.runInContext(sourceSection(plan,'let root, current','// specs/017-planning-and-execution-usability.md:86').replace(/export /g,''),scope);
  vm.runInContext('root=fixtureRoot;current={id:"generic-plan",isDraft:true};',scope);
  assert.equal(await scope.openPlanDestination('linked-sheets'),false);
  assert.match(notice,/Save or discard/);assert.match(html,/Linked Google Sheets/);assert.match(html,/data-cancel-destination/);
  assert.equal(typeof cancel,'function');cancel();assert.equal(html,'');
  vm.runInContext('current.isDraft=false;',scope);assert.equal(await scope.resumePlanDestination(),false);assert.equal(opens,0);
});
