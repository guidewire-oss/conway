#!/usr/bin/env python3
"""List in-flight epic keys (>=2 children, >=1 open) in JQL batches of 30."""
import json
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DONE = {"Closed", "Done", "Resolved"}


def main():
    by_epic = defaultdict(list)
    seen = set()
    for line in (ROOT / "data" / "epic_children.jsonl").read_text().splitlines():
        if not line.strip():
            continue
        r = json.loads(line)
        if not r.get("epic") or r["key"] in seen:
            continue
        seen.add(r["key"])
        by_epic[r["epic"]].append(r.get("status"))
    keys = sorted(
        e for e, sts in by_epic.items()
        if len(sts) >= 2 and any(s not in DONE for s in sts)
    )
    out = ROOT / "data" / "epic_batches.txt"
    with open(out, "w") as f:
        for i in range(0, len(keys), 30):
            f.write("key in (" + ", ".join(keys[i:i + 30]) + ")\n")
    print(f"{len(keys)} in-flight epics -> {out} ({(len(keys) + 29) // 30} batches)")


if __name__ == "__main__":
    main()
