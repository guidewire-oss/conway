-- +goose Up
-- specs/024-weekly-execution-review.md:295: decisions retain their identity and text.
ALTER TABLE plan_execution_decisions ADD CONSTRAINT execution_decisions_plan_identity UNIQUE(plan_id,id);
CREATE TABLE plan_execution_action_state (
 decision_id text PRIMARY KEY,
 plan_id text NOT NULL,
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','in_progress','resolved','superseded')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 FOREIGN KEY(plan_id,decision_id) REFERENCES plan_execution_decisions(plan_id,id) ON DELETE CASCADE
);
INSERT INTO plan_execution_action_state(decision_id,plan_id) SELECT id,plan_id FROM plan_execution_decisions;
CREATE TABLE plan_execution_action_transitions (
 id text PRIMARY KEY,
 decision_id text NOT NULL,
 plan_id text NOT NULL,
 from_status text NOT NULL CHECK(from_status IN ('open','in_progress','resolved','superseded')),
 status text NOT NULL CHECK(status IN ('open','in_progress','resolved','superseded')),
 version bigint NOT NULL CHECK(version>1),
 evidence text NOT NULL,
 created_by text NOT NULL,
 created_at bigint NOT NULL,
 FOREIGN KEY(plan_id,decision_id) REFERENCES plan_execution_decisions(plan_id,id) ON DELETE CASCADE,
 UNIQUE(decision_id,version),
 CHECK(from_status<>status),
 CHECK(status NOT IN ('resolved','superseded') OR length(btrim(evidence))>0)
);
CREATE TABLE plan_execution_reviews (
 id text PRIMARY KEY,
 completion_order bigserial NOT NULL,
 plan_id text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 created_at bigint NOT NULL,
 data jsonb NOT NULL
);
CREATE INDEX execution_reviews_history ON plan_execution_reviews(plan_id,completion_order DESC);

-- +goose Down
DROP TABLE plan_execution_reviews;
DROP TABLE plan_execution_action_transitions;
DROP TABLE plan_execution_action_state;
ALTER TABLE plan_execution_decisions DROP CONSTRAINT execution_decisions_plan_identity;
