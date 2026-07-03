-- +goose Up
-- Visibility: a manager can publish a snapshot so facilitators can seed games
-- from it; facilitators can publish their templates for other facilitators.
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS public boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE snapshots DROP COLUMN IF EXISTS public;
