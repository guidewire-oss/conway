// Fuzzy matching for the timeline lens filters (spec 010). Case-insensitive,
// substring OR subsequence: "aplat" matches "Apollo/App Platform" because the
// letters appear in order; "app platform" matches "Apollo/App Platform" as a
// substring after squashing. Common word endings also connect "rotate" with
// "rotation". Pure functions — no DOM — so the shape is unit-tested.

// specs/010-timeline-lens-filters.md:164: bounded word-ending matching, not
// initiative identity normalization. Keep at least four letters in a root.
function wordRoot(word) {
  let root = word;
  if (root.length > 4 && root.endsWith('s') && !root.endsWith('ss')) root = root.slice(0, -1);
  for (const suffix of ['ing', 'ed', 'ion']) {
    if (root.endsWith(suffix) && root.length - suffix.length >= 4) {
      root = root.slice(0, -suffix.length);
      break;
    }
  }
  if (root.length > 4 && root.endsWith('e')) root = root.slice(0, -1);
  return root;
}

// Initiative names require literal text or matching word roots. Empty matches all.
export function initiativeMatch(query, target) {
  const q = String(query || '').toLowerCase().replace(/\s+/g, ' ').trim();
  if (!q) return true;
  const t = String(target || '').toLowerCase().replace(/\s+/g, ' ');
  if (t.includes(q)) return true;
  const queryWords = q.match(/[\p{L}\p{N}]+/gu) || [];
  const targetWords = t.match(/[\p{L}\p{N}]+/gu) || [];
  return queryWords.length > 0 && queryWords.every((word) =>
    targetWords.some((candidate) => candidate.includes(word) || wordRoot(word) === wordRoot(candidate)));
}

// Team names retain their established shorthand matching. Initiative queries
// require matching text: specs/010-timeline-lens-filters.md:187.
export function fuzzyMatch(query, target) {
  if (initiativeMatch(query, target)) return true;
  const q = String(query || '').toLowerCase().replace(/\s+/g, ' ').trim();
  const t = String(target || '').toLowerCase().replace(/\s+/g, ' ');
  let i = 0;
  for (const ch of t) {
    if (ch === q[i]) i++;
    if (i === q.length) return true;
  }
  return false;
}
