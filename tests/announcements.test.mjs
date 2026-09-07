import test from 'node:test';
import assert from 'node:assert/strict';
import { createAnnouncementController, safeAnnouncementAction, announcementIndicators, announcementsHTML } from '../app/js/announcements.js';

const feature = (extra = {}) => ({ id: 'guide-navigation-v1', title: 'Guide', description: 'Find help', action: { type: 'menu', target: 'docs-btn', parent: 'help-btn' }, announced: false, visited: false, ...extra });
const response = body => ({ ok: true, json: async () => body });
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };

test('loading a catalog never acknowledges presentation or visits', async () => {
  const calls = [];
  const controller = createAnnouncementController({ getIdentity: () => 'account-a', request: async (...args) => { calls.push(args); return response({ features: [feature()] }); } });
  await controller.load();
  assert.equal(calls.length, 1);
  assert.deepEqual(controller.state().features, [feature()]);
  assert.deepEqual(announcementIndicators(controller.state().features), ['docs-btn', 'help-btn']);
});

test('one actual execution destination visit acknowledges both eligible feature introductions',async()=>{
  const acknowledged=[];
  const features=['execution-review-v1','weekly-execution-review-v1'].map(id=>feature({id,action:{type:'route',target:'view-execution',parent:'plan-btn',route:'?view=plan&planView=execution'}}));
  const controller=createAnnouncementController({getIdentity:()=> 'manager-a',request:async(_url,options)=>{
    if(!options)return response({features});
    const {id,kind}=JSON.parse(options.body);acknowledged.push({id,kind});return response({id,announced:false,visited:true});
  }});
  await controller.load();await controller.visit('view-execution');
  assert.deepEqual(acknowledged.map(v=>v.id).sort(),features.map(v=>v.id).sort());
  assert.ok(acknowledged.every(v=>v.kind==='visited'));
  assert.deepEqual(announcementIndicators(controller.state().features),[]);
});

test('presentation and destination visits persist separately across controller reloads and users', async () => {
  let identity = 'account-a';
  const saved = new Map();
  const request = async (url, options) => {
    const value = saved.get(identity) || feature();
    if (!options) return response({ features: [value] });
    const { id, kind } = JSON.parse(options.body);
    saved.set(identity, { ...value, [kind]: true });
    return response({ id, announced: saved.get(identity).announced, visited: saved.get(identity).visited });
  };
  const controller = createAnnouncementController({ request, getIdentity: () => identity });
  await controller.load();
  assert.equal(await controller.acknowledge(feature().id, 'announced'), true);
  assert.equal(controller.state().features[0].visited, false);
  assert.equal(await controller.visit('help-btn'), false, 'opening a parent is not a destination visit');
  assert.equal(await controller.visit('docs-btn'), true);
  assert.deepEqual(announcementIndicators(controller.state().features), []);
  const reopened = createAnnouncementController({ request, getIdentity: () => identity });
  await reopened.load();
  assert.equal(reopened.state().features[0].announced, true);
  assert.equal(reopened.state().features[0].visited, true);
  identity = 'account-b';
  await reopened.load();
  assert.equal(reopened.state().features[0].announced, false);
  assert.equal(reopened.state().features[0].visited, false);
  assert.equal(await reopened.visit('docs-btn'), true);
  assert.equal(reopened.state().features[0].announced, false, 'organic visits do not imply presentation');
});

test('failed acknowledgement leaves the feature new and can be retried', async () => {
  let fails = true;
  const controller = createAnnouncementController({ getIdentity: () => 'a', request: async (url, options) => !options ? response({ features: [feature()] }) : fails ? { ok: false } : response({ id: feature().id, announced: true, visited: false }) });
  await controller.load();
  assert.equal(await controller.acknowledge(feature().id, 'announced'), false);
  assert.equal(controller.state().features[0].announced, false);
  assert.match(controller.state().error, /could not be saved/i);
  fails = false;
  assert.equal(await controller.acknowledge(feature().id, 'announced'), true);
  assert.equal(controller.state().error, '');
});

test('late catalog and acknowledgement responses cannot cross account boundaries', async () => {
  let identity = 'a';
  const lateCatalog = deferred(), lateAck = deferred();
  let catalogCount = 0;
  const controller = createAnnouncementController({ getIdentity: () => identity, request: (url, options) => options ? lateAck.promise : ++catalogCount === 1 ? lateCatalog.promise : Promise.resolve(response({ features: [feature({ title: identity })] })) });
  const firstLoad = controller.load();
  identity = 'b';
  await controller.load();
  lateCatalog.resolve(response({ features: [feature({ title: 'a' })] }));
  await firstLoad;
  assert.equal(controller.state().features[0].title, 'b');
  const ack = controller.acknowledge(feature().id, 'visited');
  identity = 'c';
  await controller.load();
  lateAck.resolve(response({ id: feature().id, announced: true, visited: true }));
  assert.equal(await ack, false);
  assert.equal(controller.state().features[0].title, 'c');
  assert.equal(controller.state().features[0].visited, false);
});

test('duplicate pending acknowledgements share one request and later calls remain idempotent', async () => {
  const pending = deferred(); let count = 0;
  const controller = createAnnouncementController({ getIdentity: () => 'a', request: (url, options) => !options ? Promise.resolve(response({ features: [feature()] })) : (count++, pending.promise) });
  await controller.load();
  const one = controller.acknowledge(feature().id, 'visited'), two = controller.acknowledge(feature().id, 'visited');
  await Promise.resolve();
  assert.equal(count, 1);
  pending.resolve(response({ id: feature().id, announced: false, visited: true }));
  assert.deepEqual(await Promise.all([one, two]), [true, true]);
  assert.equal(await controller.acknowledge(feature().id, 'visited'), true);
  assert.equal(count, 1);
});

test('announcement text is escaped and only known local actions receive navigation controls', () => {
  const html = announcementsHTML([feature({ title: '<img src=x onerror=alert(1)>', description: '<script>bad</script>' })]);
  assert.doesNotMatch(html, /<img|<script>/);
  assert.match(html, /&lt;img/);
  for (const action of [{ type: 'route', route: 'https://example.com', target: 'view-execution', parent: 'plan-btn' }, { type: 'menu', target: 'delete-plan', parent: 'help-btn' }]) {
    assert.equal(safeAnnouncementAction(action), false);
    assert.deepEqual(announcementIndicators([feature({ action })]), []);
    assert.doesNotMatch(announcementsHTML([feature({ action })]), /data-announcement-action/);
  }
});
