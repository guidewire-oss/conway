#!/usr/bin/env python3
"""Group data/epic_children.jsonl into app/data/epics/<KEY>.json + index.json.

Keeps epics that still have open work (>=1 non-closed child, >=2 children).
Index is sorted by open-task count, capped at 20 for chart readability.
Preserves richer hand-synced snapshots (with link data) if already present.
"""
import json
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "data" / "epic_children.jsonl"
OUT_DIR = ROOT / "app" / "data" / "epics"
DONE = {"Closed", "Done", "Resolved"}
POD_ALIASES = {"Moose Factory": "MooseFactory"}
CAP = 20
# epic keys with a hand-synced snapshot (e.g. via fetch_epic.py, with richer
# link data than the bulk crawl) that this script should never overwrite —
# populate with your own keys as needed.
PRESERVE = set()


def main():
    # epic-level metadata (due date, labels, business-outcome presence) merged
    # from the full sweep so `make data` reproduces the enriched snapshots
    meta = {}
    meta_path = ROOT / "data" / "epic_meta.jsonl"
    if meta_path.exists():
        for line in meta_path.read_text().splitlines():
            if line.strip():
                m = json.loads(line)
                meta[m["key"]] = m

    by_epic = defaultdict(list)
    names = {}
    seen = set()
    for line in SRC.read_text().splitlines():
        if not line.strip():
            continue
        r = json.loads(line)
        if not r.get("epic") or r["key"] in seen:
            continue
        seen.add(r["key"])
        pod = (r.get("pod") or "").strip()
        by_epic[r["epic"]].append({
            "key": r["key"],
            "summary": r.get("summary") or "",
            "pod": POD_ALIASES.get(pod, pod) or None,
            "points": r.get("points"),
            "status": r.get("status"),
            "type": None,
            "created": r.get("created"),
            "resolved": r.get("resolved"),
            "blocks": [],
            "blockedBy": [],
        })
        if r.get("epicName"):
            names[r["epic"]] = r["epicName"]

    candidates = []
    for epic, tasks in by_epic.items():
        open_tasks = [t for t in tasks if t["status"] not in DONE]
        if len(tasks) >= 2 and open_tasks:
            candidates.append((epic, tasks, len(open_tasks)))
    candidates.sort(key=lambda c: -c[2])

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    kept = []
    for epic, tasks, _ in candidates[:CAP]:
        if epic not in PRESERVE:
            m = meta.get(epic, {})
            snap = {"epic": epic, "name": names.get(epic), "tasks": tasks}
            if m:
                snap["duedate"] = m.get("duedate")
                snap["labels"] = m.get("labels", [])
                snap["descLen"] = m.get("descLen", 0)
                snap["hasOutcome"] = m.get("hasOutcome", False)
            (OUT_DIR / f"{epic}.json").write_text(json.dumps(snap, indent=1))
        kept.append(epic)
    for p in PRESERVE:
        if p not in kept and (OUT_DIR / f"{p}.json").exists():
            kept.append(p)
    (OUT_DIR / "index.json").write_text(json.dumps(kept))
    print(f"children records: {len(seen)} across {len(by_epic)} epics; "
          f"{len(candidates)} in-flight; wrote {len(kept)} snapshots "
          f"(cap {CAP}, dropped {max(0, len(candidates) - CAP)})")


if __name__ == "__main__":
    main()
