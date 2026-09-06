import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

test('bursts of DOM mutations prepare newly rendered sortable headers once per frame', () => {
  const frames = [], headers = [];
  let mutations, scans = 0;
  const document = {
    documentElement: {},
    addEventListener() {},
    querySelectorAll() { scans++; return headers; },
  };
  const source = readFileSync(new URL('../app/js/sortable.js', import.meta.url), 'utf8').replace(/export /g, '');
  vm.runInNewContext(source, {
    document,
    MutationObserver: class {
      constructor(callback) { mutations = callback; }
      observe() {}
    },
    requestAnimationFrame(callback) { frames.push(callback); },
  });
  const initialScans = scans;
  const attributes = new Map();
  const header = {
    textContent: 'Forecast finish',
    setAttribute(name, value) { attributes.set(name, value); },
    hasAttribute(name) { return attributes.has(name); },
  };
  headers.push(header);
  for (let i = 0; i < 20; i++) mutations([{ type: 'childList' }]);
  assert.equal(scans, initialScans, 'mutation delivery must not synchronously rescan the document');
  assert.equal(frames.length, 1, 'one pending frame handles the whole mutation burst');
  frames.shift()();
  assert.equal(scans, initialScans + 1);
  assert.equal(header.tabIndex, 0);
  assert.equal(attributes.get('scope'), 'col');
  assert.equal(attributes.get('aria-sort'), 'none');
  assert.match(attributes.get('aria-label'), /Forecast finish.*Enter or Space/);
  attributes.set('aria-sort', 'ascending');
  mutations([{ type: 'childList' }]);
  mutations([{ type: 'childList' }]);
  assert.equal(frames.length, 1, 'later mutations schedule another single frame');
  frames.shift()();
  assert.equal(scans, initialScans + 2);
  assert.equal(attributes.get('aria-sort'), 'ascending', 'refreshing keyboard semantics preserves the active sort');
});
