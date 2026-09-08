package planning

import "errors"

type EvaluationCohort struct {
	ValidationCounts
	Rows []ValidationRow `json:"rows"`
}

type ForecastEvaluation struct {
	Model               string             `json:"model"`
	ReferenceID         string             `json:"referenceId"`
	Settings            ForecastSettings   `json:"settings"`
	TrainingEvidence    PredictionEvidence `json:"trainingEvidence"`
	TestEvidence        PredictionEvidence `json:"testEvidence"`
	Training            EvaluationCohort   `json:"training"`
	Test                EvaluationCohort   `json:"test"`
	TotalRecords        int                `json:"totalRecords"`
	MismatchedRecords   int                `json:"mismatchedRecords"`
	TooLateRecords      int                `json:"tooLateRecords"`
	BoundaryRecords     int                `json:"boundaryRecords"`
	Probability         *float64           `json:"probability"`
	BrierScore          *float64           `json:"brierScore"`
	BenchmarkBrierScore *float64           `json:"benchmarkBrierScore"`
	GapPercentagePoints *float64           `json:"gapPercentagePoints"`
}

// specs/030-forecast-model-evaluation.md:135: fit only archived earlier outcomes;
// the later cohort may remove overlapping work but never changes the fitted model.
func EvaluateForecastModel(reference ForecastPrediction, history []ForecastPrediction, training PredictionEvidence, trainingIssues []ExecutionIssue, later PredictionEvidence, laterIssues []ExecutionIssue) (ForecastEvaluation, error) {
	out := ForecastEvaluation{Model: "envelope-frequency-v1", ReferenceID: reference.ID, Settings: reference.Forecast.Settings, TrainingEvidence: training, TestEvidence: later, TotalRecords: len(history), Training: EvaluationCohort{Rows: []ValidationRow{}}, Test: EvaluationCohort{Rows: []ValidationRow{}}}
	if training.StartedAt <= 0 || training.CapturedAt < training.StartedAt || training.CapturedAt >= later.StartedAt || later.CapturedAt < later.StartedAt {
		return out, errors.New("the training capture must finish before the later test capture starts; choose two successful captures in chronological order")
	}
	// The full history establishes the shared limit and test representatives.
	all, err := validatePredictionHistory(reference, history, nil, later, laterIssues, true)
	if err != nil {
		return out, err
	}
	earlier := make([]ForecastPrediction, 0, len(history))
	for _, p := range history {
		if p.IssuedAt < training.StartedAt {
			earlier = append(earlier, p)
		} else if p.IssuedAt <= training.CapturedAt && ValidationMatches(reference, p) {
			out.BoundaryRecords++
		}
	}
	fit, err := validatePredictionHistory(reference, earlier, nil, training, trainingIssues, true)
	if err != nil {
		return out, err
	}
	out.Training = EvaluationCohort{ValidationCounts: fit.ValidationCounts, Rows: fit.Rows}
	out.MismatchedRecords, out.TooLateRecords = all.MismatchedRecords, all.TooLateRecords
	for _, row := range all.Rows {
		if row.IssuedAt > training.CapturedAt {
			out.Test.Rows = append(out.Test.Rows, row)
			out.Test.count(row.Status)
		}
	}
	if fit.Eligible == 0 {
		return out, nil
	}
	p := float64(fit.Covered+1) / float64(fit.Eligible+2)
	out.Probability = &p
	if out.Test.Eligible > 0 {
		n, successes := float64(out.Test.Eligible), float64(out.Test.Covered)
		brier := (successes*(1-p)*(1-p) + (n-successes)*p*p) / n
		benchmark, gap := .25, 100*(successes/n-p)
		out.BrierScore, out.BenchmarkBrierScore, out.GapPercentagePoints = &brier, &benchmark, &gap
	}
	return out, nil
}
