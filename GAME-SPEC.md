# Conway: The Flow Game — Design Spec (v0.6, for review)

A turn-based org-strategy game played on the real org flow model. Managers
play levers against simulated reality; the engine plays back the consequences
— including the ones they can't see coming.

**Status**: design for review. Sections marked 🔒 are designer-only — never
shown to players. Numbered items (L1…, E1…, H1…) exist so you can comment
"add an L14" or "change L7's cost" precisely.

---

## 1. Purpose

1. Teach systems thinking by consequence, not lecture (Beer Game lineage:
   players must *feel* the feedback loops they normally only cause).
2. Elicit each manager's private theory of what's broken — first moves are
   confessions.
3. Generate pre-debated, simulation-tested org-change proposals with managers
   who already own them.

Anti-purpose: this is NOT a performance evaluation. Stated out loud at start,
enforced at debrief.

### 1.1 Learning objectives → mechanics map (facilitator notes, 2026-06-13)

Every mechanic must serve at least one of these; anything that serves none
gets cut. Debrief explicitly walks this table.

| # | Objective | Taught by | Named at debrief as |
|---|-----------|-----------|---------------------|
| 1 | Hygiene: get the right information into Jira | Forecast quality scales with data quality in-game (unsized → vague forecasts); L13 hygiene sprint pays off visibly; H5 punishes fake hygiene; Data Hygiene tab is the same tool they'll use Monday | "Your forecast was wide because your data was thin — and that's true at your desk too" |
| 2 | Monitor & increase flow/velocity | Constraint cards, freeze bars, Kingman behavior under every decision; the briefing hint ("re-forecast after every change") trains instrument-watching | "Velocity wasn't a team property — it was a queue property" |
| 3 | Know how long an epic will take | Mandatory P85 commitments scored on reliability; the simulator IS the in-game planning tool | "Commit distributions, not dates" |
| 4 | Better team/org architecture | L5–L7, L14 (split/merge/scope moves), site constraints, H3/H8 seam loops, Epilogue valuing the org left behind | "The org you left behind scored more than the quarter you played" |
| 5 | The books, applied | Five focusing steps (constraint cards), freeze/full-kit/dosage (Rules of Flow), four types of work + unplanned work (interrupt economy), Five Ideals (locality via L9, joy via sustainability score) | The hidden-loop reveal, loop by loop, each cited to its book |
| 6 | Team dynamics & a happy team | H1 burnout/attrition, sustainability score (15%), after-hours coupling on zero-overlap edges, freeze-churn penalty | "Every red ρ bar is somebody's bad month — the score said so" |
| 7 | React to new requirements/changes | Scenario interludes are exactly this: S1 whale, S3 mandate, S5 onboarding land mid-game; slack + interfaces + WIP discipline determine absorption cost | "The teams that absorbed S3 had bought slack two rounds earlier" |
| 8 | What else (designer's additions) | (a) **Variability discipline** — small uniform batches beat heroic big ones (Kingman's variability term; dosage); (b) **Forecast humility** — saying "85%" out loud and being scored on it; (c) **Slack as investment, not waste** — E7 quiet weeks differentiate; (d) **Generative culture** — yellow-gets-help is a stated rule and the facilitators model it; (e) **System over individuals** — same managers, different policies, 3× different outcomes on identical seeds is the strongest anti-blame argument that exists; (f) **Stop starting, start finishing** — the only universally winning move across all strategies |

### 1.2 Winning principles → levers (what players can actually act on)

Winning is making the system *flow*, not doing more. Each principle the game
teaches is either a **direct lever** or an **emergent outcome** the scoring
enforces. Six of the eight map to a concrete lever; two — *beware local optima*
and *sustainable pace* — have no dedicated lever and are steered indirectly.

| Principle (book) | Lever(s) / how you act | What rewards it in the engine |
|---|---|---|
| **Manage the system — work the constraint** — *The Goal* | Freeze, WIP cap, Backfill hire, Reassign scope — aimed at the red-ringed pod | Queue cost is `ρ/(1−ρ)` per pod (convex), so relieving the *hottest* pod cuts the most cost |
| **Start less, finish more** — *Rules of Flow* · Little's law | **Freeze, WIP cap** | Overload (ρ>1) cuts throughput via a "thrash" penalty; lower WIP → lower ρ → more delivered + less queue cost |
| **Beware local optima** — *The Goal* | *No single lever* — closest is the Innovation bet's **holistic vs quick-win** flavour | The six scores trade off: spiking ROI by flogging teams tanks Team health + Epilogue; holistic bets compound, quick-wins add debt |
| **Cut coupling & handoffs** — Conway · Team Topologies · *Unicorn* | **Interface invest, Reassign scope** | Each cross-pod edge costs `count × timezone-factor`; interfaced edges stop costing; moving work in-house removes the seam |
| **Protect planned from unplanned** — *The Phoenix Project* | **Interrupt policy** (pool / office / follow-sun) | Interrupt+KTLO burden directly reduces delivery; the policy multiplies the interrupt tax down |
| **Full kit before you start** — *Rules of Flow* | **Full-kit gate** (+ Hygiene sprint, Descope to MVP raise readiness) | Readiness cuts the ops-debt→interrupt feedback loop and blunts incident damage |
| **Sustainable pace wins the long game** — *Unicorn Project* | *No dedicated lever* — emergent: keep ρ down (Freeze/Cap/Hire), avoid freeze-churn and reorg-whiplash | Morale falls with high-ρ streaks, after-hours strain, churn, and ≥3 disruptive moves; low morale → attrition → Team-health + Epilogue hit |
| **Promises & readiness compound** — *Rules of Flow* · *Phoenix/Unicorn* | **Commit a date** (promises); Hygiene / Full-kit / Descope (readiness) | Trust = kept commitments; Delight = readiness − incidents; both feed the year-2 Epilogue |

**The two emergent principles are deliberate, not gaps:** *beware local optima*
is enforced because the six dimensions pull against each other (you can't spike
one without paying elsewhere), and *sustainable pace* is what you protect by
*how* you use the other levers — there is no "rest the team" button; morale is a
consequence. (A future "slack / recovery" lever could make sustainable pace a
first-class move; today it is intentionally emergent.)

## 2. Format — offsite edition

**Players (17)**: 4 Senior Directors, 5 Managers/Senior Managers, 2 SRE
Managers, 5 Product Managers, 1 Chief Architect.
**Facilitators (not playing)**: the facilitator + PgM — run the event deck, adjudicate
rules, and take elicitation notes (especially every team's first two moves).

**Five teams of 3–4, role-balanced** — every team gets exactly one PM
(value judgment) plus 2–3 engineering leaders (capacity judgment); the forced
PM-vs-eng negotiation inside each team mirrors real planning:

| Team | Composition |
|------|-------------|
| 1–4 | 1 Senior Director (anchor) + 1 Mgr/Sr Mgr + 1 PM (+1 Mgr on two teams) |
| 5 | Chief Architect (anchor) + 1 Mgr/Sr Mgr + 1 PM |
| — | The 2 SRE Managers are placed on different teams (1 and 3, say) |

Mixing rules: no one is teamed with their own direct reporting line where
avoidable; mix sites within each team (the cross-site argument is curriculum).

**Run of show (v0.6 — the facilitator's format)**:
- **Pre-work**: participants read the books (The Goal, Rules of Flow,
  Phoenix, Unicorn) — assigned ahead of the offsite. The game assumes the
  vocabulary; the debrief rewards having internalized it.
- **At the offsite**: split into the five teams → state **the goal** plainly
  (*"maximize ROI across the whole org at minimum total cost — value
  shipped, trust kept, customers delighted, teams happy — for a full
  simulated year"*) → drop the **subtle hints** (see below) → rules →
  Round 1 opens from **our actual current position** (the live mined
  snapshot).
- **One round = 3 simulated months. Four rounds = one full year played.**
  (Round count stays an admin setup knob; 4×3mo is the offsite default.)
- **Round rhythm**: ~15 min planning/play → simulation → public scoreboard →
  **15 min open strategy discussion** — each team says what it did and why.
  Deliberate design: good strategies diffuse mid-game, exactly the behavior
  we want from real managers; escalating round weights keep it competitive
  even as play converges. → Then the **curve ball** (§6.5 scenario, always
  flavored as enterprise-product-with-large-customers reality) → next round.
- Each round, a team spends up to **3 AP** (a tight budget — an org absorbs
  only a few real changes a quarter; unspent AP don't bank). Most levers cost
  1 AP; big bets (full-kit gate, innovation) cost 2. Admin-tunable.
- **No free lunches / no dominant strategy:** every lever has a real cost so
  "apply all the good ones now" backfires — full-kit gate taxes near-term
  delivery (slower starts, fewer incidents later), a hygiene sprint spends
  that pod's quarter, and **reorg whiplash** fires if ≥3 disruptive levers
  (reassign/merge/split/interrupt-policy/full-kit/innovate) are played in one
  quarter (morale dips org-wide next quarter). Sequencing beats maxing.
- The Epilogue (§10.6) runs a further autopilot year beyond the played one.
- **Subtle hints (said once, not repeated)**:
  1. *"After every change, re-run the forecast — the tool works mid-game."*
  2. *"The goal is throughput of value, not activity."* (The Goal, ch. 4)
  3. *"Trade-offs are daily; the score is yearly."*
- Same seed-set and same scenario sequence for all teams. Decisions logged.

## 3. The world

- **Starting state = the real mined snapshot**: 35 pods, current WIP per pod,
  cycle-time distributions, the 64-edge dependency graph, the in-flight epic
  set with their fever positions, requester tiers on every epic.
- **Pairing (v0.5+)**: every pod pair-programs except the Kraków pods. The
  engine counts capacity in **work streams** (a pair pulls one item):
  pairing pods have devs/2 streams — halved healthy WIP, hotter ρ at the
  same queue. 🔒 Compensation: pairing pods take **half** the H3 knowledge-
  debt and H4 hero-collapse penalties (knowledge is shared by construction),
  and attrition σ-bumps are smaller. Kraków pods get full solo throughput
  but full bus-factor exposure — a real tradeoff, not a nerf, and a likely
  debrief discussion: should Kraków pair, or is its solo speed worth the
  fragility?
- **Per-pod burden parameters** (new in v0.2):
  - *Interrupt load*: 1–3 dev-days/week per pod (your stated range as the
    prior; calibrated per pod from data — see §3.1). Paid off the top of
    capacity every turn regardless of plans.
  - *Maintenance load (KTLO)*: recurring dev-days/week tied to each **work
    area**, not to the team. Maintenance travels with scope and is conserved
    — no org change makes it disappear; it can only be moved, automated
    (an L9-style investment), or neglected (which raises future interrupt
    load — 🔒H9).

### 3.1 Mining the burden parameters

- *Interrupt load from Jira*: issues created AND resolved within the same
  week, escalation/incident issue types, mid-sprint additions, "Reopen
  Count"/"Assignee Bounce" fields; plus PagerDuty incident counts per pod
  (PD team IDs already in the directory CSV).
- *From Slack (optional, richer)*: message volume in each pod's support/help
  channel — needs the channel-per-pod list from you.
- Thin data → pod gets the org median (2 d/wk) with a visible low-confidence
  marker, same convention as the rest of the app.
- Framing rule (stated): *the game world diverges from reality at turn 1 —
  nothing in-game is a commitment or a judgment.*
- **Horizon: 13 turns × 1 simulated week** (one quarter). Short enough to
  finish, long enough for delayed-payoff levers to matter.
- Stochastic engine: the existing Monte Carlo core (lognormal durations,
  Kingman queue scaling, handoff penalties) advanced week by week; incidents
  and demand arrive as Poisson streams calibrated from mined rates.

## 4. Round anatomy

Each round (3 simulated months), in order:

1. **Briefing** — game-state dashboard (the real app views running on game
   state) + the scenario injected at the last interlude.
2. **Planning** — 15 real minutes; team spends up to **3 AP** from the
   lever deck and, where the scenario demands it, records commitments (e.g.,
   a P85 date) and a 2–3 sentence response narrative (§6.5). Unspent AP do
   NOT bank (leader attention doesn't accumulate).
3. **Resolution** — engine simulates 13 weeks, week by week internally:
   demand arrives, queues flow, the within-round event deck (§6) draws,
   hidden loops trigger. Output: a quarterly narrative report per team.
4. **Interlude** — public scoreboard (component bars per team, not the
   weighted total) → **15-min open strategy discussion** (each team explains
   its round; facilitators draw out trade-off reasoning, never verdicts) →
   the next **curve ball** is revealed (§6.5).

## 5. The lever deck

Costs are AP; some levers have cooldowns (CD, in turns) or delayed payoffs.
"Target" = what it applies to.

| # | Lever | AP | Target | Effect | Notes / realism |
|---|-------|----|--------|--------|-----------------|
| L1 | Freeze epic/items | 1 | epic or N items | Removes from WIP; queue relief next turn | Requester reacts per tier (§9). Defrost later costs 1 AP and re-enters at reduced ramp |
| L2 | Defrost | 1 | frozen epic | Re-enters flow | Only if full-kit ≥80%, else stalls (visible rule) |
| L3 | WIP cap | 1 | pod | Cap at chosen ×/dev; intake beyond cap auto-queues | The pod's lead "complains" once (flavor); ρ falls over 2 turns |
| L4 | Full-kit gate | 2 | org-wide, once | New epics can't start below kit 80% | Permanent. Slows starts, prevents zombie epics |
| L5 | Reassign scope (same site) | 2 | epic/task set → pod | Work moves; capacity doesn't | Receiving pod runs at 60% on it for 2 turns (visible ramp) |
| L6 | Reassign scope (cross-site) | 3 | same | Same, but ramp is 40% for 3 turns + handoff penalty during ramp | People never move — hard rule |
| L7 | Merge pods (same site only) | 3, CD 4 | two pods | The app's merge mechanic, transfer% chooseable | Integration drag 2 turns; team-size cap 12 enforced |
| L8 | Interrupt policy | 2 | pod or site | Switch between: dedicated 1/day (status quo), site pool, office hours, SRE follow-the-sun | Tradeoff table in §10 |
| L9 | Interface investment | 2, payoff T+2 | one edge | Costs the upstream pod 20% capacity for 2 turns; then the edge's handoff cost drops 80% permanently | The classic under-used move; sensitive to *which* edge |
| L10 | Say no / defer demand | 1 | incoming epic | Decline an arriving epic | Requester reaction per tier; CoD of declined work is foregone score |
| L11 | Backfill placement | 0, once per run | site | One new hire lands at chosen pod, productive after 3-turn ramp | The only capacity-add in the game; placement is the decision |
| L12 | Batch handoff window | 1 | pod pair | Cross-site Q&A becomes a daily batch: handoff penalty −30%, but adds 0.5d latency floor | Models "stop drive-by pings" |
| L13 | Hygiene sprint | 1 | pod | Pod spends 25% of a week's capacity closing/sizing/linking; hygiene jumps, forecasts sharpen | Reveals hidden WIP that was never real (small instant relief) |
| L14 | **Found a new pod (split)** | 3, CD 4 | one pod → two | Take N members (same site only) from an existing pod, assign them a scope slice — the matching Jiras, epics, **and that scope's maintenance + interrupt burden** move with it | Detail in §5.1 |
| L15 | **Descope to MVP** | 1 | in-flight epic | Epic ships at 60% size for ~80% of its CoD value, this quarter | 🔒 If the cut includes the SRE/ops tasks, the scope's interrupt load rises 50% for 4 turns after "GA" — MVP's ops debt comes due (H9 adjacent). The deck does not say which cuts are safe |
| L16 | **Swarm the constraint** | 2, CD 3 | constraint pod | 1–2 named pods pause their own intake for a turn and drain the constraint's queue (their throughput added at 60% effectiveness) | Visible ramp discount; swarming pods' own queues age a turn. Powerful when timed to a hard date; wasteful as a habit |
| L17 | **Post-hoc hardening (PRR)** | 2 | shipped product | Raise a shipped product's ops-readiness (§10.5) to 80 | Costs 2× what doing the ops work pre-ship would have — shift-left, taught by price tag |
| L18 | **Innovation bet** | 3, payoff next round+ | org capability | Invest pod capacity in a product-forward capability: automation that cuts a scope's KTLO 40%, a platform feature opening a new value stream (CoD income in later rounds + Epilogue), or a self-service interface bundle | Two flavors on the card, same cost: **holistic** (multi-pod, architecture-aligned — slower payoff, compounds) vs **quick win** (single-pod hack — pays THIS round). 🔒 H10 decides what the quick win really cost |

*(Suggest L17+ here — parked: enabling-team rotation / InnerSource repo
opening, "exec air-cover" shield.)*

### 5.1 L14 mechanics — splitting a team (the merge's mirror)

The deck now contains both directions of Conway's law, and they must pull
against each other or one becomes dominant:

- **Same site only**; both resulting pods must keep **≥3 devs** (minimum
  viable on-call rotation).
- **Scope is chosen explicitly**: a work-area slice plus its Jiras (open and
  in-flight). The new pod inherits the parent's cycle-time distribution
  (same people) with a small σ bump for both halves (knowledge now spread).
- **Maintenance is conserved**: the scope's KTLO load moves with it,
  unchanged. Splitting never reduces total maintenance.
- **The interrupt floor multiplies**: the new pod immediately pays its own
  interrupt load (≥1 d/wk floor — someone now answers for that scope every
  day). Org-wide interrupt cost strictly increases with team count. This is
  the visible anti-fragmentation force: focus is bought with overhead.
- **A new edge usually forms**: parent and child share a domain seam; unless
  an interface investment (L9) lands on that seam within 2 turns, a
  parent↔child dependency edge materializes and heats up (🔒H8). You can
  split the team; you can't split the domain — unless you split the
  architecture too.
- Won't-break-obvious note 🔒: a well-chosen split (clean seam, L9 follow-up,
  burden honestly accounted) is one of the strongest moves in the game — it
  relieves a constraint AND reduces cognitive load. A careless split is one
  of the worst. The deck should not telegraph which is which.

## 6. Event deck (visible randomness)

Drawn per turn from calibrated probabilities; same sequence for all teams.

- E1 **Incident wave** — a pod takes a P1; interrupt policy determines how
  much feature capacity it burns and for how long.
- E2 **Exec ask** — Tier-1 epic arrives mid-quarter wanting a date this week.
- E3 **Customer escalation** — a frozen or deferred item's requester escalates
  (more likely if trust is low — see 🔒H2).
- E4 **Attrition notice** — a pod loses a dev in 4 turns (more likely under
  🔒H1 conditions; baseline rate otherwise).
- E5 **Dependency surprise** — mid-epic, an undeclared dependency on another
  pod surfaces (probability scales with that epic's kit score at start).
- E6 **Audit** — a sample of recently closed items is checked; gamed closures
  return as rework (see 🔒H5).
- E7 **Quiet week** — nothing. (Teams that built slack barely notice event
  weeks; teams at ρ≈1 get wrecked by every draw. That contrast IS the game.)
- E8 **Adoption shock** — a product shipped in-game (or recently in reality)
  gets adopted faster than expected. If its ops-readiness (§10.5) is below
  threshold: incident wave + a mandatory unplanned "hardening" epic lands in
  the owning pod's backlog at 2× the size the ops work would have cost
  pre-ship. The bill always arrives; the only question is the interest.

## 6.5 Scenario interludes (the narrative arc)

Between rounds, one **scenario card** is injected — the same card for every
team, but its **impact is path-dependent**: the engine computes severity from
each team's own prior decisions. Same storm, different damage — that contrast
on the scoreboard is the lesson made visible.

**Stock deck** (facilitators choose/sequence; suggested arc below):

| # | Scenario | What lands | Path-dependence (🔒 formula reads team state) |
|---|----------|-----------|------------------------------------------------|
| S1 | **The whale feature** | A Tier-1 customer epic arrives spanning 4 named pods, hard date end of next round. Team must commit a P85 date *during planning* (the simulator hint pays off here) | Lands gently on teams with constraint headroom and clean interfaces; brutally on saturated ones. Commitment quality scored: honest P85 > heroic promise |
| S2 | **The 3 AM cascade** | Major production incident chain across 2 pods | Resolution cost = f(observability/hygiene investment, SRE inclusion in past ships, interrupt model, ops debt). Ranges from "lost a week" to "lost half the round + trust" |
| S3 | **Security mandate** | Compliance work lands on EVERY pod simultaneously (non-negotiable, T1) | Teams with WIP discipline absorb it; teams at ρ≈1 watch everything slip. Tests triage under pressure |
| S4 | **The departure** | A named senior engineer leaves the constraint pod, 4-week notice | Probability boosted if that team armed H1; knowledge gap σ-bump halved if a past hygiene sprint (L13) or split-with-docs touched that pod |
| S5 | **New product onboarding** | Leadership hands the org a new product's scope — it must live somewhere | Tests team design: who has cognitive-load room? Forces L5/L7/L14 thinking; KTLO and interrupt floor attach permanently |
| S6 | **The optics demand** | Exec pressure: "show visible progress this round" — accepting grants immediate Delivery points | 🔒 Goodhart bait: accepting forces starts-without-kits and skipped ops tasks; the cost arrives in round 3 and the Epilogue. Declining costs a little trust now |
| S7 | **Upstream API break** | A platform dependency changes; every pod consuming a specific edge takes rework | Pods behind an L9-invested interface absorb it in days; raw-coupled pods eat weeks |

**Suggested arc (N=3)**: R1 ends → S1 (the whale — forces planning
discipline early) → R2 ends → S2 or S6 (the reckoning — pays out past
investment or its absence) → R3 ends → Epilogue + final scoreboard.
For other N: open with S1 (discipline), close the mid-game with a
reckoning card (S2/S6/S7), and never play two reckonings back-to-back —
teams need a round to act on what a crisis taught them.

**Custom scenarios**: the admin console (§10.7) includes a scenario editor —
title, narrative text, affected pods/epics, and impact assembled from
standard hooks (capacity hit %, epic injection with tier/size/date, interrupt
spike, trust delta), each hook optionally multiplied by a team-state reader
(ops debt, hygiene, slack-at-constraint, interfaces built). Admin-authored
cards mix into the stock deck.

**Response narratives**: for each scenario, teams write 2–3 sentences on how
they'd respond in reality (not just which levers they pull). Not scored by
the engine — read aloud at the debrief, where the gap between "what we wrote"
and "what we clicked" is often the best conversation in the room.

## 7. 🔒 Hidden mechanics (designer-only — the curriculum)

| # | Loop | Trigger | Effect |
|---|------|---------|--------|
| H1 | Burnout → attrition | Pod sustains ρ>0.9 for 3+ turns, OR ≥2 freeze/defrost cycles on its work, OR chronic cross-site after-hours coordination (zero-overlap edges left hot) | E4 probability triples for that pod; departing dev takes a knowledge penalty (pod σ rises) |
| H2 | Stakeholder trust | Freezing/deferring Tier-1/2 requesters without the (visible) "communicate decision" free action | That requester's future arrivals come as interrupts/escalations, not plannable work; compounding |
| H3 | Knowledge debt | Cross-site scope moves (L6) and merges (L7) | Beyond visible ramp: small permanent σ increase unless followed by a hygiene sprint (L13) within 2 turns ("write it down while it's fresh") |
| H4 | Hero collapse | One pod handles >40% of org interrupts for 3 turns | Bus-factor event: key person unavailable 2 turns, MTTR doubles |
| H5 | Goodhart audit | Hygiene improved >25 pts in one turn via closures (not sizing/linking) | E6 targets that pod; 30% of closures return as rework with +50% size |
| H6 | Constraint migration | The constraint pod's ρ drops below 0.75 | The constraint silently moves; teams still subordinating to the old constraint waste AP (tests whether they re-diagnose) |
| H7 | End-game integrity | Final 3 turns | Superseded in v0.4 by the Epilogue (§10.6): a full 52 autopilot weeks now value the terminal state — strip-mining the endgame is fatal, not just costly |
| H8 | Scope seam | L14 split without an L9 interface investment on the new seam within 2 turns | A parent↔child dependency edge forms and grows each turn; the split's focus gains erode into coordination tax |
| H9 | Maintenance neglect | A scope's KTLO load goes unstaffed (e.g., after a split or scope move leaves it orphaned) for 2+ turns | Its interrupt load grows 25%/turn compounding — deferred maintenance returns as incidents |
| H10 | Local-optima debt | L18 quick-win flavor chosen, or any move that improves one pod's metrics while raising org coordination/queue tax | The quick win silently registers as tech debt: +KTLO on that scope, and S7-style breakage odds double for it in later rounds and the Epilogue. Holistic bets are exempt. Reveal line: "you optimized a pod and taxed the org" |

Reveal ALL of these at debrief, with per-team trigger logs. The reveal is the
lesson; the logs make it personal without being punitive.

## 8. Scoring

Total = weighted sum; **components visible every turn, weights and H-loops
hidden until debrief.**

The score is framed to players exactly as the facilitator frames the org's goal:
**maximum ROI at minimum total cost function, with trust, delight, and happy
teams as first-class terms — not tie-breakers.**

| Component | Facilitator's words | What the engine measures | 🔒 Weight |
|-----------|---------------|--------------------------|----------|
| **ROI** | "minimum cost function across all teams, maximum ROI" | CoD-weighted value shipped MINUS the cost function (coordination tax + queue tax + interrupt burn); hard-date epics: large bonus / catastrophic miss | 25% |
| **Trust** | "being trustworthy, fulfilling work on time or before" | Of committed P85 dates, % landed on/inside — early counts, late compounds (a missed commitment also raises future scenario severity 🔒) | 15% |
| **Delight** | "delight internal & external customers — edge cases, monitoring" | Ops-readiness of ships, incidents avoided, E8/S2 outcomes; internal customers = downstream pods' wait on you | 10% |
| **Team health** | "happy teams — SRE, Engg, PM, managers" | Attrition avoided, after-hours load, freeze-churn, interrupt equity across pods | 15% |
| **Innovation & wholeness** | "innovations that take the product forward holistically, not piecemeal local optima" | L9/L18 investments that reduce org-level cost (KTLO ↓, edges deleted, constraint headroom ↑). 🔒 Local-optima moves that improve one pod while raising org cost score ZERO here and feed H10 | 10% |
| **Epilogue (year 2, autopilot)** | "trade-offs are daily; the score is yearly" | The §10.6 scorecard at +52 autopilot weeks | 25% |

**Round weighting (visible rule, exact split 🔒)**: rounds contribute to the
in-quarter components with escalating weight — round *i* of *N* weighs
proportionally to *i* (for N=3 that's 17/33/50; the spirit, not the exact
split, is disclosed). Stated to players as:
*"early rounds teach, later rounds count — nobody is out after round 1."*
Combined with the Epilogue's 30%, a team that builds well can win from
behind; a team that sprints round 1 and hollows out the org cannot.

Hard-date epics in the real snapshot are flagged in-game; missing one costs
more than any other single thing — matching your stated reality.

## 9. Requester tiers (freeze/defer reactions)

| Tier | Who | Freeze reaction |
|------|-----|-----------------|
| T1 | Contractual / customer-committed (hard dates) | Cannot freeze without an explicit "renegotiate" action (2 AP, public) |
| T2 | Exec ask / strategic program (e.g., Revelstoke) | Trust hit unless "communicate decision" free action used; possible escalation |
| T3 | Internal team ask | Mild; batched comms fine |
| T4 | Aspirational / unowned (the `continuous-improvement` bucket) | Free to freeze; often free to kill |

Tier assignments for the real epic set: drafted by you before the game (30
min of your time; also genuinely useful outside the game).

## 10. Interrupt policy options (L8 detail)

Status quo (your stated constraint): **1 person per team per day** ≈ 20%
capacity tax on a 5-dev pod, but strong domain knowledge per interrupt.

| Policy | Capacity freed | Interrupt MTTR | Hidden coupling |
|--------|---------------|----------------|-----------------|
| Dedicated 1/day (status quo) | — | 1.0× | H4 risk if same person repeats |
| Site-level pool (rotating) | +10–15% per pod | 1.4× (less domain depth) | Needs hygiene ≥60% (runbooks) or MTTR 1.8× |
| Office hours (2h/day) | +15% | 2.0× for off-hours arrivals | T1/T2 escalations bypass it (H2 interaction) |
| SRE follow-the-sun front-line | +20% per product pod | 1.2× for known issues, 2.5× for novel | Loads the SRE pods; their burnout meters are real (H1) |

This table is half-visible: players see the capacity/MTTR columns, not the
hidden-coupling column.

## 10.5 Ops debt — the "SRE gets the short end" economy

Every product/epic shipped in-game carries an **ops-readiness score (0–100)**
computed from things players control:

- SRE/prod-readiness task completed before GA (the full-kit item): +40
- Runbooks/observability in place (pod hygiene ≥60% at ship time): +25
- Error budget / alerting task done: +20
- No L15 MVP cut touched the ops tasks: +15

**The trap is adoption-shaped** (deliberately matching your description):
operational load of a shipped product = `baseLoad × adoption(t) × (1 −
readiness/100)`, where adoption follows an S-curve over ~2 quarters. A
low-readiness ship costs almost nothing at GA — the silence reads as
vindication — and becomes a pager storm two quarters later, **landing on the
SRE pods first** (they front-line interrupts), driving their H1 burnout
meters, and only then splashing back on the owning pod as E8 hardening epics.

Players see: each shipped product's readiness score and the org **Ops-debt
ticker** = Σ adoption-weighted (100 − readiness). 🔒 Hidden: the S-curve
timing and the interest rate (load multiplier), and the SRE-attrition cascade
(if an SRE pod hits H1 attrition, *everyone's* MTTR rises 50% — the org
discovers SRE was load-bearing).

**Real-world measurement of the same dynamic** (so the game metric has a
reality counterpart you can track after the offsite):
- % of shipped epics that included completed SRE tasks (full-kit gate report)
- PagerDuty incident rate per product vs product age (the adoption-lag
  signature: incidents peaking 1–2 quarters post-GA indicate unready ships)
- SRE pods' unplanned-work share and interrupt MTTR trend
- KTLO share of each pod's capacity (the org-death metric: when maintenance
  crosses ~70% of capacity, feature flow effectively stops)

## 10.6 The Epilogue — year-out simulation

After turn 13, the engine fast-forwards **52 weeks with no player actions**:
the org runs on autopilot under whatever policies, WIP caps, interfaces,
team shapes, and ops debt the team left behind.

What happens during the Epilogue:
- **Adoption curves mature** — every in-game ship reaches full operational
  load; ops debt converts to interrupt load per §10.5.
- **Entropy** — a hazard rate breaks a fraction of implementations per
  quarter; interfaces built with L9 need light upkeep or partially regress;
  unmaintained scopes compound per H9.
- **Demand never stops** — epics keep arriving at mined rates; frozen items
  defrost or die per the team's standing policy; the constraint drifts and
  nobody is steering.
- **New products age into maintenance** — each ship adds permanent KTLO,
  so the year-out KTLO share reveals whether the team built an org that can
  keep shipping or one that will spend next year keeping the lights on.

**Year-out scorecard** (replaces H7's 4-week peek; weight in §8): org flow
index at +52w, KTLO share of capacity, SRE load index, attrition count,
final-quarter value run-rate vs the played quarter (did the org speed up or
was the quarter a sugar high?).

## 10.7 Admin console (facilitators only)

**Game setup (before play)**:
- **Hard-date entry**: admin selects in-flight epics and marks contractual
  dates + requester tier — no dates baked into the spec or code; entered
  fresh at setup so the game always reflects current reality.
- **Round count** (N): chosen at setup; the engine scales round weights
  (§8) and the scenario sequence length automatically.
- Snapshot selection (which mined data state seeds the world), seed-set,
  scenario sequence (stock and/or custom from the editor; one card per
  interlude, so N−1 cards minimum plus the Epilogue), AP budget knob.

The facilitator + PgM see, live, what players never do:
- All five teams on one **divergence chart** (flow index per turn), turn
  positions, AP spent, score components.
- **Streaming move logs** per team, with first-two-moves highlighted (the
  elicitation signal).
- **Hidden-loop trigger feed** — watch a team arm H1/H8/H5 turns before the
  effect lands; this feed becomes the per-team debrief logs.
- Pacing tools: pause a team, extend the turn clock, inject a discretionary
  event (E-deck card of choice), and a "freeze game state" snapshot for the
  debrief walkthrough.
- Build note: v1 = each team's browser posts turn state to a shared drop
  (folder/lightweight endpoint); the admin view is a sixth browser tab
  reading all five. No accounts, no server complexity at the offsite.

## 11. Debrief protocol (the actual deliverable)

1. Score reveal with component breakdown per team.
2. Winner presents: their theory of the org, key moves, what they'd do for
   real. Contrarian runner-up (most different strategy in top half) presents
   second.
3. Hidden-loop reveal with per-team trigger logs.
4. Facilitated: "which 3 moves from any playthrough do we adopt in reality?"
   — captured as actual proposals with owners.
5. Anonymous 3-question pulse on the game itself (keep/kill/change).

## 12. Build notes (when we implement)

Engine reuse is high: simulateFeature, kingmanScale, mergePods, orgFlowScore,
freezeProjection, fullKitCheck, feverPoint all apply per-round. New: a round
loop with persistent game state, the lever/event/scenario decks, trust/morale
meters, seeded weekly demand+incident arrivals inside each round, decision
log, score ledger, the setup screen (hard dates, §10.7) and scenario editor.
Feasible as a new mode on the existing app; multiplayer = each team's state
in localStorage + a shared drop the admin tab reads (no real server at v1).

**Game mode vs. live mode (hard requirement)**: the app keeps a visible
**Live (Jira) / Game** toggle. Game state is a sandboxed copy created at
setup; the live views (network, fever, hygiene, scoreboard on real mined
data) remain untouched and available at all times — before, during, and
after the offsite — so players can walk out of the game, open Live mode,
and apply what they just learned to the actual org. The post-game half-life
of this exercise depends on that toggle.

## 13. Decisions log & remaining inputs

Resolved 2026-06-12 (all recommendations accepted):
1. Levers: L15 descope-to-MVP and L16 swarm added; enabling-team rotation and
   exec air-cover parked for v2.
2. Turn length: 13 × 1 week.
3. AP: 3 in sandbox, 4 scored; facilitator playtest (the facilitator + PgM + Claude)
   decides whether scored drops to 3 — **schedule this before the offsite**.
4. Requester tiers: Claude drafts from data → the facilitator corrects. **Action:
   Claude.**
5. Interrupt calibration: Jira + PagerDuty only for v1 (no Slack). Still
   open: any pods already running a non-default interrupt model?
7. Players: the 17-person offsite population in §2; facilitators: the facilitator + PgM.

Remaining inputs:
6. **Hard-date epics** — the facilitator to supply the epic keys with contractual
   dates. (Game only needs the keys + "miss = catastrophic"; dates themselves
   can stay out of the doc.)
8. Team rosters — facilitators assign per the §2 mixing rules once attendance
   is final.
