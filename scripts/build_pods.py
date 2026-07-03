#!/usr/bin/env python3
"""Build app/data/pods.json from the pod directory CSV."""
import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CSV_PATH = ROOT / "pod-directory-31cols-2026-06-11.csv"
OUT = ROOT / "app" / "data" / "pods.json"

# UTC offsets (June / DST where applicable)
SITE_TZ = {
    "San Mateo": -7.0,
    "Toronto": -4.0,
    "Birmingham": -5.0,
    "Kraków": 2.0,
    "Bengaluru": 5.5,
}
REMOTE = "*REMOTE - multicontinental*"
WORK_START, WORK_END = 9.0, 17.0


def overlap_hours(tz_a, tz_b):
    """Overlap of two 9-17 local workdays, in hours."""
    # express B's window in A's local clock
    shift = tz_a - tz_b
    starts = [WORK_START, WORK_START + shift]
    ends = [WORK_END, WORK_END + shift]
    best = 0.0
    # try B's window shifted by -24, 0, +24 (wrap around midnight)
    for day in (-24.0, 0.0, 24.0):
        lo = max(starts[0], starts[1] + day)
        hi = min(ends[0], ends[1] + day)
        best = max(best, hi - lo)
    return round(best, 1)


def main():
    pods = []
    with open(CSV_PATH, newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            name = (row.get("Pod Name") or "").strip()
            if not name:
                continue
            devs = [d.strip() for d in (row.get("Developers") or "").split(",") if d.strip()]
            loc = (row.get("Location") or "").strip()
            pods.append({
                "name": name,
                "area": (row.get("Work Areas") or "").strip(),
                "location": loc,
                "remote": loc == REMOTE,
                "tz": SITE_TZ.get(loc),
                "devCount": len(devs),
                # all pods pair-program except Kraków (Anoop, 2026-06-13)
                "pairing": loc != "Kraków",
                "category": (row.get("Individual Category") or "").strip(),
                "jira": (row.get("Jira Project") or "").strip(),
            })

    names = [p["name"] for p in pods]
    overlap = {}
    for a in pods:
        overlap[a["name"]] = {}
        for b in pods:
            if a["name"] == b["name"]:
                h = 8.0
            elif a["remote"] or b["remote"]:
                h = 2.0  # multicontinental remote: assume ~2h with anyone
            elif a["tz"] is not None and b["tz"] is not None:
                h = overlap_hours(a["tz"], b["tz"])
            else:
                h = 2.0
            overlap[a["name"]][b["name"]] = h

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps({"pods": pods, "overlap": overlap}, indent=1))
    print(f"wrote {OUT} with {len(pods)} pods")
    # sanity: print site pair overlaps
    seen = {}
    for p in pods:
        if p["tz"] is not None:
            seen[p["location"]] = p["tz"]
    sites = sorted(seen)
    for i, a in enumerate(sites):
        for b in sites[i + 1:]:
            print(f"  {a} <-> {b}: {overlap_hours(seen[a], seen[b])}h")


if __name__ == "__main__":
    sys.exit(main())
