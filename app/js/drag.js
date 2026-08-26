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
//   onPin(initiative, pod, week) — async, performs the PATCH + recompute
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
    bar.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      dragging = true;
      startX = ev.clientX;
      bar.setPointerCapture(ev.pointerId);
      bar.style.cursor = 'grabbing';
      ev.preventDefault();
    });
    bar.addEventListener('pointermove', (ev) => {
      if (!dragging) return;
      const dx = ev.clientX - startX;
      bar.style.opacity = dx === 0 ? '' : '0.75';
      // a light translate preview; the authoritative placement comes from
      // the server on release
      bar.style.transform = `translateX(${dx}px)`;
    });
    const release = async (ev) => {
      if (!dragging) return;
      dragging = false;
      bar.style.cursor = 'grab';
      bar.style.transform = '';
      bar.style.opacity = '';
      const dx = ev.clientX - startX;
      if (Math.abs(dx) < 3) return; // a click, not a drag
      const weekDelta = Math.round(dx / weekWidth(root));
      if (weekDelta === 0) return;
      const curStart = Number(bar.dataset.startWeek || 0);
      const newStart = Math.max(0, curStart + weekDelta);
      if (newStart === curStart) return;
      await onPin(initiative, pod, newStart);
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
