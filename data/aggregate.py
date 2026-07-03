#!/usr/bin/env python3
"""Aggregate Jira crawl data into edges.json, pod_stats.json and
mining_summary.md."""
import json
import math
import os
from collections import Counter
from datetime import datetime

DATA = os.path.dirname(os.path.abspath(__file__))


def _load_org_pods():
    # the roster, as already built by build_pods.py (runs first in `make data`)
    # from your pod-directory CSV — used here only for the mining-summary
    # diagnostic (pods with no Jira presence / unexpected pods in the data).
    path = os.path.join(DATA, "..", "app", "data", "pods.json")
    try:
        with open(path) as fh:
            return [p["name"] for p in json.load(fh)["pods"]]
    except (FileNotFoundError, KeyError):
        return []


ORG_PODS = _load_org_pods()


def read_jsonl(name):
    path = os.path.join(DATA, name)
    if not os.path.exists(path):
        return []
    out = []
    with open(path) as fh:
        for line in fh:
            line = line.strip()
            if line:
                out.append(json.loads(line))
    return out


def parse_dt(s):
    # e.g. 2025-06-11T12:34:56.000-0700
    return datetime.strptime(s[:19], "%Y-%m-%dT%H:%M:%S")


def percentile(sorted_vals, p):
    if not sorted_vals:
        return None
    k = (len(sorted_vals) - 1) * p
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return sorted_vals[int(k)]
    return sorted_vals[f] + (sorted_vals[c] - sorted_vals[f]) * (k - f)


links = read_jsonl("issues_links.jsonl")
cycles = read_jsonl("cycle_times.jsonl")
wip = read_jsonl("wip.jsonl")
key_pods = read_jsonl("key_pods.jsonl") + read_jsonl("key_pods_extra.jsonl")

# ---- pod lookup ----
pod_of = {}
for rec in cycles:
    if rec.get("pod"):
        pod_of[rec["key"]] = rec["pod"]
for rec in key_pods:
    if rec.get("pod"):
        pod_of[rec["key"]] = rec["pod"]
for rec in links:
    if rec.get("pod"):
        pod_of[rec["key"]] = rec["pod"]

# ---- edges ----
# Deduplicated (blockerKey, blockedKey) pairs; each link can appear from
# both ends. A link's target counts as "in project" if its key prefix matches
# one of the crawled projects (inferred from the crawled records' own keys,
# so this works for whatever project(s) you mined — not just one org's).
in_project_prefixes = {rec["key"].split("-")[0] for rec in links if "-" in rec["key"]}
pairs = set()
non_project_links = 0
all_linked = set()
for rec in links:
    k = rec["key"]
    for b in rec.get("blocks", []):
        if b.split("-")[0] not in in_project_prefixes:
            non_project_links += 1
            continue
        pairs.add((k, b))
        all_linked.add(b)
    for b in rec.get("blockedBy", []):
        if b.split("-")[0] not in in_project_prefixes:
            non_project_links += 1
            continue
        pairs.add((b, k))
        all_linked.add(b)

edge_counts = Counter()
pairs_with_pods = 0
pairs_cross_pod = 0
for blocker, blocked in pairs:
    p1 = pod_of.get(blocker)
    p2 = pod_of.get(blocked)
    if p1 and p2:
        pairs_with_pods += 1
        if p1 != p2:
            pairs_cross_pod += 1
            edge_counts[(p1, p2)] += 1

edges = [{"from": a, "to": b, "count": c} for (a, b), c in edge_counts.items()]
edges.sort(key=lambda e: (-e["count"], e["from"], e["to"]))
with open(os.path.join(DATA, "edges.json"), "w") as fh:
    json.dump(edges, fh, indent=2)

# ---- pod stats ----
cycle_days_by_pod = {}
resolved_count = Counter()
for rec in cycles:
    pod = rec.get("pod")
    if not pod or not rec.get("created") or not rec.get("resolved"):
        continue
    days = (parse_dt(rec["resolved"]) - parse_dt(rec["created"])).total_seconds() / 86400.0
    resolved_count[pod] += 1
    cycle_days_by_pod.setdefault(pod, []).append(days)

wip_count = Counter()
for rec in wip:
    if rec.get("pod"):
        wip_count[rec["pod"]] += 1

pod_stats = {}
for pod in sorted(set(list(resolved_count) + list(wip_count))):
    days = sorted(cycle_days_by_pod.get(pod, []))
    if days:
        logs = [math.log(max(d, 0.25)) for d in days]
        mu = sum(logs) / len(logs)
        var = sum((x - mu) ** 2 for x in logs) / len(logs)
        sigma = math.sqrt(var)
        ct = {
            "p50": round(percentile(days, 0.50), 2),
            "p85": round(percentile(days, 0.85), 2),
            "mean": round(sum(days) / len(days), 2),
        }
        lognorm = {"mu": round(mu, 4), "sigma": round(sigma, 4)}
    else:
        ct = {"p50": None, "p85": None, "mean": None}
        lognorm = {"mu": None, "sigma": None}
    pod_stats[pod] = {
        "resolved_count_180d": resolved_count.get(pod, 0),
        "cycle_time_days": ct,
        "lognormal": lognorm,
        "wip_count": wip_count.get(pod, 0),
        "throughput_per_week": round(resolved_count.get(pod, 0) / 26.0, 2),
    }

with open(os.path.join(DATA, "pod_stats.json"), "w") as fh:
    json.dump(pod_stats, fh, indent=2)

# ---- summary ----
linked_pod_unknown = sum(1 for k in all_linked if k not in pod_of)
links_unknown_pct = (100.0 * linked_pod_unknown / len(all_linked)) if all_linked else 0.0

jira_pods = set()
for rec in links + cycles + wip + key_pods:
    if rec.get("pod"):
        jira_pods.add(rec["pod"])
extra_pods = sorted(jira_pods - set(ORG_PODS))
zero_pods = sorted(set(ORG_PODS) - jira_pods)

top_edges = edges[:15]
top_wip = wip_count.most_common(10)

lines = []
lines.append("# Dependency Mining Summary")
lines.append("")
lines.append(f"Generated: {datetime.utcnow().strftime('%Y-%m-%d %H:%M UTC')}")
lines.append("")
lines.append("## Crawl totals")
lines.append("")
lines.append("| Crawl | JQL scope | Records | Pages | Cap hit |")
lines.append("|---|---|---|---|---|")
lines.append(f"| Linked issues (deps) | blocks/is-blocked-by, updated >= -365d | {len(links)} | 14 | no (exhausted) |")
lines.append(f"| Cycle times | resolved >= -180d | {len(cycles)} | 60 | YES (60-page cap; more resolved issues remain) |")
lines.append(f"| Current WIP | statusCategory = In Progress | {len(wip)} | 9 | no (exhausted) |")
lines.append(f"| Key-pod resolution | unknown link targets | {len(key_pods)} | 5 batches | no (cap 20) |")
lines.append("")
lines.append(f"- Distinct pods seen in Jira data: {len(jira_pods)}")
lines.append(f"- Deduplicated blocking pairs (in-project): {len(pairs)}")
lines.append(f"- Pairs with both pods known: {pairs_with_pods}; cross-pod pairs: {pairs_cross_pod}")
lines.append(f"- Distinct cross-pod edges: {len(edges)}")
lines.append(f"- Link endpoints pointing outside the tracked project(s) (ignored): {non_project_links}")
lines.append("")
lines.append("## Top 15 cross-pod blocking edges (blocker -> blocked)")
lines.append("")
lines.append("| From (blocker) | To (blocked) | Count |")
lines.append("|---|---|---|")
for e in top_edges:
    lines.append(f"| {e['from']} | {e['to']} | {e['count']} |")
lines.append("")
lines.append("## Top 10 pods by current WIP")
lines.append("")
lines.append("| Pod | WIP |")
lines.append("|---|---|")
for pod, c in top_wip:
    lines.append(f"| {pod} | {c} |")
lines.append("")
lines.append("## Data-quality caveats")
lines.append("")
lines.append(f"- {links_unknown_pct:.1f}% of linked issue targets ({linked_pod_unknown}/{len(all_linked)}) still have unknown pod (no Assigned Pod set on the target issue); edges involving them are dropped.")
lines.append(f"- Cycle-times crawl hit the 60-page cap (6,000 issues); per-pod resolved counts and throughput are therefore a floor, biased toward the most recently resolved issues. Earlier-resolved issues in the 180d window are missing.")
lines.append(f"- Cycle time is created->resolved (lead time), not in-progress time; long-lived backlog items inflate the tail. Log values clamped at 0.25 days for lognormal fit.")
lines.append(f"- Pods present in Jira but not in the org directory list ({len(extra_pods)}): {', '.join(extra_pods) if extra_pods else 'none'}. These are likely legacy/renamed pods.")
lines.append(f"- Org-directory pods with zero Jira data in these crawls ({len(zero_pods)}): {', '.join(zero_pods) if zero_pods else 'none'}.")
lines.append("- Pod values are NOT normalized: 'Moose Factory' (with space) and 'Sagara ' (trailing space) appear in Jira and are spelling variants of org pods MooseFactory/Sagara, so those two are not truly zero-data; consumers should normalize whitespace before joining to the org directory.")
lines.append("- Links crawl covers issues updated in the last 365d only; older dormant dependencies are excluded.")
lines.append("- An issue's pod is the pod currently assigned, which may differ from the pod that did the work at link time.")
lines.append("")

with open(os.path.join(DATA, "mining_summary.md"), "w") as fh:
    fh.write("\n".join(lines))

print(f"edges={len(edges)} pairs={len(pairs)} cross_pod_pairs={pairs_cross_pod} "
      f"pods={len(jira_pods)} unknown_linked={linked_pod_unknown}/{len(all_linked)} "
      f"non_project={non_project_links}")
print("TOP5 EDGES: " + "; ".join(f"{e['from']}->{e['to']}={e['count']}" for e in edges[:5]))
print("TOP5 WIP: " + "; ".join(f"{p}={c}" for p, c in top_wip[:5]))
print("EXTRA PODS: " + ", ".join(extra_pods))
print("ZERO PODS: " + ", ".join(zero_pods))
