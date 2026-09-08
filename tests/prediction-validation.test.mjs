import test from 'node:test';
import assert from 'node:assert/strict';
import {predictionValidationHTML} from '../app/js/prediction-validation.js';

test('explains the denominator, repeated work and descriptive limits with escaped record links',()=>{
 const counts={eligible:2,covered:1,before:1,after:0,pending:1,excluded:1,repeated:3,coveragePercent:50};
 const html=predictionValidationHTML({...counts,totalRecords:6,candidateRecords:4,mismatchedRecords:1,tooLateRecords:1,evidence:{snapshotId:'<capture>',capturedAt:1},settings:{lowerFactor:.8,upperFactor:1.3,disruption:.1},months:[{...counts,month:'2026-09'}],rows:[{name:'<Beacon>',status:'repeated',month:'2026-09',reason:'Overlap',representativeId:'a"b',representativeInitiative:'<Atlas>',predictionId:'record',predictionName:'<September>'}]});
 assert.match(html,/50.0%/);assert.match(html,/1 within \/ 2 completed/);assert.match(html,/3 repeated entries/);assert.match(html,/not independent samples or calibrated probabilities/);assert.match(html,/earliest recorded prediction/);assert.match(html,/data-prediction-open="a&quot;b"/);assert.doesNotMatch(html,/<Beacon>|<capture>|<September>/);
 const empty=predictionValidationHTML({...counts,eligible:0,coveragePercent:null,months:[],rows:[],settings:{lowerFactor:.8,upperFactor:1.3,disruption:.1},evidence:{snapshotId:'capture',sourceId:'source',capturedAt:1},candidateRecords:0,totalRecords:0,mismatchedRecords:0,tooLateRecords:0});
 assert.match(empty,/Unavailable/);assert.match(empty,/No matching work groups/);
});
