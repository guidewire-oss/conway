import test from 'node:test';
import assert from 'node:assert/strict';
import {reviewSummaryHTML,actionCardsHTML} from '../app/js/weeklyreview.js';

test('manual review preserves the distinction between no delivery exceptions and unknown progress',()=>{
  const html=reviewSummaryHTML({context:{reviewDate:'2026-11-01',timezone:'America/Los_Angeles'},agenda:{delivery:[],gaps:[{reason:'No snapshot selected.'}]},actions:[],counts:{delivery:0,gaps:1,openActions:0,overdueActions:0}});
  assert.match(html,/Manual review; measured progress is unknown/);
  assert.match(html,/does not establish that all delivery is on track/);
  assert.match(html,/No snapshot selected/);assert.match(html,/America\/Los_Angeles/);
});

test('review summary shows both contexts and keeps delivery and data gaps separate',()=>{
  const html=reviewSummaryHTML({context:{snapshot:{id:'snapshot-b',name:'Current capture'},baseline:{id:'baseline-b',name:'Current agreement'},previousReview:{reviewDate:'2026-10-25',snapshot:{name:'Earlier capture'},baseline:{name:'Earlier agreement'}},comparisonLabel:'Agreement changed; comparisons need review.'},agenda:{delivery:[{initiative:'Atlas',reason:'Measured finish is late.'}],gaps:[{initiative:'Beacon',reason:'Bound epic missing.'}]}});
  for(const text of ['Current capture','Current agreement','Earlier capture','Earlier agreement','Agreement changed','Delivery exceptions','Data gaps','Atlas','Beacon'])assert.ok(html.includes(text),text);
});

test('review and action fields escape stored text and attribute values',()=>{
  const text='"><img src=x onerror=alert(1)>';
  const summary=reviewSummaryHTML({context:{reviewDate:text,timezone:text,snapshot:{name:text},comparisonLabel:text},agenda:{delivery:[{initiative:text,team:text,reason:text}],gaps:[]},actions:[{action:text,owner:text,reviewDate:text,status:'open'}]});
  const cards=actionCardsHTML([{id:text,action:text,owner:text,rationale:text,initiative:text,createdBy:text,status:'open',version:1}]);
  assert.doesNotMatch(summary,/<img/);assert.doesNotMatch(cards,/<img/);assert.match(cards,/data-action-id="&quot;&gt;&lt;img/);
});

test('legacy actions are open and resolved actions keep a route to reopen with history',()=>{
  assert.match(actionCardsHTML([{id:'legacy',action:'Confirm scope'}]),/>Open<\/span>/);
  const resolved=actionCardsHTML([{id:'closed',action:'Confirm scope',status:'resolved',version:3}]);
  assert.match(resolved,/data-version="3"/);assert.match(resolved,/<option value="open">Open/);assert.match(resolved,/Load transition history/);assert.match(resolved,/Evidence is required/);
});
