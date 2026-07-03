#!/usr/bin/env python3
"""Collect linked issue keys whose pod is unknown.

Reads issues_links.jsonl and cycle_times.jsonl (and key_pods.jsonl if present),
finds all in-project linked keys (blocks + blockedBy) not present with a known
pod, counts links pointing outside the tracked project(s), and writes batches
of 50 keys as JQL lines to unknown_batches.txt.
"""
import json
import os

DATA = os.path.dirname(os.path.abspath(__file__))

known = set()
linked = set()
non_project = 0

# "in project" = shares a key prefix with the crawled records themselves, so
# this works for whatever project(s) you mined — not just one org's.
in_project_prefixes = set()
with open(os.path.join(DATA, "issues_links.jsonl")) as fh:
    for line in fh:
        rec = json.loads(line)
        if "-" in rec.get("key", ""):
            in_project_prefixes.add(rec["key"].split("-")[0])

with open(os.path.join(DATA, "issues_links.jsonl")) as fh:
    for line in fh:
        rec = json.loads(line)
        if rec.get("pod"):
            known.add(rec["key"])
        for k in rec.get("blocks", []) + rec.get("blockedBy", []):
            if k.split("-")[0] in in_project_prefixes:
                linked.add(k)
            else:
                non_project += 1

with open(os.path.join(DATA, "cycle_times.jsonl")) as fh:
    for line in fh:
        rec = json.loads(line)
        if rec.get("pod"):
            known.add(rec["key"])

kp = os.path.join(DATA, "key_pods.jsonl")
if os.path.exists(kp):
    with open(kp) as fh:
        for line in fh:
            rec = json.loads(line)
            known.add(rec["key"])

unknown = sorted(linked - known)
batches = [unknown[i:i + 50] for i in range(0, len(unknown), 50)][:20]

with open(os.path.join(DATA, "unknown_batches.txt"), "w") as out:
    for b in batches:
        out.write("key in (" + ", ".join(b) + ")\n")

print(f"linked_in_project={len(linked)} known={len(known)} unknown={len(unknown)} "
      f"batches={len(batches)} non_project_links={non_project}")
