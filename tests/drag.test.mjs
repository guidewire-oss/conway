import test from 'node:test';
import assert from 'node:assert/strict';
import { attachDrag } from '../app/js/drag.js';
import { podLanesHTML } from '../app/js/timeline.js';

// Spec 008 S4: edge-resize and the drag data contract. These specs stub the
// minimum DOM surface attachDrag touches — a bar-shaped element with a
// bounding rect and captured pointer listeners — because node:test has no
// browser. weekWidth/rowHeight fall back to 12px/week and 22px/row with no
// .tl-track/.tl-lane present, which is what these stubs exercise.

const makeBar = (ds) => {
  const listeners = {};
  return {
    dataset: { ...ds },
    style: {},
    listeners,
    addEventListener: (n, fn) => { listeners[n] = fn; },
    setPointerCapture: () => {},
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 120, height: 22 }),
  };
};

const makeRoot = (bars) => {
  const bySel = {
    '.tl-bar': bars,
    '.tl-track': null, // weekWidth falls back to WEEK_PX_HINT (12px/week)
    '.tl-lane': null,  // rowHeight falls back to 22px/row
  };
  return {
    dataset: {},
    querySelector: (sel) => (sel in bySel ? bySel[sel] : null),
    querySelectorAll: (sel) => bySel[sel] || [],
  };
};

// drag runs one gesture on one bar: pointerdown at (x0,y0), pointerup at (x1,y1).
async function drag(bar, x0, y0, x1, y1) {
  bar.listeners.pointerdown({ button: 0, pointerType: 'mouse', pointerId: 1, clientX: x0, clientY: y0, preventDefault: () => {} });
  await bar.listeners.pointerup({ clientX: x1, clientY: y1 });
}

test('a body drag pins the start week (the pre-S4 contract still holds)', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '1' });
  const pins = [];
  attachDrag(makeRoot([bar]), {
    onPin: (i, p, e) => pins.push([i, p, e]),
    onResize: () => {},
  });
  await drag(bar, 60, 5, 73, 27); // dx=13 -> +1 week, dy=22 -> +1 lane
  assert.equal(pins.length, 1);
  assert.deepEqual(pins[0].slice(0, 2), ['Big', 'P']);
  assert.equal(pins[0][2].startWeek, 3, 'start moves by the dragged weeks');
  assert.equal(pins[0][2].laneDelta, 1, 'lane moves by the dragged rows');
  assert.equal(pins[0][2].effort, undefined, 'a move never edits the estimate');
});

// Decision 4 math: the engine's forward direction is
// duration = effort / ((1-loss) x lanes), so the inverse is
// deffort = dduration x lanes x (1-loss). The loss comes from the plan.
test('a right-edge drag resizes the estimate by lanes x lossFactor', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '2' });
  const resizes = [];
  attachDrag(makeRoot([bar]), {
    lossFactor: 0.8, // a plan with 20% capacity loss
    onPin: () => assert.fail('resize must not pin'),
    onResize: (i, p, w) => resizes.push([i, p, w]),
  });
  // dx = 50px -> weekDelta 4 (12px/week): deffort = 4 x 2 x 0.8 = 6.4 -> 6
  await drag(bar, 116, 5, 166, 5);
  assert.deepEqual(resizes, [['Big', 'P', 16]]);
});

test('a right-edge drag uses the default 0.9 loss when the plan says nothing', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '2' });
  const resizes = [];
  attachDrag(makeRoot([bar]), {
    onPin: () => {},
    onResize: (i, p, w) => resizes.push(w),
  });
  // weekDelta 4: 4 x 2 x 0.9 = 7.2 -> 7
  await drag(bar, 116, 5, 166, 5);
  assert.deepEqual(resizes, [17]);
});

test('a right-edge shrink refuses to go below one week', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '2', lanes: '2' });
  let fired = 0;
  attachDrag(makeRoot([bar]), {
    onPin: () => { fired++; },
    onResize: () => { fired++; },
  });
  // weekDelta -3: -3 x 2 x 0.9 = -5.4 -> -5; 2 - 5 < 1 -> refused, nothing sent
  await drag(bar, 116, 5, 80, 5);
  assert.equal(fired, 0);
});

test('a left-edge drag moves the start and shrinks the effort so the finish anchors', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '1' });
  const pins = [];
  attachDrag(makeRoot([bar]), {
    onPin: (i, p, e) => pins.push([i, p, e]),
    onResize: () => assert.fail('resize-w goes through onPin with an effort'),
  });
  // dx=25 -> weekDelta 2 (later start): effort 10 - round(2 x 1 x 0.9) = 8
  await drag(bar, 2, 5, 27, 5);
  assert.equal(pins.length, 1);
  assert.equal(pins[0][2].startWeek, 4, 'left edge drags the start');
  assert.equal(pins[0][2].effort, 8, 'the estimate shrinks so the finish holds');
  assert.equal(pins[0][2].laneDelta, 0, 'a horizontal resize never changes lanes');
});

test('a left-edge drag that would shrink below one week is refused', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '8', estimate: '2', lanes: '1' });
  let fired = 0;
  attachDrag(makeRoot([bar]), {
    onPin: () => { fired++; },
    onResize: () => { fired++; },
  });
  // weekDelta 3: effort 2 - round(3 x 1 x 0.9) = 2 - 3 < 1 -> refused
  await drag(bar, 2, 5, 38, 5);
  assert.equal(fired, 0);
});

test('a click (no movement) dispatches nothing', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '1' });
  let fired = 0;
  attachDrag(makeRoot([bar]), {
    onPin: () => { fired++; },
    onResize: () => { fired++; },
  });
  await drag(bar, 60, 5, 62, 7); // dx=2, dy=2 — under the 3px threshold
  assert.equal(fired, 0);
});

test('a click leaves the bar\'s rendered width intact', async () => {
  // Regression: release() reset the drag preview with style.width = '',
  // wiping the % width barHTML rendered — an absolutely positioned bar then
  // shrank to shrink-to-fit (its label width) on every mere click.
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '1' });
  bar.style.width = '47.87%';
  attachDrag(makeRoot([bar]), { onPin: () => {}, onResize: () => {} });
  await drag(bar, 60, 5, 61, 5); // a click
  assert.equal(bar.style.width, '47.87%', 'the rendered width survives a click');
});

test('a pointercancel restores the rendered width', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '2' });
  bar.style.width = '47.87%';
  attachDrag(makeRoot([bar]), { onPin: () => {}, onResize: () => {} });
  bar.listeners.pointerdown({ button: 0, pointerType: 'mouse', pointerId: 1, clientX: 116, clientY: 5, preventDefault: () => {} });
  bar.style.width = '500px'; // the resize preview wrote a px width
  bar.listeners.pointercancel();
  assert.equal(bar.style.width, '47.87%', 'cancel restores the rendered width, not a wipe');
});

test('a resize drag restores the original width on release', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '2' });
  bar.style.width = '47.87%';
  attachDrag(makeRoot([bar]), { onPin: () => {}, onResize: () => {} });
  await drag(bar, 116, 5, 129, 5);
  assert.equal(bar.style.width, '47.87%', 'the preview\'s px width is undone; the render owns the width');
});

test('an edge gesture with no horizontal movement dispatches nothing', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', estimate: '10', lanes: '1' });
  let fired = 0;
  attachDrag(makeRoot([bar]), {
    onPin: () => { fired++; },
    onResize: () => { fired++; },
  });
  await drag(bar, 116, 5, 118, 25); // dx=2 (a click's worth), dy=20 — no weeks
  assert.equal(fired, 0, 'a mostly-vertical edge gesture is not an edit');
});

test('the left edge of a week-0 bar is a move, not a resize-w', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '0', estimate: '10', lanes: '1' });
  const pins = [];
  attachDrag(makeRoot([bar]), {
    onPin: (i, p, e) => pins.push([i, p, e]),
    onResize: () => {},
  });
  await drag(bar, 2, 5, 15, 5); // x=2, but the start cannot move earlier
  assert.equal(pins.length, 1);
  assert.equal(pins[0][2].startWeek, 1, 'acts as a body move off week 0');
  assert.equal(pins[0][2].effort, undefined, 'a move never edits the estimate');
});

test('a bar without data-estimate (in-flight) falls through to a move', async () => {
  const bar = makeBar({ initiative: 'Big', pod: 'P', startWeek: '2', lanes: '2' });
  const pins = [];
  attachDrag(makeRoot([bar]), {
    onPin: (i, p, e) => pins.push([i, p, e]),
    onResize: () => assert.fail('no estimate on the bar means no resize'),
  });
  await drag(bar, 116, 5, 129, 5);
  assert.equal(pins.length, 1);
  assert.equal(pins[0][2].effort, undefined, 'in-flight bars are moved, not resized');
});

test('bars carry the estimate and lanes for the resize gesture', () => {
  const ps = { pod: 'P', tracks: 2, slices: [{
    initiative: 'Big', pod: 'P', startWeek: 3, finishWeek: 9, lanesUsed: 2,
    remainingWeeks: 6, latestStartWeek: 5, slackWeeks: 2,
  }] };
  const html = podLanesHTML(ps, { horizonWeeks: 26 });
  assert.match(html, /data-estimate="6"/, 'duration is the fallback');
  assert.match(html, /data-lanes="2"/);
});

test('the estimate attribute prefers the plan initiative effort over the duration', () => {
  const ps = { pod: 'P', tracks: 2, slices: [{
    initiative: 'Big', pod: 'P', startWeek: 3, finishWeek: 9, lanesUsed: 2,
    remainingWeeks: 6, latestStartWeek: 5, slackWeeks: 2,
  }] };
  // Spec 008 S4: estimateEdits is pod -> ABSOLUTE effort weeks; the slice's
  // remainingWeeks is the post-division duration. A 5-lane slice must not
  // PATCH its duration as if it were effort.
  const html = podLanesHTML(ps, {
    horizonWeeks: 26,
    planInitiatives: [{ name: 'Big', work: { P: { weeks: 200 } } }],
  });
  assert.match(html, /data-estimate="200"/);
});

test('in-flight initiatives carry no estimate attribute', () => {
  const ps = { pod: 'P', tracks: 2, slices: [{
    initiative: 'Big', pod: 'P', startWeek: 3, finishWeek: 9, lanesUsed: 2,
    remainingWeeks: 6, latestStartWeek: 5, slackWeeks: 2,
  }] };
  const html = podLanesHTML(ps, {
    horizonWeeks: 26,
    planInitiatives: [{ name: 'Big', inFlight: true, work: { P: { weeks: 200 } } }],
  });
  assert.ok(!/data-estimate=/.test(html), 'the resize gesture is withheld');
  assert.match(html, /data-lanes="2"/, 'lane geometry still present');
});
