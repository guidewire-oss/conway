-- +goose Up
-- Usage analytics (spec 016): an append-only event stream, one row per
-- business event (login, plan created, schedule computed...). The analytics
-- page aggregates it; nothing reads it on the request path. jsonb meta for
-- per-event dimensions (counts, names) — read whole, written whole.
CREATE TABLE IF NOT EXISTS analytics_events (
  id BIGSERIAL PRIMARY KEY,
  ts TIMESTAMPTZ NOT NULL DEFAULT now(),
  username TEXT NOT NULL DEFAULT '',
  event TEXT NOT NULL,
  plan_id TEXT NOT NULL DEFAULT '',
  meta JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS analytics_events_ts_idx ON analytics_events (ts DESC);
CREATE INDEX IF NOT EXISTS analytics_events_event_idx ON analytics_events (event);
CREATE INDEX IF NOT EXISTS analytics_events_user_idx ON analytics_events (username);

-- +goose Down
DROP TABLE IF EXISTS analytics_events;
