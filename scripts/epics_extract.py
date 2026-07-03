#!/usr/bin/env python3
"""Append epic-children records from a saved Jira search result; print next cursor."""
import json
import sys
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent / "data" / "epic_children.jsonl"


def main(raw_path):
    raw = json.loads(Path(raw_path).read_text())
    with open(OUT, "a") as f:
        for node in raw["issues"]["nodes"]:
            fl = node["fields"]
            f.write(json.dumps({
                "key": node["key"],
                "epic": (fl.get("parent") or {}).get("key"),
                "epicName": ((fl.get("parent") or {}).get("fields") or {}).get("summary"),
                "pod": (fl.get("customfield_10026") or {}).get("value"),
                "summary": (fl.get("summary") or "")[:110],
                "status": (fl.get("status") or {}).get("name"),
                "points": fl.get("customfield_10014"),
                "created": fl.get("created"),
                "resolved": fl.get("resolutiondate"),
            }) + "\n")
    info = raw["issues"]["pageInfo"]
    print(f"appended {len(raw['issues']['nodes'])}; hasNext={info['hasNextPage']}")
    if info["hasNextPage"]:
        print(f"CURSOR:{info['endCursor']}")


if __name__ == "__main__":
    main(sys.argv[1])
