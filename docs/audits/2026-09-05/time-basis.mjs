import vm from 'node:vm';import {readFileSync} from 'node:fs';
import {simulateFeature,feverPoint} from '../../../app/js/sim.js';
const source=readFileSync('app/js/simulator.js','utf8');
const result=simulateFeature({tasks:[{id:'PROJ-1',pod:'Atlas',size:1}],deps:[]},{Atlas:{mu:Math.log(7),sigma:.1,rho0:.2}},{},{trials:10000,seed:20260611});
const today=Date.UTC(2026,8,1,12);
class FixedDate extends Date {constructor(...args){super(...(args.length?args:[today]));}static now(){return today;}}
const scope=vm.createContext({Date:FixedDate});vm.runInContext(source.slice(source.indexOf('function fmtDate('),source.indexOf('function renderStats(')),scope);
console.log('calendarSamplesThroughWorkingDateConversion',{inputCalendarMedian:7,simulatedP50:result.p50,displayedCalendarDays:Math.round(result.p50*7/5),displayedDate:scope.fmtDate(result.p50)});
const flow=readFileSync('app/js/flow.js','utf8');
const expression=flow.match(/const elapsed = ([^;]+);/)[1];
for(const observationDate of ['2026-09-08','2026-10-06']){
  const scope=vm.createContext({Date:{now:()=>new Date(observationDate+'T00:00:00Z').getTime()},start:new Date('2026-09-01T00:00:00Z')});
  const elapsed=vm.runInContext(expression,scope);
  console.log('sameFrozenSnapshot',{snapshotDate:'2026-09-08',viewedOn:observationDate,unchangedCompletion:.5,elapsed,...feverPoint(.5,elapsed,10,14)});
}
