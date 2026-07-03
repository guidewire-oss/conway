-- +goose Up
CREATE TABLE IF NOT EXISTS games (
  id          text PRIMARY KEY,
  owner       text NOT NULL,
  name        text NOT NULL DEFAULT '',
  join_code   text UNIQUE,
  rounds      int    NOT NULL DEFAULT 4,
  ap          int    NOT NULL DEFAULT 5,
  timer_secs  int    NOT NULL DEFAULT 300,
  open        boolean NOT NULL DEFAULT false,
  open_round  int    NOT NULL DEFAULT 0,
  deadline    bigint NOT NULL DEFAULT 0,
  created_at  bigint NOT NULL DEFAULT 0,
  expires_at  bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS games_owner_idx ON games(owner);

-- +goose Down
DROP TABLE IF EXISTS games;
