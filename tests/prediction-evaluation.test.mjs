import test from 'node:test';
import assert from 'node:assert/strict';
import {predictionEvaluationHTML} from '../app/js/prediction-evaluation.js';

const fixture=()=>({model:'envelope-frequency-v1',totalRecords:8,mismatchedRecords:1,tooLateRecords:1,boundaryRecords:1,probability:.6,brierScore:.26,benchmarkBrierScore:.25,gapPercentagePoints:-10,settings:{lowerFactor:.8,upperFactor:1.3,disruption:.1},trainingEvidence:{snapshotId:'<training>',sourceId:'<source>',capturedAt:1},testEvidence:{snapshotId:'later',startedAt:2},training:{eligible:3,covered:2,pending:1,excluded:1,repeated:1,rows:[]},test:{eligible:2,covered:1,pending:0,excluded:0,repeated:0,coveragePercent:50,rows:[{name:'<Beacon>',predictionId:'a"b',predictionName:'<record>',reason:'<reason>',status:'within',issuedAt:2}]}});
test('distinguishes a probability estimate from observed coverage with scores, limits and escaped provenance',()=>{
 const html=predictionEvaluationHTML(fixture());
 for(const text of ['60.0%','50.0%','0.260','0.250','-10.0 percentage points','retrospective test','not a certified','3 completed','2 completed'])assert.ok(html.includes(text),text);
 assert.match(html,/data-prediction-open="a&quot;b"/);assert.doesNotMatch(html,/<training>|<source>|<Beacon>|<record>|<reason>/);
});
test('leads with recovery when training or later completed outcomes are unavailable',()=>{
 const v=fixture();v.probability=v.brierScore=v.benchmarkBrierScore=v.gapPercentagePoints=null;
 assert.match(predictionEvaluationHTML(v),/No completed training observations/);
 v.probability=.6;v.test.coveragePercent=null;
 const html=predictionEvaluationHTML(v);assert.match(html,/No eligible test completions/);assert.match(html,/Unavailable <span class="fs-6">observed coverage/);assert.doesNotMatch(html,/NaN|undefined/);
});
