-- +goose Up
CREATE TABLE IF NOT EXISTS game_teams (
  game_id    text NOT NULL,
  name       text NOT NULL,
  code       text UNIQUE,
  created_at bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (game_id, name)
);

-- +goose Down
DROP TABLE IF EXISTS game_teams;
