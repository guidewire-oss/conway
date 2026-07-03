#!/usr/bin/env python3
"""Build app/data/hygiene.json: per-pod Jira data-quality metrics.

Each metric maps to a concrete model degradation, so teams know WHY it
matters, not just that a dashboard turned red.
"""
import json
import statistics
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
POD_ALIASES = {"Moose Factory": "MooseFactory"}
NOW = datetime.now(timezone.utc)


def load_jsonl(name):
    p = ROOT / "data" / name
    if not p.exists():
        return []
    return [json.loads(l) for l in p.read_text().splitlines() if l.strip()]


def pod_of(r):
    p = (r.get("pod") or "").strip()
    return POD_ALIASES.get(p, p) or None


def days_since(iso):
    if not iso:
        return None
    return (NOW - datetime.fromisoformat(iso)).total_seconds() / 86400


def main():
    children = load_jsonl("epic_children.jsonl")
    wip = load_jsonl("wip_detail.jsonl")
    links = load_jsonl("issues_links.jsonl")
    cycles = load_jsonl("cycle_times.jsonl")
    epic_meta = {r["key"]: r for r in load_jsonl("epic_meta.jsonl")}

    # owner pod of an epic = modal pod among its children
    epic_pods = defaultdict(lambda: defaultdict(int))
    for r in children:
        p = pod_of(r)
        if p and r.get("epic"):
            epic_pods[r["epic"]][p] += 1
    owner = {e: max(pods, key=pods.get) for e, pods in epic_pods.items()}

    LIST_CAP = 300
    issues = defaultdict(lambda: {
        "unsized": [], "stale": [], "unassigned": [], "nooutcome": [],
    })

    for key, m in epic_meta.items():
        if m.get("hasOutcome") is False:
            p = owner.get(key)
            if p and len(issues[p]["nooutcome"]) < LIST_CAP:
                issues[p]["nooutcome"].append({
                    "key": key, "summary": (m.get("name") or "")[:100],
                    "detail": "no business outcome in description"
                              + (f" · due {m['duedate']}" if m.get("duedate") else ""),
                })

    # full epic-meta map for the app (owner pod attached)
    meta_out = {k: {**m, "ownerPod": owner.get(k)} for k, m in epic_meta.items()}
    (ROOT / "app" / "data" / "epic_meta.json").write_text(json.dumps(meta_out, indent=1))

    sized = defaultdict(lambda: [0, 0])     # pod -> [sized, total]
    points = defaultdict(list)
    seen = set()
    for r in children:
        p = pod_of(r)
        if not p or r["key"] in seen:
            continue
        seen.add(r["key"])
        sized[p][1] += 1
        if r.get("points") is not None:
            sized[p][0] += 1
            points[p].append(r["points"])
        elif r.get("status") not in ("Closed", "Done", "Resolved") \
                and len(issues[p]["unsized"]) < LIST_CAP:
            issues[p]["unsized"].append({
                "key": r["key"], "summary": (r.get("summary") or "")[:100],
                "detail": f"epic {r.get('epic')}",
            })

    stale = defaultdict(lambda: [0, 0])     # pod -> [stale>14d, total wip]
    unassigned = defaultdict(int)
    for r in wip:
        p = pod_of(r)
        if not p:
            continue
        stale[p][1] += 1
        d = days_since(r.get("updated"))
        if d is not None and d > 14:
            stale[p][0] += 1
            if len(issues[p]["stale"]) < LIST_CAP:
                issues[p]["stale"].append({
                    "key": r["key"], "summary": (r.get("summary") or "")[:100],
                    "detail": f"untouched {d:.0f}d",
                })
        if not r.get("assignee"):
            unassigned[p] += 1
            if len(issues[p]["unassigned"]) < LIST_CAP:
                issues[p]["unassigned"].append({
                    "key": r["key"], "summary": (r.get("summary") or "")[:100],
                    "detail": "in progress, no assignee",
                })

    linked = defaultdict(set)
    for r in links:
        p = pod_of(r)
        if p:
            linked[p].add(r["key"])
    resolved = defaultdict(int)
    for r in cycles:
        p = pod_of(r)
        if p:
            resolved[p] += 1

    pods = set(sized) | set(stale) | set(linked) | set(resolved)
    out = {}
    for p in pods:
        s_done, s_tot = sized[p]
        st_stale, st_tot = stale[p]
        sized_pct = s_done / s_tot if s_tot else None
        stale_pct = st_stale / st_tot if st_tot else None
        unass_pct = unassigned[p] / st_tot if st_tot else None
        link_density = len(linked[p]) / resolved[p] if resolved[p] else None
        # score: average of the available components (sized, fresh, assigned)
        comps = []
        if sized_pct is not None:
            comps.append(sized_pct)
        if stale_pct is not None:
            comps.append(1 - stale_pct)
        if unass_pct is not None:
            comps.append(1 - unass_pct)
        out[p] = {
            "sizedPct": round(sized_pct, 3) if sized_pct is not None else None,
            "sampleSized": s_tot,
            "medianPoints": statistics.median(points[p]) if points[p] else None,
            "staleWipPct": round(stale_pct, 3) if stale_pct is not None else None,
            "unassignedWipPct": round(unass_pct, 3) if unass_pct is not None else None,
            "wipCount": st_tot,
            "linkDensity": round(link_density, 3) if link_density is not None else None,
            "score": round(sum(comps) / len(comps), 3) if comps else None,
        }

    path = ROOT / "app" / "data" / "hygiene.json"
    path.write_text(json.dumps(out, indent=1))
    issues_path = ROOT / "app" / "data" / "hygiene_issues.json"
    issues_path.write_text(json.dumps(issues, indent=1))
    n_issues = sum(len(v[c]) for v in issues.values() for c in v)
    print(f"wrote {issues_path}: {n_issues} problematic issues listed")
    scored = [(v["score"], k) for k, v in out.items() if v["score"] is not None]
    scored.sort()
    print(f"wrote {path}: {len(out)} pods")
    print("worst 5 hygiene:", [(k, s) for s, k in scored[:5]])
    print("best 3 hygiene:", [(k, s) for s, k in scored[-3:]])


if __name__ == "__main__":
    main()
