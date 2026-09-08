package planning

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"math"
	"time"
)

var _ = Describe("prospective forecast assessment", func() {
	var in BaselineInputs
	var forecast PortfolioForecast
	var initial, later []ExecutionIssue
	var issued time.Time
	var stamp PredictionEvidence
	BeforeEach(func() {
		issued = time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
		in = NewBaselineInputs([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{{Name: "Beacon", EpicKeys: []string{"PROJ-1"}, KitPct: 1, Work: map[string]TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}}, Params{HorizonWeeks: 12}, SchedulingParams{PeriodStart: "2026-09-07", WipModel: "strict", EstimateModel: "effort"})
		var err error
		forecast, err = ComputeForecast(in, ForecastSettings{LowerFactor: .8, UpperFactor: 1.3, Disruption: .1})
		Expect(err).NotTo(HaveOccurred())
		initial = []ExecutionIssue{{Key: "PROJ-1", Type: "Epic", Pod: "Atlas", StatusCategory: "indeterminate"}, {Key: "PROJ-2", ParentKey: "PROJ-1", Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"}}
		finish := issued.Add(14 * 24 * time.Hour)
		later = append([]ExecutionIssue{}, initial...)
		later[1].StatusCategory = "done"
		later[1].Resolved = &finish
		stamp = PredictionEvidence{SnapshotID: "capture-b", SourceID: "source-a", ConfigFingerprint: "config-a", StartedAt: issued.Add(20 * 24 * time.Hour).Unix(), CapturedAt: issued.Add(21 * 24 * time.Hour).Unix()}
	})
	prediction := func(in BaselineInputs, f PortfolioForecast, issues []ExecutionIssue, issued time.Time) ForecastPrediction {
		return ForecastPrediction{IssuedAt: issued.Unix(), Inputs: in, Forecast: f, Evidence: PredictionEvidence{SnapshotID: "capture-a", SourceID: "source-a", ConfigFingerprint: "config-a", CapturedAt: issued.Add(-time.Hour).Unix()}, Issues: issues}
	}
	It("compares frozen dates with complete prospective outcomes and keeps zero-sample coverage unknown", func() {
		p := prediction(in, forecast, initial, issued)
		value := AssessPrediction(p, in.Initiatives, stamp, later)
		Expect(value.Eligible).To(Equal(1))
		Expect(value.Rows[0].ActualFinish).NotTo(BeEmpty())
		Expect(value.CoveragePercent).NotTo(BeNil())
		Expect(value.Rows[0].VarianceWeeks).NotTo(BeNil())
		pending := AssessPrediction(p, in.Initiatives, stamp, initial)
		Expect(pending.Pending).To(Equal(1))
		Expect(pending.Eligible).To(BeZero())
		Expect(pending.CoveragePercent).To(BeNil())
		Expect(p.Forecast).To(Equal(forecast))
	})
	It("excludes inputs when both recorded and current scope cannot be serialized", func() {
		p := prediction(in, forecast, initial, issued)
		current := NewBaselineInputs(in.Teams, in.Initiatives, in.Params, in.Scheduling)
		for _, item := range []*Initiative{&p.Inputs.Initiatives[0], &current.Initiatives[0]} {
			work := item.Work["Atlas"]
			work.Weeks = math.NaN()
			item.Work["Atlas"] = work
		}
		current.Initiatives[0].KitPct = .5
		value := AssessPrediction(p, current.Initiatives, stamp, later)
		Expect(value.Excluded).To(Equal(1))
		Expect(value.Eligible).To(BeZero())
		Expect(value.Rows[0].Reason).To(ContainSubstring("could not be compared"))
	})
	It("withholds unplanned team work even when all captured children finish", func() {
		initial = append(initial, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Type: "Story", Pod: "Other team", StatusCategory: "indeterminate"})
		later = append(later, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Type: "Story", Pod: "Other team", StatusCategory: "done", Resolved: later[1].Resolved})
		value := AssessPrediction(prediction(in, forecast, initial, issued), in.Initiatives, stamp, later)
		Expect(value.Excluded).To(Equal(1))
		Expect(value.Eligible).To(BeZero())
	})
	It("uses frozen minimum and maximum finish dates with inclusive endpoints", func() {
		for index, week := range []int{4, 3, 2} {
			forecast.Scenarios[index].Schedule.Initiatives[0].RawFinishWeek = week
		}
		p := prediction(in, forecast, initial, issued)
		for _, example := range []struct {
			week     float64
			status   string
			coverage float64
		}{{1.5, "before", 0}, {2, "within", 100}, {3, "within", 100}, {4, "within", 100}, {4.5, "after", 0}} {
			origin, _ := time.Parse("2006-01-02", "2026-09-07")
			finish := origin.Add(time.Duration(example.week*168) * time.Hour)
			later[1].Resolved = &finish
			stamp.StartedAt = issued.Add(40 * 24 * time.Hour).Unix()
			stamp.CapturedAt = stamp.StartedAt + 60
			value := AssessPrediction(p, in.Initiatives, stamp, later)
			Expect(value.Eligible).To(Equal(1))
			Expect(*value.CoveragePercent).To(Equal(example.coverage))
			Expect(value.Rows[0].Status).To(Equal(example.status))
			Expect(value.Rows[0].EarliestFinish).To(Equal("2026-09-21"))
			Expect(value.Rows[0].LatestFinish).To(Equal("2026-10-05"))
			Expect(*value.Rows[0].VarianceWeeks).To(BeNumerically("~", example.week-3, 0.00001))
		}
	})
	It("excludes changed membership, mappings, incomplete evidence and retrospective outcomes", func() {
		p := prediction(in, forecast, initial, issued)
		for _, change := range []func([]ExecutionIssue) []ExecutionIssue{
			func(v []ExecutionIssue) []ExecutionIssue {
				return append(v, ExecutionIssue{Key: "PROJ-3", ParentKey: "PROJ-1", Type: "Story", Pod: "Atlas", StatusCategory: "done", Resolved: v[1].Resolved})
			},
			func(v []ExecutionIssue) []ExecutionIssue { return v[:1] },
			func(v []ExecutionIssue) []ExecutionIssue { v[1].Pod = "Beacon"; return v },
			func(v []ExecutionIssue) []ExecutionIssue { v[1].Resolved = nil; return v },
			func(v []ExecutionIssue) []ExecutionIssue { v[1].StatusCategory = "unknown"; return v },
			func(v []ExecutionIssue) []ExecutionIssue { v[1].Resolved = &issued; return v },
			func(v []ExecutionIssue) []ExecutionIssue {
				future := time.Unix(stamp.CapturedAt+1, 0)
				v[1].Resolved = &future
				return v
			},
		} {
			value := AssessPrediction(p, in.Initiatives, stamp, change(append([]ExecutionIssue{}, later...)))
			Expect(value.Excluded).To(Equal(1))
			Expect(value.Eligible).To(BeZero())
			Expect(value.Rows[0].Reason).NotTo(BeEmpty())
		}
	})
	It("keeps pending and excluded initiatives outside the completed coverage denominator", func() {
		p := prediction(in, forecast, nil, issued)
		p.Inputs.Initiatives = nil
		observed := []ExecutionIssue{}
		for s := range p.Forecast.Scenarios {
			p.Forecast.Scenarios[s].Schedule.Initiatives = nil
		}
		for index := 0; index < 4; index++ {
			it := in.Initiatives[0]
			it.Name = fmt.Sprintf("Initiative %d", index)
			it.EpicKeys = []string{fmt.Sprintf("PROJ-%d", index*10+1)}
			p.Inputs.Initiatives = append(p.Inputs.Initiatives, it)
			for s := range p.Forecast.Scenarios {
				row := ScheduledInitiative{Name: it.Name, RawFinishWeek: 2 + s, CommitWeek: 3 + s, Slices: []WorkSlice{{Pod: "Atlas", StartWeek: 0, FinishWeek: 2 + s}}}
				p.Forecast.Scenarios[s].Schedule.Initiatives = append(p.Forecast.Scenarios[s].Schedule.Initiatives, row)
			}
			ep := ExecutionIssue{Key: it.EpicKeys[0], Type: "Epic", Pod: "Atlas", StatusCategory: "indeterminate"}
			child := ExecutionIssue{Key: fmt.Sprintf("PROJ-%d", index*10+2), ParentKey: ep.Key, Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"}
			p.Issues = append(p.Issues, ep, child)
			if index < 2 {
				origin, _ := time.Parse("2006-01-02", "2026-09-07")
				finish := origin.AddDate(0, 0, 21+index*14)
				child.StatusCategory = "done"
				child.Resolved = &finish
			}
			if index == 3 {
				child.Pod = "Other"
			}
			observed = append(observed, ep, child)
		}
		stamp.CapturedAt = issued.Add(60 * 24 * time.Hour).Unix()
		value := AssessPrediction(p, p.Inputs.Initiatives, stamp, observed)
		Expect(value.Eligible).To(Equal(2))
		Expect(value.Covered).To(Equal(1))
		Expect(value.Pending).To(Equal(1))
		Expect(value.Excluded).To(Equal(1))
		Expect(*value.CoveragePercent).To(Equal(50.0))
	})
	It("rejects mismatched provenance, early captures, changed inputs and already completed scope", func() {
		p := prediction(in, forecast, initial, issued)
		for _, change := range []func(*PredictionEvidence){func(v *PredictionEvidence) { v.SourceID = "other" }, func(v *PredictionEvidence) { v.ConfigFingerprint = "changed" }, func(v *PredictionEvidence) { v.StartedAt = issued.Unix() }} {
			other := stamp
			change(&other)
			Expect(AssessPrediction(p, in.Initiatives, other, later).Excluded).To(Equal(1))
		}
		changed := NewBaselineInputs(in.Teams, in.Initiatives, in.Params, in.Scheduling)
		changed.Initiatives[0].EpicKeys = []string{"PROJ-9"}
		Expect(AssessPrediction(p, changed.Initiatives, stamp, later).Excluded).To(Equal(1))
		p.Issues = later
		Expect(AssessPrediction(p, in.Initiatives, stamp, later).Excluded).To(Equal(1))
	})
})
