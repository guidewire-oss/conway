-- +goose Up
-- SRE/platform-reliability classification is per-org (not derivable from
-- Jira), set via the pod directory / roster upload — see world.go's use of
-- this to seed the game engine's per-pod IsSre flag.
ALTER TABLE snapshot_pods ADD COLUMN IF NOT EXISTS sre boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE snapshot_pods DROP COLUMN IF EXISTS sre;
