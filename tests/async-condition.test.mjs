import test from 'node:test';
import assert from 'node:assert/strict';
import {waitForAsyncFunction} from './browser/async-condition.mjs';

test('awaits false promises and polls until the condition returns its ready value',async()=>{
 let calls=0;
 const page={async evaluate(predicate,arg){calls++;return predicate({attempt:calls,...arg});}};
 const result=await waitForAsyncFunction(page,async({attempt,ready})=>attempt>=3?ready:false,{ready:'capture-2'},{polling:1});
 assert.equal(result,'capture-2');assert.equal(calls,3);
});
test('surfaces predicate errors and bounds false or never-settling conditions',async()=>{
 const failure=Error('request failed');
 await assert.rejects(waitForAsyncFunction({evaluate:async()=>{throw failure;}},()=>true),error=>error===failure);
 for(const evaluate of [async()=>false,()=>new Promise(()=>{})]){
  await assert.rejects(waitForAsyncFunction({evaluate},()=>false,undefined,{timeout:20,polling:1}),/Timed out waiting for asynchronous browser condition/);
 }
});
