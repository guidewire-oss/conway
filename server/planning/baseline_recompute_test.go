package planning

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BaselineInputs.RecomputeWith", func() {
	var in BaselineInputs

	BeforeEach(func() {
		teams, inits := Demo()
		in = NewBaselineInputs(teams, inits, Params{HorizonWeeks: 26, CapacityLoss: 0.1}, DemoScheduling())
	})

	It("can rebuild without the WIP-model comparison", func() {
		Expect(in.RecomputeWith(ScheduleOptions{}).WipModels).To(BeEmpty())
	})

	// AC 7.1: recomputing from the stored inputs reproduces the stored schedule
	// exactly. Against a schedule stored the way saveBaseline stores one -- marshalled
	// -- not against another call to the same function.
	//
	// The first version of this spec asserted Recompute() equals
	// RecomputeWith(CompareWipModels: true). Recompute is implemented as exactly that
	// call, so it could never fail: it checked that one function delegates to another
	// while claiming to check AC 7.1.
	It("reproduces a stored schedule exactly, comparison included", func() {
		stored, err := json.Marshal(in.Recompute())
		Expect(err).NotTo(HaveOccurred())

		again, err := json.Marshal(in.Recompute())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(again)).To(Equal(string(stored)))

		var back BaselineInputs
		blob, err := json.Marshal(in)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(blob, &back)).To(Succeed())
		roundTripped, err := json.Marshal(back.Recompute())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(roundTripped)).To(Equal(string(stored)),
			"a baseline that cannot be reproduced through storage cannot answer 'why week 4' later")

		Expect(back.Recompute().WipModels).NotTo(BeEmpty(),
			"the comparison is part of what a baseline stored, so narrowing it would break AC 7.1")
	})
})
