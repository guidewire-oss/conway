#!/usr/bin/env python3
"""Merge epic-level metadata (due date, labels, business-outcome presence)
into the existing app/data/epics/<KEY>.json snapshots."""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EPICS = ROOT / "app" / "data" / "epics"

# markers that count as "a business outcome was written down"
OUTCOME_MARKERS = re.compile(
    r"business|outcome|cost of delay|success metric|why|customer value|problem",
    re.IGNORECASE,
)
MIN_DESC = 200  # chars; below this a description can't hold an outcome


def main(raw_path):
    raw = json.loads(Path(raw_path).read_text())
    updated = 0
    for node in raw["issues"]["nodes"]:
        f = node["fields"]
        path = EPICS / f"{node['key']}.json"
        if not path.exists():
            continue
        snap = json.loads(path.read_text())
        desc = f.get("description") or ""
        snap["name"] = snap.get("name") or f.get("summary")
        snap["duedate"] = f.get("duedate")
        snap["labels"] = f.get("labels") or []
        snap["descLen"] = len(desc)
        snap["hasOutcome"] = len(desc) >= MIN_DESC and bool(OUTCOME_MARKERS.search(desc))
        path.write_text(json.dumps(snap, indent=1))
        updated += 1
    print(f"updated {updated} epic snapshots")
    for node in raw["issues"]["nodes"]:
        f = node["fields"]
        desc = f.get("description") or ""
        has = len(desc) >= MIN_DESC and bool(OUTCOME_MARKERS.search(desc))
        print(f"  {node['key']}: due={f.get('duedate')} descLen={len(desc)} outcome={has} labels={f.get('labels')}")


if __name__ == "__main__":
    main(sys.argv[1])
