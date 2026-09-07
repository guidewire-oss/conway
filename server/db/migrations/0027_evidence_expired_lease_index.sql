-- +goose Up
-- specs/026-reliable-evidence-foundation.md:128
CREATE INDEX evidence_expired_lease ON evidence_sources(lease_until) WHERE active_run <> '';
-- +goose Down
DROP INDEX evidence_expired_lease;
