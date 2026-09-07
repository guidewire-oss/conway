import test from 'node:test';
import assert from 'node:assert/strict';
import { forecastDate, forecastHTML, forecastEvidenceHTML } from '../app/js/portfolio-forecast.js';
test('forecast dates preserve unknowns and undated plans',()=>{
 assert.equal(forecastDate('',null),'Unknown');assert.equal(forecastDate('',0),'Week 0');assert.equal(forecastDate('2026-09-07',2),'2026-09-21 (week 2)');
});
test('forecast rendering escapes source names and keeps unknown work visible',()=>{
 const result={fingerprint:'abcdef',settings:{lowerFactor:.8,upperFactor:1.3,disruption:.1},limitations:['<script>'],scenarios:[{name:'<img>',commitWeek:null,rawFinishWeek:null,unknown:1,schedule:{periodStart:'',initiatives:[{name:'<Beacon>',provisional:true,commitWeek:0,verdict:'fits',slices:[{}]}]}}]};
 const html=forecastHTML(result,'plan');assert.doesNotMatch(html,/<img>|<script>|<Beacon>/);assert.match(html,/&lt;Beacon&gt;/);assert.match(html,/Not calibrated/);assert.match(html,/Unknown/);assert.doesNotMatch(html,/Week 0/);
});
test('diagnostics distinguish inferred ratios from predictive validation',()=>{
 const html=forecastEvidenceHTML({snapshot:{name:'<Atlas>',source:'jira',ageDays:3},baseline:null,coverage:{tracked:0,total:2},calibration:[],gaps:['<missing>']});assert.match(html,/No eligible completed-work samples/);assert.match(html,/do not measure effort/);assert.match(html,/recorded before outcomes/);assert.doesNotMatch(html,/<Atlas>|<missing>/);
});

test('synthetic snapshots never supply historical samples',()=>{
 const html=forecastEvidenceHTML({snapshot:{name:'Example',source:'baseline'},calibration:[{pod:'Atlas',factor:1.5,sampleCount:12}]});assert.match(html,/excluded from historical diagnostics/);assert.doesNotMatch(html,/1.50|12 completed/);
});
