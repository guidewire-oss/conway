#!/usr/bin/env python3
"""Build app/data/wip_detail.json: per-pod WIP issues with freeze analytics."""
import json
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
POD_ALIASES = {"Moose Factory": "MooseFactory"}
NOW = datetime.now(timezone.utc)
# WIP that is started-but-waiting (a queue), not actively being worked
WAITING_STATUSES = {"In Review", "On Hold", "Blocked", "InfoSec Review", "Waiting", "Pending"}


def load_jsonl(path):
    return [json.loads(l) for l in path.read_text().splitlines() if l.strip()]


def days_since(iso):
    if not iso:
        return None
    return round((NOW - datetime.fromisoformat(iso)).total_seconds() / 86400, 1)


def main():
    wip = load_jsonl(ROOT / "data" / "wip_detail.jsonl")
    links = load_jsonl(ROOT / "data" / "issues_links.jsonl")

    # keys whose completion someone is waiting on (they appear as a blocker)
    blockers = defaultdict(set)
    for rec in links:
        for k in rec.get("blockedBy", []):
            blockers[k].add(rec["key"])
        for k in rec.get("blocks", []):
            blockers[rec["key"]].add(k)

    by_pod = defaultdict(list)
    split = defaultdict(lambda: {"active": 0, "waiting": 0})
    for rec in wip:
        pod = (rec.get("pod") or "").strip()
        pod = POD_ALIASES.get(pod, pod)
        if not pod:
            continue
        if rec.get("status") in WAITING_STATUSES:
            split[pod]["waiting"] += 1
        else:
            split[pod]["active"] += 1
        age = days_since(rec.get("created"))
        stale = days_since(rec.get("updated"))
        blocked_keys = sorted(blockers.get(rec["key"], set()))
        if blocked_keys:
            verdict = "keep"          # others wait on this; freezing it freezes them
        elif (stale or 0) > 14 or not rec.get("assignee"):
            verdict = "freeze"
        elif (stale or 0) > 7:
            verdict = "review"
        else:
            verdict = "keep"
        by_pod[pod].append({
            "key": rec["key"],
            "summary": (rec.get("summary") or "")[:110],
            "status": rec.get("status"),
            "assignee": rec.get("assignee"),
            "points": rec.get("points"),
            "ageDays": age,
            "staleDays": stale,
            "blocksKeys": blocked_keys,
            "verdict": verdict,
        })
    for pod in by_pod:
        by_pod[pod].sort(key=lambda r: (-(r["staleDays"] or 0), r["key"]))  # stable tiebreak

    out = ROOT / "app" / "data" / "wip_detail.json"
    out.write_text(json.dumps(dict(sorted(by_pod.items())), indent=1))
    split_out = ROOT / "app" / "data" / "wip_split.json"
    split_out.write_text(json.dumps(dict(sorted(split.items())), indent=1))
    total = sum(len(v) for v in by_pod.values())
    freeze = sum(1 for v in by_pod.values() for r in v if r["verdict"] == "freeze")
    act = sum(v["active"] for v in split.values())
    wait = sum(v["waiting"] for v in split.values())
    print(f"wrote {out}: {total} WIP issues across {len(by_pod)} pods; "
          f"{freeze} auto-flagged freeze candidates")
    print(f"wrote {split_out}: {act} active / {wait} waiting "
          f"({100 * wait // max(1, act + wait)}% of WIP is waiting, not being worked)")


if __name__ == "__main__":
    main()
