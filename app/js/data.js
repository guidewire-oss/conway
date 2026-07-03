// Single data-access seam for Observe. Reads always come from the selected
// snapshot in the DB (/api/snapshots/{id}/...) — the server (and Postgres)
// are required to run Conway at all, so there's no local-file fallback here.
import { authFetch } from './auth.js';

let snapshotId = 'baseline';

export function setSnapshot(id) { snapshotId = id || 'baseline'; }
export function getSnapshot() { return snapshotId; }

// The org's Jira base URL (CONWAY_JIRA_BASE_URL) for "view in Jira" links —
// '' when unconfigured, in which case callers should render the key as plain
// text rather than build a broken link. Fetched once and cached.
let jiraBaseUrl = null;
export async function getJiraBaseUrl() {
  if (jiraBaseUrl !== null) return jiraBaseUrl;
  try {
    const r = await fetch('/api/config');
    jiraBaseUrl = r.ok ? ((await r.json()).jiraBaseUrl || '') : '';
  } catch { jiraBaseUrl = ''; }
  return jiraBaseUrl;
}

// Resolve a logical data path (e.g. 'pods.json', 'epics/index.json') to a URL.
export function dataURL(path) {
  return `/api/snapshots/${encodeURIComponent(snapshotId)}/data/${path}`;
}

// Fetch + parse a snapshot document, or null on any error (callers default).
export async function dataJson(path) {
  try {
    const r = await authFetch(dataURL(path));
    if (!r.ok) return null;
    return await r.json();
  } catch { return null; }
}

// Fetch a document from a SPECIFIC snapshot (used by the compare view to read
// a second snapshot alongside the one Observe is currently showing).
export async function snapshotDataJson(id, path) {
  try {
    const r = await authFetch(`/api/snapshots/${encodeURIComponent(id)}/data/${path}`);
    if (!r.ok) return null;
    return await r.json();
  } catch { return null; }
}

// Call a table-backed query endpoint on the current snapshot:
// apiGet('fever?n=25'), apiGet('wip?pod=X&page=0'), apiGet('epic/KEY').
// These compute filtered/paginated views in SQL — no JSON blobs. null on error.
export async function apiGet(path) {
  try {
    const r = await authFetch(`/api/snapshots/${encodeURIComponent(snapshotId)}/${path}`);
    return r.ok ? await r.json() : null;
  } catch { return null; }
}

// List available snapshots.
export async function listSnapshots() {
  try {
    const r = await authFetch('/api/snapshots');
    return r.ok ? ((await r.json()) || []) : [];
  } catch { return []; }
}
