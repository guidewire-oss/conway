package planning

import (
	"math"
	"strings"
	"testing"
)

func TestParseTeamsCSV(t *testing.T) {
	csv := "Pod Name,Developers,Location,Pairs\n" +
		"Alpha,\"a@x,b@x,c@x,d@x,e@x,f@x\",Bengaluru,no\n" + // 6 devs, no pairing -> 6 tracks
		"Gamma,\"g@x,h@x,i@x,j@x\",Krakow,yes\n" + // 4 devs, pairs -> 2 tracks
		"Empty,,Remote,no\n" // 0 devs
	teams, err := ParseTeamsCSV([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 3 {
		t.Fatalf("want 3 teams, got %d", len(teams))
	}
	if teams[0].Devs != 6 || teams[0].Pairs || teams[0].EffectiveTracks() != 6 {
		t.Fatalf("Alpha: %+v tracks=%d", teams[0], teams[0].EffectiveTracks())
	}
	if teams[1].Devs != 4 || !teams[1].Pairs || teams[1].EffectiveTracks() != 2 {
		t.Fatalf("Gamma (pairs -> 2 tracks): %+v tracks=%d", teams[1], teams[1].EffectiveTracks())
	}
	if teams[1].Site != "Krakow" {
		t.Fatalf("site: %q", teams[1].Site)
	}
}

func TestEffectiveTracks(t *testing.T) {
	if got := (Team{Devs: 7, Pairs: true}).EffectiveTracks(); got != 4 { // ceil(7/2)
		t.Fatalf("pairing 7 devs -> 4 tracks, got %d", got)
	}
	if got := (Team{Devs: 7, Tracks: 2}).EffectiveTracks(); got != 2 { // override wins
		t.Fatalf("override should win, got %d", got)
	}
}

func TestUtilization(t *testing.T) {
	// Alpha: 6 tracks; Gamma: 2 tracks. Horizon 26, loss 10% -> cap factor 23.4/track.
	teams := []Team{{Name: "Alpha", Devs: 6}, {Name: "Gamma", Devs: 4, Pairs: true}}
	plan := &Plan{Initiatives: []Initiative{{Work: map[string]TeamWork{
		"Alpha": {Weeks: 20, InPath: true},
		"Gamma": {Weeks: 60, InPath: true}, // way over its 2-track capacity
	}}}}
	loads := Utilization(plan, teams, Params{HorizonWeeks: 26, CapacityLoss: 0.10})
	byName := map[string]PodLoad{}
	for _, l := range loads {
		byName[l.Team] = l
	}
	aj := byName["Alpha"]
	if aj.Tracks != 6 || math.Abs(aj.CapacityWeeks-6*26*0.9) > 1e-9 {
		t.Fatalf("Alpha capacity: %+v", aj)
	}
	if aj.Rho >= 1 || aj.Constraint {
		t.Fatalf("Alpha should be under capacity: rho=%.3f", aj.Rho)
	}
	co := byName["Gamma"]
	if !co.Constraint || co.Rho <= 1 {
		t.Fatalf("Gamma should be the constraint: %+v", co)
	}
	// hottest-first ordering
	if loads[0].Team != "Gamma" {
		t.Fatalf("constraint should sort first, got %s", loads[0].Team)
	}
}

func TestUtilizationNoCapacity(t *testing.T) {
	plan := &Plan{Initiatives: []Initiative{{Work: map[string]TeamWork{"Ghost": {Weeks: 5, InPath: true}}}}}
	loads := Utilization(plan, nil, Params{}) // Ghost has demand but no roster entry
	// InfiniteRho (not literal +Inf) — encoding/json can't marshal Inf/NaN, so a
	// plan with any zero-capacity, demand-carrying pod would silently return an
	// empty response body otherwise.
	if len(loads) != 1 || !loads[0].Constraint || loads[0].Rho != InfiniteRho {
		t.Fatalf("team with demand but no capacity must be flagged: %+v", loads)
	}
}

// Spec 014 AC 2.1/2.2: the roster may carry a per-pod capacity loss as a
// percent ("15", "15%") or a fraction ("0.15"); an empty cell inherits the
// plan global; an out-of-range cell refuses the roster, naming the pod.
func TestParseTeamsLossColumn(t *testing.T) {
	csv := "Pod Name,Developers,Capacity Loss %\n" +
		"Alpha,3,15\n" +
		"Beta,2,15%\n" +
		"Gamma,2,0.15\n" +
		"Delta,2,\n"
	teams, err := ParseTeamsCSV([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"Alpha": 0.15, "Beta": 0.15, "Gamma": 0.15, "Delta": 0}
	for _, tm := range teams {
		if tm.CapacityLoss != want[tm.Name] {
			t.Fatalf("%s loss = %v, want %v", tm.Name, tm.CapacityLoss, want[tm.Name])
		}
	}
}

func TestParseTeamsLossOutOfRangeRefusesTheRoster(t *testing.T) {
	for _, cell := range []string{"150%", "-3", "100%", "abc"} {
		csv := "Pod Name,Developers,Capacity Loss %\nAlpha,3," + cell + "\n"
		if _, err := ParseTeamsCSV([]byte(csv)); err == nil {
			t.Fatalf("cell %q must refuse the roster", cell)
		} else if !strings.Contains(err.Error(), "Alpha") {
			t.Fatalf("refusal must name the pod: %v", err)
		}
	}
}
