#!/usr/bin/env python3
"""Convert a raw Jira search result (epic children) into app/data/epics/<KEY>.json."""
import json
import sys
from pathlib import Path


def convert(raw_path, epic_key, out_dir):
    raw = json.loads(Path(raw_path).read_text())
    tasks = []
    for node in raw["issues"]["nodes"]:
        f = node["fields"]
        pod = (f.get("customfield_10026") or {}).get("value")
        blocks, blocked_by = [], []
        for link in f.get("issuelinks") or []:
            if link.get("type", {}).get("name") != "Blocks":
                continue
            if link.get("outwardIssue"):
                blocks.append(link["outwardIssue"]["key"])
            if link.get("inwardIssue"):
                blocked_by.append(link["inwardIssue"]["key"])
        tasks.append({
            "key": node["key"],
            "summary": f.get("summary", ""),
            "pod": pod,
            "points": f.get("customfield_10014"),
            "status": (f.get("status") or {}).get("name"),
            "type": (f.get("issuetype") or {}).get("name"),
            "created": f.get("created"),
            "resolved": f.get("resolutiondate"),
            "blocks": blocks,
            "blockedBy": blocked_by,
        })
    out = Path(out_dir) / f"{epic_key}.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps({"epic": epic_key, "tasks": tasks}, indent=1))
    pods = sorted({t["pod"] for t in tasks if t["pod"]})
    print(f"wrote {out}: {len(tasks)} tasks, pods: {pods}")


if __name__ == "__main__":
    convert(sys.argv[1], sys.argv[2], sys.argv[3])
