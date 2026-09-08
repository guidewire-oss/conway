# Fit before evaluating later work

Retrospective model evaluation needs separate authorities for fitting and
scoring. Today's initiative inputs must not change a historical fit, and later
overlap must not improve an earlier representative. Fit from the earlier
capture and archived prediction inputs; use later evidence only for test scores.
Keep pending, excluded and repeated work outside completed-outcome denominators.

Provenance: specs/030-forecast-model-evaluation.md:135 records the decision.
Observed 2026-09-07 through the model-evaluation Ginkgo cases and the isolated
API case in `server/forecast_integration_test.go`: changing current plan inputs
retains the same report, while future-only outcome changes retain the fit.
The lesson points to that specification rather than defining a second model.
