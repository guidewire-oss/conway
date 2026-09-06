import test from 'node:test';
import assert from 'node:assert/strict';
import { fuzzyMatch, initiativeMatch } from '../app/js/filter.js';
import { podLensHTML, portfolioTimelineHTML } from '../app/js/timeline.js';

const name = 'Credential secrets rotation';
const slice = (pod, startWeek) => ({ initiative: name, pod, startWeek, finishWeek: startWeek + 2, lanesUsed: 1, remainingWeeks: 2 });
const work = [{ name, work: { Atlas: { inPath: true }, Beacon: { inPath: true } } }];
const schedule = {
  horizonWeeks: 26,
  initiatives: [{ name, startWeek: 0, rawFinishWeek: 32, commitWeek: 32, slices: [slice('Atlas', 0), slice('Beacon', 30)] },
    { name: 'Reporting automation readiness', startWeek: 0, rawFinishWeek: 2, commitWeek: 2, slices: [{ ...slice('Cedar', 0), initiative: 'Reporting automation readiness' }] }],
  podWeeks: [
    { pod: 'Atlas', tracks: 1, slices: [slice('Atlas', 0)], weeks: [] },
    { pod: 'Beacon', tracks: 1, slices: [slice('Beacon', 30)], weeks: [] },
    { pod: 'Cedar', tracks: 1, slices: [{ ...slice('Cedar', 0), initiative: 'Reporting automation readiness' }], weeks: [] },
  ],
};

test('initiative search recognizes common word endings without matching unrelated names', () => {
  for (const query of ['rotate', 'ROTATE', 'rotated', 'rotating', 'rotations', 'rotate credential']) {
    assert.equal(initiativeMatch(query, name), true, query);
    assert.equal(initiativeMatch(query, 'Billing dashboard'), false, query);
  }
  assert.equal(initiativeMatch('rotation', 'Rotate credentials'), true);
  assert.equal(initiativeMatch('xyz rotate', name), false, 'every query word must match');
  assert.equal(initiativeMatch('rate', 'Rating'), false, 'short roots are not inferred');
  assert.equal(initiativeMatch('rotate', 'Reporting automation readiness'), false, 'scattered letters are not initiative matches');
  assert.equal(initiativeMatch('aplat', 'Apollo/App Platform'), false, 'initiative search requires text');
  assert.equal(fuzzyMatch('aplat', 'Apollo/App Platform'), true, 'existing shorthand');
  assert.equal(initiativeMatch('', name), true);
});

test('by-pod search shows matching bars and outside-view work on every assigned team', () => {
  const html = podLensHTML(schedule, { initiativeQuery: 'rotate', hideEmptyPods: true, planInitiatives: work });
  assert.match(html, /data-pod="Atlas"/);
  assert.match(html, /data-initiative="Credential secrets rotation"/);
  assert.match(html, /data-pod="Beacon"/);
  assert.match(html, /Work outside this view/);
  assert.match(html, /data-select-init="Credential secrets rotation"/);
  assert.doesNotMatch(html, /data-pod="Cedar"|Billing dashboard|Reporting automation readiness/);
  assert.equal(schedule.initiatives.filter(i => initiativeMatch('rotate', i.name)).length, 1, 'live count uses the same matcher');
});

test('word-ending search finds held assignments and agrees with portfolio grouping', () => {
  const held = { ...schedule, initiatives: [{ name, verdict: 'beyond-horizon', bindingConstraint: 'lead', slices: [] }],
    podWeeks: schedule.podWeeks.map(p => ({ ...p, slices: [] })) };
  const opts = { initiativeQuery: 'rotate', hideEmptyPods: true, planInitiatives: work };
  const html = podLensHTML(held, opts);
  assert.match(html, /data-pod="Atlas"/);
  assert.match(html, /data-pod="Beacon"/);
  assert.match(html, /Assigned work without a placement/);
  assert.doesNotMatch(html, /data-pod="Cedar"/);
  assert.match(portfolioTimelineHTML(held, opts), /data-init="Credential secrets rotation"/);
});
