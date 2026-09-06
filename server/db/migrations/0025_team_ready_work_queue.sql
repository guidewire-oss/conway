-- +goose Up
-- specs/025-team-ready-work-queue.md:352: immutable decisions use monotonic ordering.
CREATE TABLE plan_ready_queue_events (
 id text PRIMARY KEY,
 event_order bigserial NOT NULL,
 plan_id text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 team text NOT NULL CHECK(length(btrim(team))>0),
 initiative text NOT NULL CHECK(length(btrim(initiative))>0),
 kind text NOT NULL CHECK(kind IN ('confirmation','release','defer','reconsider')),
 created_at bigint NOT NULL,
 data jsonb NOT NULL
);
CREATE INDEX ready_queue_latest ON plan_ready_queue_events(plan_id,event_order DESC);
CREATE INDEX ready_queue_history ON plan_ready_queue_events(plan_id,team,initiative,event_order);

-- +goose Down
DROP TABLE plan_ready_queue_events;
