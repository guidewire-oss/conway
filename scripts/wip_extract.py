#!/usr/bin/env python3
"""Append WIP issue details from a saved Jira search result; print next cursor."""
import json
import sys
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent / "data" / "wip_detail.jsonl"


def main(raw_path):
    raw = json.loads(Path(raw_path).read_text())
    with open(OUT, "a") as f:
        for node in raw["issues"]["nodes"]:
            fl = node["fields"]
            f.write(json.dumps({
                "key": node["key"],
                "pod": (fl.get("customfield_10026") or {}).get("value"),
                "summary": fl.get("summary", ""),
                "created": fl.get("created"),
                "updated": fl.get("updated"),
                "assignee": (fl.get("assignee") or {}).get("displayName"),
                "status": (fl.get("status") or {}).get("name"),
                "points": fl.get("customfield_10014"),
            }) + "\n")
    info = raw["issues"]["pageInfo"]
    print(f"appended {len(raw['issues']['nodes'])}; hasNext={info['hasNextPage']}")
    if info["hasNextPage"]:
        print(f"CURSOR:{info['endCursor']}")


if __name__ == "__main__":
    main(sys.argv[1])
