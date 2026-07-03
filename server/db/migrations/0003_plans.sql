-- +goose Up
CREATE TABLE IF NOT EXISTS plans (
  id            text PRIMARY KEY,
  owner         text NOT NULL,
  name          text NOT NULL DEFAULT '',
  horizon_weeks double precision NOT NULL DEFAULT 26,
  capacity_loss double precision NOT NULL DEFAULT 0.10,
  teams         jsonb,
  initiatives   jsonb,
  created_at    bigint NOT NULL DEFAULT 0,
  updated_at    bigint NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS plans;
