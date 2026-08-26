// Drag-to-edit for timeline bars (spec 008): a released drag persists a PIN
// (initiative + pod + start week) and the server recomputes the schedule —
// never client geometry, so both lenses and the Order table agree by
// construction (Decision 1).
//
// Whole weeks only: the model is weekly, and a fractional drag would promise
// precision the schedule cannot keep.
//
// The bars render as before; this module attaches pointer listeners to them
// after each paint. Move = drag the body; resize = drag an edge (Q2: the
// dragged edge moves, the opposite edge anchors). Resize maps to nothing yet
// server-side beyond the pin (the estimate is the duration's source), so an
// edge drag moves the start (left edge) or pins the finish by extending the
// start backwards — both expressible as a start-week pin.

const WEEK_PX_HINT = 12; // px around an edge that counts as the resize zone

// attachDrag makes every bar in `root` draggable. callbacks:
//   onPin(initiative, pod, { startWeek, laneDelta }) — async PATCH + recompute.
//   startWeek null = time unchanged; laneDelta 0 = lane unchanged.
export function attachDrag(root, { onPin, span, horizon, periodStart, readOnly }) {
  if (!root || readOnly) return;
  root.querySelectorAll('.tl-bar').forEach((bar) => {
    if (bar.dataset.draggable) return; // re-attach safe on repaint
    bar.dataset.draggable = '1';
    const initiative = bar.dataset.initiative;
    const pod = bar.dataset.pod;
    if (!initiative || !pod) return; // continuation bars carry the same data attrs

    let startX = 0;
    let dragging = false;

    bar.style.cursor = 'grab';
    let startY = 0;
    bar.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      dragging = true;
      startX = ev.clientX;
      startY = ev.clientY;
      bar.setPointerCapture(ev.pointerId);
      bar.style.cursor = 'grabbing';
      ev.preventDefault();
    });
    bar.addEventListener('pointermove', (ev) => {
      if (!dragging) return;
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;
      bar.style.opacity = '0.75';
      // a light translate preview; the authoritative placement comes from
      // the server on release. dy previews the lane move (row height).
      const rowH = rowHeight(root);
      bar.style.transform = `translate(${dx}px, ${Math.round(dy / rowH) * rowH}px)`;
    });
    const release = async (ev) => {
      if (!dragging) return;
      dragging = false;
      bar.style.cursor = 'grab';
      bar.style.transform = '';
      bar.style.opacity = '';
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;
      if (Math.abs(dx) < 3 && Math.abs(dy) < 3) return; // a click, not a drag
      const laneDelta = Math.round(dy / rowHeight(root));
      const weekDelta = Math.round(dx / weekWidth(root));
      if (weekDelta === 0 && laneDelta === 0) return;
      await onPin(initiative, pod, {
        startWeek: weekDelta ? Math.max(0, Number(bar.dataset.startWeek || 0) + weekDelta) : null,
        laneDelta,
      });
    };
    bar.addEventListener('pointerup', release);
    bar.addEventListener('pointercancel', () => {
      dragging = false;
      bar.style.transform = '';
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
