-- +goose Up
-- How a snapshot counts "in-progress" work for the freeze panel and hygiene
-- stale/unassigned lists: 'leaf' (every non-epic issue, avoids conflating an
-- epic with its own active children) or 'epic_or_parentless' (an epic counts
-- as one unit; only its parentless siblings are counted individually).
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS wip_mode text NOT NULL DEFAULT 'leaf';

-- +goose Down
ALTER TABLE snapshots DROP COLUMN IF EXISTS wip_mode;
