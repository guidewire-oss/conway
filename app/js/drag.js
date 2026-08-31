// Drag-to-edit for timeline bars (spec 008): a released drag persists a PIN
// (initiative + pod + start week) and the server recomputes the schedule —
// never client geometry, so both lenses and the Order table agree by
// construction (Decision 1).
//
// Whole weeks only: the model is weekly, and a fractional drag would promise
// precision the schedule cannot keep.
//
// Spec 008 S4 adds edge-resize (Decision 4): the right edge edits the pod's
// estimate; the left edge moves the start AND shrinks/grows the estimate by
// the same duration so the finish stays anchored (Q2). The body drag moves
// the whole bar (start-week pin). Effort math: the engine's forward direction
// is duration = effort / ((1−loss) × lanes), so the inverse is
// Δeffort = Δduration × lanes × (1−loss) — the plan's capacity loss, never a
// hardcoded default.

const WEEK_PX_HINT = 12; // px around an edge that counts as the resize zone

// attachDrag makes every bar in `root` draggable. callbacks:
//   onPin(initiative, pod, { startWeek, laneDelta, effort }) — async PATCH +
//   recompute. startWeek null = time unchanged; laneDelta 0 = lane unchanged;
//   effort (weeks, optional) = the new estimate when the gesture resized.
//   onResize(initiative, pod, newEffortWeeks) — async PATCH estimateEdits.
//   lossFactor = 1 − capacityLoss of the plan (default 0.9).
export function attachDrag(root, { onPin, onResize, span, horizon, periodStart, readOnly, lossFactor = 0.9 }) {
  if (!root || readOnly) return;
  // Touch/coarse-pointer devices never attach drag handlers (cubic P2): a
  // swipe over a bar is a scroll, not a pin.
  if (typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches) return;
  root.querySelectorAll('.tl-bar').forEach((bar) => {
    if (bar.dataset.draggable) return; // re-attach safe on repaint
    bar.dataset.draggable = '1';
    const initiative = bar.dataset.initiative;
    const pod = bar.dataset.pod;
    if (!initiative || !pod) return; // continuation bars carry the same data attrs

    let startX = 0;
    let startY = 0;
    let startW = 0; // width at pointerdown — resize previews grow from it
    let savedWidth = null; // the bar's rendered inline width, restored on release
    let dragging = false;
    let mode = 'move';

    // Edge-zone detection: the cursor position within the bar determines the
    // drag mode. 12px zones on each edge, grab on the body. Narrow bars
    // shrink the zones (a third of the width each) so they never overlap —
    // an 8px bar would otherwise be all edge and the left zone would win.
    const EDGE = 12;
    const detectMode = (ev) => {
      const rect = bar.getBoundingClientRect();
      const edge = Math.min(EDGE, rect.width / 3);
      const x = ev.clientX - rect.left;
      if (x < edge && Number(bar.dataset.startWeek || 0) > 0) return 'resize-w';
      if (x > rect.width - edge) return 'resize-e';
      return 'move';
    };

    bar.style.cursor = 'grab';
    bar.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      if (ev.pointerType !== 'mouse') return; // coarse pointers scroll, never pin
      dragging = true;
      mode = detectMode(ev);
      startX = ev.clientX;
      startY = ev.clientY;
      startW = bar.getBoundingClientRect().width;
      // The preview writes px widths over the rendered % width; release and
      // cancel must restore THIS, not clear it — clearing collapses an
      // absolutely positioned bar to shrink-to-fit, and every click on a bar
      // wiped its width (the "shrinking timeline").
      savedWidth = bar.style.width;
      bar.setPointerCapture(ev.pointerId);
      bar.style.cursor = mode === 'move' ? 'grabbing' : mode === 'resize-w' ? 'w-resize' : 'e-resize';
      ev.preventDefault();
    });
    bar.addEventListener('pointermove', (ev) => {
      if (!dragging) return;
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;
      bar.style.opacity = '0.75';
      // preview per mode: resize-e grows the width, resize-w moves the left
      // edge (translate + shrink), move translates the whole bar
      if (mode === 'resize-e') {
        bar.style.width = `${Math.max(2, startW + dx)}px`;
      } else if (mode === 'resize-w') {
        bar.style.transform = `translateX(${Math.max(-startW + 2, dx)}px)`;
        bar.style.width = `${Math.max(2, startW - dx)}px`;
      } else {
        const rowH = rowHeight(root);
        bar.style.transform = `translate(${dx}px, ${Math.round(dy / rowH) * rowH}px)`;
      }
    });
    const release = async (ev) => {
      if (!dragging) return;
      dragging = false;
      bar.style.cursor = 'grab';
      bar.style.transform = '';
      bar.style.width = savedWidth ?? '';
      bar.style.opacity = '';
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;
      if (Math.abs(dx) < 3 && Math.abs(dy) < 3) return; // a click, not a drag
      const laneDelta = Math.round(dy / rowHeight(root));
      const weekDelta = Math.round(dx / weekWidth(root));
      if (weekDelta === 0 && laneDelta === 0) return;
      const estimate = Number(bar.dataset.estimate);
      const lanes = Number(bar.dataset.lanes || 1);
      // Spec 014: the pod's effective loss rides the bar (data-loss, percent)
      // — a 30%-loss pod converts duration to effort at 0.7, not at the plan
      // global. The opts.lossFactor (the plan global) is the fallback for a
      // schedule rendered before the pod carried the field.
      const lossPct = Number(bar.dataset.loss);
      const factor = Number.isFinite(lossPct) && lossPct > 0 ? 1 - lossPct / 100 : lossFactor;
      const delta = Math.round(weekDelta * lanes * factor); // Decision 4 math
      if (mode === 'resize-w') {
        // Left edge (Q2): the dragged edge moves, the finish anchors. The
        // start moves AND the estimate shrinks by the same duration, so the
        // finish holds instead of sliding with the start.
        if (weekDelta === 0) return;
        if (!Number.isFinite(estimate) || estimate - delta < 1) return; // cannot shrink below one week
        const curStart = Number(bar.dataset.startWeek || 0);
        await onPin(initiative, pod, {
          startWeek: Math.max(0, curStart + weekDelta), laneDelta: 0, effort: estimate - delta,
        });
      } else if (mode === 'resize-e' && onResize && bar.dataset.estimate !== undefined) {
        // Right edge: more/less duration = more/less effort; the engine
        // re-divides by lanes on the server. In-flight bars carry no
        // data-estimate (their remaining effort is progress-adjusted and the
        // absolute estimate cannot be derived) and fall through to a move.
        if (weekDelta === 0) return;
        if (!Number.isFinite(estimate) || estimate + delta < 1) return; // cannot shrink below one week
        await onResize(initiative, pod, estimate + delta);
      } else {
        await onPin(initiative, pod, {
          startWeek: weekDelta ? Math.max(0, Number(bar.dataset.startWeek || 0) + weekDelta) : null,
          laneDelta,
        });
      }
    };
    bar.addEventListener('pointerup', release);
    bar.addEventListener('pointercancel', () => {
      dragging = false;
      bar.style.transform = '';
      bar.style.width = savedWidth ?? '';
      bar.style.opacity = '';
    });
  });
}

// weekWidth derives one week's pixel width from the axis the bars sit in —
// the bars are percentage-positioned inside .tl-track, so measure that.
function weekWidth(root) {
  const track = root.querySelector('.tl-track');
  if (!track) return WEEK_PX_HINT;
  const w = track.getBoundingClientRect().width;
  const horizon = Number(root.dataset.horizon || 26);
  return w / Math.max(1, horizon);
}

// rowHeight is one lane row's pixel height — vertical drags snap to rows.
function rowHeight(root) {
  const lane = root.querySelector('.tl-lane');
  return lane ? lane.getBoundingClientRect().height : 22;
}
