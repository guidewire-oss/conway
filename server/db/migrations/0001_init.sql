-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
  username   text PRIMARY KEY,
  display    text   NOT NULL DEFAULT '',
  role       text   NOT NULL DEFAULT 'player',
  salt       text   NOT NULL,
  hash       text   NOT NULL,
  expires_at bigint NOT NULL DEFAULT 0,
  created_at bigint NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS app_kv (
  key text PRIMARY KEY,
  val bytea
);

-- +goose Down
DROP TABLE IF EXISTS app_kv;
DROP TABLE IF EXISTS accounts;
