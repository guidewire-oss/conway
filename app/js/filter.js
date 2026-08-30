// Fuzzy matching for the timeline lens filters (spec 010). Case-insensitive,
// substring OR subsequence: "aplat" matches "Apollo/App Platform" because the
// letters appear in order; "app platform" matches "Apollo/App Platform" as a
// substring after squashing. Pure functions — no DOM — so the shape is
// unit-tested.

// fuzzyMatch reports whether query matches target. Empty query matches all.
export function fuzzyMatch(query, target) {
  const q = String(query || '').toLowerCase().replace(/\s+/g, ' ').trim();
  if (!q) return true;
  const t = String(target || '').toLowerCase().replace(/\s+/g, ' ');
  if (t.includes(q)) return true;
  // subsequence: each query char appears in order (classic fuzzy find)
  let i = 0;
  for (const ch of t) {
    if (ch === q[i]) i++;
    if (i === q.length) return true;
  }
  return i === q.length;
}
