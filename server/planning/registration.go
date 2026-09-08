package planning

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ForecastRegistration struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	RegisteredAt     int64                `json:"registeredAt"`
	CreatedBy        string               `json:"createdBy"`
	Model            string               `json:"model"`
	Probability      float64              `json:"probability"`
	Reference        ForecastPrediction   `json:"reference"`
	TrainingEvidence PredictionEvidence   `json:"trainingEvidence"`
	Training         EvaluationCohort     `json:"training"`
	History          []ForecastPrediction `json:"history"`
}

// specs/031-prospective-forecast-registration.md:112: freeze the fit and scope
// before any future test predictions; never refit a retained registration.
func RegisterForecastModel(reference ForecastPrediction, history []ForecastPrediction, evidence PredictionEvidence, issues []ExecutionIssue, at int64) (ForecastRegistration, error) {
	var out ForecastRegistration
	if evidence.StartedAt <= 0 || evidence.CapturedAt < evidence.StartedAt || evidence.CapturedAt >= at {
		return out, errors.New("the training capture must finish before registration")
	}
	fit, err := validatePredictionHistory(reference, history, nil, evidence, issues, true)
	if err != nil {
		return out, err
	}
	if fit.Eligible == 0 {
		return out, errors.New("no eligible completed training observations; choose a capture containing completed, unchanged scope")
	}
	retained := []ForecastPrediction{}
	entries := 0
	for _, p := range history {
		if ValidationMatches(reference, p) && p.IssuedAt <= at {
			entries += len(p.Inputs.Initiatives)
			if entries > 5000 {
				return ForecastRegistration{}, fmt.Errorf("%w: registration supports at most 5000 retained initiative entries", ErrValidationLimit)
			}
			retained = append(retained, p)
		}
	}
	out = ForecastRegistration{RegisteredAt: at, Model: "envelope-frequency-v1", Probability: float64(fit.Covered+1) / float64(fit.Eligible+2), Reference: reference, TrainingEvidence: evidence, Training: EvaluationCohort{ValidationCounts: fit.ValidationCounts, Rows: fit.Rows}, History: retained}
	// Own the archive even when a caller subsequently edits its in-memory inputs.
	data, err := json.Marshal(out)
	if err != nil {
		return ForecastRegistration{}, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}

func AssessRegisteredModel(model ForecastRegistration, history []ForecastPrediction, later PredictionEvidence, issues []ExecutionIssue) (ForecastEvaluation, error) {
	out := ForecastEvaluation{Model: model.Model, ReferenceID: model.Reference.ID, Settings: model.Reference.Forecast.Settings, TrainingEvidence: model.TrainingEvidence, TestEvidence: later, Training: model.Training, Test: EvaluationCohort{Rows: []ValidationRow{}}}
	if later.StartedAt <= model.RegisteredAt || later.CapturedAt < later.StartedAt {
		return out, errors.New("choose a capture started after registration")
	}
	if len(history) > MaxValidationRecords {
		return out, fmt.Errorf("%w: registered model assessment supports at most %d predictions", ErrValidationLimit, MaxValidationRecords)
	}
	combined := append([]ForecastPrediction{}, model.History...)
	known := map[string]bool{}
	for _, p := range combined {
		known[p.ID] = true
	}
	for _, p := range history {
		if !known[p.ID] {
			combined = append(combined, p)
			known[p.ID] = true
		}
	}
	all, err := validatePredictionHistory(model.Reference, combined, nil, later, issues, true)
	if err != nil {
		return out, err
	}
	out.TotalRecords, out.MismatchedRecords, out.TooLateRecords = all.TotalRecords, all.MismatchedRecords, all.TooLateRecords
	for _, row := range all.Rows {
		if row.IssuedAt > model.RegisteredAt {
			out.Test.Rows = append(out.Test.Rows, row)
			out.Test.count(row.Status)
		} else {
			out.BoundaryRecords++
		}
	}
	p := model.Probability
	out.Probability = &p
	if out.Test.Eligible > 0 {
		n, successes := float64(out.Test.Eligible), float64(out.Test.Covered)
		brier := (successes*(1-p)*(1-p) + (n-successes)*p*p) / n
		benchmark, gap := .25, 100*(successes/n-p)
		out.BrierScore, out.BenchmarkBrierScore, out.GapPercentagePoints = &brier, &benchmark, &gap
	}
	return out, nil
}
