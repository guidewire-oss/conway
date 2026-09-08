package planning

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("forecast model evaluation", func() {
	var training, later PredictionEvidence
	var history []ForecastPrediction
	var trainingIssues, laterIssues []ExecutionIssue
	var add func(string, time.Month, int) ForecastPrediction
	BeforeEach(func() {
		stamp := func(month time.Month) int64 { return time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC).Unix() }
		training = PredictionEvidence{SnapshotID: "training", SourceID: "source", ConfigFingerprint: "config", StartedAt: stamp(time.October), CapturedAt: stamp(time.October) + 60}
		later = PredictionEvidence{SnapshotID: "test", SourceID: "source", ConfigFingerprint: "config", StartedAt: stamp(time.December), CapturedAt: stamp(time.December) + 60}
		history, trainingIssues, laterIssues = nil, nil, nil
		add = func(id string, month time.Month, finishDays int) ForecastPrediction {
			key := "PROJ-" + id
			it := Initiative{Name: key, EpicKeys: []string{key}, KitPct: 1, Work: map[string]TeamWork{"Atlas": {Weeks: 2, Estimated: true, InPath: true}}}
			in := NewBaselineInputs([]Team{{Name: "Atlas", Tracks: 1}}, []Initiative{it}, Params{HorizonWeeks: 12}, SchedulingParams{PeriodStart: time.Unix(stamp(month), 0).Format("2006-01-02"), WipModel: "strict", EstimateModel: "effort"})
			forecast, err := ComputeForecast(in, ForecastSettings{LowerFactor: .8, UpperFactor: 1.3, Disruption: .1})
			Expect(err).NotTo(HaveOccurred())
			initial := []ExecutionIssue{{Key: key, Type: "Epic", Pod: "Atlas", StatusCategory: "indeterminate"}, {Key: key + "-child", ParentKey: key, Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"}}
			finish := time.Unix(stamp(month), 0).AddDate(0, 0, finishDays)
			completed := append([]ExecutionIssue{}, initial...)
			completed[1].StatusCategory, completed[1].Resolved = "done", &finish
			p := ForecastPrediction{ID: id, Name: id, IssuedAt: stamp(month), Inputs: in, Forecast: forecast, Evidence: PredictionEvidence{SourceID: "source", ConfigFingerprint: "config"}, Issues: initial}
			history = append(history, p)
			if p.IssuedAt < training.StartedAt {
				trainingIssues = append(trainingIssues, completed...)
			}
			laterIssues = append(laterIssues, completed...)
			return p
		}
	})
	It("fits only earlier completed evidence and scores later distinct work with explicit denominators", func() {
		ref := add("1", time.September, 14)
		add("2", time.September, 14)
		add("3", time.September, 3)
		add("4", time.November, 14)
		add("5", time.November, 3)
		r, err := EvaluateForecastModel(ref, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Model).To(Equal("envelope-frequency-v1"))
		Expect(r.Training.Eligible).To(Equal(3))
		Expect(r.Test.Eligible).To(Equal(2))
		Expect(r.Probability).NotTo(BeNil())
		Expect(*r.Probability).To(BeNumerically("~", .6))
		Expect(*r.BrierScore).To(BeNumerically("~", .26))
		Expect(*r.BenchmarkBrierScore).To(Equal(.25))
		Expect(*r.GapPercentagePoints).To(BeNumerically("~", -10))
		// Change only future observations; fitting must remain unchanged.
		laterIssues[7].StatusCategory, laterIssues[7].Resolved = "indeterminate", nil
		again, err := EvaluateForecastModel(ref, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Probability).To(Equal(r.Probability))
		Expect(again.Training).To(Equal(r.Training))
		Expect(again.Test.Pending).To(Equal(1))
	})
	It("reserves earlier pending scope and suppresses transitive overlap without refitting on future bridges", func() {
		a := add("1", time.September, 14)
		b := add("2", time.November, 14)
		before, err := EvaluateForecastModel(a, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(before.Test.Eligible).To(Equal(1))
		bridge := b
		bridge.ID = "bridge"
		bridge.IssuedAt++
		bridge.Inputs = NewBaselineInputs(b.Inputs.Teams, b.Inputs.Initiatives, b.Inputs.Params, b.Inputs.Scheduling)
		bridge.Inputs.Initiatives[0].EpicKeys = []string{"PROJ-1", "PROJ-2"}
		history = append(history, bridge)
		r, err := EvaluateForecastModel(a, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Probability).To(Equal(before.Probability))
		Expect(r.Test.Eligible).To(BeZero())
		Expect(r.Test.Repeated).To(Equal(2))
		Expect(r.BrierScore).To(BeNil())
		trainingIssues[1].StatusCategory, trainingIssues[1].Resolved = "indeterminate", nil
		r, err = EvaluateForecastModel(a, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Training.Pending).To(Equal(1))
		Expect(r.Probability).To(BeNil())
		Expect(r.Test.Eligible).To(BeZero())
		Expect(r.Test.Repeated).To(Equal(2))
	})
	It("keeps capture boundaries, mismatches and unavailable samples explicit", func() {
		ref := add("1", time.September, 14)
		for i, issued := range []int64{training.StartedAt, training.CapturedAt, training.CapturedAt + 1, later.StartedAt} {
			p := add(fmt.Sprint(i+2), time.November, 14)
			history[len(history)-1].IssuedAt = issued
			_ = p
		}
		other := ref
		other.ID = "other"
		other.Forecast.Settings.UpperFactor = 2
		history = append(history, other)
		r, err := EvaluateForecastModel(ref, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.BoundaryRecords).To(Equal(2))
		Expect(r.TooLateRecords).To(Equal(1))
		Expect(r.MismatchedRecords).To(Equal(1))
		Expect(r.Test.Rows).To(HaveLen(1))
		Expect(r.Test.Rows[0].PredictionID).To(Equal("4"))
		r, err = EvaluateForecastModel(ref, nil, training, nil, later, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Probability).To(BeNil())
		Expect(r.BrierScore).To(BeNil())
		Expect(r.GapPercentagePoints).To(BeNil())
	})
	It("rejects invalid chronology, configuration and oversized histories without a partial model", func() {
		ref := add("1", time.September, 14)
		bad := training
		bad.CapturedAt = later.StartedAt
		_, err := EvaluateForecastModel(ref, history, bad, trainingIssues, later, laterIssues)
		Expect(err).To(MatchError(ContainSubstring("finish before")))
		bad = training
		bad.ConfigFingerprint = "other"
		_, err = EvaluateForecastModel(ref, history, bad, trainingIssues, later, laterIssues)
		Expect(err).To(MatchError(ContainSubstring("source")))
		_, err = EvaluateForecastModel(ref, make([]ForecastPrediction, MaxValidationRecords+1), training, trainingIssues, later, laterIssues)
		Expect(err).To(MatchError(ContainSubstring("200")))
	})
	It("excludes changed captured membership from training and test without treating gaps as failed predictions", func() {
		ref := add("1", time.September, 14)
		add("2", time.November, 14)
		trainingIssues = append(trainingIssues, ExecutionIssue{Key: "PROJ-extra", ParentKey: "PROJ-1", Type: "Story", Pod: "Atlas", StatusCategory: "indeterminate"})
		laterIssues[3].ParentKey = "PROJ-reparented"
		r, err := EvaluateForecastModel(ref, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Training.Excluded).To(Equal(1))
		Expect(r.Test.Excluded).To(Equal(1))
		Expect(r.Training.Eligible).To(BeZero())
		Expect(r.Test.Eligible).To(BeZero())
		Expect(r.Probability).To(BeNil())
		Expect(r.BrierScore).To(BeNil())
		Expect(r.Training.Rows[0].Reason).To(ContainSubstring("membership"))
		Expect(r.Test.Rows[0].Reason).To(ContainSubstring("membership"))
	})
	It("reserves work recorded during the training capture from later test reuse", func() {
		ref := add("1", time.September, 14)
		boundary := add("2", time.November, 14)
		history[len(history)-1].IssuedAt = training.StartedAt
		repeat := boundary
		repeat.ID = "repeat"
		history = append(history, repeat)
		r, err := EvaluateForecastModel(ref, history, training, trainingIssues, later, laterIssues)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.BoundaryRecords).To(Equal(1))
		Expect(r.Training.Eligible).To(Equal(1))
		Expect(r.Test.Eligible).To(BeZero())
		Expect(r.Test.Repeated).To(Equal(1))
		Expect(r.Test.Rows[0].RepresentativeID).To(Equal(boundary.ID))
		Expect(r.BrierScore).To(BeNil())
	})
})
