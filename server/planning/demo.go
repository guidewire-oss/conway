package planning

// Demo returns a realistic, self-contained sample plan (fictional pod names, a
// year's worth of platform initiatives) engineered so a couple of pods are
// clearly over capacity — so the network/constraints light up out of the box
// without an upload. Delta (a small pod that everything depends on) ends up
// the red constraint; Ember runs hot amber.
func Demo() ([]Team, []Initiative) {
	teams := []Team{
		{Name: "Atlas", Tracks: 6, Site: "Austin"},
		{Name: "Beacon", Tracks: 7, Site: "Dublin"},
		{Name: "Cascade", Tracks: 7, Site: "Toronto"},
		{Name: "Delta", Tracks: 2, Site: "Remote"},
		{Name: "Ember", Tracks: 2, Site: "Warsaw"},
		{Name: "Fjord", Tracks: 4, Site: "Oslo"},
		{Name: "Granite", Tracks: 5, Site: "Denver"},
		{Name: "Harbor", Tracks: 6, Site: "Singapore"},
		{Name: "Ibis", Tracks: 5, Site: "Remote"},
		{Name: "Juniper", Tracks: 4, Site: "Berlin"},
	}
	w := func(weeks float64, deps ...string) TeamWork {
		return TeamWork{Weeks: weeks, Estimated: true, InPath: true, DependsOn: deps}
	}
	mk := func(name string, work map[string]TeamWork) Initiative {
		return Initiative{Name: name, Work: work}
	}
	inits := []Initiative{
		mk("Self-service app platform", map[string]TeamWork{"Delta": w(10), "Atlas": w(5, "Delta"), "Cascade": w(4, "Atlas")}),
		mk("Telemetry GA", map[string]TeamWork{"Delta": w(12), "Granite": w(4, "Delta")}),
		mk("Autoscaling rollout", map[string]TeamWork{"Delta": w(10), "Fjord": w(3, "Delta")}),
		mk("Managed database MVP", map[string]TeamWork{"Delta": w(9), "Harbor": w(5, "Delta")}),
		mk("Disaster recovery for event streaming", map[string]TeamWork{"Delta": w(8), "Ibis": w(4, "Delta")}),
		mk("Bring-your-own-auth (early access)", map[string]TeamWork{"Ember": w(10), "Atlas": w(4, "Ember"), "Beacon": w(3)}),
		mk("SCIM provisioning", map[string]TeamWork{"Beacon": w(6), "Ember": w(11, "Beacon")}),
		mk("Secrets rotation", map[string]TeamWork{"Ember": w(12), "Juniper": w(4, "Ember")}),
		mk("Tenant isolation", map[string]TeamWork{"Ember": w(13, "Cascade"), "Cascade": w(5), "Granite": w(3)}),
		mk("Access-control rollout", map[string]TeamWork{"Granite": w(6), "Harbor": w(4), "Ibis": w(3)}),
	}
	return teams, inits
}
