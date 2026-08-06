-- +goose Up
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS sso boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE accounts DROP COLUMN IF EXISTS sso;
