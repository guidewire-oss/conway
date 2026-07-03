#!/usr/bin/env python3
"""Aggregate mined Jira JSONL into app/data/edges.json and app/data/pod_stats.json."""
import json
import math
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DATA = ROOT / "data"
OUT = ROOT / "app" / "data"


def load_jsonl(name):
    path = DATA / name
    if not path.exists():
        return []
    return [json.loads(l) for l in path.read_text().splitlines() if l.strip()]


# whitespace variants seen in Jira vs the org directory
POD_ALIASES = {"Moose Factory": "MooseFactory"}


def pod_of(rec):
    p = (rec.get("pod") or "").strip()
    return POD_ALIASES.get(p, p) or None


def main():
    links = load_jsonl("issues_links.jsonl")
    cycles = load_jsonl("cycle_times.jsonl")
    wip = load_jsonl("wip.jsonl")
    key_pods = load_jsonl("key_pods.jsonl") + load_jsonl("key_pods_extra.jsonl")

    pod_by_key = {}
    for rec in cycles + links + key_pods:
        p = pod_of(rec)
        if p:
            pod_by_key[rec["key"]] = p

    # deduplicated (blocker, blocked) pairs -> pod edges
    pairs = set()
    for rec in links:
        for k in rec.get("blocks", []):
            pairs.add((rec["key"], k))
        for k in rec.get("blockedBy", []):
            pairs.add((k, rec["key"]))
    edge_counts = Counter()
    unknown = 0
    for blocker, blocked in pairs:
        pa, pb = pod_by_key.get(blocker), pod_by_key.get(blocked)
        if not pa or not pb:
            unknown += 1
            continue
        if pa != pb:
            edge_counts[(pa, pb)] += 1
    # deterministic order (set iteration isn't stable across runs): count desc,
    # then names — so `make data` reproduces byte-identical output
    edges = [{"from": a, "to": b, "count": c}
             for (a, b), c in sorted(edge_counts.items(), key=lambda kv: (-kv[1], kv[0][0], kv[0][1]))]
    OUT.joinpath("edges.json").write_text(json.dumps(edges, indent=1))

    # cycle times per pod. created->resolved overstates flow time for tickets
    # that aged in the backlog, so: drop epics, drop >180d records (bulk
    # backlog-cleanup artifacts), winsorize the rest at the pod's p95.
    EXCLUDE_TYPES = {"Epic", "Parent Epic"}
    MAX_DAYS = 180
    by_pod = defaultdict(list)
    excluded = 0
    for rec in cycles:
        p = pod_of(rec)
        if not p or not rec.get("resolved") or not rec.get("created"):
            continue
        if rec.get("issuetype") in EXCLUDE_TYPES:
            excluded += 1
            continue
        try:
            c = datetime.fromisoformat(rec["created"])
            r = datetime.fromisoformat(rec["resolved"])
        except ValueError:
            continue
        days = max((r - c).total_seconds() / 86400, 0.25)
        if days > MAX_DAYS:
            excluded += 1
            continue
        by_pod[p].append(days)
    for p, xs in by_pod.items():
        xs.sort()
        cap = xs[int(0.95 * (len(xs) - 1))]
        by_pod[p] = [min(x, cap) for x in xs]
    print(f"excluded {excluded} records (epics or >{MAX_DAYS}d backlog artifacts)")

    wip_counts = Counter(pod_of(r) for r in wip if pod_of(r))

    def pct(xs, q):
        xs = sorted(xs)
        idx = (q / 100) * (len(xs) - 1)
        lo, hi = math.floor(idx), math.ceil(idx)
        return xs[lo] + (xs[hi] - xs[lo]) * (idx - lo)

    stats = {}
    for p, xs in by_pod.items():
        logs = [math.log(x) for x in xs]
        mu = sum(logs) / len(logs)
        sigma = math.sqrt(sum((l - mu) ** 2 for l in logs) / len(logs)) or 0.1
        stats[p] = {
            "resolved_count_180d": len(xs),
            "cycle_time_days": {
                "p50": round(pct(xs, 50), 2),
                "p85": round(pct(xs, 85), 2),
                "mean": round(sum(xs) / len(xs), 2),
            },
            "lognormal": {"mu": round(mu, 4), "sigma": round(sigma, 4)},
            "wip_count": wip_counts.get(p, 0),
            "throughput_per_week": round(len(xs) / 26, 2),
        }
    # pods with WIP but nothing resolved still deserve an entry
    for p, w in wip_counts.items():
        stats.setdefault(p, {
            "resolved_count_180d": 0,
            "cycle_time_days": {"p50": 7.0, "p85": 17.0, "mean": 10.0},
            "lognormal": {"mu": math.log(7), "sigma": 0.9},
            "wip_count": w,
            "throughput_per_week": 0.0,
        })
    OUT.joinpath("pod_stats.json").write_text(json.dumps(stats, indent=1))

    print(f"issues with links: {len(links)}, cycle records: {len(cycles)}, wip records: {len(wip)}")
    print(f"dedup link pairs: {len(pairs)}, dropped (unknown pod): {unknown}")
    print(f"cross-pod edges: {len(edges)}, pods with stats: {len(stats)}")
    print("top 10 edges:")
    for e in edges[:10]:
        print(f"  {e['from']} -> {e['to']}: {e['count']}")
    print("top 5 WIP:", wip_counts.most_common(5))


if __name__ == "__main__":
    main()
