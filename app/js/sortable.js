// Session-only column sorting for any <table class="sortable">. Click a header
// to sort its tbody by that column (numeric-aware); click again to reverse.
// Delegated, so it keeps working after a table re-renders, and nothing persists
// beyond the page.

function numeric(s) {
  // pull a leading/embedded number out of cells like "44%", "12 items", "ρ 0.83"
  const m = String(s).replace(/,/g, '').match(/-?\d+(\.\d+)?/);
  return m ? parseFloat(m[0]) : null;
}

export function sortTable(table, col) {
  const tbody = table.tBodies[0];
  if (!tbody) return;
  const prev = +(table.dataset.sortCol ?? -1);
  let dir = +(table.dataset.sortDir || 1);
  dir = prev === col ? -dir : 1;
  table.dataset.sortCol = col;
  table.dataset.sortDir = dir;

  const rows = [...tbody.rows].filter((r) => r.cells.length > col);
  rows.sort((a, b) => {
    const va = a.cells[col].textContent.trim();
    const vb = b.cells[col].textContent.trim();
    const na = numeric(va);
    const nb = numeric(vb);
    if (na !== null && nb !== null && na !== nb) return (na - nb) * dir;
    return va.localeCompare(vb, undefined, { numeric: true }) * dir;
  });
  rows.forEach((r) => tbody.appendChild(r));

  const ths = table.tHead?.rows[0]?.cells;
  if (ths) [...ths].forEach((th, i) => {
    th.classList.toggle('sort-asc', i === col && dir > 0);
    th.classList.toggle('sort-desc', i === col && dir < 0);
    th.setAttribute('aria-sort', i === col ? (dir > 0 ? 'ascending' : 'descending') : 'none');
  });
}

// register once; import for the side effect.
function activateSort(e) {
  if (e.target.closest('.help')) return; // clicking a ? tooltip shouldn't sort
  const th = e.target.closest('table.sortable thead th');
  if (!th || th.dataset.nosort !== undefined) return;
  const table = th.closest('table');
  const col = [...th.parentNode.children].indexOf(th);
  sortTable(table, col);
}

// specs/017-planning-and-execution-usability.md:93: dynamically rendered
// sortable headers retain table semantics and become keyboard operable.
export function prepareSortable(root = document) {
  root.querySelectorAll('table.sortable thead th:not([data-nosort])').forEach((th) => {
    th.tabIndex = 0;
    th.setAttribute('scope', 'col');
    if (!th.hasAttribute('aria-sort')) th.setAttribute('aria-sort', 'none');
    th.setAttribute('aria-label', `${th.textContent.trim()}. Sort with Enter or Space.`);
  });
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', activateSort);
  document.addEventListener('keydown', (e) => {
    if ((e.key === 'Enter' || e.key === ' ') && e.target.matches('table.sortable thead th:not([data-nosort])')) {
      e.preventDefault();
      activateSort(e);
    }
  });
  prepareSortable();
  let preparationPending = false;
  new MutationObserver(() => {
    if (preparationPending) return;
    preparationPending = true;
    requestAnimationFrame(() => {
      preparationPending = false;
      prepareSortable();
    });
  }).observe(document.documentElement, { childList: true, subtree: true });
}
