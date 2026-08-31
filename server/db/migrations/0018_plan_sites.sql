-- +goose Up
-- The per-plan site table (spec 003): each site a plan's roster references,
-- with its IANA timezone and working-hours window. One jsonb blob, like teams
-- and scheduling in this same table — read whole into ComputeSchedule's
-- consumers and written whole by the sites editor. Nothing queries inside it.
--
-- Nullable rather than NOT NULL DEFAULT '[]': a plan imported before this
-- migration has no sites yet, and an absent blob reads as "everything
-- defaulted", which is exactly the honest state.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS sites jsonb;

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS sites;
