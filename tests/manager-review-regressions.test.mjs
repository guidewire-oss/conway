import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
import {normalizeGameTiming} from '../app/js/gamesui.js';
import {capacitySectionHTML,fitSentence} from '../app/js/report.js';
import {fitNote} from '../app/js/order.js';
import {baselinesDrawerHTML} from '../app/js/baseline.js';
import {helpButton} from '../app/js/terms.js';

function moduleContext(file,bindings={}) {
  const source=readFileSync(new URL('../app/js/'+file,import.meta.url),'utf8').replace(/^import[\s\S]*?;\n/gm,'').replace(/^export \{[^}]*\};\n/gm,'').replace(/export /g,'');
  const scope=vm.createContext({console,Date,helpButton,...bindings});vm.runInContext(source,scope);return scope;
}

test('Hygiene escapes imported names in visible text, drill attributes and IDs',()=>{
  const name='Team"><img src=x onerror="alert(1)">';
  const escaped='Team&quot;&gt;&lt;img src=x onerror=&quot;alert(1)&quot;&gt;';
  const elements=new Map();
  const el=id=>{if(!elements.has(id))elements.set(id,{innerHTML:'',setAttribute(){},querySelectorAll:()=>[]});return elements.get(id);};
  const scope=moduleContext('hygiene.js',{document:{getElementById:el,querySelectorAll:()=>[]},getSnapshot:()=> 'generic-snapshot',heatColor:()=> '#777'});
  scope.render({pods:[{name}],hygiene:{[name]:{score:.5,sampleSized:2}}});
  const html=el('hygiene-table').innerHTML;
  assert.ok(html.includes(`data-pod="${escaped}"`));assert.ok(html.includes(`id="hyg-${escaped}"`));assert.ok(html.includes(`<td>▸ ${escaped}</td>`));
  assert.doesNotMatch(html,/<img|data-pod="Team"|id="hyg-Team"/);
  scope.renderCats(name,null);
  assert.ok(el('hyg-'+name).innerHTML.includes(`id="hyg-issues-${escaped}"`));
  assert.doesNotMatch(el('hyg-'+name).innerHTML,/<img/);
});

test('simulator team labels, full-kit details and what-if attributes escape imported markup',()=>{
  const text='Team"><img src=x onerror="alert(1)">',elements=new Map();
  const el=id=>{if(!elements.has(id))elements.set(id,{innerHTML:'',addEventListener(){},querySelectorAll:()=>[]});return elements.get(id);};
  const scope=moduleContext('simulator.js',{document:{getElementById:el},heatColor:()=> '#777',
    fullKitCheck:()=>({score:.5,items:[{status:'warn',label:text,detail:text}]}),suggestDeps:()=>[{fromPod:text,toPod:text,count:3}],fixtureState:{pods:[],stats:{[text]:{rho0:.5}},edges:[]}});
  vm.runInContext('state=fixtureState;',scope);
  scope.renderKit({epic:text});scope.renderSuggestions({tasks:[],deps:[]});scope.renderCrit({podCriticality:{[text]:.5}});scope.renderWhatIf({tasks:[{pod:text}]});
  for(const id of ['fullkit','suggestions','crit-list','whatif']){assert.doesNotMatch(el(id).innerHTML,/<img/,id);assert.match(el(id).innerHTML,/&lt;img/,id);}
  assert.match(el('whatif').innerHTML,/data-pod="Team&quot;&gt;&lt;img/);assert.match(el('whatif').innerHTML,/id="rv-Team&quot;&gt;&lt;img/);
});

test('scoreboard, Flow and network inspector escape roster markup in every tested label context',()=>{
  const name='Team"><img src=x onerror="alert(1)">',neighbor='Neighbor<script>alert(1)</script>',site='Site<svg onload=alert(1)>',area='Area<img src=x>';
  const elements=new Map(),handlers=new Map();
  const el=id=>{if(!elements.has(id))elements.set(id,{innerHTML:'',textContent:'',addEventListener:(type,handler)=>handlers.set(id+':'+type,handler),querySelectorAll:()=>[]});return elements.get(id);};
  const document={getElementById:el,querySelectorAll:()=>[]};
  const stats={rho0:.7,wip:5,p50:4,p85:7,throughputWk:3,sigma:.5,resolved180:12};
  const pod={name,location:site,area,devCount:2,streams:2};
  const state={pods:[pod],stats:{[name]:stats},overlap:{[name]:{}},edges:[{from:name,to:neighbor,count:3}]};
  const scoreboard=moduleContext('scoreboard.js',{document,heatColor:()=> '#777'});scoreboard.initScoreboard(state);
  const flow=moduleContext('flow.js',{document,constraintScores:()=>[{pod:name,queueFactor:2,dependents:1,demand:3}],freezeProjection:()=>({frozen:2,currentDays:10,projectedDays:6})});
  flow.renderConstraints(state);flow.renderFreeze(state,1);
  const graph=moduleContext('graph.js',{document});let hidden;
  graph.showPanel(pod,state,false,null,value=>{hidden=value;});
  for(const id of ['score-table','constraint-cards','freeze-bars','netpanel']){assert.doesNotMatch(el(id).innerHTML,/<img|<script|<svg/,id);assert.match(el(id).innerHTML,/&lt;img/,id);}
  assert.match(el('freeze-bars').innerHTML,/data-pod="Team&quot;&gt;&lt;img/);assert.match(el('freeze-bars').innerHTML,/id="drill-Team&quot;&gt;&lt;img/);
  assert.match(el('constraint-cards').innerHTML,/Neighbor&lt;script/);assert.match(el('netpanel').innerHTML,/Neighbor&lt;script/);
  handlers.get('np-hide:click')();assert.equal(hidden,name);assert.doesNotMatch(el('netpanel').innerHTML,/<img/);assert.match(el('netpanel').innerHTML,/&lt;img/);
});

test('Flow fever uses elapsed calendar days for both progress and due-date risk',async()=>{
  const now=Date.UTC(2026,0,8);class FixedDate extends Date{static now(){return now;}}
  const elements=new Map(),elapsed=[];
  const el=id=>{if(!elements.has(id))elements.set(id,{innerHTML:'',value:'25'});return elements.get(id);};
  const chart=new Proxy({}, {get:(_target,key)=>key==='node'?()=>({getBoundingClientRect:()=>({width:900})}):()=>chart});
  const scale=()=>{const fn=value=>value;fn.domain=()=>fn;fn.range=()=>fn;return fn;};
  const axis=()=>{const fn=()=>{};fn.ticks=()=>fn;fn.tickFormat=()=>fn;return fn;};
  const scope=moduleContext('flow.js',{Date:FixedDate,document:{getElementById:el},d3:{select:()=>chart,scaleLinear:scale,axisBottom:axis,axisLeft:axis,format:()=>String},simulateFeature:()=>({p50:5,p85:7}),feverPoint:(_pct,days)=>{elapsed.push(days);return {zone:'green',consumed:0,ratio:0};}});
  const task={key:'PROJ-1',pod:'Team A',points:3,status:'In Progress',created:'2026-01-01T00:00:00Z',blockedBy:[]};
  scope.loadEpics=async()=>({total:2,epics:[{epic:'PROJ-10',name:'Atlas has eight days remaining',duedate:'2026-01-16',tasks:[task]},{epic:'PROJ-20',name:'Beacon has six days remaining',duedate:'2026-01-14',tasks:[task]}]});
  await scope.renderFever({pods:[{name:'Team A'}],stats:{'Team A':{mu:1,sigma:.2,rho0:.5}},overlap:{}});
  assert.deepEqual(elapsed,[7,7]);
  assert.doesNotMatch(el('fever-list').innerHTML,/Atlas has eight/,'seven forecast days fit inside eight remaining calendar days');
  assert.match(el('fever-list').innerHTML,/Beacon has six/);assert.match(el('fever-list').innerHTML,/date at risk/);
});

const forecastPanels=['stat-cards','cdf','tornado','gantt','crit-list','suggestions','whatif'];
function simulatorHarness(tasks=[],stats={}) {
  const elements=new Map(), listeners=new Map(),frames=[];
  const el=id=>{if(!elements.has(id))elements.set(id,{innerHTML:'OLD FORECAST',textContent:'',addEventListener:(kind,handler)=>listeners.set(id+':'+kind,handler)});return elements.get(id);};
  const rows=tasks.map(task=>({querySelector:selector=>({value:({'.t-id':task.id,'.t-pod':task.pod,'.t-size':'M','.t-deps':task.deps||''})[selector]})}));
  const scope=moduleContext('simulator.js',{
    document:{getElementById:el,querySelector:el,querySelectorAll:selector=>selector==='#task-table tbody tr'?rows:[]},
    simulatorSourceHTML:source=>source.dirty?'Forecast needs recalculation':'Current forecast',
    requestAnimationFrame:fn=>frames.push(fn),fixtureState:{pods:Object.keys(stats).map(name=>({name})),stats,overlap:{}},
    simulateFeature:()=>{throw new Error('Invalid dependency graph');},
  });
  vm.runInContext('state=fixtureState;lastResult={r:{p50:999},feature:{tasks:[]}};scenarioSource.dirty=false;',scope);
  forecastPanels.forEach(el);
  return {scope,el,frames,listeners};
}

for(const [name,tasks,stats,pattern] of [
  ['empty scenario',[],{},/at least one named task/i],
  ['missing team statistics',[{id:'T1',pod:'Missing'}],{},/no team statistics/i],
  ['invalid dependency graph',[{id:'T1',pod:'Team A'}],{'Team A':{mu:1,sigma:.2,rho0:.5}},/Invalid dependency graph/],
]) test(`simulator ${name} clears every previous result and invalidates cached redraw`,()=>{
  const {scope,el}=simulatorHarness(tasks,stats);scope.run();
  assert.equal(vm.runInContext('lastResult',scope),null);
  assert.equal(vm.runInContext('scenarioSource.dirty',scope),true);
  assert.match(el('stat-cards').innerHTML,/role="alert"/);assert.match(el('stat-cards').innerHTML,pattern);
  for(const id of forecastPanels.filter(id=>id!=='stat-cards'))assert.equal(el(id).innerHTML,'',id);
  assert.match(el('simulator-source').innerHTML,/recalculation/);
});

test('empty snapshot and an already queued activation cannot restore an obsolete forecast',()=>{
  const {scope,el,frames,listeners}=simulatorHarness();
  let renders=0;scope.renderAll=()=>renders++;
  scope.initSimulator({pods:[],stats:{},overlap:{}});
  assert.equal(vm.runInContext('lastResult',scope),null);assert.match(el('stat-cards').innerHTML,/No teams/);
  vm.runInContext('lastResult={r:{p50:999},feature:{tasks:[]}};',scope);
  listeners.get('button[data-view=simulator]:click')();assert.equal(frames.length,1);
  scope.run();frames[0]();assert.equal(renders,0);
  for(const id of forecastPanels.filter(id=>id!=='stat-cards'))assert.equal(el(id).innerHTML,'',id);
});

test('seven elapsed cycle-time days display seven calendar days later',()=>{
  class FixedDate extends Date {constructor(...args){super(...(args.length?args:[2026,0,5,12,0,0]));}}
  const panel={innerHTML:''};const scope=moduleContext('simulator.js',{Date:FixedDate,document:{getElementById:()=>panel}});
  const expected=new Date(2026,0,12,12).toLocaleDateString(undefined,{month:'short',day:'numeric'});
  assert.equal(scope.fmtDate(7),expected);
  scope.renderStats({p50:7,p85:7,p95:7});assert.ok(panel.innerHTML.includes('~'+expected));
  assert.match(panel.innerHTML,/elapsed calendar days/);assert.doesNotMatch(panel.innerHTML,/working days|five-day/);
});

for(const [rounds,timer,expected] of [
  ['','',{rounds:4,timerSecs:300,minutes:20}],['0','0',{rounds:4,timerSecs:300,minutes:20}],
  ['9','10',{rounds:8,timerSecs:30,minutes:4}],['2','4000',{rounds:2,timerSecs:3600,minutes:120}],
  ['2.5','60.5',{rounds:4,timerSecs:300,minutes:20}],['NaN','Infinity',{rounds:4,timerSecs:300,minutes:20}],
  ['3','90',{rounds:3,timerSecs:90,minutes:5}],
]) test(`game timing normalizes rounds=${rounds} timer=${timer}`,()=>{assert.deepEqual(normalizeGameTiming(rounds,timer),expected);});

test('game creation submits the same normalized timing used for its duration preview',async()=>{
  const values={'g-name':'Generic workshop','g-rounds':'99','g-timer':'1','g-ap':'5','g-scenario':'default'};let submitted;
  const scope=moduleContext('gamesui.js',{document:{getElementById:id=>({value:values[id]})},authFetch:async(_path,options)=>{submitted=JSON.parse(options.body);return {ok:true};}});
  scope.refreshGames=()=>{};await scope.createGame();
  const timing=normalizeGameTiming(values['g-rounds'],values['g-timer']);
  assert.equal(submitted.rounds,timing.rounds);assert.equal(submitted.timerSecs,timing.timerSecs);
  assert.equal(Math.ceil(submitted.rounds*submitted.timerSecs/60),timing.minutes);
});

test('capacity reporting retains missing evidence alongside known hot and overloaded teams',()=>{
  for(const missing of [undefined,null,NaN]) {
    const html=capacitySectionHTML({podWeeks:[{pod:'Hot team',flatRho:.9,tracks:2},{pod:'Overloaded team',flatRho:1.3,tracks:2},{pod:'Unknown team',flatRho:missing,tracks:2}]});
    assert.match(html,/Capacity evidence is incomplete/);assert.match(html,/Hot team/);assert.match(html,/Overloaded team/);
    assert.doesNotMatch(html,/Every pod is comfortably inside capacity|NaN/);
  }
});

test('unavailable period fit stays unknown and escapes its reason in Order and Report',()=>{
  const fit={unavailableReason:'Invalid <img src=x> names',fits:null,askedTrackWeeks:null,availableTrackWeeks:null,beyondHorizon:[]};
  for(const html of [fitNote(fit,26),fitSentence({fit,initiatives:[]})]){assert.match(html,/Period fit is unknown/);assert.match(html,/&lt;img/);assert.doesNotMatch(html,/<img|all .*fit|0.*track/i);}
});

test('the baseline drawer keeps its empty warning hidden until an error is reported',()=>{
  const html=baselinesDrawerHTML([],null);
  const warning=html.match(/<p\b[^>]*class="bl-drawer-error plan-warn"[^>]*>/)?.[0];
  assert.ok(warning);assert.match(warning,/\bhidden\b/);assert.match(warning,/role="alert"/);
});

function modalHarness() {
  const doc={activeElement:null,querySelectorAll:()=>[],body:{classList:{remove(){}}}};
  const scope=moduleContext('modal.js',{document:doc,window:{getComputedStyle:el=>({visibility:el.visibility||'visible'})}});
  function element(id,parent=null){
    const classes=new Set(),attributes=new Map(),handlers=new Map();
    const node={id,parent,isConnected:true,hidden:false,firstElementChild:null,handlers,focusCount:0,
      getClientRects:()=>node.hidden||node.parent?.hidden?[]:[{}],closest:selector=>selector==='[hidden]'?(node.hidden?node:node.parent?.hidden?node.parent:null):null,
      focus(){node.focusCount++;doc.activeElement=node;},contains(other){return other===node||other?.parent===node;},
      setAttribute:(key,value)=>attributes.set(key,value),hasAttribute:key=>attributes.has(key),querySelector:()=>null,
      addEventListener:(kind,handler)=>handlers.set(kind,handler),removeEventListener:kind=>handlers.delete(kind),
      classList:{add:value=>classes.add(value),remove:value=>classes.delete(value),contains:value=>classes.has(value)}};
    return node;
  }
  return {scope,doc,element};
}

test('fallback modal handoff closes the old dialog and restores its visible original trigger',()=>{
  const {scope,doc,element}=modalHarness();const trigger=element('open'),first=element('first'),inside=element('inside',first),second=element('second');
  doc.activeElement=trigger;scope.openModal(first);inside.focus();scope.openModal(second);
  assert.equal(first.hidden,true);assert.equal(second.hidden,false);assert.equal(first.handlers.has('keydown'),false);assert.equal(second.handlers.has('keydown'),true);
  scope.closeModal(second);assert.equal(second.hidden,true);assert.equal(doc.activeElement,trigger);assert.equal(inside.focusCount,1,'hidden dialog controls must not receive restored focus');
});

test('modal closing refuses a formerly valid trigger that is now hidden',()=>{
  const {scope,doc,element}=modalHarness();const trigger=element('open'),dialog=element('dialog');doc.activeElement=trigger;scope.openModal(dialog);trigger.hidden=true;
  scope.closeModal(dialog);assert.equal(trigger.focusCount,0);assert.notEqual(doc.activeElement,trigger);
});
