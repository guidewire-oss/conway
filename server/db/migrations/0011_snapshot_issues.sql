-- +goose Up
-- Issue-level data lives in queryable tables (not JSON blobs), so the heavy
-- views (fever, WIP freeze, hygiene, epics) can be filtered/paginated in SQL.
CREATE TABLE IF NOT EXISTS snapshot_issues (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  key         text NOT NULL,
  pod         text NOT NULL DEFAULT '',
  issue_type  text NOT NULL DEFAULT '',
  status      text NOT NULL DEFAULT '',
  status_cat  text NOT NULL DEFAULT '',  -- new | indeterminate | done
  assignee    text NOT NULL DEFAULT '',
  points      double precision,
  summary     text NOT NULL DEFAULT '',
  desc_len    int NOT NULL DEFAULT 0,
  parent_key  text NOT NULL DEFAULT '',
  created     timestamptz,
  updated     timestamptz,
  resolved    timestamptz,
  due_date    text NOT NULL DEFAULT '',
  PRIMARY KEY (snapshot_id, key)
);
CREATE INDEX IF NOT EXISTS idx_issues_pod ON snapshot_issues (snapshot_id, pod);
CREATE INDEX IF NOT EXISTS idx_issues_parent ON snapshot_issues (snapshot_id, parent_key);
CREATE INDEX IF NOT EXISTS idx_issues_statuscat ON snapshot_issues (snapshot_id, status_cat);
CREATE INDEX IF NOT EXISTS idx_issues_type ON snapshot_issues (snapshot_id, issue_type);

CREATE TABLE IF NOT EXISTS snapshot_issue_links (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  blocker_key text NOT NULL,  -- this issue blocks…
  blocked_key text NOT NULL,  -- …this one (it waits on the blocker)
  PRIMARY KEY (snapshot_id, blocker_key, blocked_key)
);
CREATE INDEX IF NOT EXISTS idx_links_blocked ON snapshot_issue_links (snapshot_id, blocked_key);
CREATE INDEX IF NOT EXISTS idx_links_blocker ON snapshot_issue_links (snapshot_id, blocker_key);

-- Per-pod structure (mutable via roster re-association) and the derived
-- aggregates materialized at import (immutable; functions of the issues).
CREATE TABLE IF NOT EXISTS snapshot_pods (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  name        text NOT NULL,
  location    text NOT NULL DEFAULT '',
  pairing     boolean NOT NULL DEFAULT true,
  dev_count   int NOT NULL DEFAULT 0,
  streams     int NOT NULL DEFAULT 0,   -- explicit work-lanes; 0 = derive
  PRIMARY KEY (snapshot_id, name)
);
CREATE TABLE IF NOT EXISTS snapshot_pod_stats (
  snapshot_id   text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  pod           text NOT NULL,
  resolved_180d int NOT NULL DEFAULT 0,
  p50 double precision NOT NULL DEFAULT 0,
  p85 double precision NOT NULL DEFAULT 0,
  mean double precision NOT NULL DEFAULT 0,
  mu double precision NOT NULL DEFAULT 0,
  sigma double precision NOT NULL DEFAULT 0,
  wip_count int NOT NULL DEFAULT 0,
  throughput_per_week double precision NOT NULL DEFAULT 0,
  PRIMARY KEY (snapshot_id, pod)
);
CREATE TABLE IF NOT EXISTS snapshot_edges (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  from_pod text NOT NULL,
  to_pod   text NOT NULL,
  count    int NOT NULL DEFAULT 0,
  PRIMARY KEY (snapshot_id, from_pod, to_pod)
);
CREATE TABLE IF NOT EXISTS snapshot_pod_hygiene (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  pod text NOT NULL,
  sized_pct double precision,
  median_points double precision,
  stale_wip_pct double precision,
  unassigned_wip_pct double precision,
  link_density double precision,
  score double precision,
  sample_sized int NOT NULL DEFAULT 0,
  wip_count int NOT NULL DEFAULT 0,
  PRIMARY KEY (snapshot_id, pod)
);

-- +goose Down
DROP TABLE IF EXISTS snapshot_pod_hygiene;
DROP TABLE IF EXISTS snapshot_edges;
DROP TABLE IF EXISTS snapshot_pod_stats;
DROP TABLE IF EXISTS snapshot_pods;
DROP TABLE IF EXISTS snapshot_issue_links;
DROP TABLE IF EXISTS snapshot_issues;
