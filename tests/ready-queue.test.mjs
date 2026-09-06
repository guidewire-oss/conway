import test from 'node:test';
import assert from 'node:assert/strict';
import {readyItemHTML,readyHistoryHTML} from '../app/js/readyqueueui.js';

const context={periodStart:'2026-09-07'};
const item=(extra={})=>({initiative:'Atlas',team:'Team A',state:'waiting',kind:'work',plannedStartWeek:null,plannedFinishWeek:null,reasons:[],checklist:[],canRelease:false,...extra});

test('unknown placement never becomes week zero, while a zero-effort checkpoint keeps its actual date',()=>{
  const held=readyItemHTML(item(),context);
  assert.match(held,/Not placed/);assert.doesNotMatch(held,/Week 0/);
  const milestone=readyItemHTML(item({kind:'milestone',plannedStartWeek:0,plannedFinishWeek:0}),context);
  assert.match(milestone,/Acceptance checkpoint/);assert.match(milestone,/Week 0/);assert.match(milestone,/no work week or lane is invented/);
});

test('release is available only for eligible work, deferral requires reconsideration, carryover has no release form',()=>{
  assert.doesNotMatch(readyItemHTML(item(),context),/<option value="release"/);
  assert.match(readyItemHTML(item({canRelease:true,state:'ready'}),context),/<option value="release"/);
  const deferred=readyItemHTML(item({state:'deferred'}),context);
  assert.match(deferred,/<option value="reconsider"/);assert.doesNotMatch(deferred,/<option value="release"/);
  for(const state of ['in_progress','complete'])assert.doesNotMatch(readyItemHTML(item({state}),context),/<form data-ready-(?:decision|confirmation)/);
});

test('current and historical releases disclose permission separately from observed activity',()=>{
  const release={decision:'release',owner:'Team lead',createdAt:1};
  assert.match(readyItemHTML(item({lastDecision:release,releaseCurrent:true}),context),/Permission recorded; start unconfirmed/);
  assert.match(readyItemHTML(item({lastDecision:release,releaseCurrent:false}),context),/Historical permission; recheck/);
});

test('earlier checklist evidence is retained and clearly needs reconfirmation',()=>{
  const html=readyItemHTML(item({confirmation:{createdAt:1,createdBy:'Team lead',asOfWeek:0},confirmationCurrent:false,checklist:[{key:'scope_ready',label:'Scope accepted',checked:true,owner:'Scope owner',evidence:'Acceptance reviewed.'}]}),context);
  assert.match(html,/Earlier context: review and reconfirm/);assert.match(html,/Acceptance reviewed/);assert.match(html,/do not change the plan's readiness percentage/);
});

test('queue cards and history escape stored names, evidence and attribute values',()=>{
  const bad='"><img src=x onerror=alert(1)>';
  const html=readyItemHTML(item({initiative:bad,reasons:[{message:bad,owner:bad}],checklist:[{key:'scope_ready',label:bad,owner:bad,evidence:bad}]}),context);
  const history=readyHistoryHTML({decisions:[{decision:'defer',owner:bad,evidence:bad,createdBy:bad,asOfWeek:0}]});
  assert.doesNotMatch(html,/<img/);assert.doesNotMatch(history,/<img/);assert.match(html,/data-ready-initiative="&quot;&gt;&lt;img/);
});

test('history orders equal-time events by persisted order across both record types',()=>{
  const html=readyHistoryHTML({confirmations:[{eventOrder:3,createdAt:100,checks:[],createdBy:'Latest confirmation'}],decisions:[{eventOrder:2,createdAt:100,decision:'defer',owner:'Middle decision'},{eventOrder:1,createdAt:100,decision:'reconsider',owner:'Oldest decision'}]});
  assert.ok(html.indexOf('Latest confirmation')<html.indexOf('Middle decision'));
  assert.ok(html.indexOf('Middle decision')<html.indexOf('Oldest decision'));
});
