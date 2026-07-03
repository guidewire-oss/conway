# Snapshots & Scenarios

One object underlies both the manager's analytics and the facilitator's games: a
**network snapshot** — a dated, named capture of the org network (pods, the
cross-pod dependency edges, per-pod flow stats, and hygiene). Snapshots live in
the database; Observe renders them and Train seeds games from them. There is no
separate "template" store — a template is just a snapshot a facilitator owns and
edits.

A snapshot has:

- **source** — `baseline` (the mined seed shipped with the app), `jira` (a manager's
  live import), or `template` (a facilitator's editable scenario).
- **visibility** — `private` (only the owner + admins) or `public` (everyone can
  see and seed from it).
- **owner** — who created it.

## Manager flow (Observe)

1. **Import from Jira** (Observe bar → 📥). Pick projects; structure is **auto from
   Jira** by default (pods from the pod field, dev-count ≈ distinct assignees) or
   from a Plan. The result is a private snapshot.
2. **Compare over time.** The Org Network view's *compare to* picker diffs the
   current snapshot against another — new edges (green), dropped edges (red
   dashed), and per-pod WIP deltas — for a temporal view of how the org moved.
3. **Try levers.** The Org Network's simulation panel and the Levers view let you
   rehearse changes against a snapshot before committing to them.
4. **Publish.** 🗂 Snapshots → *make public* shares a snapshot so facilitators can
   build games from it. *make private* unshares it.

## Facilitator flow (Train → 🎮 Games)

The **Scenario library** in the Games panel lists every snapshot/template you can
use — your own, anything public, plus the built-in difficulty presets.

- **Seed a game.** The scenario dropdown groups *Difficulty presets*, *Live org
  snapshots*, *Scenario templates*, and *Plans*. Pick any to be the game's world.
- **Customize for a scenario.** *Duplicate* a snapshot/template → you get an
  editable copy you own. *Download* it as JSON, edit, and *Upload* it back as a new
  template. The pods in the file are the game's teams.
- **Author from scratch.** *Download sample format*, fill it in, *Upload network
  file* → a named template (private by default; *make public* to share with other
  facilitators).

Deleting a template a game already started from is safe — the running game keeps
its world; only the stored template is removed.

## The network file format

Download/upload uses a single human-editable JSON file. `stats` and `overlap` are
optional (sensible defaults are applied: overlap from shared sites; loads
synthesized from dev-count). `pods` are the teams.

```json
{
  "name": "Crisis scenario A",
  "pods": [
    {"name": "Platform", "location": "San Mateo", "pairing": true,  "devCount": 6},
    {"name": "Payments", "location": "Bengaluru", "pairing": true,  "devCount": 5},
    {"name": "Mobile",   "location": "Toronto",   "pairing": false, "devCount": 4}
  ],
  "edges": [
    {"from": "Payments", "to": "Platform", "count": 5},
    {"from": "Mobile",   "to": "Platform", "count": 3}
  ],
  "stats": {
    "Platform": {"wip": 14, "throughputPerWeek": 3, "cycleP50": 8, "cycleP85": 20, "hygiene": 0.6}
  },
  "overlap": {"Payments": {"Platform": 2}}
}
```

- **pods** — the teams. `location` drives the cross-site coordination seam;
  `pairing` halves effective tracks; `devCount` is the headcount.
- **edges** — directed blocking dependencies (`from` blocks `to`), `count` = how
  many links (edge thickness / coupling weight).
- **stats** — per-pod flow: `wip`, `throughputPerWeek`, cycle-time `cycleP50`/
  `cycleP85`, `hygiene` (0–1). Omitted pods get defaults.
- **overlap** — optional per-pair work-hour overlap; omit to derive from sites.

`GET /api/sample/network.json` returns a worked example.

## API summary

| Method & path | Who | What |
|---|---|---|
| `GET /api/snapshots` | any signed-in | list visible snapshots (own + public + baseline) |
| `GET /api/snapshots/{id}/data/{doc}` | any visible | a stored world/observe doc |
| `GET /api/snapshots/{id}/export` | any visible | download the editable NetworkFile |
| `POST /api/snapshots/import` | manager | live Jira import → `jira` snapshot |
| `POST /api/snapshots/import-network` | facilitator | upload NetworkFile → `template` |
| `POST /api/snapshots/{id}/clone` | facilitator | duplicate a visible snapshot → `template` |
| `PATCH /api/snapshots/{id}` | owner/admin | rename and/or set `public` |
| `DELETE /api/snapshots/{id}` | owner/admin | delete (baseline is protected) |

Games store the chosen world as a scenario string: `snap:<id>` (any snapshot or
template), `plan:<id>` (a Plan), or a difficulty preset (`default`/`balanced`/
`constrained`/`crisis`). Seeding for `snap:<id>` runs through `worldFromSnapshot`.
