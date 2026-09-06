// Resumable workspace state (specs/017-planning-and-execution-usability.md:80).
const VIEWS = ['home', 'plan', 'network', 'flow', 'scoreboard', 'hygiene', 'simulator', 'game'];
const PLAN_VIEWS = ['order', 'timeline', 'network', 'execution'];
export function readRoute(url) {
  const p = new URL(url, 'http://localhost').searchParams;
  return {
    view: VIEWS.includes(p.get('view')) ? p.get('view') : (p.has('plan') ? 'plan' : 'home'),
    plan: p.get('plan') || '',
    planView: PLAN_VIEWS.includes(p.get('planView')) ? p.get('planView') : 'order',
    networkLens: p.get('networkLens') === 'what-if' ? 'what-if' : 'observe',
    lens: p.get('lens') === 'pod' ? 'pod' : 'initiative',
    initiative: p.get('initiative') || '', team: p.get('team') || '', selected: p.get('selected') || '',
  };
}
let restoring = 0;
export async function restoringRoute(fn) {
  restoring += 1;
  // Only suppress the synchronous programmatic navigation. A user clicking
  // another destination while its data loads must still update the URL.
  let result;
  try { result = fn(); } finally { restoring -= 1; }
  return await result;
}
export function writeRoute(patch, replace = false) {
  if (restoring) return;
  const url = new URL(location.href);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null || value === undefined || value === '') url.searchParams.delete(key);
    else url.searchParams.set(key, value);
  }
  if (url.href !== location.href) history[replace ? 'replaceState' : 'pushState'](null, '', url);
}
