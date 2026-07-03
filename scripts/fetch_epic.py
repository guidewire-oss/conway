#!/usr/bin/env python3
"""Fetch an epic's children from Jira Cloud and write app/data/epics/<KEY>.json.

Self-serve alternative to syncing via Claude. Requires a Jira API token:
  export JIRA_BASE_URL=https://yourorg.atlassian.net
  export JIRA_EMAIL=you@yourorg.com
  export JIRA_API_TOKEN=...   (create at id.atlassian.com -> Security -> API tokens)
Usage: python3 scripts/fetch_epic.py ABC-98848
"""
import json
import os
import sys
import urllib.parse
import urllib.request
from base64 import b64encode
from pathlib import Path

from epic_to_json import convert

BASE = os.environ.get("JIRA_BASE_URL", "https://yourorg.atlassian.net")
FIELDS = "summary,status,issuetype,customfield_10026,customfield_10014,issuelinks"


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    epic_key = sys.argv[1].upper()
    email = os.environ.get("JIRA_EMAIL")
    token = os.environ.get("JIRA_API_TOKEN")
    if not email or not token:
        sys.exit("set JIRA_EMAIL and JIRA_API_TOKEN (see script docstring)")

    jql = f'parent = {epic_key} OR "Epic Link" = {epic_key} ORDER BY created ASC'
    url = (f"{BASE}/rest/api/3/search/jql?jql={urllib.parse.quote(jql)}"
           f"&fields={FIELDS}&maxResults=100")
    auth = b64encode(f"{email}:{token}".encode()).decode()

    nodes = []
    next_token = None
    while True:
        page_url = url + (f"&nextPageToken={next_token}" if next_token else "")
        req = urllib.request.Request(page_url, headers={
            "Authorization": f"Basic {auth}", "Accept": "application/json"})
        with urllib.request.urlopen(req) as resp:
            data = json.load(resp)
        nodes.extend(data.get("issues", []))
        next_token = data.get("nextPageToken")
        if not next_token or data.get("isLast", True):
            break

    root = Path(__file__).resolve().parent.parent
    raw = root / "data" / f"raw_epic_{epic_key}.json"
    raw.write_text(json.dumps({"issues": {"nodes": nodes}}))
    convert(raw, epic_key, root / "app" / "data" / "epics")
    raw.unlink()


if __name__ == "__main__":
    main()
