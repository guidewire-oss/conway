// modal.js — the one modal controller (the `cv-modal` extension).
//
// Every modal in the app was the same hand-rolled shape: an overlay div, an
// innerHTML box, `ov.hidden` toggling, a ✕ listener, no focus handling. This
// wraps that shape in Bootstrap's Modal (focus trap, real backdrop, ESC) so
// the ~10 call sites get the framework's behaviour without rewriting their
// templates.
//
// Two deliberate deviations from Bootstrap defaults, both product decisions
// the templates documented before this existed:
//   - no click-outside-to-close: an in-progress form or a deliberate
//     inspection shouldn't vanish on a stray click — the ✕ is the exit;
//   - the overlay div is created lazily by callers and reused, so this module
//     adapts an existing node rather than owning the markup.
//
// Extend-don't-parallel rule: Bootstrap has modal markup/JS; this bridges our
// legacy overlay shape onto it rather than inventing a second modal system.
// Registered in docs/COMPONENTS.md.

// A Map, not a WeakMap: the one-modal-at-a-time handoff needs to iterate the
// live instances; entries are bounded by the app's ~10 overlays.
const instances = new Map();
// Per-overlay listener cleanup, so the dispose-and-rebuild path (a reused
// overlay replacing innerHTML) never stacks a second set of handlers.
const cleanups = new Map();

function bsModal() {
  return window.bootstrap?.Modal;
}

// modalFor adapts a legacy overlay node into a Bootstrap Modal, once.
// Bootstrap's Modal requires the .modal > .modal-dialog structure — without
// the dialog, show() misfires (backdrop appears, .show never lands, hide
// hangs). The overlay's existing first child becomes the dialog: our classes
// keep the visual, BS gets the structure it expects.
function modalFor(ov) {
  const existing = instances.get(ov);
  if (existing) {
    // A reused overlay may have replaced its innerHTML since the last show,
    // orphaning the wrapped dialog. Re-wrap — and recreate the instance,
    // because BS caches its dialog reference at construction; a new wrapper
    // under a stale instance means show/hide drive a detached dialog.
    if (!ov.firstElementChild?.classList.contains('modal-dialog')) {
      cleanups.get(ov)?.();
      cleanups.delete(ov);
      existing.dispose();
      instances.delete(ov);
      return modalFor(ov);
    }
    return existing;
  }
  ov.classList.add('modal');
  wrapDialog(ov);
  const M = bsModal();
  if (!M) {
    // Bootstrap CSS is loaded but its JS is not: .modal hides the overlay
    // (display:none beats the hidden-attr removal), so undo the class before
    // the caller's fallback runs — otherwise the modal opens invisible.
    ov.classList.remove('modal');
    return null;
  }
  const m = new M(ov, {
    backdrop: 'static', // no click-outside-to-close (see module comment)
    keyboard: true,     // ESC closes — new, and what the framework is for
  });
  // ESC independent of focus: BS wires ESC through its focus trap, which
  // requires focus inside the modal; our legacy overlays never receive it.
  // One document listener per instance, alive for the instance's life —
  // NOT once-per-show, which any other keydown would consume.
  const onKey = (ev) => { if (ev.key === 'Escape' && ov.classList.contains('show')) m.hide(); };
  const onShown = () => document.addEventListener('keydown', onKey);
  const onHidden = () => { document.removeEventListener('keydown', onKey); ov.hidden = true; };
  ov.addEventListener('shown.bs.modal', onShown);
  // onHidden both detaches the ESC listener and syncs the legacy `hidden`
  // attr — every hide path (✕, ESC, programmatic) leaves callers' state
  // machines truthful.
  ov.addEventListener('hidden.bs.modal', onHidden);
  // detach(ov) is used by the re-wrap path: dispose() alone leaves these
  // element listeners attached, and a recreated instance would stack a second
  // set — accumulating ESC handlers on every innerHTML replacement.
  const detach = () => {
    ov.removeEventListener('shown.bs.modal', onShown);
    ov.removeEventListener('hidden.bs.modal', onHidden);
    document.removeEventListener('keydown', onKey);
  };
  cleanups.set(ov, detach);
  instances.set(ov, m);
  return m;
}

function wrapDialog(ov) {
  const inner = ov.firstElementChild;
  if (inner && !inner.classList.contains('modal-dialog')) {
    const dialog = document.createElement('div');
    dialog.className = 'modal-dialog modal-dialog-centered';
    dialog.tabIndex = -1; // BS focustrap focuses the dialog; without tabindex it cannot, and ESC/focus-trap die
    dialog.style.maxWidth = 'var(--bs-modal-width, min(1120px, 100%))';
    dialog.style.width = 'auto';
    dialog.style.margin = '0 auto';
    ov.replaceChild(dialog, inner);
    inner.classList.add('modal-content'); // BS restores pointer-events on .modal-content; without it, clicks inside the dialog are dead
    dialog.appendChild(inner);
  }
}

// openModal shows a legacy overlay through Bootstrap's Modal: focus is
// trapped inside, ESC closes, the backdrop dims. Without the bundle (static
// hosting), falls back to plain display so the modal still opens.
export function openModal(ov) {
  if (!ov) return;
  // One modal at a time: opening a second (e.g. the halt modal over an open
  // admin panel) while the first is shown leaves the first visible with a
  // stolen backdrop. Dispose of any shown instance first — same cleanup the
  // strays path needs for legacy `ov.hidden = true` writers.
  for (const [el, inst] of instances) {
    if (el !== ov && el.classList.contains('show')) { inst.hide(); }
  }
  document.querySelectorAll('.modal-backdrop').forEach((b) => b.remove());
  document.body.classList.remove('modal-open');
  const m = modalFor(ov);
  if (m) { ov.hidden = false; m.show(); return; }
  ov.hidden = false;
}

// closeModal hides through the framework so focus returns to the invoker —
// the thing `ov.hidden = true` never did.
export function closeModal(ov) {
  if (!ov) return;
  const m = instances.get(ov);
  if (m) { m.hide(); return; }
  ov.hidden = true;
}
