#!/usr/bin/env python3
"""Append epic metadata records from a saved Jira search result file."""
import json
import re
import sys
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent / "data" / "epic_meta.jsonl"
OUTCOME_MARKERS = re.compile(
    r"business|outcome|cost of delay|success metric|why|customer value|problem",
    re.IGNORECASE,
)
MIN_DESC = 200


def main(raw_path):
    raw = json.loads(Path(raw_path).read_text())
    with open(OUT, "a") as f:
        for node in raw["issues"]["nodes"]:
            fl = node["fields"]
            desc = fl.get("description") or ""
            f.write(json.dumps({
                "key": node["key"],
                "name": (fl.get("summary") or "")[:90],
                "duedate": fl.get("duedate"),
                "labels": fl.get("labels") or [],
                "status": (fl.get("status") or {}).get("name"),
                "descLen": len(desc),
                "hasOutcome": len(desc) >= MIN_DESC and bool(OUTCOME_MARKERS.search(desc)),
            }) + "\n")
    print(f"appended {len(raw['issues']['nodes'])}")


if __name__ == "__main__":
    main(sys.argv[1])
