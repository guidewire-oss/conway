package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"math"
)

var _ = Describe("portfolio forecast scenarios", func() {
	fixture := func() BaselineInputs {
		return NewBaselineInputs([]Team{{Name: "Atlas", Tracks: 1, CapacityLoss: .2}}, []Initiative{
			{Name: "Beacon", KitPct: 1, Work: map[string]TeamWork{"Atlas": {Weeks: 4, Estimated: true, InPath: true}}},
			{Name: "Cedar", KitPct: 1, AfterInitiatives: []string{"Beacon"}, Work: map[string]TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}},
		}, Params{HorizonWeeks: 26}, SchedulingParams{PeriodStart: "2026-09-07", WipModel: "strict", EstimateModel: "wall-clock"})
	}
	It("replays constrained scenarios without changing inputs or the central schedule", func() {
		in := fixture()
		fingerprint := in.Fingerprint()
		settings := ForecastSettings{LowerFactor: .75, UpperFactor: 1.5, Disruption: .2}
		result, err := ComputeForecast(in, settings)
		Expect(err).NotTo(HaveOccurred())
		Expect(in.Fingerprint()).To(Equal(fingerprint))
		Expect(result.Fingerprint).To(Equal(fingerprint))
		Expect(result.Scenarios).To(HaveLen(3))
		Expect(result.Scenarios[1].Schedule).To(Equal(in.RecomputeWith(ScheduleOptions{})))
		replay, err := ComputeForecast(in, settings)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay).To(Equal(result))
		for _, scenario := range result.Scenarios {
			Expect(scenario.CommitWeek).NotTo(BeNil())
			rows := map[string]ScheduledInitiative{}
			for _, row := range scenario.Schedule.Initiatives {
				rows[row.Name] = row
			}
			Expect(rows["Cedar"].StartWeek).To(BeNumerically(">=", rows["Beacon"].RawFinishWeek))
		}
		adverse := forecastInputs(in, 1.5, .2)
		Expect(adverse.Teams[0].CapacityLoss).To(BeNumerically("~", .36))
		Expect(adverse.Initiatives[0].Work["Atlas"].Weeks).To(Equal(6.0))
	})
	It("keeps provisional and out-of-horizon work in the denominator with unknown portfolio dates", func() {
		for _, missing := range []bool{true, false} {
			in := fixture()
			if missing {
				in.Initiatives[0].Work["Atlas"] = TeamWork{InPath: true}
			} else {
				in.Params.HorizonWeeks = 1
			}
			result, err := ComputeForecast(in, ForecastSettings{LowerFactor: .75, UpperFactor: 1.5})
			Expect(err).NotTo(HaveOccurred())
			for _, scenario := range result.Scenarios {
				Expect(scenario.Schedule.Initiatives).To(HaveLen(2))
				Expect(scenario.CommitWeek).To(BeNil())
				Expect(scenario.Unknown).To(BeNumerically(">", 0))
			}
		}
	})
	It("refuses empty plans and invalid or nonfinite settings", func() {
		_, err := ComputeForecast(BaselineInputs{}, ForecastSettings{LowerFactor: 1, UpperFactor: 1})
		Expect(err).To(HaveOccurred())
		for _, settings := range []ForecastSettings{{LowerFactor: 0, UpperFactor: 1}, {LowerFactor: 1.1, UpperFactor: 1.5}, {LowerFactor: 1, UpperFactor: .5}, {LowerFactor: 1, UpperFactor: 4}, {LowerFactor: 1, UpperFactor: 1, Disruption: -.1}, {LowerFactor: 1, UpperFactor: 1, Disruption: .9}, {LowerFactor: math.NaN(), UpperFactor: 1}} {
			_, err = ComputeForecast(fixture(), settings)
			Expect(err).To(HaveOccurred())
		}
	})
	It("rejects both infinities and NaN in each setting before scheduling", func() {
		for _, value := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
			for _, settings := range []ForecastSettings{
				{LowerFactor: value, UpperFactor: 1},
				{LowerFactor: 1, UpperFactor: value},
				{LowerFactor: 1, UpperFactor: 1, Disruption: value},
			} {
				result, err := ComputeForecast(fixture(), settings)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("choose lower factor"))
				Expect(result.Scenarios).To(BeEmpty())
			}
		}
	})
})
