import test from 'node:test';
import assert from 'node:assert/strict';
import {
  fmtWhen, activeBaseline, baselineChipHTML, baselineListHTML,
  deltaCell, compareTableHTML, baselinePanelHTML,
} from '../app/js/baseline.js';

const saved = [
  { id: 'b2', name: 'v2', active: true, createdAt: 1767225600, createdBy: 'ann@example.com', diverged: false },
  { id: 'b1', name: 'v1', active: false, createdAt: 1766620800, createdBy: 'bo@example.com', diverged: true },
];

test('activeBaseline finds the one actuals are measured against', () => {
  assert.equal(activeBaseline(saved).id, 'b2');
  assert.equal(activeBaseline([]), null);
  assert.equal(activeBaseline(undefined), null);
});

test('fmtWhen is empty rather than 1970 for a missing timestamp', () => {
  assert.equal(fmtWhen(0), '');
  assert.equal(fmtWhen(undefined), '');
  assert.ok(fmtWhen(1767225600).length > 0);
});

test('the chip names the active baseline', () => {
  const html = baselineChipHTML(saved);
  assert.match(html, /v2/);
  assert.match(html, /bl-current/);
  assert.ok(!html.includes('bl-diverged'));
});

// FR-030: divergence is why the chip is always visible rather than in a panel.
test('the chip goes amber when the plan has moved since the active baseline', () => {
  const moved = [{ ...saved[0], diverged: true }];
  const html = baselineChipHTML(moved);
  assert.match(html, /bl-diverged/);
  assert.match(html, /diverged/);
  assert.match(html, /inputs have changed since this baseline/, 'the tooltip says what it means');
});

test('the chip says none rather than going blank on a plan with no baseline', () => {
  const html = baselineChipHTML([]);
  assert.match(html, /none/);
  assert.ok(!html.includes('bl-diverged'));
});

// AC 7.3: the others remain readable as history, not just the active one.
test('the list shows every baseline, marking the active one', () => {
  const html = baselineListHTML(saved);
  assert.match(html, /v1/);
  assert.match(html, /v2/);
  assert.equal((html.match(/class="tag">active/g) || []).length, 1);
  assert.match(html, /ann@example.com/);
  assert.match(html, /bo@example.com/, 'FR-033: who saved it');
});

test('the list offers make-active only for the ones that are not', () => {
  const html = baselineListHTML(saved);
  const activateButtons = html.match(/class="bl-activate"/g) || [];
  assert.equal(activateButtons.length, 1, 'exactly the inactive one');
  assert.match(html, /data-id="b1"/);
  assert.ok(!/bl-activate" data-id="b2"/.test(html));
});

test('the list explains itself when a plan has no baselines', () => {
  const html = baselineListHTML([]);
  assert.match(html, /No baselines yet/);
  assert.match(html, /reproduced and compared/, 'say what saving one buys');
});

test('deltaCell always signs a movement', () => {
  assert.match(deltaCell(3), /\+3w/);
  assert.match(deltaCell(-2), /−2w/);
  assert.match(deltaCell(0), /—/);
  assert.match(deltaCell(undefined), /—/);
  assert.match(deltaCell(3), /ord-red/, 'later is worse');
  assert.match(deltaCell(-2), /ord-green/, 'earlier is better');
});

// AC 7.4: baseline vs current, the delta, and added/removed listed separately.
test('the comparison shows both weeks and the delta for each initiative', () => {
  const html = compareTableHTML({
    baseline: { id: 'b2', name: 'v2' },
    diverged: true,
    comparison: {
      moved: 1,
      initiatives: [{
        name: 'Telemetry GA', proposedRank: 1,
        baselineStartWeek: 0, startWeek: 2, startDeltaWeeks: 2,
        baselineCommitWeek: 17, commitWeek: 20, commitDeltaWeeks: 3,
        baselineVerdict: 'on-time', verdict: 'late', verdictChanged: true,
      }],
      added: ['Brand new'],
      removed: ['Dropped one'],
    },
  });
  assert.match(html, /Telemetry GA/);
  assert.match(html, /w0 → w2/);
  assert.match(html, /w17 → w20/);
  assert.match(html, /\+3w/);
  assert.match(html, /on-time →/, 'a changed verdict is the part people act on');
  assert.match(html, /Added since:.*Brand new/s);
  assert.match(html, /Removed since:.*Dropped one/s);
  assert.match(html, /1 initiative has moved/);
  assert.match(html, /inputs moved/, 'divergence is flagged on the comparison too');
});

test('the comparison says so plainly when nothing has moved', () => {
  const html = compareTableHTML({
    baseline: { name: 'v2' }, diverged: false,
    comparison: { moved: 0, initiatives: [{ name: 'A', baselineStartWeek: 0, startWeek: 0, baselineCommitWeek: 5, commitWeek: 5, verdict: 'on-time' }] },
  });
  assert.match(html, /Nothing has moved/);
  assert.ok(!html.includes('inputs moved'));
});

test('the comparison renders nothing at all when none is open', () => {
  assert.equal(compareTableHTML(null), '');
  assert.equal(compareTableHTML({}), '');
});

test('an initiative name from a spreadsheet cannot inject markup', () => {
  const nasty = '<img src=x onerror=alert(1)>';
  const html = compareTableHTML({
    baseline: { name: nasty },
    comparison: { moved: 0, initiatives: [{ name: nasty, verdict: nasty }], added: [nasty], removed: [nasty] },
  });
  assert.ok(!html.includes('<img'));
  assert.match(html, /&lt;img/);
  assert.ok(!baselineListHTML([{ id: nasty, name: nasty, createdBy: nasty }]).includes('<img'));
  assert.ok(!baselineChipHTML([{ name: nasty, active: true }]).includes('<img'));
});

test('the panel composes without leaking undefined', () => {
  const html = baselinePanelHTML(saved, null);
  assert.ok(!html.includes('undefined'));
  assert.ok(!html.includes('NaN'));
  assert.match(html, /Save as baseline/);
  assert.match(html, /id="bl-name"/);
});

test('the panel survives a plan with nothing saved and nothing compared', () => {
  const html = baselinePanelHTML(undefined, undefined);
  assert.ok(!html.includes('undefined'));
  assert.match(html, /No baselines yet/);
});
