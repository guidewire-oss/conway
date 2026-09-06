package planning

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Saved lane integrity", func() {
	teams := []Team{{Name: "Atlas", Tracks: 2}}
	sp := SchedulingParams{PeriodStart: specPeriodStart, EstimateModel: EstimateWallClock, WipModel: WipOff, BufferPct: pctOf(0)}
	params := Params{HorizonWeeks: 26}
	item := func(name string, lane int) Initiative {
		return Initiative{Name: name, Work: map[string]TeamWork{"Atlas": podWork(2)}, PinnedLanes: map[string]int{"Atlas": lane}}
	}
	It("permits a free lower lane beside a saved upper lane", func() {
		Expect(ValidateLanePinsWithParams([]Initiative{item("First", 1), item("Second", 0)}, sp, params, teams)).To(BeEmpty())
	})
	It("rejects two saved reservations on the same upper lane", func() {
		Expect(ValidateLanePinsWithParams([]Initiative{item("First", 1), item("Second", 1)}, sp, params, teams)).NotTo(BeEmpty())
	})
	It("retains a distant start pin when checking lane overlap", func() {
		a, b := item("First", 0), item("Second", 0)
		a.PinnedStarts = map[string]int{"Atlas": 10}
		Expect(ValidateLanePinsWithParams([]Initiative{a, b}, sp, params, teams)).To(BeEmpty())
	})
	It("uses the actual capacity loss rather than a fixed default", func() {
		a, b := item("First", 0), item("Second", 0)
		b.PinnedStarts = map[string]int{"Atlas": 2}
		Expect(ValidateLanePinsWithParams([]Initiative{a, b}, sp, params, teams)).To(BeEmpty())
		p := params
		p.CapacityLoss = .5
		Expect(ValidateLanePinsWithParams([]Initiative{a, b}, sp, p, teams)).NotTo(BeEmpty())
	})
	It("rejects pins outside physical width", func() {
		Expect(ValidateLanePinsWithParams([]Initiative{item("First", 2)}, sp, params, teams)).NotTo(BeEmpty())
	})
	It("leaves unpinned work flexible around complementary saved pins", func() {
		a, b := item("Early", 0), item("Late", 1)
		b.PinnedStarts = map[string]int{"Atlas": 2}
		flex := Initiative{Name: "Flexible", Work: map[string]TeamWork{"Atlas": podWork(4)}}
		Expect(ValidateLanePinsWithParams([]Initiative{a, b, flex}, sp, params, teams)).To(BeEmpty())
	})
})
