-- +goose Up
-- Rosters get the same visibility model as snapshots: private to the owner
-- unless shared (public), so another manager can reuse a shared roster.
ALTER TABLE rosters ADD COLUMN IF NOT EXISTS public boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE rosters DROP COLUMN IF EXISTS public;
