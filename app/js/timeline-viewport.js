// specs/034-readable-scrollable-timelines.md:88: presentation only; never edits a plan.
export function attachTimelineViewport(root, state = {}) {
  const plots = [...root.querySelectorAll('.tl-plot')];
  const defaultWidth = plot => plot.clientWidth <= 600 ? 96 : 160;
  const maximum = plot => Math.max(96, Math.min(640, plot.clientWidth - 120));
  const apply = () => plots.forEach(plot => {
    const width = Math.round(Math.max(96, Math.min(maximum(plot), state.labelWidth ?? defaultWidth(plot))));
    plot.style.setProperty('--tl-label-width', `${width}px`);
    const separator = plot.querySelector('.tl-label-resizer');
    separator.setAttribute('aria-valuenow', String(width));
    separator.setAttribute('aria-valuemax', String(maximum(plot)));
  });
  apply();
  const observer = new ResizeObserver(apply);
  plots.forEach(plot => {
    observer.observe(plot);
    const scroll = plot.querySelector('.tl-scroll');
    const key = plot.closest('[data-pod]')?.dataset.pod || 'initiative';
    scroll.scrollLeft = state.scroll?.[key] || 0;
    scroll.addEventListener('scroll', () => {
      state.scroll ??= {};
      state.scroll[key] = scroll.scrollLeft;
    });
    const separator = plot.querySelector('.tl-label-resizer');
    let drag;
    separator.addEventListener('pointerdown', event => {
      if (event.button !== 0) return;
      event.preventDefault(); event.stopPropagation();
      separator.focus({preventScroll: true});
      drag = {id: event.pointerId, x: event.clientX, width: Number(separator.getAttribute('aria-valuenow'))};
      separator.setPointerCapture(event.pointerId);
    });
    separator.addEventListener('pointermove', event => {
      if (!drag || event.pointerId !== drag.id) return;
      state.labelWidth = Math.max(96, Math.min(maximum(plot), drag.width + event.clientX - drag.x));
      apply();
    });
    separator.addEventListener('lostpointercapture', () => { drag = null; });
    separator.addEventListener('pointerup', event => {
      if (separator.hasPointerCapture(event.pointerId)) separator.releasePointerCapture(event.pointerId);
      drag = null;
    });
    separator.addEventListener('click', event => event.stopPropagation());
    separator.addEventListener('keydown', event => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault(); event.stopPropagation();
      const width = Number(separator.getAttribute('aria-valuenow'));
      state.labelWidth = event.key === 'Home' ? defaultWidth(plot) : event.key === 'End' ? maximum(plot)
        : Math.max(96, Math.min(maximum(plot), width + (event.key === 'ArrowRight' ? 16 : -16)));
      apply();
    });
  });
  return () => observer.disconnect();
}
