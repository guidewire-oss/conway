import test from 'node:test';
import assert from 'node:assert/strict';
import {predictionAssessmentHTML} from '../app/js/prediction-history.js';

test('shows coverage numerator and denominator without treating missing outcomes as zero',()=>{
 const empty=predictionAssessmentHTML({coveragePercent:null,eligible:0,covered:0,pending:2,excluded:1,rows:[]});
 assert.match(empty,/Unavailable/);assert.match(empty,/2 pending/);assert.doesNotMatch(empty,/0%/);
 const value=predictionAssessmentHTML({coveragePercent:50,eligible:2,covered:1,pending:1,excluded:3,rows:[{name:'<Beacon>',status:'excluded',reason:'Scope changed',earliestFinish:'2026-09-14',latestFinish:'2026-09-28',actualFinish:'',varianceWeeks:null}]});
 assert.match(value,/1 of 2/);assert.match(value,/50.0%/);assert.match(value,/&lt;Beacon&gt;/);assert.match(value,/Scope changed/);assert.match(value,/not a probability/i);
});
