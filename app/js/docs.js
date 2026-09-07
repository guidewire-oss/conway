// The in-app manual (spec 012 FR-002): docs.html ships offline inside app/,
// themed via conway.css. openDocs(section) shows it in a full overlay and
// scrolls to a section — used by the nav Docs button, per-view help buttons,
// and warning deep links.

import { openModal, closeModal } from './modal.js';

let overlay;
let requestedSection = 'docs-top';

// specs/012-in-app-usage-guide.md:221 — a hidden frame cannot reliably apply
// fragment scrolling; repeat opens also need positioning when the hash is unchanged.
function positionSection() {
  const frame = overlay.querySelector('#docs-frame');
  if (frame.dataset.loaded !== '1') return;
  frame.contentDocument?.getElementById(requestedSection)?.scrollIntoView({ block: 'start', behavior: 'instant' });
}

function ensureOverlay() {
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = 'docs-overlay';
  overlay.className = 'modal fade';
  overlay.tabIndex = -1;
  overlay.innerHTML = `
    <div class="modal-dialog modal-xl modal-dialog-centered modal-dialog-scrollable" style="max-width: 1140px;">
      <div class="modal-content" style="background: var(--bg); color: var(--text);">
        <div class="modal-header" style="border-bottom: 1px solid var(--border); flex-wrap: wrap; gap: 12px;">
          <h5 class="modal-title">Conway guide</h5>
          <div class="d-flex align-items-center gap-3">
            <a id="docs-separate" href="docs.html" target="_blank" rel="noopener" class="small">Open guide in new tab</a>
            <button type="button" class="btn btn-sm btn-outline-secondary" data-bs-dismiss="modal" aria-label="Close guide and return to your work">Close</button>
          </div>
        </div>
        <div class="modal-body" style="padding: 0;">
          <iframe id="docs-frame" src="about:blank" title="Conway in-app manual"
            style="width: 100%; height: calc(100vh - 160px); border: 0; background: var(--bg);"></iframe>
        </div>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  overlay.addEventListener('shown.bs.modal', () => {
    positionSection();
    window.dispatchEvent(new CustomEvent('conway:feature-opened', { detail: { action: 'guide' } }));
  });
  // specs/017-planning-and-execution-usability.md:92 — iframe focus stays
  // in its document. Reattach after each navigation, including the first load.
  const frame = overlay.querySelector('#docs-frame');
  frame.addEventListener('load', () => {
    const doc = frame.contentDocument;
    delete frame.dataset.loaded;
    if (!doc) return;
    if (frame.contentWindow.location.pathname.endsWith('/docs.html')) frame.dataset.loaded = '1';
    positionSection();
    frame.contentWindow.addEventListener?.('hashchange', () => {
      overlay.querySelector('#docs-separate').href = `docs.html${frame.contentWindow.location.hash}`;
    });
    doc.documentElement.setAttribute('data-bs-theme', document.documentElement.getAttribute('data-bs-theme') || 'dark');
    doc.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape' && !ev.defaultPrevented) { ev.preventDefault(); closeModal(overlay); }
    });
  });
  return overlay;
}

// openDocs shows the manual, optionally scrolled to a section id
// ("order", "timeline", "warnings", "docs-top", ...).
export function openDocs(section) {
  const ov = ensureOverlay();
  requestedSection = section || 'docs-top';
  openModal(ov);
  const frame = ov.querySelector('#docs-frame');
  const target = `docs.html${section ? `#${section}` : ''}`;
  ov.querySelector('#docs-separate').href = target;
  // setting the hash on a loaded frame scrolls it; a fresh load picks it up
  if (frame.dataset.loaded === '1' && frame.contentDocument) {
    frame.contentWindow.location.hash = section || '';
    // Re-apply theme on every open — a theme toggle between opens would
    // leave the iframe in its old palette (cubic P2).
    frame.contentWindow.document.documentElement.setAttribute('data-bs-theme',
      document.documentElement.getAttribute('data-bs-theme') || 'dark');
    positionSection();
  } else {
    frame.src = target;
  }
}

// initDocs wires the delegated entry: any [data-docs] button opens the
// manual at its section. Called once from main.js at boot.
// specs/012-in-app-usage-guide.md:198 — view IDs and manual anchors differ.
export function manualSectionForView(view, planView) {
  if (view === 'plan') return ({
    'view-order': 'order', 'plan-view-network': 'plan-network',
    'view-forecast': 'portfolio-forecasts', 'view-timeline': 'timeline', 'view-execution': 'execution', 'view-ready': 'next-work', 'view-report': 'health-report',
  }[planView] || 'planning-loop');
  return ({ home: 'start', network: 'network', scoreboard: 'scoreboard',
    hygiene: 'hygiene', simulator: 'simulator', flow: 'flow-actions', game: 'learning',
  }[view] || 'what');
}

export function initDocs() {
  document.addEventListener('click', (ev) => {
    const b = ev.target.closest?.('[data-docs]');
    if (!b) return;
    ev.preventDefault();
    let section = b.dataset.docs;
    if (section === 'context') {
      const view = document.querySelector('main > .view.active')?.id?.replace('view-', '');
      section = manualSectionForView(view, document.querySelector('.plan-views .btn.active')?.id);
    }
    openDocs(section === 'docs-top' ? '' : section);
  });
}
