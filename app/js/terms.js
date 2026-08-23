// terms.js — Conway's one glossary (IA audit, Finding 3).
//
// Every domain term is defined ONCE here and every view renders the same
// definition through the same affordance. Before this, the scoreboard had
// 14 good inline help dots, hygiene/simulator had their own copies of the
// same helper, and the Order view — the densest jargon surface in the app —
// had zero (measured 2026-08-23).
//
// Accessibility (WCAG 2.2 SC 1.3.1, failure F111): the affordance is a real
// <button> with an aria-label — not a bare span whose meaning is conveyed by
// presentation alone. Keyboard focusable; the tooltip shows on focus as well
// as hover.
//
// Writing rule (NN/g heuristic #2, match the real world): the SHORT label is
// what a planning manager says; the tooltip's first sentence must make sense
// to someone who has never read The Goal. The theory credit comes after.

// The definitions. Keys are stable ids — views call term('rho') etc.
export const TERMS = {
  rho: {
    label: 'Load ρ',
    tip: 'How busy this team is against its capacity: work in progress divided by what it can handle at once. At 1.0 the queue is full and delay grows sharply; above 1 it never catches up on its own.',
  },
  wip: {
    label: 'WIP',
    tip: 'Work in progress — items started but not finished. The single strongest lever on flow: less started means more finished.',
  },
  drum: {
    label: 'Drum',
    tip: 'The busiest team — the constraint the whole plan staggers around. Protecting its time protects every date.',
  },
  buffer: {
    label: 'Buffer',
    tip: 'Protective time added after the scheduled work, so a slip eats the buffer instead of the promise. A flat 25% of the chain by default.',
  },
  commit: {
    label: 'Commit week',
    tip: 'The week you can promise: scheduled finish plus buffer. Promising the raw finish date leaves nothing for reality.',
  },
  target: {
    label: 'Target date',
    tip: 'The date the business wants this done by. The scheduler treats it as fixed input; it never moves it for you.',
  },
  'weighted-late': {
    label: 'Weighted weeks late',
    tip: 'The plan\u2019s total lateness, where a week late on an important initiative counts more. The engine picks the order that minimises this number.',
  },
  verdict: {
    label: 'Verdict',
    tip: 'What the order says about the date: on time; late by N weeks; cannot fit (no ordering meets it — only less scope, more capacity or a later date helps); or no date set.',
  },
  binds: {
    label: 'Binds',
    tip: 'What held this start back — dependency, team capacity, the org limit on started work, or a freeze.',
  },
  'binding-constraint': {
    label: 'Binding constraint',
    tip: 'The one thing that set this start week. Named per initiative so "why week 14?" always has an answer.',
  },
  'wip-model': {
    label: 'Work-in-progress model',
    tip: 'Which initiatives count against the org-wide limit on started work: every one, only those touching the constraint, or none. A belief about multitasking the scheduler cannot settle for you.',
  },
  'full-kit': {
    label: 'Full-kit gate',
    tip: 'A readiness bar: work may not start until the prerequisites are this complete. Prevents starting what cannot finish.',
  },
  baseline: {
    label: 'Baseline',
    tip: 'The agreed order, frozen with the exact inputs that produced it — so "why did it say week 4?" is answerable months later, and drift is measured, not remembered.',
  },
  diverged: {
    label: 'Diverged',
    tip: 'The plan\u2019s inputs have changed since this baseline was saved, so its schedule no longer describes the plan. Compare to see what moved.',
  },
  'atc': {
    label: 'ATC rank',
    tip: 'Appetite-to-cost ranking: how much value per unit of the constraint\u2019s time. Cheap-to-the-drum, high-value work goes first.',
  },
  chain: {
    label: 'Chain',
    tip: 'The longest path of team-work this initiative needs, dependencies included — the part whose slip moves the date.',
  },
  carryover: {
    label: 'Carryover',
    tip: 'Work already in flight when the period starts. It cannot be un-started; the schedule accounts for its remaining weeks.',
  },
  streams: {
    label: 'Streams',
    tip: 'Parallel work-lanes a team runs — pairs count as one. Real capacity, not headcount.',
  },
  kingman: {
    label: 'Wait multiplier',
    tip: 'How much time-in-queue multiplies as utilisation rises (Kingman\u2019s formula): near full it explodes — at 90% load, work waits ~9\u00d7 its touch time. The reason "just one more thing" is so expensive.',
  },
  freeze: {
    label: 'Freeze window',
    tip: 'A period when starts or completions are blocked — a change freeze or a site holiday. The schedule routes around it and names it as the reason.',
  },
};

// term(id) renders the affordance for a glossary entry: the visible label is
// optional (use it beside a bare column header; omit when the term itself is
// already on screen — then only the ? button appears).
export function term(id, label) {
  const t = TERMS[id];
  if (!t) return '';
  const text = String(t.tip).replace(/"/g, '&quot;');
  return `${label ? `${esc(label)} ` : ''}` +
    `<button type="button" class="help term-tip" data-tip="${text}" ` +
    `aria-label="What does ${esc(t.label)} mean?" title="${text}">?</button>`;
}

// esc matches order.js's — duplicated here on purpose so terms.js has no
// import cycle with the view modules that import it.
const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
