// Bootstrap form-class adoption (spec 011 FR-001): every input, select and
// textarea gets form-control/form-select; checkboxes and radios get
// form-check-input. One delegated pass at boot plus a MutationObserver for
// dynamically rendered views (the whole app renders via innerHTML), so
// native focus rings, sizing and validation states come from Bootstrap
// instead of 47 scattered CSS overrides.

const BOOT_ATTRS = 'input, select, textarea';

function adopt(root) {
  root.querySelectorAll(BOOT_ATTRS).forEach((el) => {
    if (el.type === 'checkbox' || el.type === 'radio') {
      el.classList.add('form-check-input');
      return;
    }
    if (el.type === 'range' || el.type === 'file' || el.type === 'hidden') return;
    if (el.tagName === 'SELECT') {
      el.classList.add('form-select');
      return;
    }
    if (el.type === 'search') {
      el.classList.add('form-control', 'form-control-sm');
      return;
    }
    el.classList.add('form-control');
  });
}

export function initForms() {
  adopt(document);
  // Views render via innerHTML constantly; observe and adopt the new nodes.
  const obs = new MutationObserver((muts) => {
    for (const m of muts) {
      for (const n of m.addedNodes) {
        if (n.nodeType !== 1) continue;
        if (n.matches?.(BOOT_ATTRS)) adopt(n.parentElement || n);
        else if (n.querySelectorAll) adopt(n);
      }
    }
  });
  obs.observe(document.body, { childList: true, subtree: true });
}
