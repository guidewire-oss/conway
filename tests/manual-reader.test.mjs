import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const context = vm.createContext({});
vm.runInContext(readFileSync(new URL('../app/js/manual-reader.js', import.meta.url), 'utf8'), context);
const sections = [
  { id: 'overview', title: 'Overview', content: 'Overview. Buffer settings protect the plan.' },
  { id: 'buffer', title: 'Buffer calculations', content: 'Buffer calculations and planning settings.' },
  { id: 'capacity', title: 'Capacity settings', content: 'Capacity settings depend on available tracks.' },
];

test('manual search requires every query word and ranks topic headings first', () => {
  const result = context.searchManualSections(sections, '  BUFFER   settings ');
  assert.deepEqual(Array.from(result, (section) => section.id), ['buffer', 'overview']);
});

test('blank and unmatched manual queries return no misleading matches', () => {
  assert.equal(context.searchManualSections(sections, '   ').length, 0);
  assert.equal(context.searchManualSections(sections, 'buffer capacity').length, 0);
});

test('manual search preserves section order for equally relevant results and leaves the index unchanged', () => {
  const before = JSON.stringify(sections);
  assert.deepEqual(Array.from(context.searchManualSections(sections, 'plan'), (section) => section.id), ['overview', 'buffer']);
  assert.equal(JSON.stringify(sections), before);
});
