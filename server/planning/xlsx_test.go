package planning

import "testing"

// buildXLSX reuses the production writer so tests exercise the same code path.
func buildXLSX(rows [][]string) []byte { return WriteXLSX("FullKit exercise", rows) }

func TestReadXLSXRoundTrip(t *testing.T) {
	rows, err := ReadXLSX(buildXLSX(sampleRows()), "FullKit exercise")
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	got := ParseMatrix(rows, nil, false)
	if len(got.Teams) != 3 {
		t.Fatalf("teams after round-trip: %v", got.Teams)
	}
	if len(got.Initiatives) != 2 {
		t.Fatalf("initiatives after round-trip: %d", len(got.Initiatives))
	}
	aj := got.Initiatives[0].Work["Alpha"]
	if aj.Weeks != 3 || !aj.Estimated {
		t.Fatalf("numeric cell lost in round-trip: %+v", aj)
	}
	if len(aj.DependsOn) != 2 || aj.DependsOn[0] != "Beta" || aj.DependsOn[1] != "Gamma" {
		t.Fatalf("deps lost/untrimmed in round-trip: %v", aj.DependsOn)
	}
}

func TestReadXLSXSheetMatchCaseInsensitive(t *testing.T) {
	if _, err := ReadXLSX(buildXLSX(sampleRows()), "  fullkit EXERCISE "); err != nil {
		t.Fatalf("should match sheet case-insensitively/trimmed: %v", err)
	}
}

// the generated sample initiatives must read back into the demo plan.
func TestWriteInitiativesRoundTrip(t *testing.T) {
	teams, inits := Demo()
	rows, err := ReadXLSX(WriteInitiativesXLSX(teams, inits), "FullKit exercise")
	if err != nil {
		t.Fatalf("read generated xlsx: %v", err)
	}
	got := ParseMatrix(rows, nil, false)
	if len(got.Initiatives) != len(inits) {
		t.Fatalf("initiatives: %d vs %d", len(got.Initiatives), len(inits))
	}
	teamsBack, err := ParseTeamsRows(readCSV(t, WriteTeamsCSV(teams)))
	if err != nil {
		t.Fatalf("teams csv round-trip parse: %v", err)
	}
	if len(teamsBack) != len(teams) {
		t.Fatalf("teams csv round-trip: %d vs %d", len(teamsBack), len(teams))
	}
	// a known dependency survives
	found := false
	for _, it := range got.Initiatives {
		if w, ok := it.Work["Atlas"]; ok {
			for _, d := range w.DependsOn {
				if d == "Delta" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected Atlas -> Delta dependency to survive the write/read")
	}
}

func readCSV(t *testing.T, b []byte) [][]string {
	rows, err := ReadGrid(b, "")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
