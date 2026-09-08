-- +goose Up
-- specs/031-prospective-forecast-registration.md:112: retain registered fits.
CREATE TABLE plan_forecast_registrations (
 plan_id text NOT NULL,
 reference_id text NOT NULL,
 id text NOT NULL,
 request_hash text NOT NULL,
 data jsonb NOT NULL,
 PRIMARY KEY(plan_id,id),
 FOREIGN KEY(plan_id,reference_id) REFERENCES plan_forecast_predictions(plan_id,id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE plan_forecast_registrations;
