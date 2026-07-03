import { constraintScores } from './sim.js';
import { isStaff, authMode } from './auth.js';
import { apiGet } from './data.js';

// Players get a rules-only guide (how the game is played + what it's about), with
// no strategy. The full leader/analytics guide is admin-only.
const isPlayer = () => authMode() === 'auth' && !isStaff();

function renderGameGuide() {
  const title = document.getElementById('guide-title');
  if (title) title.textContent = 'How the Flow Game works';
  document.getElementById('guide-personas').innerHTML = '';
  document.getElementById('guide-body').innerHTML = `
    <p class="guide-intro">A turn-based game where every team steers the <b>same</b> starting
      organisation — a set of pods (teams) and the dependencies between them — over several rounds.
      One round = one quarter. Everyone begins from the identical position; what differs is the
      choices you make.</p>

    <h3>The goal</h3>
    <p>Leave the organisation healthier and higher-performing than you found it. You are judged across
      several dimensions at once — there is no single number to chase, and the dimensions trade off
      against each other.</p>

    <h3>How a round works</h3>
    <ol class="guide-path">
      <li>You get an <b>Action Point (AP)</b> budget for the round (shown at the top of the board).</li>
      <li>To make a move, pick its target — a pod or a dependency — in a <b>lever</b> card and click
        <b>apply</b>. It spends its AP cost <b>immediately and is locked in for the round — there is no
        undo</b>. Applied moves show as chips. Keep applying until your AP runs out (or stop early).</li>
      <li>Costs vary: most levers cost 1 AP, a couple cost more, a couple are free, and some can be used
        only once in the whole game. Each card's <b>?</b> shows its cost and what it does.</li>
      <li>Click <b>Resolve round ▶</b> to play out your moves and see what happened. The next round
        starts with a <b>fresh AP budget — unused AP does not carry over</b>.</li>
      <li>After the final round, an <b>Epilogue</b> plays out: the org runs on its own for a year
        (it counts toward your final score).</li>
    </ol>

    <h3>Your levers</h3>
    <p>Your toolkit is the set of lever cards (Freeze, WIP cap, Hygiene sprint, Interface invest,
      Interrupt policy, Reassign scope, Descope to MVP, Full-kit gate, Backfill hire, Innovation bet,
      Commit a date). Each card has a <b>?</b> explaining what that move does and its AP cost. Which
      moves to make, and when, is up to you.</p>

    <h3>Curve balls — and how to hedge</h3>
    <p>Between rounds an event hits the org. You can't prevent it — only be ready. Expect things like:</p>
    <ul class="guide-path">
      <li>A Tier-1 <b>"whale" feature</b> drops a pile of work on your busiest pods with a hard date.</li>
      <li>A <b>production incident</b> whose damage scales with how much <i>unready</i> work you've shipped.</li>
      <li>A <b>security mandate</b> or an exec <b>"show visible progress"</b> demand that piles work onto
        every pod (and the optics demand trims readiness).</li>
    </ul>
    <p>The hedge isn't prediction — it's a <b>healthy, low-WIP, production-ready</b> org that can absorb a
      shock (the incident especially punishes shipping unready work). Every team faces the same events in
      the same order, so it's a level test.</p>

    <h3>Good to know</h3>
    <ul class="guide-path">
      <li><b>You learn by playing.</b> Exact effect sizes aren't shown and there's no undo — make your read,
        commit, then read the end-of-round story: it tells you what reinforced and what traded off.</li>
      <li><b>Move order rarely matters.</b> The round resolves on the end state, and bonuses come from
        <i>combinations</i> of moves, not their sequence.</li>
      <li><b>Committing a date is upside and risk.</b> Keep it and Trust rises; miss it and Trust falls.
        A pledge is checked when the next round resolves — and some curve balls commit you whether you like it or not.</li>
      <li><b>No single lever wins.</b> You're scored on several goals at once, weighted toward ROI and the
        year-2 epilogue — balancing them beats spiking one number in one round.</li>
    </ul>

    <h3>How you're judged</h3>
    <p>Six things at once — and they deliberately pull against each other, so no single lever wins.
      Roughly half the score is "real value now" plus "is what you left behind still healthy a year later."</p>
    <ul class="guide-path">
      <li><b>ROI</b> — value delivered minus the cost of delivering it. The costs that bite are the hidden
        ones: time lost to cross-team handoffs (worst across dead-zone timezones) and the way wait-time
        explodes once a team is overloaded. <i>The Goal: throughput of value at minimum operating expense —
        looking busy while queues eat the gains is the trap.</i></li>
      <li><b>Trust</b> — you can pledge a date (Commit a date), but you only keep it if that team isn't
        drowning when it comes due. A promise on top of an overloaded team is a broken one. <i>Rules of
        Flow: a kept promise compounds; a missed one turns every future ask into an escalation.</i></li>
      <li><b>Delight</b> — not "did you ship" but "did you ship something <i>ready</i>" (monitored,
        handed off). Half-baked work scores low and comes back as incidents. <i>Phoenix/Unicorn:
        readiness is how the people downstream feel quality.</i></li>
      <li><b>Team health</b> — overload, after-hours coordination, and stop-start thrash grind morale down;
        push too hard too long and a team burns out, costing capacity and knowledge. <i>Unicorn Project:
        joy and flow are the same problem — sustainable pace is what keeps delivering.</i></li>
      <li><b>Innovation</b> — system-level bets compound; quick local wins harden into debt. <i>The Goal's
        local-optimum trap: optimising one corner can starve the whole.</i></li>
      <li><b>Epilogue (year 2)</b> — when you stop, the org runs a year on autopilot from exactly the
        state you left it in. Low-WIP, rested, low-maintenance, shipping-ready → it keeps compounding;
        overloaded and debt-laden → it decays, however good your last scoreboard looked. <i>You're judged
        by the system you leave behind, not the snapshot on your last day.</i></li>
    </ul>

    <h3>What it takes to win — the ideas behind the game</h3>
    <p>The game rewards running the organisation the way these books argue real ones should be run.
      Winning is less about doing more and more about making the system <i>flow</i>.</p>
    <ul class="guide-path">
      <li><b>Manage the system, not the parts.</b> An hour lost at the bottleneck is lost for everyone;
        an hour saved anywhere else is a mirage. Find what constrains the whole and work there.
        — <i>The Goal</i> (Goldratt)</li>
      <li><b>Start less, finish more.</b> Piling on work-in-progress doesn't finish things faster — it
        makes everything wait. Flow time = WIP ÷ throughput. — <i>Goldratt's Rules of Flow</i> · Little's law</li>
      <li><b>Beware local optima.</b> A team that looks busy and "efficient" can still be starving the
        whole. Optimise the end-to-end flow of value, not each corner. — <i>The Goal</i></li>
      <li><b>Cut coupling and handoffs.</b> Every cross-team dependency — especially across timezones —
        adds delay. Clean interfaces and well-placed work beat constant coordination.
        — Conway's law · Team Topologies · <i>The Unicorn Project</i> (Locality)</li>
      <li><b>Protect planned work from the unplanned.</b> Firefighting and interruptions are the most
        expensive work there is; shield teams from them. — <i>The Phoenix Project</i> (Gene Kim)</li>
      <li><b>Full kit before you start.</b> Beginning before the prerequisites are ready guarantees
        stop-start delay. — <i>Goldratt's Rules of Flow</i></li>
      <li><b>Sustainable pace wins the long game.</b> Burned-out teams slow down and break; healthy teams
        flow — joy and flow are the same problem. — <i>The Unicorn Project</i></li>
      <li><b>Promises and readiness compound.</b> Dates you keep build trust; work that's genuinely
        production-ready creates delight — and both still pay off in the year-2 epilogue after you stop
        steering. — <i>Rules of Flow</i> · <i>Phoenix / Unicorn</i></li>
    </ul>
    <p class="hint">There's no single winning move and no formula to game — the score blends goals that
      pull against each other. The teams that do best treat it as a system to balance over time, not a
      number to spike in one round.</p>

    <h3>Reading the board</h3>
    <p>The <b>Pods</b> view lists each team's current state; the <b>Network</b> view is your dependency
      map — drag nodes to untangle it, click a team to inspect it. The picture shifts as your moves take
      effect. If a team is pushed too hard for too long its people can burn out — you'll see an
      <b>attrition</b> flag on it.</p>

    <h3>It's a model, not your real org</h3>
    <p>The starting board is a stylized model derived from Jira — not your live numbers. Loads are shown
      on a <b>relative</b> scale (who's hotter than whom, not exact WIP), the dependency map shows only the
      links teams actually recorded (real coupling is likely higher), and stats like interrupts and morale
      start from sensible defaults. When the model disagrees with your gut — "it says I'm the constraint;
      I'd say it's them" — that gap is the point: argue it out. Play the <i>principles</i>, not the figures.</p>

    <p class="hint">You play your own private copy of the game — you can't see or affect another team's
      board, and the rules run on the server.</p>`;
}


// Live, persona-targeted insights: Observation -> Action -> Why (book principle).

const BOOKS = {
  goal: '<i>The Goal</i> (Goldratt)',
  phoenix: '<i>The Phoenix Project</i> (Kim)',
  unicorn: '<i>The Unicorn Project</i> (Kim)',
  flow: "<i>Goldratt's Rules of Flow</i>",
};

function computeInsights(state, wipSummary) {
  const real = Object.fromEntries(
    Object.entries(state.stats).filter(([n]) => state.pods.some((p) => p.name === n)),
  );
  const ranked = constraintScores(real, state.edges);
  const top = ranked[0];
  const topPod = state.pods.find((p) => p.name === top.pod);
  const downstream = state.edges.filter((e) => e.from === top.pod)
    .sort((a, b) => b.count - a.count).map((e) => e.to);

  const zeroOverlap = state.edges
    .filter((e) => (state.overlap[e.from]?.[e.to] ?? 0) <= 0 && e.count >= 2)
    .sort((a, b) => b.count - a.count);

  const freezable = { length: wipSummary.freezable ?? 0 };
  const totalWip = wipSummary.total ?? 0;

  const noData = state.pods.filter((p) => state.stats[p.name].synthetic).map((p) => p.name);
  const highVar = state.pods
    .filter((p) => !state.stats[p.name].synthetic && state.stats[p.name].sigma > 1.2)
    .map((p) => p.name);
  const srePods = state.pods.filter((p) => p.sre).map((p) => p.name);

  const ins = [];
  ins.push({
    who: ['exec', 'lead'],
    obs: `<b>${top.pod}</b> (${topPod.location}, ${topPod.devCount} devs) is the system constraint: `
      + `queue factor ×${top.queueFactor.toFixed(1)}, blocking ${top.dependents} pods`
      + `${downstream.length ? ` (${downstream.slice(0, 4).join(', ')})` : ''}.`,
    act: `This quarter, judge ${top.pod} by flow, not output: cap its WIP, route every new ask through one `
      + `intake, and have dependent pods batch requests to its cadence. Do NOT add headcount yet.`,
    why: `${BOOKS.goal}: "An hour lost at the bottleneck is an hour lost for the entire system" — and an hour `
      + `saved anywhere else is a mirage. Exploit and subordinate before you elevate; capacity added to an `
      + `unmanaged constraint is absorbed by the same chaos that created the queue.`,
  });
  ins.push({
    who: ['exec'],
    obs: `${freezable.length} of ${totalWip} in-progress items org-wide are stale or unassigned with nothing `
      + `depending on them (Flow Actions → click any pod's bar to see its list).`,
    act: `Mandate a one-week WIP triage: each pod lead sorts their red list into finish / freeze / kill. `
      + `Track the count down. Expect resistance — freezing feels like failure; reframe it as admitting reality.`,
    why: `${BOOKS.flow}: bad multitasking is the #1 destroyer of flow in multi-project organisations. `
      + `Little's law makes it mechanical: flow time = WIP ÷ throughput, so the only free speedup you own `
      + `as a leader is the numerator.`,
  });
  if (zeroOverlap.length) {
    ins.push({
      who: ['exec'],
      obs: `Dependency pairs with ZERO working-hour overlap: `
        + zeroOverlap.slice(0, 3).map((e) => `${e.from}→${e.to} ×${e.count}`).join(', ')
        + '. Every clarification on these edges costs a full day.',
      act: `These edges are reorganisation candidates: move the coupled work into one timezone (inverse `
        + `Conway manoeuvre), or invest in making the interface self-service so the conversation disappears.`,
      why: `${BOOKS.unicorn}: the First Ideal is Locality & Simplicity — a team should deliver value without `
        + `coordinating across the planet. Architecture and org structure mirror each other; fix whichever is cheaper.`,
    });
  }
  if (srePods.length) {
    ins.push({
      who: ['exec', 'pm'],
      obs: `SRE pods (${srePods.join(', ')}) have almost no presence in Jira — operational `
        + `work lives in separate SRE projects, invisible to feature planning.`,
      act: `Make production-readiness a first-class task in every epic (the import button will then forecast it). `
        + `Review the fever chart weekly; epics that go yellow get help, not blame.`,
      why: `${BOOKS.phoenix}: unplanned work is the most expensive kind — and ops work that surfaces after GA `
        + `is always unplanned. The Second Way is amplifying feedback; SRE involved at design time is that loop.`,
    });
  }
  if (highVar.length) {
    ins.push({
      who: ['lead'],
      obs: `High-variability pods (cycle-time σ > 1.2): ${highVar.slice(0, 6).join(', ')}. Their forecasts are `
        + `wide because task sizes are wildly uneven.`,
      act: `Slice work smaller and more uniformly; separate interrupt work from planned work explicitly `
        + `(two lanes). Variability — not just load — drives the queue.`,
      why: `${BOOKS.goal} / Kingman: wait time scales with utilisation AND variability. Halving variability `
        + `buys the same queue relief as a capacity increase, at zero cost. ${BOOKS.flow} calls this dosage: `
        + `feed work in consistent, completable units.`,
    });
  }
  if (noData.length) {
    ins.push({
      who: ['lead', 'pm'],
      obs: `No Jira flow data for: ${noData.join(', ')} — their forecasts run on synthetic defaults `
        + `(amber "no data" badges).`,
      act: `Either bring their work into the tracked project(s) or extend the miner to their projects. `
        + `Until then, treat any forecast touching them as low-confidence.`,
      why: `${BOOKS.unicorn}: the Third Ideal, improvement of daily work — you cannot improve a flow you `
        + `cannot see. Telemetry first, then decisions.`,
    });
  }
  ins.push({
    who: ['pm'],
    obs: `The simulator's suggested-dependency panel cross-checks every feature plan against 12 months of `
      + `actual blocking history (e.g. it flags SRE work that plans habitually omit).`,
    act: `Before committing any roadmap date: import the epic, accept/reject each suggested dependency, then `
      + `commit the P85 — never the P50 — and say "85% confident" out loud when you do.`,
    why: `${BOOKS.flow}: full-kit — starting without everything you need guarantees stop-start delay. `
      + `Single-date commitments hide risk; percentile commitments price it honestly.`,
  });
  return ins;
}

const PERSONAS = {
  exec: {
    label: 'Executive / VP',
    intro: 'You manage the system, not the tasks. Your levers: where attention goes, what gets frozen, '
      + 'how teams are shaped, and what "done" means.',
    path: [
      ['Flow Actions', 'One screen: the constraint, the freeze list, and at-risk epics. If you read nothing else, read the #1 card weekly and watch whether it MOVES — a constraint that never moves means the org is not acting on it.'],
      ['Network', 'The shape of your org as it actually behaves. Left = upstream platform, right = consumers. Look for loud halos (constraints) and edges that cross oceans.'],
      ['Flow Scoreboard', 'Sort by Dependents for "who is everyone waiting on", by Queue ρ for "who is drowning". The dangerous cell is both: <i>hub under load</i>.'],
      ['Feature Simulator', 'When a team asks for headcount or a date slips: import the epic and test the claim. The tornado chart shows where a week of help actually buys calendar time.'],
    ],
  },
  lead: {
    label: 'Engineering Lead',
    intro: 'You own a queue. Your levers: what enters it, what order it drains, and what you refuse to start.',
    path: [
      ['Flow Scoreboard', 'Find your pod. Queue ρ ≥ 0.85 means your promises are already late — Kingman makes the wait explode before the work does.'],
      ['Flow Actions → your freeze bar', 'Click it. The red rows are your triage list: finish, freeze, or kill. The "keep" rows are sacred — others wait on them; finish those first.'],
      ['Network → click your pod', 'Your real upstream/downstream contracts. Anything you are blocked on with low overlap hours: switch from ad-hoc questions to batched, written, full-kit requests.'],
      ['Feature Simulator', 'Sketch your next quarter as tasks and see whose queues you will sit in. Negotiate sequence with those pods NOW, not at handover time.'],
    ],
  },
  pm: {
    label: 'PM / Delivery',
    intro: 'You sell dates. Your levers: scope, sequence, and which promises you make.',
    path: [
      ['Feature Simulator', 'Import your epic. Accept/reject the suggested dependencies (they come from real blocking history). Commit the P85.'],
      ['Flow Actions → fever chart', 'Your in-flight epics as dots. Green: say nothing. Yellow: descope or unblock. Red: the buffer is gone — renegotiate the date this week, not at the end.'],
      ['Network', 'Before planning, check the pods on your critical path. A plan through two red halos is a plan to slip.'],
      ['Flow Scoreboard', 'Cycle P85 per pod is your unit of planning. A "small ask" to a pod with P85=70d is not small.'],
    ],
  },
};

export function initGuide(state) {
  let wipSummary = {};
  apiGet('wip-summary').then((d) => { wipSummary = d || {}; });

  const overlay = document.getElementById('guide-overlay');
  document.getElementById('guide-btn').addEventListener('click', () => {
    overlay.hidden = false;
    if (isPlayer()) {
      // players get the full briefing (the single canonical "how to play")
      const t = document.getElementById('guide-title'); if (t) t.textContent = 'How to play';
      document.getElementById('guide-personas').innerHTML = '';
      document.getElementById('guide-body').innerHTML =
        '<iframe class="briefing-frame" src="briefing.html" title="How to play"></iframe>';
      return;
    }
    const title = document.getElementById('guide-title');
    if (title) title.textContent = 'How to use this — pick your seat';
    renderGuide('exec');
  });
  overlay.addEventListener('click', (ev) => { if (ev.target === overlay) overlay.hidden = true; });
  document.getElementById('guide-close').addEventListener('click', () => { overlay.hidden = true; });

  function renderGuide(who) {
    const p = PERSONAS[who];
    document.getElementById('guide-personas').innerHTML = Object.entries(PERSONAS)
      .map(([k, v]) => `<button class="tab ${k === who ? 'active' : ''}" data-p="${k}">${v.label}</button>`).join('');
    document.querySelectorAll('#guide-personas button')
      .forEach((b) => b.addEventListener('click', () => renderGuide(b.dataset.p)));

    const insights = computeInsights(state, wipSummary).filter((i) => i.who.includes(who));
    document.getElementById('guide-body').innerHTML = `
      <div class="insight"><b>Three lenses.</b> <b>Observe</b> — current state from Jira (this app's analytics).
        <b>Plan</b> — upload a period's roster + initiatives to get a directed dependency network, per-pod
        utilization (ρ), and what-if levers shown before → after (manager-owned). <b>Train</b> — the learning
        game. ρ is the primary signal; lead time is directional, not a date.</div>
      <div class="insight"><b>Snapshots &amp; scenarios.</b> Managers can <b>Import from Jira</b> to capture a
        dated org <b>snapshot</b>, <b>compare</b> snapshots over time, and <b>publish</b> one for facilitators.
        Facilitators seed a game from a public snapshot or an editable <b>scenario template</b> — download the
        network as JSON, edit it (pods = teams, edges, loads), re-upload, name it, optionally share. See
        docs/snapshots-and-scenarios.md.</div>
      <p class="guide-intro">${p.intro}</p>
      <h3>Where to look, in order</h3>
      <ol class="guide-path">${p.path.map(([t, d]) => `<li><b>${t}</b> — ${d}</li>`).join('')}</ol>
      <h3>Today's insights from your data</h3>
      ${insights.map((i) => `
        <div class="insight">
          <div class="i-obs">${i.obs}</div>
          <div class="i-act"><b>Do:</b> ${i.act}</div>
          <div class="i-why"><b>Why:</b> ${i.why}</div>
        </div>`).join('')}
      <p class="hint">Numbers update with every data refresh — these are computed live, not canned.</p>`;
  }
}
