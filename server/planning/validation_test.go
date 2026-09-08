package planning

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("forecast history validation", func() {
	var evidence PredictionEvidence
	var current []Initiative
	var issues []ExecutionIssue
	var makePrediction func(string, string, int64) ForecastPrediction
	BeforeEach(func() {
		current = nil
		issues = nil
		evidence = PredictionEvidence{SourceID: "source", ConfigFingerprint: "config", StartedAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).Unix(), CapturedAt: time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC).Unix()}
		makePrediction = func(id, key string, issued int64) ForecastPrediction {
			it := Initiative{Name: key, EpicKeys: []string{key}, KitPct: 1, Work: map[string]TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}
			in := NewBaselineInputs([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{it}, Params{HorizonWeeks: 12}, SchedulingParams{PeriodStart: time.Unix(issued, 0).Format("2006-01-02"), WipModel: "strict", EstimateModel: "effort"})
			forecast, err := ComputeForecast(in, ForecastSettings{LowerFactor: .8, UpperFactor: 1.3, Disruption: .1})
			Expect(err).NotTo(HaveOccurred())
			initial := []ExecutionIssue{{Key: key, Type: "Epic", Pod: "Atlas", StatusCategory: "indeterminate"}, {Key: key + "-child", ParentKey: key, Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"}}
			finish := time.Unix(issued, 0).AddDate(0, 0, 14)
			completed := append([]ExecutionIssue{}, initial...)
			completed[1].StatusCategory = "done"
			completed[1].Resolved = &finish
			current = append(current, it)
			issues = append(issues, completed...)
			return ForecastPrediction{ID: id, Name: id, IssuedAt: issued, Inputs: in, Forecast: forecast, Evidence: PredictionEvidence{SourceID: "source", ConfigFingerprint: "config"}, Issues: initial}
		}
	})
	stamp := func(month time.Month) int64 { return time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC).Unix() }
	It("counts distinct work once, shows monthly denominators and reproduces results regardless of history order", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		b := makePrediction("b", "PROJ-2", stamp(time.October))
		repeat := a
		repeat.ID = "repeat"
		repeat.IssuedAt++
		report, err := ValidatePredictionHistory(a, []ForecastPrediction{repeat, b, a}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.Eligible).To(Equal(2))
		Expect(report.Covered).To(Equal(2))
		Expect(report.Repeated).To(Equal(1))
		Expect(report.CoveragePercent).NotTo(BeNil())
		Expect(*report.CoveragePercent).To(Equal(100.0))
		Expect(report.Months).To(HaveLen(2))
		Expect(report.Months[0].Month).To(Equal("2026-09"))
		Expect(report.Months[1].Eligible).To(Equal(1))
		Expect(report.Rows[1].Status).To(Equal("repeated"))
		Expect(report.Rows[1].RepresentativeID).To(Equal("a"))
		again, err := ValidatePredictionHistory(a, []ForecastPrediction{a, b, repeat}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(again).To(Equal(report))
	})
	It("selects before scoring and suppresses transitive overlap even across a later bridging record", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		c := makePrediction("c", "PROJ-2", stamp(time.September)+1)
		bridge := a
		bridge.ID = "bridge"
		bridge.IssuedAt += 2
		bridge.Inputs = NewBaselineInputs(a.Inputs.Teams, a.Inputs.Initiatives, a.Inputs.Params, a.Inputs.Scheduling)
		bridge.Inputs.Initiatives[0].EpicKeys = []string{"PROJ-1", "PROJ-2"}
		a.Forecast.Scenarios = nil // The original cannot be replaced by a better later result.
		report, err := ValidatePredictionHistory(a, []ForecastPrediction{c, bridge, a}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.Excluded).To(Equal(1))
		Expect(report.Repeated).To(Equal(2))
		Expect(report.Eligible).To(BeZero())
		Expect(report.CoveragePercent).To(BeNil())
		Expect(report.Months[0].CoveragePercent).To(BeNil())
	})
	It("separates mismatched settings and late records and refuses incompatible evidence", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		other := a
		other.ID = "other"
		other.Forecast.Settings.UpperFactor = 2
		late := a
		late.ID = "late"
		late.IssuedAt = evidence.StartedAt
		report, err := ValidatePredictionHistory(a, []ForecastPrediction{a, other, late}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.MismatchedRecords).To(Equal(1))
		Expect(report.TooLateRecords).To(Equal(1))
		Expect(report.CandidateRecords).To(Equal(1))
		evidence.ConfigFingerprint = "other"
		_, err = ValidatePredictionHistory(a, []ForecastPrediction{a}, current, evidence, issues)
		Expect(err).To(MatchError(ContainSubstring("source and extraction settings")))
	})
	It("keeps pending and changed scope out of completed coverage with visible monthly counts", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		b := makePrediction("b", "PROJ-2", stamp(time.October))
		issues[1].StatusCategory = "indeterminate"
		issues[1].Resolved = nil
		current[1].KitPct = .5
		report, err := ValidatePredictionHistory(a, []ForecastPrediction{a, b}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.Pending).To(Equal(1))
		Expect(report.Excluded).To(Equal(1))
		Expect(report.Eligible).To(BeZero())
		Expect(report.CoveragePercent).To(BeNil())
		Expect(report.Months[0].Pending).To(Equal(1))
		Expect(report.Months[1].Excluded).To(Equal(1))
	})
	It("groups shared captured children even when epic bindings differ", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		b := makePrediction("b", "PROJ-2", stamp(time.September)+1)
		b.Issues[1].Key = a.Issues[1].Key
		report, err := ValidatePredictionHistory(a, []ForecastPrediction{b, a}, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.Repeated).To(Equal(1))
		Expect(report.Eligible + report.Pending + report.Excluded).To(Equal(1))
		Expect(report.Rows[1].RepresentativeID).To(Equal("a"))
	})
	It("aggregates mixed outcomes without admitting pending, excluded or repeated entries into fractions", func() {
		records := []ForecastPrediction{}
		for i := 0; i < 5; i++ {
			month := time.September
			if i >= 3 {
				month = time.October
			}
			records = append(records, makePrediction(fmt.Sprint(i), fmt.Sprintf("PROJ-%d", i), stamp(month)))
		}
		before, after := time.Unix(records[1].IssuedAt, 0).AddDate(0, 0, 3), time.Unix(records[2].IssuedAt, 0).AddDate(0, 0, 70)
		issues[3].Resolved = &before
		issues[5].Resolved = &after
		issues[7].StatusCategory = "indeterminate"
		issues[7].Resolved = nil
		current[4].KitPct = .5
		repeated := records[1]
		repeated.ID = "repeat"
		repeated.IssuedAt++
		records = append(records, repeated)
		report, err := ValidatePredictionHistory(records[0], records, current, evidence, issues)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.Eligible).To(Equal(3))
		Expect(report.Covered).To(Equal(1))
		Expect(report.Before).To(Equal(1))
		Expect(report.After).To(Equal(1))
		Expect(report.Pending).To(Equal(1))
		Expect(report.Excluded).To(Equal(1))
		Expect(report.Repeated).To(Equal(1))
		Expect(*report.CoveragePercent).To(BeNumerically("~", 100.0/3, 0.00001))
		Expect(report.Months).To(HaveLen(2))
		Expect(report.Months[0].Eligible).To(Equal(3))
		Expect(*report.Months[0].CoveragePercent).To(BeNumerically("~", 100.0/3, 0.00001))
		Expect(report.Months[1].CoveragePercent).To(BeNil())
		Expect(report.Months[1].Pending).To(Equal(1))
		Expect(report.Months[1].Excluded).To(Equal(1))
	})
	It("rejects oversized histories and candidate scope instead of returning biased partial counts", func() {
		a := makePrediction("a", "PROJ-1", stamp(time.September))
		_, err := ValidatePredictionHistory(a, make([]ForecastPrediction, 201), current, evidence, issues)
		Expect(err).To(MatchError(ContainSubstring("200")))
		huge := a
		huge.Inputs.Initiatives = make([]Initiative, 5001)
		for i := range huge.Inputs.Initiatives {
			huge.Inputs.Initiatives[i] = Initiative{Name: fmt.Sprint(i)}
		}
		_, err = ValidatePredictionHistory(a, []ForecastPrediction{huge}, current, evidence, issues)
		Expect(err).To(MatchError(ContainSubstring("5000")))
	})
})
