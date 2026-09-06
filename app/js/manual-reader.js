// specs/012-in-app-usage-guide.md:221 — progressively enhance the offline guide.
function searchManualSections(sections, query) {
  const terms = query.toLowerCase().trim().split(/\s+/).filter(Boolean);
  if (!terms.length) return [];
  return sections.filter((section) => terms.every((term) => section.content.toLowerCase().includes(term)))
    .map((section) => ({ ...section, relevance: terms.filter((term) => section.title.toLowerCase().includes(term)).length }))
    .sort((a, b) => b.relevance - a.relevance);
}

function initManualReader(doc) {
  const search = doc.getElementById('manual-search');
  const results = doc.getElementById('manual-results');
  const contents = doc.getElementById('manual-contents');
  const toggle = doc.getElementById('contents-toggle');
  const clear = doc.getElementById('search-clear');
  const current = doc.getElementById('current-section');
  const headings = [...doc.querySelectorAll('main h2[id], main h3[id]')];
  const sections = headings.map((heading) => {
    let content = heading.textContent;
    for (let sibling = heading.nextElementSibling; sibling && !headings.includes(sibling); sibling = sibling.nextElementSibling) content += ' ' + sibling.textContent;
    return { id: heading.id, title: heading.textContent.replace(/^\d+\.\s*/, ''), content };
  });
  const links = [...contents.querySelectorAll('a[href^="#"]')];
  doc.documentElement.classList.add('reader-ready');
  doc.getElementById('reader-return').hidden = window.self !== window.top;

  function closeContents() {
    contents.classList.remove('is-open');
    toggle.setAttribute('aria-expanded', 'false');
  }
  function closeSearch() {
    results.hidden = true;
  }
  function renderSearch() {
    results.replaceChildren();
    clear.hidden = !search.value;
    if (!search.value.trim()) { closeSearch(); return; }
    closeContents();
    const matches = searchManualSections(sections, search.value);
    const count = doc.createElement('p');
    count.setAttribute('role', 'status');
    count.textContent = matches.length ? `${matches.length} matching section${matches.length === 1 ? '' : 's'}` : 'No matching sections. Try fewer words or browse Contents.';
    results.append(count);
    matches.forEach((section) => {
      const link = doc.createElement('a');
      link.href = '#' + section.id;
      link.textContent = section.title;
      results.append(link);
    });
    results.hidden = false;
  }
  search.addEventListener('input', renderSearch);
  search.addEventListener('focus', () => { if (search.value.trim()) renderSearch(); });
  search.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown' && !results.hidden) {
      const first = results.querySelector('a');
      if (first) { event.preventDefault(); first.focus(); }
    }
  });
  clear.addEventListener('click', () => { search.value = ''; renderSearch(); search.focus(); });
  toggle.addEventListener('click', () => {
    const open = toggle.getAttribute('aria-expanded') !== 'true';
    closeSearch();
    contents.classList.toggle('is-open', open);
    toggle.setAttribute('aria-expanded', String(open));
  });
  doc.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;
    if (!results.hidden) {
      closeSearch();
      search.focus({ preventScroll: true });
      // Focusing can reopen results; close after returning focus.
      closeSearch();
    } else if (toggle.getAttribute('aria-expanded') === 'true') {
      closeContents();
      toggle.focus({ preventScroll: true });
    } else return;
    event.preventDefault();
    event.stopImmediatePropagation();
  });
  doc.addEventListener('click', (event) => {
    const anchor = event.target.closest('a[href^="#"]');
    if (anchor) {
      const target = doc.getElementById(anchor.getAttribute('href').slice(1));
      closeSearch(); closeContents();
      if (target) { target.tabIndex = -1; target.focus({ preventScroll: true }); }
    } else if (!event.target.closest('.reader-toolbar, #manual-contents')) {
      closeSearch(); closeContents();
    }
  });

  let scheduled = false;
  function updateCurrent() {
    scheduled = false;
    const boundary = doc.querySelector('.reader-toolbar').getBoundingClientRect().bottom + 36;
    let active = 'docs-top';
    for (const heading of headings) {
      if (heading.getBoundingClientRect().top <= boundary) active = heading.id;
      else break;
    }
    links.forEach((link) => {
      if (link.getAttribute('href') === '#' + active) link.setAttribute('aria-current', 'location');
      else link.removeAttribute('aria-current');
    });
    current.textContent = sections.find((section) => section.id === active)?.title || 'Guide overview';
  }
  function requestUpdate() {
    if (!scheduled) { scheduled = true; window.requestAnimationFrame(updateCurrent); }
  }
  window.addEventListener('scroll', requestUpdate, { passive: true });
  window.addEventListener('resize', () => { closeContents(); requestUpdate(); });
  window.addEventListener('hashchange', () => { closeSearch(); closeContents(); requestUpdate(); });
  updateCurrent();
}

if (typeof document !== 'undefined') initManualReader(document);
