import { openModal, closeModal } from './modal.js';

const endpoint = '/api/announcements';
const copyFeature = feature => ({ ...feature, action: { ...feature.action } });

// specs/022-feature-announcements.md:164: server state is authoritative. This
// controller deliberately has no browser persistence or optimistic acknowledgements.
export function createAnnouncementController({ request, getIdentity, onChange = () => {} }) {
  let identity = getIdentity(), epoch = 0, loadTicket = 0, disposed = false;
  let features = [], error = '', loadError = '', loading = false;
  const pending = new Map();
  const failed = new Set();
  const saveError = 'Announcement history could not be saved. New indicators remain until saved; this introduction may appear again after sign-in.';
  const snapshot = () => ({ features: features.map(copyFeature), error: loadError || error, loadError, loading });
  const emit = () => { if (!disposed) onChange(snapshot()); };
  function syncIdentity() {
    const next = getIdentity();
    if (next !== identity) {
      identity = next; epoch++; loadTicket++; features = []; error = ''; loadError = ''; loading = false; pending.clear(); failed.clear(); emit();
    }
    return !disposed && !!identity;
  }
  const owns = (owner, generation) => syncIdentity() && identity === owner && epoch === generation;
  function state() { syncIdentity(); return snapshot(); }
  async function load() {
    if (!syncIdentity()) return snapshot();
    const owner = identity, generation = epoch, ticket = ++loadTicket;
    loading = true; loadError = ''; emit();
    try {
      const response = await request(endpoint);
      if (!response.ok) throw new Error('load');
      const body = await response.json();
      if (!owns(owner, generation) || ticket !== loadTicket) return snapshot();
      if (!Array.isArray(body.features)) throw new Error('catalog');
      const previous = new Map(features.map(feature => [feature.id, feature]));
      features = body.features.filter(feature => feature && typeof feature.id === 'string' && typeof feature.title === 'string' && typeof feature.description === 'string' && feature.action && typeof feature.action.target === 'string').map(feature => ({
        id: feature.id, title: feature.title, description: feature.description, action: { ...feature.action },
        announced: feature.announced === true || previous.get(feature.id)?.announced === true,
        visited: feature.visited === true || previous.get(feature.id)?.visited === true,
      }));
      for (const key of failed) {
        const separator = key.indexOf(':'), kind = key.slice(0, separator), id = key.slice(separator + 1);
        const feature = features.find(item => item.id === id);
        if (!feature || feature[kind]) failed.delete(key);
      }
      error = failed.size ? saveError : '';
      loading = false; emit();
    } catch {
      if (owns(owner, generation) && ticket === loadTicket) {
        loading = false; loadError = 'What\'s new could not be loaded. Try again.'; emit();
      }
    }
    return snapshot();
  }
  async function acknowledge(id, kind) {
    if (!syncIdentity() || !['announced', 'visited'].includes(kind)) return false;
    const feature = features.find(item => item.id === id);
    if (!feature) return false;
    if (feature[kind]) return true;
    const key = `${kind}:${id}`;
    if (pending.has(key)) return pending.get(key);
    const owner = identity, generation = epoch;
    let complete;
    const operation = new Promise(resolve => { complete = resolve; });
    // Reserve the operation before invoking a request that may throw synchronously.
    pending.set(key, operation);
    void (async () => {
      try {
        if (!owns(owner, generation)) return false;
        const response = await request(`${endpoint}/ack`, { method: 'POST', body: JSON.stringify({ id, kind }) });
        if (!response.ok) throw new Error('ack');
        const body = await response.json();
        if (!owns(owner, generation)) return false;
        if (body.id !== id || body[kind] !== true) throw new Error('state');
        const current = features.find(item => item.id === id);
        if (!current) return false;
        current.announced ||= body.announced === true;
        current.visited ||= body.visited === true;
        failed.delete(key); error = failed.size ? saveError : ''; emit(); return true;
      } catch {
        if (owns(owner, generation)) {
          failed.add(key); error = saveError;
          emit();
        }
        return false;
      } finally {
        if (identity === owner && epoch === generation) pending.delete(key);
      }
    })().then(complete);
    return operation;
  }
  async function visit(target) {
    if (!syncIdentity()) return false;
    const matches = features.filter(feature => feature.action.target === target);
    if (!matches.length) return false;
    const results = await Promise.all(matches.map(feature => acknowledge(feature.id, 'visited')));
    return results.every(Boolean);
  }
  async function retry() {
    if (!syncIdentity()) return false;
    const results = await Promise.all([...failed].map(key => {
      const separator = key.indexOf(':');
      return acknowledge(key.slice(separator + 1), key.slice(0, separator));
    }));
    return results.every(Boolean);
  }
  function dispose() { disposed = true; epoch++; features = []; pending.clear(); }
  return { load, acknowledge, visit, retry, state, dispose };
}

const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));

// Catalog destinations are a small explicit integration contract, not arbitrary
// selectors, URLs, or code supplied by a response.
export function safeAnnouncementAction(action) {
  if (!action || typeof action !== 'object') return false;
  if (action.type === 'menu') return action.target === 'docs-btn' && action.parent === 'help-btn' && !action.route;
  if (action.type !== 'route' || action.parent !== 'plan-btn') return false;
  return (action.target === 'view-execution' && action.route === '?view=plan&planView=execution') ||
    (action.target === 'plan-linked-sheets' && action.route === '?view=plan');
}

export function announcementIndicators(features) {
  const targets = new Set();
  for (const feature of features) {
    if (feature.visited || !safeAnnouncementAction(feature.action)) continue;
    targets.add(feature.action.target);
    if (feature.action.parent) targets.add(feature.action.parent);
  }
  return [...targets];
}

export function announcementsHTML(features) {
  if (!features.length) return '<p>No feature announcements are available for your account.</p>';
  return `<ul class="announcement-list">${features.map(feature => `<li><h3>${escapeHTML(feature.title)}</h3><p>${escapeHTML(feature.description)}</p>${safeAnnouncementAction(feature.action) ? `<button type="button" class="btn btn-primary" data-announcement-action="${escapeHTML(feature.id)}">Explore feature<span class="visually-hidden">: ${escapeHTML(feature.title)}</span></button>` : ''}<span class="announcement-state">${feature.visited ? 'Visited' : 'Not yet visited'}</span></li>`).join('')}</ul>`;
}

// Mount only after authentication and workspace restoration. onAction must
// return true only after the actual feature opens; opening its parent menu or
// selecting a plan is insufficient. Organic visits call visit(target) too.
export function mountAnnouncements({ request, getIdentity, onAction, onStateChange, replayButton, root = document }) {
  const doc = root.ownerDocument || root;
  let overlay, displayed = [], mounted = true, openIdentity;
  const status = doc.createElement('p');
  status.className = 'announcement-status'; status.setAttribute('role', 'status'); status.hidden = true;
  (root.body || root).appendChild(status);
  function refreshIndicators() {
    if (!mounted) return;
    root.querySelectorAll('[data-announcement-indicator]').forEach(node => node.remove());
    for (const id of announcementIndicators(controller.state().features)) {
      const target = doc.getElementById(id);
      if (!target || !root.contains(target)) continue;
      const badge = doc.createElement('span');
      badge.className = 'announcement-indicator'; badge.dataset.announcementIndicator = '';
      badge.innerHTML = '<span class="announcement-dot" aria-hidden="true"></span><span class="announcement-new-label">New feature</span>';
      target.appendChild(badge);
    }
  }
  function update(next) {
    if (!mounted) return;
    status.textContent = next.error; status.hidden = !next.error;
    if (overlay) {
      const message = overlay.querySelector('[data-announcement-error]');
      message.textContent = next.error; message.hidden = !next.error;
      const retry = overlay.querySelector('[data-announcement-retry]');
      retry.hidden = !next.error;
      retry.disabled = next.loading;
      retry.textContent = next.loadError ? 'Retry loading announcements' : 'Retry saving announcement history';
      if (openIdentity !== getIdentity()) closeModal(overlay);
    }
    refreshIndicators();
    onStateChange?.(next);
  }
  const controller = createAnnouncementController({ request, getIdentity, onChange: update });
  function acknowledgeDisplayed() {
    if (!mounted || !getIdentity() || openIdentity !== getIdentity()) return;
    for (const id of displayed) void controller.acknowledge(id, 'announced');
  }
  function ensureOverlay() {
    if (overlay) return;
    overlay = doc.createElement('div'); overlay.id = 'announcements-overlay'; overlay.className = 'modal'; overlay.hidden = true;
    overlay.innerHTML = `<div class="modal-dialog modal-dialog-centered modal-dialog-scrollable"><div class="modal-content"><div class="modal-header"><h2 class="modal-title">What's new</h2><button type="button" class="btn btn-outline-secondary" data-announcement-close aria-label="Close what's new and return to your work">Close</button></div><div class="modal-body"><p>Explore when you are ready. New feature indicators remain until you visit the feature.</p><div data-announcement-content></div><p role="status" data-announcement-error hidden></p><button type="button" class="btn btn-outline-secondary" data-announcement-retry hidden>Retry saving announcement history</button></div></div></div>`;
    (root.body || root).appendChild(overlay);
    overlay.addEventListener('shown.bs.modal', acknowledgeDisplayed);
    overlay.querySelector('[data-announcement-close]').addEventListener('click', () => closeModal(overlay));
    overlay.querySelector('[data-announcement-retry]').addEventListener('click', () => {
      if (controller.state().loadError) { void open(); return; }
      void controller.retry(); acknowledgeDisplayed();
    });
    overlay.addEventListener('click', async event => {
      const button = event.target.closest('[data-announcement-action]');
      if (!button || !overlay.contains(button)) return;
      const feature = controller.state().features.find(item => item.id === button.dataset.announcementAction);
      if (!feature || !safeAnnouncementAction(feature.action)) return;
      const owner = getIdentity();
      closeModal(overlay);
      try {
        const opened = await onAction?.({ ...feature.action }, copyFeature(feature));
        if (mounted && owner && owner === getIdentity() && opened === true) await controller.visit(feature.action.target);
      } catch {
        if (mounted && owner === getIdentity()) { status.hidden = false; status.textContent = 'The feature could not be opened. Try again from What\'s new.'; }
      }
    });
  }
  function show(features) {
    if (!mounted || !getIdentity()) return;
    ensureOverlay(); openIdentity = getIdentity(); displayed = features.map(feature => feature.id);
    const next = controller.state();
    overlay.querySelector('[data-announcement-content]').innerHTML = next.loadError && !features.length
      ? '<p>Feature announcements are temporarily unavailable.</p>' : announcementsHTML(features);
    // specs/022-feature-announcements.md:180: the first failed load precedes
    // modal creation, so initialize its retry state as well as later updates.
    update(next);
    const alreadyPresented = overlay.classList.contains('show');
    // Keep the original external invoker when recovery refreshes this dialog.
    if (!alreadyPresented) openModal(overlay);
    // Recovered content can enter an already-open dialog. Bootstrap does not
    // emit another shown event for it, although the new items are now visible.
    if (alreadyPresented || !doc.defaultView?.bootstrap?.Modal) acknowledgeDisplayed();
  }
  async function open() {
    const owner = getIdentity();
    await controller.load();
    if (mounted && owner && owner === getIdentity()) show(controller.state().features);
  }
  const replay = replayButton || doc.getElementById('announcements-open');
  const replayClick = () => { void open(); };
  replay?.addEventListener('click', replayClick);
  // Re-rendered plan controls receive their indicators without treating DOM
  // mutation or parent-menu opening as evidence of a visit.
  const Observer = doc.defaultView?.MutationObserver;
  let observer;
  if (Observer) {
    observer = new Observer(records => {
      if (records.some(record => [...record.addedNodes].some(node => node.nodeType === 1 && !node.hasAttribute?.('data-announcement-indicator') && (node.id || node.querySelector?.('[id]'))))) refreshIndicators();
    });
    observer.observe(root.body || root, { childList: true, subtree: true });
  }
  const initialIdentity = getIdentity();
  const ready = controller.load().then(next => {
    if (mounted && initialIdentity && initialIdentity === getIdentity()) { const unseen = next.features.filter(feature => !feature.announced); if (unseen.length) show(unseen); }
  });
  function dispose() {
    mounted = false; observer?.disconnect(); replay?.removeEventListener('click', replayClick);
    controller.dispose(); if (overlay) { closeModal(overlay); overlay.remove(); } status.remove();
    root.querySelectorAll('[data-announcement-indicator]').forEach(node => node.remove());
  }
  return { ...controller, open, refreshIndicators, ready, dispose };
}
