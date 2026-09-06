-- +goose Up
-- specs/023-linked-google-sheets.md:308: provenance cannot cross source boundaries.
ALTER TABLE plan_sheet_versions
 ADD CONSTRAINT plan_sheet_versions_source_identity UNIQUE (source_id,id);
ALTER TABLE plan_sheet_applications
 ADD CONSTRAINT plan_sheet_applications_source_version
 FOREIGN KEY (source_id,version_id) REFERENCES plan_sheet_versions(source_id,id);

-- +goose Down
ALTER TABLE plan_sheet_applications DROP CONSTRAINT plan_sheet_applications_source_version;
ALTER TABLE plan_sheet_versions DROP CONSTRAINT plan_sheet_versions_source_identity;
