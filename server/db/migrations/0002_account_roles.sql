-- +goose Up
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS roles text[];

-- +goose Down
ALTER TABLE accounts DROP COLUMN IF EXISTS roles;
