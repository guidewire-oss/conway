package planning

import "testing"

func TestDemoHasConstraintsAndNetwork(t *testing.T) {
	teams, inits := Demo()
	plan := &Plan{Initiatives: inits}
	for _, tm := range teams {
		plan.Teams = append(plan.Teams, tm.Name)
	}
	net := BuildNetwork(plan)
	if len(net.Edges) < 5 {
		t.Fatalf("demo should have a rich dependency network, got %d edges", len(net.Edges))
	}
	loads := Utilization(plan, teams, Params{HorizonWeeks: 26, CapacityLoss: 0.10})
	constraints := 0
	for _, l := range loads {
		if l.Constraint {
			constraints++
		}
	}
	if constraints == 0 {
		t.Fatalf("demo should surface at least one over-capacity pod; loads=%v", loads)
	}
}
