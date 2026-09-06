-- +goose Up
-- specs/022-feature-announcements.md:164: independent, per-account history.
-- No account FK: saving the auth store replaces account rows in a transaction.
CREATE TABLE feature_announcement_state (
  subject text NOT NULL,
  feature_id text NOT NULL,
  announced_at timestamptz,
  visited_at timestamptz,
  PRIMARY KEY (subject, feature_id)
);

-- +goose Down
DROP TABLE feature_announcement_state;
