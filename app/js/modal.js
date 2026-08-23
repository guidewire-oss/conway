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

const instances = new WeakMap();

function bsModal() {
  return window.bootstrap?.Modal;
}

// modalFor adapts a legacy overlay node into a Bootstrap Modal, once.
// Bootstrap's Modal requires the .modal > .modal-dialog structure — without
// the dialog, show() misfires (backdrop appears, .show never lands, hide
// hangs). The overlay's existing first child becomes the dialog: our classes
// keep the visual, BS gets the structure it expects.
function modalFor(ov) {
  if (instances.has(ov)) return instances.get(ov);
  ov.classList.add('modal');
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
  const M = bsModal();
  if (!M) return null; // bundle absent (static dev): callers' fallback runs
  const m = new M(ov, {
    backdrop: 'static', // no click-outside-to-close (see module comment)
    keyboard: true,     // ESC closes — new, and what the framework is for
  });
  // BS wires ESC through its focus trap, which requires focus inside the
  // modal; our legacy overlays never receive it (BS's own templates put
  // tabindex=-1 on .modal-dialog AND move focus; ours get focus stolen by
  // the invoking button's re-render). A document-level ESC closes the open
  // modal regardless of where focus sits — the honest behavior for a tool
  // where modals are deliberate.
  ov.addEventListener('shown.bs.modal', () => {
    document.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape' && ov.classList.contains('show')) m.hide();
    }, { once: true });
  });
  // Sync the legacy `hidden` attr on EVERY hide path (✕, ESC, programmatic),
  // so callers' state machines stay truthful.
  ov.addEventListener('hidden.bs.modal', () => { ov.hidden = true; });
  instances.set(ov, m);
  return m;
}

// openModal shows a legacy overlay through Bootstrap's Modal: focus is
// trapped inside, ESC closes, the backdrop dims. Without the bundle (static
// hosting), falls back to plain display so the modal still opens.
export function openModal(ov) {
  if (!ov) return;
  // A previous direct `ov.hidden = true` (any legacy path) can orphan BS's
  // backdrop: the class was stripped without the hide transition running.
  // Clear strays before showing so backdrops never stack.
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
  if (m) { m.hide(); return; } // hidden-sync is bound for the instance's life
  ov.hidden = true;
}
