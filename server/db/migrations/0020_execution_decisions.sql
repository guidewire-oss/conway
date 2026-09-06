-- +goose Up
-- specs/017-planning-and-execution-usability.md:90: append-only review history.
CREATE TABLE plan_execution_decisions (
 id text PRIMARY KEY,
 plan_id text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 action text NOT NULL,
 owner text NOT NULL,
 review_date text NOT NULL,
 rationale text NOT NULL,
 initiative text NOT NULL DEFAULT '',
 snapshot_id text NOT NULL DEFAULT '',
 baseline_id text NOT NULL DEFAULT '',
 created_by text NOT NULL,
 created_at bigint NOT NULL
);
CREATE INDEX plan_execution_decisions_plan ON plan_execution_decisions(plan_id,created_at DESC);
-- +goose Down
DROP TABLE plan_execution_decisions;
