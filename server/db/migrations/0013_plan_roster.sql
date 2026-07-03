-- +goose Up
-- A plan's team structure can come from a saved roster instead of a manual
-- CSV/XLSX upload — pods are copied into plans.teams at association time (a
-- frozen snapshot of that roster, matching how a Jira import pins its roster)
-- so later edits to the roster don't silently drift the plan. roster_id is
-- kept purely as a reference/label, same convention as snapshots.roster_id.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS roster_id text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS roster_id;
