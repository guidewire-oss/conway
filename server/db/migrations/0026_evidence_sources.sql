-- +goose Up
-- specs/026-reliable-evidence-foundation.md:128
CREATE TABLE evidence_sources (
 id text PRIMARY KEY, owner text NOT NULL, config jsonb NOT NULL,
 credential bytea NOT NULL, version bigint NOT NULL DEFAULT 1,
 next_at bigint NOT NULL, active_run text NOT NULL DEFAULT '', lease_until bigint NOT NULL DEFAULT 0,
 last_success bigint NOT NULL DEFAULT 0, last_snapshot text NOT NULL DEFAULT '',
 last_status text NOT NULL DEFAULT 'idle', last_error text NOT NULL DEFAULT ''
);
CREATE TABLE evidence_runs (
 id text PRIMARY KEY, run_order bigserial NOT NULL, source_id text NOT NULL REFERENCES evidence_sources(id) ON DELETE CASCADE,
 status text NOT NULL CHECK(status IN ('running','succeeded','failed','interrupted')),
 started_at bigint NOT NULL, finished_at bigint NOT NULL DEFAULT 0,
 snapshot_id text NOT NULL DEFAULT '', error text NOT NULL DEFAULT '',
 config jsonb NOT NULL, version bigint NOT NULL
);
CREATE INDEX evidence_run_history ON evidence_runs(source_id,run_order DESC);
CREATE INDEX evidence_snapshot ON evidence_runs(snapshot_id) WHERE snapshot_id <> '';
CREATE INDEX evidence_due ON evidence_sources(next_at);
-- +goose Down
DROP TABLE evidence_runs;
DROP TABLE evidence_sources;
