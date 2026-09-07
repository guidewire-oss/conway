import test from 'node:test';
import assert from 'node:assert/strict';
import {captureFreshnessHTML} from '../app/js/measure-context.js';
test('selected capture freshness crosses the saved threshold without claiming data quality',()=>{
 const s={createdAt:100,capture:{sourceId:'s',sourceName:'Atlas',freshnessHours:24}};
 assert.match(captureFreshnessHTML(s,86499),/Fresh capture/);assert.match(captureFreshnessHTML(s,86500),/Stale capture/);
 assert.equal(captureFreshnessHTML({createdAt:100}), '');
});
test('captured source names are escaped in Measure and execution help',()=>{
 const html=captureFreshnessHTML({createdAt:100,capture:{sourceId:'s',sourceName:'<img src=x>',freshnessHours:24}},101);
 assert.doesNotMatch(html,/<img/);assert.match(html,/&lt;img src=x&gt;/);
});
