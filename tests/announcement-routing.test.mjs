import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
import {readRoute} from '../app/js/navigation.js';
const main=readFileSync(new URL('../app/js/main.js',import.meta.url),'utf8');
const plan=readFileSync(new URL('../app/js/planui.js',import.meta.url),'utf8');

function announcementsHarness(){
  const listeners=new Map(),visits=[],destinations=[];let identity='account-a',features=[],finish,options,disposals=0,refreshes=0;
  const ready=new Promise(resolve=>{finish=resolve;});
  const controller={ready,state:()=>({features}),visit:target=>visits.push(target),dispose:()=>disposals++,refreshIndicators:()=>refreshes++};
  const scope=vm.createContext({queueMicrotask,authMode:()=> 'auth',authUser:()=>identity,authToken:()=> 'token',authGameID:()=> '',authFetch(){},openDocs:()=>destinations.push('guide'),openPlanDestination:async target=>{destinations.push(target);return false;},
    window:{addEventListener:(name,fn)=>listeners.set(name,fn)},document:{addEventListener(){},getElementById:()=>({}),querySelector:()=>null},mountAnnouncements:value=>{options=value;return controller;}});
  vm.runInContext(main.slice(main.indexOf('let measureContext ='),main.indexOf('export const state')),scope);
  const tail=main.slice(main.indexOf('  // specs/022-feature-announcements'),main.indexOf('async function restoreWorkspace()'));
  vm.runInContext('async function mountNews(){\n'+tail,scope);
  return {scope,visits,destinations,listeners,controller,setIdentity:value=>{identity=value;},setFeatures:value=>{features=value;},finish,options:()=>options,counters:()=>({disposals,refreshes})};
}
const guide={id:'guide',action:{target:'docs-btn'}};

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
  vm.runInContext(plan.slice(plan.indexOf('let root, current'),plan.indexOf('// specs/017-planning-and-execution-usability.md:86')).replace(/export /g,''),scope);
  vm.runInContext('root=fixtureRoot;',scope);
  assert.equal(await scope.openPlanDestination('linked-sheets'),false);assert.match(scope.pendingDestinationHTML(),/Choose a plan/);assert.match(scope.pendingDestinationHTML(),/data-cancel-destination/);assert.equal(opens,0);
  vm.runInContext('current={id:"generic-plan"};',scope);assert.equal(await scope.resumePlanDestination(),true);assert.equal(opens,1);assert.equal(scope.pendingDestinationHTML(),'');
  vm.runInContext('current=null;',scope);await scope.openPlanDestination('linked-sheets');scope.wirePendingDestination();cancel();vm.runInContext('current={id:"generic-plan"};',scope);
  assert.equal(await scope.resumePlanDestination(),false);assert.equal(opens,1);assert.equal(scope.pendingDestinationHTML(),'');assert.equal(clicks,2);
});

test('network route restoration selects the saved observe or what-if lens',async()=>{
  for(const lens of ['observe','what-if']){
    const clicked=[];const scope=vm.createContext({location:{href:'https://example.test/?view=network&networkLens='+lens},readRoute,restoringRoute:async fn=>fn(),authMode:()=> 'auth',isStaff:()=>true,hasRole:()=>true,document:{querySelector:selector=>({click:()=>clicked.push(selector)})},restorePlanLocation(){}});
    vm.runInContext(main.slice(main.indexOf('async function restoreWorkspace()'),main.indexOf('// Rosters, Import, and Snapshots')),scope);await scope.restoreWorkspace();
    assert.equal(clicked[0],lens==='what-if'?'#net-plan':'#net-observe');
  }
});
