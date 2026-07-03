-- +goose Up
-- A snapshot is a dated, named capture of the org network (the data Observe
-- renders and Train seeds from). Its documents are stored generically as
-- path -> JSON blob, mirroring the files under app/data so one store serves
-- both pillars with no duplicated fetch path.
CREATE TABLE IF NOT EXISTS snapshots (
  id         text PRIMARY KEY,
  owner      text NOT NULL DEFAULT '',
  name       text NOT NULL DEFAULT '',
  scope      jsonb,                      -- selected Jira project keys (null for baseline)
  source     text NOT NULL DEFAULT '',   -- 'baseline' | 'jira' | 'plan'
  created_at bigint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS snapshot_docs (
  snapshot_id text NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  path        text NOT NULL,             -- e.g. 'pods.json', 'epics/index.json'
  body        jsonb NOT NULL,
  PRIMARY KEY (snapshot_id, path)
);

-- +goose Down
DROP TABLE IF EXISTS snapshot_docs;
DROP TABLE IF EXISTS snapshots;
