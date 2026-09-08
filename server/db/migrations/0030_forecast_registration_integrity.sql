-- +goose Up
-- specs/031-prospective-forecast-registration.md:165: preserve plan-only removal.
ALTER TABLE plan_forecast_registrations
 DROP CONSTRAINT plan_forecast_registrations_plan_id_reference_id_fkey,
 ADD CONSTRAINT forecast_registration_plan_fk FOREIGN KEY(plan_id)
 REFERENCES plans(id) ON DELETE CASCADE,
 ADD CONSTRAINT forecast_registration_prediction_fk FOREIGN KEY(plan_id,reference_id)
 REFERENCES plan_forecast_predictions(plan_id,id)
 ON DELETE NO ACTION DEFERRABLE INITIALLY IMMEDIATE;

CREATE INDEX forecast_registration_reference ON plan_forecast_registrations(plan_id,reference_id);

-- +goose Down
DROP INDEX forecast_registration_reference;
ALTER TABLE plan_forecast_registrations
 DROP CONSTRAINT forecast_registration_plan_fk,
 DROP CONSTRAINT forecast_registration_prediction_fk,
 ADD CONSTRAINT plan_forecast_registrations_plan_id_reference_id_fkey
 FOREIGN KEY(plan_id,reference_id) REFERENCES plan_forecast_predictions(plan_id,id) ON DELETE CASCADE;
