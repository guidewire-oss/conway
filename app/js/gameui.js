import { openModal, closeModal } from './modal.js';
// Thin client for the Flow Game. The rules live ONLY on the server (Go engine);
// this file sends moves and renders the sanitized view the server returns, so
// players can't read the mechanics from source. Requires the Conway server
// with a working world (Postgres reachable, baseline seeded or a snapshot
// imported) — the tab explains that if serverGame comes back false.
import { authFetch, hasRole, authGameID } from './auth.js';

// a joined team reads its own game's session; everyone else the default game
async function fetchConfig() {
  if (authGameID()) {
    try { const r = await authFetch('/api/game/config'); if (r.ok) return await r.json(); } catch { /* fall through */ }
  }
  try { return await (await fetch('/api/config')).json(); } catch { return {}; }
}
import { renderGameNetwork, GAMENET_LEGEND } from './gamenet.js';

const hlp = (t) => ` <span class="help" data-tip="${t.replace(/"/g, '&quot;')}">?</span>`;
// plain-language only (no scoring formulas — the rules live on the server)
const POD_TIPS = {
  Pod: 'A team you steer. Conway\'s law: the org ships its communication structure — these pods and the lines between them are that structure.',
  Site: 'Where the team sits. Work that crosses sites with little overlapping work-day is the slow, expensive seam (Unicorn Project: Locality).',
  WIP: 'How much the team has open at once. The Goal & Rules of Flow: piling on more does not finish more — it just makes everything wait longer.',
  'Load ρ': 'How full the plate is (a full motorway, not a fast one). Near and above 1, delay grows fast — the queue, not the people, is the problem.',
  Morale: 'How the people are holding up. Slow to earn, quick to lose; tired teams slow down and slip. Unicorn Project: joy and flow are the same problem.',
  Interrupt: 'The standing tax of firefighting and answering interruptions — paid before any planned work gets done. Phoenix Project: unplanned work is the most expensive kind.',
  KTLO: '"Keep the lights on" — recurring maintenance that travels with scope and never disappears; only moves, gets automated, or, neglected, grows into incidents.',
  Readiness: 'How production-ready shipped work is (monitoring, hand-off, the unglamorous parts). Phoenix/Unicorn: low readiness feels free at launch and pages you for it later.',
  Hygiene: 'How well-tracked and clean the team\'s work is. You can\'t steer flow you can\'t see.',
};
const SCORE_TIPS = {
  ROI: 'Value you delivered against the total cost of delivering it. The Goal: throughput of value at minimum operating cost.',
  Trust: 'Did you keep the dates you promised? Rules of Flow: a kept promise compounds; a missed one turns future asks into escalations.',
  Delight: 'Did the people who depend on you — inside and out — get something solid? Phoenix/Unicorn: readiness and few incidents are how customers feel quality.',
  'Team health': 'Are your people sustainable, or running on fumes? Unicorn Project\'s ideals: psychological safety and focus are not soft — they drive flow.',
  Innovation: 'Did you move the whole system forward holistically, rather than patching local corners into tomorrow\'s tech debt? (The Goal\'s local-optimum trap.)',
  'Epilogue (yr 2)': 'After you stop, the org runs on its own for a year. What you leave behind is part of your score — short-term cleverness that leaves a fragile org does not survive it.',
};
// plain-language "what it is / what it could do" — intent and upside only, no
// penalties or scoring math (the rules live on the server).
const LEVER_TIPS = {
  freeze: 'Park some of a team\'s in-progress items so its plate clears and work flows again instead of everything inching forward at once. Much of a team\'s WIP is usually waiting (in review, on hold, blocked) rather than being actively worked — those are the cheapest to park. (The Goal & Rules of Flow: stop starting, start finishing.)',
  wipCap: 'Set a ceiling on how much a team runs at once, so fresh work waits in a tidy line instead of piling onto the floor — load stays in the healthy zone and the queue stays short. (Little\'s law / Kanban.)',
  hygieneSprint: 'A focused tidy-up of the team\'s tracking: size the unsized, close the stale, link dependencies, write the missing outcomes. Makes the team\'s flow visible and trustworthy. (You can\'t steer flow you can\'t see.)',
  interfaceInvest: 'Build a clean, self-serve interface on a dependency so the downstream team stops queuing on the upstream team for every request — the handoff becomes a paved road. (Team Topologies: reduce coupling with good interfaces.)',
  interruptPolicy: 'Choose how a team absorbs interruptions — a shared pool, office hours, follow-the-sun, or a dedicated responder — so firefighting stops leaking into planned work. (Phoenix Project: protect planned work.)',
  reassignScope: 'Move a slice of one team\'s work to another — relieve an overloaded team, or pull a dependency in-house so it stops crossing a slow timezone seam.',
  descopeMvp: 'Trim a piece of work to its essential minimum so value ships sooner. (Goldratt: deliver the kit that matters, defer the rest.)',
  fullKitGate: 'Require work to have its prerequisites ready before it starts, org-wide, so teams stop launching half-ready work that stalls mid-flight. (Rules of Flow: full kit.)',
  hire: 'Add capacity to a team to lift a genuine headcount constraint. (Elevate the constraint — after you\'ve exploited and subordinated it.)',
  innovate: 'Invest in a system-level improvement — a holistic bet that moves the whole org forward, or a quick local win. (The Goal: optimise the whole, not a corner.)',
  commit: 'Publicly commit a team to a delivery date next round — a promise that builds trust when you keep it. (Rules of Flow: reliable promises compound.)',
};

let view = null;          // latest sanitized gameView from the server
let lastReport = null;    // last resolve report
let lastScenario = null;  // { title, text }
let serverGame = false;   // does the backend expose the game?
let session = { open: false, rounds: 4, ap: 5, openRound: 0, deadline: 0, timerSecs: 300 }; // from /api/config
let halted = null; // null | 'closed' | 'reset' — facilitator stopped play under us
let poll = null;   // background poll: detects round opens / auto-submits + ticks the timer
function setSession(cfg) {
  session = {
    open: !!cfg.gameOpen, rounds: cfg.rounds ?? 4, ap: cfg.ap ?? 5,
    openRound: cfg.openRound ?? 0, deadline: cfg.deadline ?? 0, timerSecs: cfg.timerSecs ?? 300,
  };
}

async function api(path, opts = {}) {
  try {
    const r = await authFetch(path, opts);
    if (!r.ok) return { _err: r.status };
    return await r.json();
  } catch { return { _err: 'network' }; }
}

export async function initGameUI() {
  try {
    const base = await (await fetch('/api/config')).json();
    serverGame = !!base.serverGame;
    setSession(authGameID() ? await fetchConfig() : base);
  } catch { serverGame = false; }

  document.getElementById('game-new').addEventListener('click', startGame);
  document.querySelector('button[data-view=game]').addEventListener('click', refresh);
  document.getElementById('pods-view-net')?.addEventListener('click', () => setPodsView('net'));
  document.getElementById('pods-view-table')?.addEventListener('click', () => setPodsView('table'));

  if (serverGame && !noInvite()) {
    const g = await api('/api/game');
    if (g && !g._err && g.round) view = g;
    startPoll();
  }
  render();
}

async function refresh() { render(); }

async function startGame() {
  const r = await api('/api/game/new', { method: 'POST', body: JSON.stringify({}) });
  if (r._err) {
    setErr(r._err === 409 ? 'The facilitator hasn\'t opened the round yet.' : 'Could not start — is the server running and are you signed in?');
    return;
  }
  view = r; lastReport = null; lastScenario = null; halted = null;
  render();
}

// Reconcile against the facilitator stopping play: a reset clears our game on the
// server (GET /api/game → {game:null}); a close flips the session to not-open. In
// either case we surface a loud modal instead of leaving a silently dead board.
// true for a system admin, or a facilitator's short-lived "test this game"
// session (minted by the 🧪 test button in Run games — see auth.js testtoken).
function canTestFreely() { return hasRole('admin') || hasRole('tester'); }

// true for a signed-in staff account (facilitator/manager) that hasn't joined
// any specific game — they should wait to be invited, not fall through to the
// legacy shared default game.
function noInvite() { return !canTestFreely() && !authGameID() && (hasRole('facilitator') || hasRole('manager')); }

function reconcileHalt(g) {
  if (canTestFreely()) return; // facilitators test their own game freely
  if (view && !view.over && g && !g._err && !g.round) { handleHalt('reset'); return; }
  if (view && !view.over && !session.open) { handleHalt('closed'); return; }
  if (halted === 'closed' && session.open) { halted = null; } // facilitator re-opened
}

function handleHalt(kind) {
  if (halted === kind) return;
  halted = kind;
  if (kind === 'reset') { view = null; lastReport = null; lastScenario = null; }
  render();
  showHaltModal(kind);
}

// Background poll: keeps the team in sync with the admin (round opens, timer
// auto-submits) and ticks the countdown. One 1s tick; data refresh every 4s.
let pollN = 0;
let lastSig = '';
// changes only when the round/round-open/over/open state changes — used so the
// 4s poll re-renders on a real transition but not mid-edit (which would wipe a
// half-composed move).
function sig() { return `${view ? view.round : '-'}|${view ? view.over : '-'}|${session.openRound}|${session.open}`; }
function startPoll() {
  if (poll) return;
  lastSig = sig();
  poll = setInterval(async () => {
    pollN++;
    if (pollN % 4 === 0) { // every ~4s: re-sync session + game
      try { setSession(await fetchConfig()); } catch { /* keep last */ }
      let g = null;
      if (serverGame) {
        g = await api('/api/game');
        if (g && !g._err && g.round) view = g;
      }
      reconcileHalt(g); // facilitator closed/reset under us → loud modal
      if (sig() !== lastSig) render(); // round opened / auto-submitted / state changed
      else tickTimer();
    } else {
      tickTimer(); // cheap: just update the countdown text
    }
  }, 1000);
}

function fmtCountdown() {
  if (!session.deadline) return '';
  const left = Math.max(0, session.deadline - Math.floor(Date.now() / 1000));
  const m = Math.floor(left / 60), s = left % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}
function tickTimer() {
  const el = document.getElementById('game-timer');
  if (el) el.textContent = fmtCountdown();
}

// true when this team may stage/submit right now (its round is the open one).
function canStage() {
  if (!view || view.over) return false;
  if (canTestFreely()) return true;
  return session.open && view.round === session.openRound;
}

function renderSetup() {
  const msg = document.getElementById('game-setup-msg');
  const btn = document.getElementById('game-new');
  if (!msg || !btn) return;
  startPoll();
  if (canTestFreely()) {
    msg.innerHTML = '<h2>Facilitator test</h2><p class="hint">Open rounds and set the timer in the ⚙ Admin panel. You can Begin here to test the game yourself.</p>';
    btn.hidden = false; btn.textContent = 'Begin (test) ▶'; return;
  }
  if (session.open && session.openRound === 1) {
    msg.innerHTML = `<h2>Round 1 is open</h2>
      <p class="hint">${session.rounds} rounds · ${session.ap} Activity Points per round. Skim the ✦ Guide, then begin — you'll plan your moves and submit before the timer ends.</p>`;
    btn.hidden = false; btn.textContent = 'Begin Round 1 ▶';
  } else if (session.open && session.openRound > 1) {
    msg.innerHTML = `<h2>The game has already started (Round ${session.openRound})</h2>
      <p class="hint">Ask your facilitator — new teams join from Round 1.</p>`;
    btn.hidden = true;
  } else {
    msg.innerHTML = '<h2>Hold tight — the facilitator hasn\'t opened Round 1 yet ⏳</h2>'
      + '<p class="hint">This screen updates the moment they do — no need to refresh. Good time to skim the ✦ Guide.</p>';
    btn.hidden = true;
  }
}

function setErr(msg, ok) {
  const el = document.getElementById('game-err');
  if (el) { el.textContent = msg; el.style.color = ok ? 'var(--green)' : ''; }
}

// ---- rendering (pure view -> DOM; no rules here) ----

// the game-scoped network, embedded in the game screen alongside the pods table
// (Table | Network toggle). reflects the current view and reshapes as levers fire.
function renderGameNet() {
  const el = document.getElementById('gamenet-svg');
  if (!el || el.offsetParent === null) return; // not visible (other tab / Table view) — skip the d3 work
  renderGameNetwork(el, view, { panel: document.getElementById('gamenet-panel') });
  const lg = document.getElementById('gamenet-legend');
  if (lg) lg.innerHTML = GAMENET_LEGEND;
}

function setPodsView(mode) {
  const net = document.getElementById('game-net');
  const tbl = document.getElementById('game-pods');
  if (!net || !tbl) return;
  const showNet = mode === 'net';
  net.hidden = !showNet; tbl.hidden = showNet;
  document.getElementById('pods-view-net')?.classList.toggle('active', showNet);
  document.getElementById('pods-view-table')?.classList.toggle('active', !showNet);
  if (showNet) renderGameNet(); // re-measure now the svg is visible
}

function render() {
  lastSig = sig(); // keep the poll's change-detector in sync with what we last drew
  const setup = document.getElementById('game-setup');
  const board = document.getElementById('game-board');
  const notice = document.getElementById('game-notice');

  if (!serverGame) {
    setup.hidden = true; board.hidden = true;
    notice.hidden = false;
    notice.innerHTML = '<div class="panel-card"><h2>The Flow Game needs a working world</h2>'
      + '<p class="hint">The server is up, but no org data is loaded yet — check that Postgres is reachable and '
      + 'CONWAY_SEED_BASELINE has seeded the demo org, or import your own from Jira.</p></div>';
    return;
  }
  notice.hidden = true;

  // staff signed in without a game invite (no join code, not testing) never
  // see a live board — even if the legacy shared default game is open.
  if (noInvite()) {
    setup.hidden = false; board.hidden = true;
    const msg = document.getElementById('game-setup-msg');
    const btn = document.getElementById('game-new');
    if (msg) {
      msg.innerHTML = '<h2>Waiting for an invite ⏳</h2>'
        + '<p class="hint">You haven\'t been invited to a game yet. Ask a facilitator for a join link — '
        + 'or open 🎮 Run games to create and test your own.</p>';
    }
    if (btn) btn.hidden = true;
    return;
  }

  if (!view) { setup.hidden = false; board.hidden = true; renderSetup(); return; }
  setup.hidden = true; board.hidden = false;

  const done = view.over;
  const closed = !done && !session.open && !canTestFreely();
  const staging = canStage();
  const timer = (staging && session.deadline) ? ` · ⏱ <span id="game-timer">${fmtCountdown()}</span>` : '';
  document.getElementById('game-status').innerHTML = done
    ? `<b>Game over</b> — ${view.totalRounds} rounds played + Epilogue`
    : closed
      ? '<b>Game closed</b> — the facilitator has paused play'
      : staging
        ? `<b>Round ${view.round} / ${view.totalRounds}</b> · <span class="game-ap">${view.apLeft} Activity Points left</span>${timer}`
        : `<b>Round ${view.round - 1} submitted</b> · waiting for the facilitator to open Round ${view.round} of ${view.totalRounds}`;

  renderScore(view.score, done, view.final);
  renderPods(view.pods);
  if (done) renderEpilogue(view.final);
  else if (closed) renderClosed();
  else if (staging) renderLevers();
  else renderWaiting();
  renderReport();
  renderHistory(view.history);
  renderGameNet(); // board is visible now — renders only if the Network sub-view is showing
}

// Between rounds: the team has submitted and waits for the admin to open the next.
function renderWaiting() {
  document.getElementById('game-levers').innerHTML = `
    <div class="panel-card">
      <h3>Round ${view.round - 1} locked in ✓</h3>
      <p class="hint">Your moves are submitted and can't be changed. The facilitator will review the
        leaderboard and discuss strategies, then open <b>Round ${view.round}</b> — this screen will switch
        to it automatically. Use the wait to study your Network and plan your next moves.</p>
    </div>`;
}

// Facilitator closed the game while a round was live — replace the levers with a
// clear, non-actionable banner (the loud modal fired on the transition).
function renderClosed() {
  document.getElementById('game-levers').innerHTML = `
    <div class="panel-card halt-card">
      <h3>⛔ The facilitator has closed this game</h3>
      <p class="hint">Play is paused — no further moves can be submitted. If the facilitator
        re-opens the game, this screen switches back automatically. No need to refresh.</p>
    </div>`;
}

// A loud, unmissable interruption when the facilitator stops play under the team:
// a reset wipes the board, a close pauses it. Either way, demand acknowledgement.
function showHaltModal(kind) {
  const reset = kind === 'reset';
  let ov = document.getElementById('halt-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'halt-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
  }
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>${reset ? '🔄 This game was reset' : '⛔ The game has been closed'}</h2></div>
      <p class="story">${reset
    ? 'The facilitator reset this game — your board has been cleared and play starts over.'
    : 'The facilitator has closed the game. No further moves can be made right now.'}</p>
      <p class="hint">${reset
    ? 'When they open Round 1 again, you can begin a fresh game from this screen.'
    : 'If they re-open the game, your screen switches back automatically.'}</p>
      <div class="row-actions"><button id="halt-ok" class="primary">${reset ? 'Back to start ▶' : 'OK'}</button></div>
    </div>`;
  openModal(ov);
  ov.querySelector('#halt-ok').addEventListener('click', () => { closeModal(ov); render(); });
}

function bar(label, v, hint) {
  const col = v >= 66 ? 'var(--green)' : v >= 40 ? 'var(--amber)' : 'var(--red)';
  return `<div class="score-row"><span class="score-lbl">${label}${hlp(SCORE_TIPS[label] || '')}</span>
    <span class="score-track"><span style="width:${v}%;background:${col}"></span></span>
    <span class="score-val">${v.toFixed(0)}</span>${hint ? `<span class="hint"> ${hint}</span>` : ''}</div>`;
}

function renderScore(s, done, final) {
  s = s || {};
  document.getElementById('game-score').innerHTML = `
    <div class="score-total">Score <b>${(s.total ?? 0).toFixed(0)}</b>${done ? ' (final)' : ''}</div>
    ${bar('ROI', s.roi ?? 0, 'value − cost')}
    ${bar('Trust', s.trust ?? 0, 'committed dates hit')}
    ${bar('Delight', s.delight ?? 0, 'readiness − incidents')}
    ${bar('Team health', s.teamHealth ?? 0, 'morale − attrition')}
    ${bar('Innovation', s.innovation ?? 0, 'holistic − local debt')}
    ${done && final ? bar('Epilogue (yr 2)', final.epilogue.total, `KTLO ${(final.epilogue.ktloShare * 100).toFixed(0)}%`)
      : '<div class="hint">Epilogue (year 2 autopilot) scores at game end — 25% of final</div>'}`;
}

function renderPods(pods) {
  const rows = (pods || []).map((p) => {
    const heat = p.rho >= 1 ? 'var(--red)' : p.rho >= 0.85 ? 'var(--amber)' : 'var(--green)';
    const mcol = p.morale >= 0.7 ? 'var(--green)' : p.morale >= 0.5 ? 'var(--amber)' : 'var(--red)';
    return `<tr>
      <td>${p.name}${p.isSre ? ' <span class="hint">SRE</span>' : ''}${p.attrited ? ' <span class="flag red">attrition</span>' : ''}</td>
      <td>${p.location.replace('*REMOTE - multicontinental*', 'Remote')}${p.pairing ? '' : ' <span class="hint">solo</span>'}</td>
      <td>${p.wip}</td>
      <td style="color:${heat}">${p.rho > 3 ? '3+' : p.rho.toFixed(2)}</td>
      <td style="color:${mcol}">${(p.morale * 100).toFixed(0)}%</td>
      <td>${p.interrupt.toFixed(1)}d</td>
      <td>${p.ktlo.toFixed(1)}d</td>
      <td>${(p.readiness * 100).toFixed(0)}%</td>
      <td>${(p.hygiene * 100).toFixed(0)}%</td>
    </tr>`;
  }).join('');
  const th = (label) => `<th>${label}${hlp(POD_TIPS[label] || '')}</th>`;
  document.getElementById('game-pods').innerHTML = `
    <thead><tr>${['Pod', 'Site', 'WIP', 'Load ρ', 'Morale', 'Interrupt', 'KTLO', 'Readiness', 'Hygiene'].map(th).join('')}</tr></thead>
    <tbody>${rows}</tbody>`;
}

const podNames = () => (view.pods || []).map((p) => p.name);
const opts = (names, sel) => names.map((n) => `<option ${n === sel ? 'selected' : ''}>${n}</option>`).join('');
const apOf = (id) => (view.levers.find((l) => l.id === id) || {}).ap ?? 1;

function snapshotSelects() {
  const m = {};
  document.querySelectorAll('#game-levers select').forEach((s) => { if (s.id) m[s.id] = s.value; });
  return m;
}
function restoreSelects(m) {
  document.querySelectorAll('#game-levers select').forEach((s) => { if (s.id && m[s.id] != null) s.value = m[s.id]; });
}

function describeMove(m) {
  const who = m.pod || (m.from ? `${m.from}→${m.to}` : '');
  const extra = m.n ? ` ×${m.n}` : m.flavor ? ` (${m.flavor})` : m.model ? ` (${m.model})`
    : m.capX ? ` ${m.capX}×` : m.cutOps ? ' (cut ops)' : '';
  return `${m.lever} ${who}${extra}`.trim();
}
function movesHtml() {
  const mine = view.movesThisRound || [];
  if (!mine.length) return '<span class="hint">No moves planned yet — pick a lever and Add. You can remove a planned move until you submit.</span>';
  return `<b>Planned this round:</b> ${mine.map((m, i) =>
    `<span class="move-chip">${describeMove(m)} <a class="chip-x" data-unstage="${i}" title="remove">✕</a></span>`).join(' ')}`;
}

function renderLevers() {
  const saved = snapshotSelects();
  const pn = podNames();
  const edgeOpts = (view.edges || []).map((e, i) => `<option value="${i}">${e.from} → ${e.to} ×${e.count}</option>`).join('');
  document.getElementById('game-levers').innerHTML = `
    <h3>Levers <span class="hint">(spend up to ${view.apPerRound} Activity Points (AP) per round)</span></h3>
    <div class="lever-grid">
      <div class="lever"><b>Freeze</b>${hlp(LEVER_TIPS.freeze)} <span class="ap">${apOf('freeze')}AP</span><br>
        <select id="lv-freeze-pod">${opts(pn)}</select>
        <input id="lv-freeze-n" type="number" value="5" min="1" style="width:54px">
        <button data-do="freeze">add</button></div>
      <div class="lever"><b>WIP cap</b>${hlp(LEVER_TIPS.wipCap)} <span class="ap">${apOf('wipCap')}AP</span><br>
        <select id="lv-wip-pod">${opts(pn)}</select>
        <select id="lv-wip-x" title="WIP ceiling: tighter = more relief"><option value="0.8">tight</option><option value="1">healthy</option><option value="1.3">loose</option></select>
        <button data-do="wipCap">add</button></div>
      <div class="lever"><b>Hygiene sprint</b>${hlp(LEVER_TIPS.hygieneSprint)} <span class="ap">${apOf('hygieneSprint')}AP</span><br>
        <select id="lv-hyg-pod">${opts(pn)}</select>
        <button data-do="hygieneSprint">add</button></div>
      <div class="lever"><b>Interface invest</b>${hlp(LEVER_TIPS.interfaceInvest)} <span class="ap">${apOf('interfaceInvest')}AP</span><br>
        <select id="lv-iface">${edgeOpts}</select>
        <button data-do="interfaceInvest">add</button></div>
      <div class="lever"><b>Interrupt policy</b>${hlp(LEVER_TIPS.interruptPolicy)} <span class="ap">${apOf('interruptPolicy')}AP</span><br>
        <select id="lv-int-pod">${opts(pn)}</select>
        <select id="lv-int-model"><option value="pool">site pool</option><option value="office">office hours</option><option value="followsun">follow-sun</option><option value="dedicated">dedicated</option></select>
        <button data-do="interruptPolicy">add</button></div>
      <div class="lever"><b>Reassign scope</b>${hlp(LEVER_TIPS.reassignScope)} <span class="ap">${apOf('reassignScope')}AP</span><br>
        <select id="lv-re-from">${opts(pn)}</select>→<select id="lv-re-to">${opts(pn, pn[1])}</select>
        <select id="lv-re-frac"><option value="0.25">25%</option><option value="0.5">50%</option></select>
        <button data-do="reassignScope">add</button></div>
      <div class="lever"><b>Descope to MVP</b>${hlp(LEVER_TIPS.descopeMvp)} <span class="ap">${apOf('descopeMvp')}AP</span><br>
        <select id="lv-mvp-pod">${opts(pn)}</select>
        <label class="hint"><input id="lv-mvp-cut" type="checkbox"> cut ops</label>
        <button data-do="descopeMvp">add</button></div>
      <div class="lever"><b>Full-kit gate</b>${hlp(LEVER_TIPS.fullKitGate)} <span class="ap">${apOf('fullKitGate')}AP</span><br>
        <button data-do="fullKitGate">enable org-wide</button></div>
      <div class="lever"><b>Backfill hire</b>${hlp(LEVER_TIPS.hire)} <span class="ap">${apOf('hire')}AP, once</span><br>
        <select id="lv-hire-pod">${opts(pn)}</select>
        <button data-do="hire">place</button></div>
      <div class="lever"><b>Innovation bet</b>${hlp(LEVER_TIPS.innovate)} <span class="ap">${apOf('innovate')}AP</span><br>
        <select id="lv-inv-pod">${opts(pn)}</select>
        <select id="lv-inv-flavor"><option value="holistic">holistic</option><option value="quickwin">quick win</option></select>
        <button data-do="innovate">add</button></div>
      <div class="lever"><b>Commit a date</b>${hlp(LEVER_TIPS.commit)} <span class="ap">${apOf('commit')}AP</span><br>
        <select id="lv-cm-pod">${opts(pn)}</select>
        <span class="hint">due R${view.round + 1}</span>
        <button data-do="commit">commit</button></div>
    </div>
    <div id="game-moves" class="game-moves">${movesHtml()}</div>
    <div class="row-actions">
      <button id="game-submit" class="primary">Submit Round ${view.round} ▶</button>
      <span class="hint">Submitting locks this round — you can't change it after.</span>
    </div>
    <div id="game-err" class="hint"></div>`;
  restoreSelects(saved);
  document.querySelectorAll('#game-levers [data-do]').forEach((b) => b.addEventListener('click', () => doStage(b.dataset.do)));
  document.querySelectorAll('#game-levers [data-unstage]').forEach((b) => b.addEventListener('click', () => doUnstage(+b.dataset.unstage)));
  document.getElementById('game-submit').addEventListener('click', doSubmit);
}

function readMove(lever) {
  const v = (id) => document.getElementById(id)?.value;
  const chk = (id) => document.getElementById(id)?.checked;
  switch (lever) {
    case 'freeze': return { lever, pod: v('lv-freeze-pod'), n: +v('lv-freeze-n') };
    case 'wipCap': return { lever, pod: v('lv-wip-pod'), capX: +v('lv-wip-x') };
    case 'hygieneSprint': return { lever, pod: v('lv-hyg-pod') };
    case 'interfaceInvest': { const e = view.edges[+v('lv-iface')]; return { lever, from: e.from, to: e.to }; }
    case 'interruptPolicy': return { lever, pod: v('lv-int-pod'), model: v('lv-int-model') };
    case 'reassignScope': return { lever, from: v('lv-re-from'), to: v('lv-re-to'), frac: +v('lv-re-frac') };
    case 'descopeMvp': return { lever, pod: v('lv-mvp-pod'), cutOps: chk('lv-mvp-cut') };
    case 'hire': return { lever, pod: v('lv-hire-pod') };
    case 'innovate': return { lever, pod: v('lv-inv-pod'), flavor: v('lv-inv-flavor') };
    case 'commit': return { lever, pod: v('lv-cm-pod'), dueRound: view.round + 1 };
    default: return { lever };
  }
}

async function doStage(lever) {
  const move = readMove(lever);
  const r = await api('/api/game/stage', { method: 'POST', body: JSON.stringify(move) });
  if (r._err) { setErr('⚠ add failed (server/auth)'); return; }
  view = r.view;
  render();
  if (!r.ok) setErr(`⚠ ${r.error}`);
  else setErr(`✓ planned: ${describeMove(move)} — ${view.apLeft} Activity Points left`, true);
}

async function doUnstage(i) {
  const r = await api('/api/game/unstage', { method: 'POST', body: JSON.stringify({ index: i }) });
  if (r._err) { setErr('⚠ remove failed (server/auth)'); return; }
  view = r.view;
  render();
}

async function doSubmit() {
  if (!confirm('Submit this round? Your planned moves lock in and can\'t be changed.')) return;
  const rb = document.getElementById('game-submit');
  if (rb) { rb.disabled = true; rb.textContent = 'Submitting…'; }
  const r = await api('/api/game/submit', { method: 'POST' });
  if (r._err) { setErr(r._err === 409 ? '⚠ round not open' : '⚠ submit failed (server/auth)'); if (rb) { rb.disabled = false; } return; }
  lastReport = r.report || null;
  lastScenario = r.scenarioTitle ? { title: r.scenarioTitle, text: r.scenarioText } : null;
  view = r.view;
  render();
  showResolveModal(lastReport, lastScenario);
}

// A clear "this round is done, here's what happened, now begin the next one"
// confirmation — the visual cue between rounds.
function showResolveModal(rep, scenario) {
  if (!rep) return;
  let ov = document.getElementById('resolve-overlay');
  if (!ov) {
    ov = document.createElement('div');
    ov.id = 'resolve-overlay';
    ov.className = 'modal-overlay';
    document.body.appendChild(ov);
    // no click-outside-to-close — "Got it"/"See your epilogue" below is the
    // deliberate exit, matching the halt modal's must-acknowledge pattern.
  }
  const over = view.over;
  const dCol = rep.scoreDelta > 0 ? 'var(--green)' : rep.scoreDelta < 0 ? 'var(--red)' : 'var(--muted)';
  const beats = (rep.narrative || []).slice(0, 4)
    .map((s) => `<div class="beat beat-${s.tone}"><span class="beat-i">${TONE_ICON[s.tone] || ''}</span><span>${s.text}</span></div>`).join('');
  const title = over ? `All ${view.totalRounds} rounds played` : `Round ${rep.round} submitted ✓`;
  const sub = over
    ? 'The game is complete — see how your organisation fared.'
    : `Your moves are locked in. The facilitator will review the leaderboard and discuss, then open <b>Round ${view.round} of ${view.totalRounds}</b> — your screen will switch to it automatically.`;
  const btnLabel = over ? 'See your epilogue ▶' : 'Got it';
  ov.innerHTML = `<div class="modal-box">
      <div class="modal-head"><h2>${title}</h2>
        <span class="report-delta" style="color:${dCol}">${rep.scoreDelta > 0 ? '+' : ''}${rep.scoreDelta} → score ${view.score?.total ?? ''}</span></div>
      <p class="story">${rep.headline}</p>
      <p class="hint">${sub}</p>
      <div class="beats">${beats || '<span class="hint">A calm quarter — nothing notable shifted.</span>'}</div>
      ${scenario ? `<div class="curveball"><b>⚡ Heading into the next quarter — ${scenario.title}</b><br>${scenario.text}</div>` : ''}
      <div class="row-actions"><button id="resolve-continue" class="primary">${btnLabel}</button></div>
    </div>`;
  openModal(ov);
  ov.querySelector('#resolve-continue').addEventListener('click', () => { closeModal(ov); });
}

const TONE_ICON = { bad: '▼', warn: '⚠', good: '▲', info: '•' };

function renderReport() {
  const el = document.getElementById('game-report');
  if (!lastReport) { el.innerHTML = ''; return; }
  const r = lastReport;
  const dCol = r.scoreDelta > 0 ? 'var(--green)' : r.scoreDelta < 0 ? 'var(--red)' : 'var(--muted)';
  const beats = (r.narrative || []).map((s) =>
    `<div class="beat beat-${s.tone}"><span class="beat-i">${TONE_ICON[s.tone]}</span><span>${s.text}</span></div>`).join('');
  const watch = (r.watch || []).length
    ? `<div class="watch"><b>What to watch next round</b><ul>${r.watch.map((w) => `<li>${w}</li>`).join('')}</ul></div>` : '';
  el.innerHTML = `
    <div class="panel-card report">
      <div class="report-head"><h3>Quarter ${r.round} — ${r.headline}</h3>
        <span class="report-delta" style="color:${dCol}">score ${r.scoreDelta > 0 ? '+' : ''}${r.scoreDelta}</span></div>
      <p class="hint">Event: <b>${r.event}</b> · value delivered ${r.valueDelivered} · cost function ${r.costFn} · commitments hit ${r.commitmentsHit}</p>
      ${r.story ? `<p class="story">${r.story}</p>` : ''}
      <div class="beats">${beats || '<span class="hint">A calm quarter — nothing notable shifted.</span>'}</div>
      ${watch}
      ${lastScenario ? `<div class="curveball"><b>⚡ Curve ball — ${lastScenario.title}</b><br>${lastScenario.text}</div>` : ''}
    </div>`;
}

function renderHistory(history) {
  const el = document.getElementById('game-history');
  const hist = history || [];
  if (!hist.length) { el.innerHTML = ''; return; }
  const rows = hist.map((r) => {
    const dCol = r.scoreDelta > 0 ? 'var(--green)' : r.scoreDelta < 0 ? 'var(--red)' : 'var(--muted)';
    const top = (r.narrative || []).slice(0, 2).map((s) => `<li class="beat-${s.tone}">${s.text}</li>`).join('');
    return `<tr>
      <td class="dl-q">Q${r.round}</td>
      <td class="dl-out"><b>${r.headline.replace(/^.*— /, '')}</b><ul class="dl-beats">${top}</ul></td>
      <td class="dl-score" style="color:${dCol}">${r.scoreDelta > 0 ? '+' : ''}${r.scoreDelta}<br><span class="hint">→ ${r.score.total.toFixed(0)}</span></td>
    </tr>`;
  }).join('');
  el.innerHTML = `<div class="panel-card"><h3>Decision log <span class="hint">— how each quarter played out</span></h3>
    <table class="dl-table"><thead><tr><th>Qtr</th><th>What resulted</th><th>Score</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function renderEpilogue(final) {
  if (!final) return;
  const e = final.epilogue;
  document.getElementById('game-levers').innerHTML = `
    <div class="panel-card"><h3>Epilogue — year 2 on autopilot</h3>
      ${e.narrative ? `<p class="epilogue-letter">${e.narrative}</p>` : ''}
      <p>With no further actions, the org you built ran another year.</p>
      <p>Run-rate value <b>${e.runRateValue}</b> · KTLO share of capacity
        <b style="color:${e.ktloShare > 0.6 ? 'var(--red)' : 'var(--green)'}">${(e.ktloShare * 100).toFixed(0)}%</b>
        · avg morale <b>${(e.avgMorale * 100).toFixed(0)}%</b></p>
      <p class="hint">${e.ktloShare > 0.6
    ? 'Maintenance is crowding out delivery — the org will spend year 2 keeping the lights on.'
    : 'The org keeps shipping — you left it healthier than you found it.'}</p>
      ${canTestFreely() ? '<button id="game-again" class="primary">New game (test)</button>' : ''}</div>`;
  document.getElementById('game-again')?.addEventListener('click', startGame);
}
