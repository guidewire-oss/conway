package planning

import (
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("reviewed remedy proposals", func() {
	var inputs BaselineInputs
	BeforeEach(func() {
		teams := []Team{{Name: "Delta", Tracks: 1}, {Name: "Atlas", Tracks: 6}}
		inits := []Initiative{
			{Name: "Early", Work: map[string]TeamWork{"Delta": podWork(10)}, StatedPriority: 3, TargetDate: dateAtWeek(specPeriodStart, 14)},
			{Name: "Late", Work: map[string]TeamWork{"Delta": podWork(10)}, StatedPriority: 2, PriorityLocked: true, TargetDate: dateAtWeek(specPeriodStart, 20), DateLocked: true, EpicKeys: []string{"PROJ-1"}},
			{Name: "Other", Work: map[string]TeamWork{"Atlas": podWork(4)}, StatedPriority: 4},
		}
		inputs = NewBaselineInputs(teams, inits, Params{HorizonWeeks: 26}, SchedulingParams{PeriodStart: specPeriodStart, WipModel: WipStrict, BufferPct: pctOf(0.25), MaxConcurrentInitiatives: 3})
	})
	It("reproduces every offered consequence with isolated input copies", func() {
		before, err := json.Marshal(inputs)
		Expect(err).NotTo(HaveOccurred())
		offered := ComputeRemedies(inputs.Teams, inputs.Initiatives, inputs.Params, inputs.Scheduling, []string{"Late"})
		Expect(offered).NotTo(BeEmpty())
		for _, choice := range offered {
			out, was, after, err := PreviewRemedy(inputs, choice)
			Expect(err).NotTo(HaveOccurred(), choice.Kind)
			for _, it := range after.Initiatives {
				if it.Name == choice.Target {
					Expect(it.Verdict).To(Equal(choice.ResultingVerdict), choice.Kind)
					Expect(it.WeeksLate).To(Equal(choice.TargetWeeksLate), choice.Kind)
				}
			}
			Expect(round1(after.ObjectiveScore-was.ObjectiveScore)).To(Equal(choice.ObjectiveDelta), choice.Kind)
			if len(out.Initiatives) > 0 {
				out.Initiatives[0].Work["Delta"] = podWork(99)
			}
			got, err := json.Marshal(inputs)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(before), "preview or returned output must not change working inputs")
		}
	})
	It("rejects forged impact claims and unoffered magnitudes", func() {
		choices := ComputeRemedies(inputs.Teams, inputs.Initiatives, inputs.Params, inputs.Scheduling, []string{"Late"})
		Expect(choices).NotTo(BeEmpty())
		forged := choices[0]
		forged.ObjectiveDelta = -999999
		_, _, _, err := PreviewRemedy(inputs, forged)
		Expect(err).To(HaveOccurred())
		forged = choices[0]
		forged.Magnitude = 999999
		_, _, _, err = PreviewRemedy(inputs, forged)
		Expect(err).To(HaveOccurred())
		forged = choices[0]
		forged.Note = "defer Other out of this period"
		_, _, _, err = PreviewRemedy(inputs, forged)
		Expect(err).To(HaveOccurred())
	})
	It("reports a deferral as removed scope so the displaced commitment stays visible", func() {
		choices := ComputeRemedies(inputs.Teams, inputs.Initiatives, inputs.Params, inputs.Scheduling, []string{"Late"})
		found := false
		for _, choice := range choices {
			if choice.Kind != "defer-other" {
				continue
			}
			found = true
			_, was, after, err := PreviewRemedy(inputs, choice)
			Expect(err).NotTo(HaveOccurred())
			Expect(CompareToBaseline(was, after).Removed).NotTo(BeEmpty())
		}
		Expect(found).To(BeTrue(), "fixture must offer a deferral")
	})
})
