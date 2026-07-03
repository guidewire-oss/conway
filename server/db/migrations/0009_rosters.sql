-- +goose Up
-- Rosters are reusable, editable team-structure definitions (pods: name, site,
-- pairing, headcount, lanes). A Jira import associates with one; the snapshot
-- records which, so the association can be changed later.
CREATE TABLE IF NOT EXISTS rosters (
  id         text PRIMARY KEY,
  owner      text NOT NULL DEFAULT '',
  name       text NOT NULL DEFAULT '',
  pods       jsonb,
  created_at bigint NOT NULL DEFAULT 0,
  updated_at bigint NOT NULL DEFAULT 0
);

ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS roster_id text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE snapshots DROP COLUMN IF EXISTS roster_id;
DROP TABLE IF EXISTS rosters;
