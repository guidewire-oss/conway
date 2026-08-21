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
  -- NOT NULL: a baseline without these cannot reproduce its own schedule, which is
  -- the whole of FR-029. The save path always supplies both, so the constraint
  -- costs nothing and closes the case where a future one forgets.
  inputs      jsonb NOT NULL,
  schedule    jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS plan_baselines_plan_idx ON plan_baselines (plan_id, created_at DESC);

-- At most one active baseline per plan. This is half of FR-031's "exactly one":
-- an index can forbid a second, but cannot require a first. The other half is held
-- by the write paths — the first baseline saved is forced active, and moving the
-- flag clears and sets inside one transaction that rolls back if the target is not
-- there — so no path leaves a plan holding baselines with none active.
CREATE UNIQUE INDEX IF NOT EXISTS plan_baselines_one_active_idx
  ON plan_baselines (plan_id) WHERE active;

-- +goose Down
DROP TABLE IF EXISTS plan_baselines;
