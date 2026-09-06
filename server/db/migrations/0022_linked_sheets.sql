-- +goose Up
-- specs/023-linked-google-sheets.md:220: immutable observations, separate applications.
CREATE TABLE plan_sheet_sources (
 id text PRIMARY KEY,
 plan_id text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK (kind IN ('teams','initiatives')),
 status text NOT NULL CHECK (status IN ('active','paused','disconnected')),
 data jsonb NOT NULL,
 lease_token text NOT NULL DEFAULT '',
 lease_until bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX plan_sheet_sources_live_kind ON plan_sheet_sources(plan_id,kind) WHERE status <> 'disconnected';
CREATE TABLE plan_sheet_versions (
 id text PRIMARY KEY,
 source_id text NOT NULL REFERENCES plan_sheet_sources(id) ON DELETE CASCADE,
 captured_at bigint NOT NULL,
 data jsonb NOT NULL
);
CREATE INDEX plan_sheet_versions_source ON plan_sheet_versions(source_id,captured_at DESC,id);
CREATE TABLE plan_sheet_applications (
 id text PRIMARY KEY,
 source_id text NOT NULL REFERENCES plan_sheet_sources(id) ON DELETE CASCADE,
 version_id text NOT NULL REFERENCES plan_sheet_versions(id),
 applied_at bigint NOT NULL,
 data jsonb NOT NULL
);
-- +goose Down
DROP TABLE plan_sheet_applications;
DROP TABLE plan_sheet_versions;
DROP TABLE plan_sheet_sources;
