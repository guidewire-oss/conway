package planning

import "testing"

func TestSimulateLeversImproveFlow(t *testing.T) {
	teams, inits := Demo()
	params := Params{HorizonWeeks: 26, CapacityLoss: 0.10}
	// add capacity to the red constraint (Delta) + reduce WIP 15%
	before, after := Simulate(teams, inits, params, []Lever{
		{Type: "addCapacity", Pod: "Delta", N: 3},
		{Type: "reduceWip", N: 0.15},
	})
	if before.Constraints == 0 {
		t.Fatalf("baseline should have a constraint; got %d", before.Constraints)
	}
	if after.Constraints > before.Constraints {
		t.Fatalf("levers should not add constraints: %d -> %d", before.Constraints, after.Constraints)
	}
	// Delta ρ must drop after adding capacity
	rho := func(r SimResult, pod string) float64 {
		for _, l := range r.Loads {
			if l.Team == pod {
				return l.Rho
			}
		}
		return -1
	}
	if rho(after, "Delta") >= rho(before, "Delta") {
		t.Fatalf("Delta ρ should fall: %.2f -> %.2f", rho(before, "Delta"), rho(after, "Delta"))
	}
	// more initiatives should fit (or at least not fewer), median lead should drop
	if after.Fitting < before.Fitting {
		t.Fatalf("fitting should not decrease: %d -> %d", before.Fitting, after.Fitting)
	}
	if after.MedianLeadWeeks > before.MedianLeadWeeks {
		t.Fatalf("median lead should not increase: %.1f -> %.1f", before.MedianLeadWeeks, after.MedianLeadWeeks)
	}
}

func TestDeferRemovesInitiative(t *testing.T) {
	teams, inits := Demo()
	_, after := Simulate(teams, inits, Params{}, []Lever{{Type: "defer", Initiative: "Telemetry GA"}})
	if after.Total != len(inits)-1 {
		t.Fatalf("defer should drop one initiative: %d", after.Total)
	}
}

func TestReassignAndDrop(t *testing.T) {
	teams, inits := Demo()
	params := Params{HorizonWeeks: 26, CapacityLoss: 0.10}
	rho := func(r SimResult, pod string) float64 {
		for _, l := range r.Loads {
			if l.Team == pod {
				return l.Rho
			}
		}
		return 0
	}
	// reassign Delta's work to Beacon (7 tracks) -> Delta rho drops
	_, after := Simulate(teams, inits, params, []Lever{{Type: "reassign", Pod: "Delta", ToPod: "Beacon"}})
	if rho(after, "Delta") > 0.01 {
		t.Fatalf("reassign should empty Delta demand: %.2f", rho(after, "Delta"))
	}
	// drop a pod from one initiative
	beforeDrop := ComputeResult(teams, inits, params, 0)
	_, afterDrop := Simulate(teams, inits, params, []Lever{{Type: "dropPod", Pod: "Delta", Initiative: "Telemetry GA"}})
	if afterDrop.Loads == nil || rho(afterDrop, "Delta") >= rho(beforeDrop, "Delta") {
		t.Fatalf("dropPod should reduce Delta load: %.2f -> %.2f", rho(beforeDrop, "Delta"), rho(afterDrop, "Delta"))
	}
}
