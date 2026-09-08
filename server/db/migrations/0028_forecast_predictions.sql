-- +goose Up
-- specs/028-portfolio-forecasts.md:194: immutable plan-scoped prediction history.
CREATE TABLE plan_forecast_predictions (
 id text NOT NULL,
 plan_id text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 recorded_order bigserial NOT NULL,
 issued_at bigint NOT NULL,
 name text NOT NULL,
 snapshot_id text NOT NULL,
 request_hash text NOT NULL,
 data jsonb NOT NULL,
 PRIMARY KEY(plan_id,id)
);
CREATE INDEX forecast_prediction_history ON plan_forecast_predictions(plan_id,recorded_order DESC);

-- +goose Down
DROP TABLE plan_forecast_predictions;
