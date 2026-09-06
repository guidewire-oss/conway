import test from 'node:test';
import assert from 'node:assert/strict';
import { executionEvidenceHTML, decisionsHTML, confirmedEpicKeys } from '../app/js/executionui.js';

const evidence = overrides => ({ snapshot: { id: 's1', source: 'jira', createdAt: 1700000000, ageDays: 2 }, baseline: { name: 'Agreement', createdAt: 1700000000, periodStart: '2026-01-05' }, coverage: { total: 2, bound: 1, tracked: 1 }, initiatives: [], ...overrides });

test('missing execution evidence remains unknown rather than a measured zero', () => {
  const html = executionEvidenceHTML({ initiatives: [{ name: 'Atlas', slices: [{ pod: 'Team A', gaps: [] }] }] });
  assert.match(html, /Unknown of Unknown initiatives/);
  assert.match(html, /Unknown of Unknown comparable team pairs/);
  assert.match(html, /Start not measured/);
  assert.match(html, /Finish not measured/);
  assert.doesNotMatch(html, /undefined|NaN|Invalid Date/);
});

test('synthetic snapshot sources are distinguished from organizational observations', () => {
  assert.match(executionEvidenceHTML(evidence({ snapshot: { source: 'template' } })), /Synthetic example evidence/);
  assert.match(executionEvidenceHTML(evidence({ snapshot: { source: 'baseline' } })), /Synthetic example evidence/);
  assert.match(executionEvidenceHTML(evidence({ snapshot: {} })), /source type is unknown/);
  assert.doesNotMatch(executionEvidenceHTML(evidence({})), /Synthetic example evidence/);
});

test('inferred starts and resolution-based finishes carry separate provenance', () => {
  const html = executionEvidenceHTML(evidence({ initiatives: [{ name: 'Atlas', tracked: true, slices: [{ pod: 'Team A', actualStartWeek: 2, actualFinishWeek: 5, startInferred: true, confidence: 'low', gaps: [] }] }] }));
  assert.match(html, /Inferred from issue activity/);
  assert.match(html, /Resolution timestamps in snapshot/);
  assert.match(html, /low confidence/);
});

test('team review retains untracked planned initiatives and original scope', () => {
  const data = evidence({ initiatives: [{ name: 'Atlas', slices: [], tracked: false }, { name: 'Beacon', slices: [], originalScopeSlices: [{ pod: 'Team A', gaps: [] }] }, { name: 'Other', slices: [{ pod: 'Team B' }] }] });
  const html = executionEvidenceHTML(data, { team: 'Team A', plan: { initiatives: [{ name: 'Atlas', work: { 'Team A': { inPath: true } } }] } });
  assert.match(html, /Atlas/); assert.match(html, /Beacon/); assert.doesNotMatch(html, /<h4>Other/);
  assert.match(html, /coverage above is for the whole plan/);
});

test('suggestions remain opt-in and all data-driven markup is escaped', () => {
  const html = executionEvidenceHTML(evidence({ initiatives: [{ name: '<img src=x>', epicKeys: [], suggestions: [{ key: 'PROJ-1', summary: '<script>bad</script>' }] }] }));
  assert.match(html, /type="checkbox" name="suggestion" value="PROJ-1"/);
  assert.doesNotMatch(html, /checked|<img|<script>/);
});

test('confirmed epic bindings normalize and deduplicate explicit selections', () => {
  const form = new FormData(); form.set('epicKeys', 'proj-1, PROJ-2; proj-1'); form.append('suggestion', 'PROJ-2'); form.append('suggestion', 'PROJ-3');
  assert.deepEqual(confirmedEpicKeys(form), ['PROJ-1', 'PROJ-2', 'PROJ-3']);
  form.set('epicKeys', 'not-a-key'); assert.throws(() => confirmedEpicKeys(form), /Use Jira epic keys/);
});

test('empty explicit bindings can intentionally remove all working-plan mappings', () => {
  assert.deepEqual(confirmedEpicKeys(new FormData()), []);
});

test('decisions preserve author and evidence references without executing markup', () => {
  const html = decisionsHTML([{ action: '<img src=x>', owner: 'Owner', rationale: '<script>x</script>', reviewDate: '2026-09-08', snapshotId: 's1', baselineId: 'b1', createdBy: 'Planner', createdAt: 1700000000 }]);
  assert.match(html, /snapshot s1 · baseline b1/); assert.match(html, /by Planner/); assert.doesNotMatch(html, /<img|<script>/);
});

test('remaining-work forecast includes its conditional basis and keeps absence unknown', () => {
  const html = executionEvidenceHTML(evidence({ initiatives: [{ name: 'Atlas', slices: [{ pod: 'Team A', remainingWeeks: 2.5, forecastFinishWeek: 7, forecastBasis: 'Conditional baseline duration times unfinished child fraction' }, { pod: 'Team B', remainingWeeks: null, forecastFinishWeek: null, gaps: ['Insufficient snapshot evidence'] }] }] }));
  assert.match(html, /2.5 calendar weeks remaining/);
  assert.match(html, /week 7/);
  assert.match(html, /Conditional baseline duration times unfinished child fraction/);
  assert.match(html, /No remaining-work forecast basis supplied/);
  assert.match(html, /Insufficient snapshot evidence/);
});
