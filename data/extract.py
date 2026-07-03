#!/usr/bin/env python3
"""Extract compact records from a saved Jira search result file (JSON)
and append them to a JSONL file.

Usage: extract.py <mode> <input_file> <output_jsonl>
  mode: links | cycle | wip | keypods

Prints pagination info to stdout: "hasNextPage=<bool> endCursor=<token> count=<n>"
"""
import json
import sys


def pod_of(node):
    f = node.get("fields", {}) or {}
    cf = f.get("customfield_10026")
    if isinstance(cf, dict):
        return cf.get("value")
    return None


def extract_links(node):
    f = node.get("fields", {}) or {}
    blocks = []
    blocked_by = []
    for link in f.get("issuelinks") or []:
        ltype = (link.get("type") or {}).get("name")
        if ltype != "Blocks":
            continue
        out = link.get("outwardIssue")
        inw = link.get("inwardIssue")
        if out and out.get("key"):
            blocks.append(out["key"])
        if inw and inw.get("key"):
            blocked_by.append(inw["key"])
    return {
        "key": node.get("key"),
        "pod": pod_of(node),
        "created": f.get("created"),
        "resolved": f.get("resolutiondate"),
        "status": (f.get("status") or {}).get("name"),
        "issuetype": (f.get("issuetype") or {}).get("name"),
        "summary": f.get("summary"),
        "blocks": blocks,
        "blockedBy": blocked_by,
    }


def extract_cycle(node):
    f = node.get("fields", {}) or {}
    return {
        "key": node.get("key"),
        "pod": pod_of(node),
        "created": f.get("created"),
        "resolved": f.get("resolutiondate"),
        "issuetype": (f.get("issuetype") or {}).get("name"),
    }


def extract_wip(node):
    return {"key": node.get("key"), "pod": pod_of(node)}


def extract_keypods(node):
    return {"key": node.get("key"), "pod": pod_of(node)}


EXTRACTORS = {
    "links": extract_links,
    "cycle": extract_cycle,
    "wip": extract_wip,
    "keypods": extract_keypods,
}


def find_issues_block(data):
    """Locate {nodes: [...], pageInfo: {...}} in possibly-wrapped JSON."""
    if isinstance(data, dict):
        if "nodes" in data and "pageInfo" in data:
            return data
        if "issues" in data:
            return find_issues_block(data["issues"])
        # some wrappers nest under other keys
        for v in data.values():
            r = find_issues_block(v)
            if r is not None:
                return r
    elif isinstance(data, list):
        for v in data:
            r = find_issues_block(v)
            if r is not None:
                return r
    return None


def main():
    mode, infile, outfile = sys.argv[1], sys.argv[2], sys.argv[3]
    with open(infile) as fh:
        raw = fh.read()
    try:
        data = json.loads(raw)
    except json.JSONDecodeError:
        # file may contain leading non-JSON text; find first { or [
        idx = min([i for i in (raw.find("{"), raw.find("[")) if i >= 0])
        data = json.loads(raw[idx:])

    block = find_issues_block(data)
    if block is None:
        print("ERROR: could not find issues block", file=sys.stderr)
        sys.exit(1)

    nodes = block.get("nodes") or []
    extractor = EXTRACTORS[mode]
    with open(outfile, "a") as out:
        for node in nodes:
            out.write(json.dumps(extractor(node), separators=(",", ":")) + "\n")

    pi = block.get("pageInfo") or {}
    print(f"hasNextPage={pi.get('hasNextPage')} endCursor={pi.get('endCursor')} count={len(nodes)}")


if __name__ == "__main__":
    main()
