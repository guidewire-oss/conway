import test from 'node:test';
import assert from 'node:assert/strict';
import {withPredictionCleanup} from './browser/prediction-history.mjs';

test('retains the journey failure and every cleanup failure while attempting all cleanup',async()=>{
 const original=Error('journey assertion'),restore=Error('restore failed'),browser=Error('browser disconnected'),calls=[];
 await assert.rejects(withPredictionCleanup(
  async()=>{calls.push('journey');throw original;},
  async()=>{calls.push('restore');throw restore;},
  async()=>{calls.push('browser');throw browser;}
 ),error=>{assert.ok(error instanceof AggregateError);assert.deepEqual(error.errors,[original,restore,browser]);return true;});
 assert.deepEqual(calls,['journey','restore','browser']);
});

test('preserves a lone journey or cleanup error and completes successful cleanup',async()=>{
 const original=Error('original');let cleaned=0;
 await assert.rejects(withPredictionCleanup(async()=>{throw original;},async()=>{cleaned++;}),error=>error===original);
 await assert.rejects(withPredictionCleanup(async()=>{},async()=>{throw original;},async()=>{cleaned++;}),error=>error===original);
 await withPredictionCleanup(async()=>{},async()=>{cleaned++;});
 assert.equal(cleaned,3);
});
