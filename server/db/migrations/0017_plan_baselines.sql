-- +goose Up
-- The agreed execution order for a period, frozen with the inputs that produced
-- it (spec 001 Story 7, FR-029 to FR-033, §11 Decision 27).
--
-- inputs and schedule are jsonb blobs rather than normalised rows for the same
-- reason plans.scheduling is: nothing queries inside them. A baseline is written
-- whole, read whole and compared whole. What it must do is reproduce its own
-- schedule from its own inputs a quarter later, which is a property of storing
-- them together, not of their shape.
CREATE TABLE IF NOT EXISTS plan_baselines (
  id          text PRIMARY KEY,
  plan_id     text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  name        text NOT NULL DEFAULT '',
  active      boolean NOT NULL DEFAULT false,
  created_at  bigint NOT NULL DEFAULT 0,
  created_by  text NOT NULL DEFAULT '',
  -- SHA-256 of the canonical JSON of inputs; lets the plan's current inputs be
  -- flagged as diverged without re-reading the blob (FR-030).
  fingerprint text NOT NULL DEFAULT '',
  -- The baseline this one superseded: the active one at the time it was saved.
  compared_to text NOT NULL DEFAULT '',
  inputs      jsonb,
  schedule    jsonb
);

CREATE INDEX IF NOT EXISTS plan_baselines_plan_idx ON plan_baselines (plan_id, created_at DESC);

-- FR-031's "exactly one marked active" as a database guarantee rather than a code
-- convention: a bug in the activate path fails loudly instead of leaving a plan
-- with two answers to "what are actuals measured against".
CREATE UNIQUE INDEX IF NOT EXISTS plan_baselines_one_active_idx
  ON plan_baselines (plan_id) WHERE active;

-- +goose Down
DROP TABLE IF EXISTS plan_baselines;
