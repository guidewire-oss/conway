-- +goose Up
ALTER TABLE games ADD COLUMN IF NOT EXISTS scenario text NOT NULL DEFAULT 'default';

-- +goose Down
ALTER TABLE games DROP COLUMN IF EXISTS scenario;
