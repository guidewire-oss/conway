// The in-app manual (spec 012 FR-002): docs.html ships offline inside app/,
// themed via conway.css. openDocs(section) shows it in a full overlay and
// scrolls to a section — used by the nav Docs button, per-view help buttons,
// and warning deep links.

import { openModal, closeModal } from './modal.js';

let overlay;

function ensureOverlay() {
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = 'docs-overlay';
  overlay.className = 'modal fade';
  overlay.tabIndex = -1;
  overlay.innerHTML = `
    <div class="modal-dialog modal-xl modal-dialog-centered modal-dialog-scrollable" style="max-width: 1140px;">
      <div class="modal-content" style="background: var(--bg); color: var(--text);">
        <div class="modal-header" style="border-bottom: 1px solid var(--border);">
          <h5 class="modal-title">Conway — in-app manual</h5>
          <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="Close"></button>
        </div>
        <div class="modal-body" style="padding: 0;">
          <iframe id="docs-frame" src="about:blank" title="Conway in-app manual"
            style="width: 100%; height: calc(100vh - 160px); border: 0; background: var(--bg);"></iframe>
        </div>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  // Forward Escape from inside the iframe: key events stay in the iframe's
  // document and BS Modal never sees them (cubic P2).
  frame.addEventListener('load', () => {
    frame.contentWindow.document.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape') { const m = bootstrap.Modal.getInstance(ov); if (m) m.hide(); }
    });
  }, { once: true });
  return overlay;
}

// openDocs shows the manual, optionally scrolled to a section id
// ("order", "timeline", "warnings", "docs-top", ...).
export function openDocs(section) {
  const ov = ensureOverlay();
  openModal(ov);
  const frame = ov.querySelector('#docs-frame');
  const target = `docs.html${section ? `#${section}` : ''}`;
  // setting the hash on a loaded frame scrolls it; a fresh load picks it up
  if (frame.dataset.loaded === '1') {
    frame.contentWindow.location.hash = section || '';
  } else {
    frame.src = target;
    frame.addEventListener('load', () => {
      frame.dataset.loaded = '1';
      // Propagate the app's theme to the iframe document (cubic P2: the
      // manual stayed dark in light mode).
      frame.contentWindow.document.documentElement.setAttribute('data-bs-theme',
        document.documentElement.getAttribute('data-bs-theme') || 'dark');
    }, { once: true });
  }
}

// initDocs wires the delegated entry: any [data-docs] button opens the
// manual at its section. Called once from main.js at boot.
export function initDocs() {
  document.addEventListener('click', (ev) => {
    const b = ev.target.closest?.('[data-docs]');
    if (!b) return;
    const section = b.dataset.docs;
    openDocs(section === 'docs-top' ? '' : section);
  });
}
