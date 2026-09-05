import vm from 'node:vm';import {readFileSync} from 'node:fs';
const source=readFileSync('app/js/executionui.js','utf8');
let submit,finish,savedBody;
const note={setAttribute(){},textContent:''},button={disabled:false},form={values:{action:'Initial action',owner:'Owner',reviewDate:'2026-10-01',rationale:'Evidence'},querySelector:()=>button,reset(){this.values={action:'',owner:'',reviewDate:'',rationale:''}}};
const scope=vm.createContext({host:{querySelector:s=>s==='#execution-decision-form'?{addEventListener:(_n,f)=>submit=f}:note},data:{snapshot:{id:'s1'},baseline:{id:'b1'}},plan:{id:'plan-a'},live:()=>true,FormData:class{constructor(f){this.values={...f.values}}[Symbol.iterator](){return Object.entries(this.values)[Symbol.iterator]()}},json:(_url,options)=>{savedBody=JSON.parse(options.body);return new Promise(resolve=>finish=resolve)},loadDecisions:async()=>{}});
vm.runInContext(source.slice(source.indexOf("  host.querySelector('#execution-decision-form').addEventListener"),source.indexOf('  const decisions = loadDecisions();')),scope);
const saving=submit({preventDefault(){},currentTarget:form});form.values.action='Next action typed during save';finish({});await saving;
console.log('decision',{savedAction:savedBody.action,actionAfterResponse:form.values.action});
const nodes=new Map(),node=key=>{if(!nodes.has(key))nodes.set(key,{value:'',dataset:{},innerHTML:'',textContent:'',listeners:{},setAttribute(){},addEventListener(n,f){this.listeners[n]=f}});return nodes.get(key)};
const host={isConnected:true,innerHTML:'',querySelector:node,querySelectorAll:()=>[]};let snapshotCalls=0;
const mount=vm.createContext({URL,location:{href:'https://example.test/'},esc:s=>s,icon:()=>'',decisionsHTML:()=>'',executionEvidenceHTML:()=>'',when:()=>''});
vm.runInContext(source.slice(source.indexOf('export async function mountExecution')).replace('export async function','async function'),mount);
await mount.mountExecution(host,{plan:{id:'plan-a',initiatives:[]},request:async url=>url==='/api/snapshots'?(snapshotCalls++,null):({ok:true,json:async()=>({decisions:[]})})});
await node('#execution-refresh').listeners.click();
console.log('catalogRecovery',{snapshotCalls,status:node('#execution-status').textContent,options:node('#execution-snapshot').innerHTML});
const sim=readFileSync('app/js/simulator.js','utf8');
for(const mode of ['empty','missing-team','invalid']){
  const cards={innerHTML:'Old successful statistics'},old={name:'previous successful forecast'};
  const context=vm.createContext({readFeature:()=>({tasks:mode==='empty'?[]:[{pod:mode==='missing-team'?'Missing':'Atlas'}],deps:[]}),state:{stats:{Atlas:{mu:1,sigma:1,rho0:.2}},overlap:{}},scenarioSource:{dirty:true},paintSource(){},lastResult:old,rhoOverride:{},esc:s=>s,document:{getElementById:()=>cards},simulateFeature(){throw new Error('Cycle in task graph')},renderAll(){}});
  vm.runInContext(sim.slice(sim.indexOf('function run()'),sim.indexOf('function renderAll(')),context);context.run();
  console.log('simulator',{mode,keepsPreviousResult:context.lastResult===old,cards:cards.innerHTML});
}
