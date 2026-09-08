package planning

import (
	"fmt"
	"math"
)

type ForecastSettings struct {
	LowerFactor float64 `json:"lowerFactor"`
	UpperFactor float64 `json:"upperFactor"`
	Disruption  float64 `json:"disruption"`
}
type ForecastScenario struct {
	Name          string    `json:"name"`
	Factor        float64   `json:"factor"`
	Disruption    float64   `json:"disruption"`
	CommitWeek    *int      `json:"commitWeek"`
	RawFinishWeek *int      `json:"rawFinishWeek"`
	Unknown       int       `json:"unknown"`
	Schedule      *Schedule `json:"schedule"`
}
type PortfolioForecast struct {
	Fingerprint string             `json:"fingerprint"`
	Settings    ForecastSettings   `json:"settings"`
	Scenarios   []ForecastScenario `json:"scenarios"`
	Limitations []string           `json:"limitations"`
}

// specs/028-portfolio-forecasts.md:152: shared uncertainty is applied to inputs,
// never padded onto independently calculated initiative finish dates.
func forecastInputs(in BaselineInputs, factor, disruption float64) BaselineInputs {
	out := NewBaselineInputs(in.Teams, in.Initiatives, in.Params, in.Scheduling)
	for j := range out.Initiatives {
		for pod, work := range out.Initiatives[j].Work {
			if work.InPath && work.Estimated && work.Weeks > 0 {
				work.Weeks *= factor
				out.Initiatives[j].Work[pod] = work
			}
		}
	}
	if disruption > 0 {
		for j := range out.Teams {
			loss := out.Teams[j].EffectiveLoss(out.Params.WithDefaults().CapacityLoss)
			out.Teams[j].CapacityLoss = 1 - (1-loss)*(1-disruption)
		}
	}
	return out
}

// ForecastKnown excludes modeled placeholder dates from whole-portfolio claims.
func ForecastKnown(row ScheduledInitiative) bool {
	return !row.Provisional && len(row.Slices) > 0 && row.Verdict != "unschedulable" && row.Verdict != "beyond-horizon"
}
func ComputeForecast(in BaselineInputs, settings ForecastSettings) (PortfolioForecast, error) {
	out := PortfolioForecast{}
	if math.IsNaN(settings.LowerFactor) || math.IsNaN(settings.UpperFactor) || math.IsNaN(settings.Disruption) || settings.LowerFactor < .25 || settings.LowerFactor > 1 || settings.UpperFactor < 1 || settings.UpperFactor > 3 || settings.Disruption < 0 || settings.Disruption > .75 {
		return out, fmt.Errorf("choose lower factor 0.25–1, upper factor 1–3 and additional disruption 0–75%%")
	}
	if len(in.Teams) == 0 || len(in.Initiatives) == 0 {
		return out, fmt.Errorf("add a roster and initiatives before forecasting")
	}
	if err := ValidateInitiativeNames(in.Initiatives); err != nil {
		return out, err
	}
	out.Fingerprint = in.Fingerprint()
	out.Settings = settings
	out.Limitations = []string{
		"Scenario outcomes, not probabilities or calibrated confidence intervals. Shorter estimates can change dispatch order; dates are not guaranteed monotonic bounds.",
		"Saved inputs only. Unknown estimates and unplaced work prevent a reliable whole-portfolio finish. Unsaved edits and future scope changes are excluded.",
		"Commitment includes the existing project buffer; finish excludes it. Shared disruption consumes remaining productive capacity in the longer-estimate scenario only.",
	}
	for _, scenario := range []ForecastScenario{{Name: "Shorter estimates", Factor: settings.LowerFactor}, {Name: "Current plan", Factor: 1}, {Name: "Longer estimates + disruption", Factor: settings.UpperFactor, Disruption: settings.Disruption}} {
		inputs := forecastInputs(in, scenario.Factor, scenario.Disruption)
		scenario.Schedule = inputs.RecomputeWith(ScheduleOptions{})
		finish, commit := 0, 0
		for _, row := range scenario.Schedule.Initiatives {
			if !ForecastKnown(row) {
				scenario.Unknown++
				continue
			}
			finish = max(finish, row.RawFinishWeek)
			commit = max(commit, row.CommitWeek)
		}
		if scenario.Unknown == 0 {
			scenario.RawFinishWeek = &finish
			scenario.CommitWeek = &commit
		}
		out.Scenarios = append(out.Scenarios, scenario)
	}
	return out, nil
}
