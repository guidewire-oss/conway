// Package planning turns the "FullKit exercise" planning spreadsheet into a
// structured year plan and the directed cross-pod dependency network, so a
// manager can simulate flow and try the game's levers on real data.
//
// v2 sheet layout (one row per initiative):
//
//	S.No | Initiative | PM | Eng | Architect | PgM | Full-Kit total |
//	  <Team> Sequence | <Team> | <Team> Sequence | <Team> | ...
//
// For each initiative × team:
//   - the <Team> cell holds the estimate in weeks ("No Dependency" = not in path,
//     "TBD"/blank = unestimated);
//   - the "<Team> Sequence" cell holds a comma-separated list of pods this team
//     must wait on ("NONE" = none; the long "replace this…" text = unfilled).
package planning

import (
	"strconv"
	"strings"
)

// TeamWork is one team's involvement in one initiative.
type TeamWork struct {
	Weeks     float64  `json:"weeks"`               // estimated weeks (0 when unestimated)
	Estimated bool     `json:"estimated"`           // a number was entered in the estimate cell
	InPath    bool     `json:"inPath"`              // team participates in this initiative
	DependsOn []string `json:"dependsOn,omitempty"` // pods this team waits on (dep -> team)
}

// Initiative is one planned feature/project for the year.
type Initiative struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Leads       map[string]string   `json:"leads,omitempty"`
	Work        map[string]TeamWork `json:"work"` // team name -> involvement
}

// Plan is the whole parsed sheet.
type Plan struct {
	Teams       []string     `json:"teams"`
	Initiatives []Initiative `json:"initiatives"`
}

// teamCol pairs a team's estimate column with its (optional) dependency column.
type teamCol struct {
	team string
	est  int // index of the <Team> estimate column
	seq  int // index of the "<Team> Sequence" dependency column, or -1
}

// leadKey normalises a "... Lead" header into a short key.
func leadKey(h string) string {
	l := strings.ToLower(h)
	switch {
	case strings.Contains(l, "pm"):
		return "pm"
	case strings.Contains(l, "eng"):
		return "eng"
	case strings.Contains(l, "arch"):
		return "architect"
	case strings.Contains(l, "pgm") || strings.Contains(l, "program"):
		return "pgm"
	}
	return ""
}

// cleanDeps parses a dependency cell into trimmed, de-duped pod names, dropping
// blanks, "NONE", and the unfilled "replace this with a pod name…" placeholder.
func cleanDeps(cell string) []string {
	s := strings.TrimSpace(cell)
	if s == "" || strings.HasPrefix(strings.ToLower(s), "replace this with") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" || strings.EqualFold(p, "none") {
			continue
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// filterKnownDeps drops any dependency name that doesn't case-insensitively
// match a roster pod (knownPods keys are already lowercased/trimmed).
func filterKnownDeps(deps []string, knownPods map[string]bool) []string {
	if len(deps) == 0 {
		return deps
	}
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if knownPods[strings.ToLower(strings.TrimSpace(d))] {
			out = append(out, d)
		}
	}
	return out
}

var numCleaner = strings.NewReplacer(",", "", " ", "", "w", "", "W", "", "wks", "", "wk", "")

// parseWeeks reads a numeric estimate cell. Non-numeric (TBD, No Dependency,
// blank) returns ok=false.
func parseWeeks(cell string) (float64, bool) {
	s := strings.TrimSpace(cell)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(numCleaner.Replace(s), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ParseMatrix builds a Plan from a dense grid (row 0 = header). It auto-detects
// the Initiative/lead columns and the paired "<Team> Sequence" / "<Team>"
// team columns, so it tolerates the sheet's inconsistent header spacing.
//
// roster + strict: a dependency cell is free text a person typed, not a
// validated field — when strict is true, a dependency name that doesn't
// case-insensitively match a name in roster (trimmed) is dropped, so a note
// like "Requirements unknown" can't become a phantom node in the network.
// With no roster, strict has no effect (nothing to validate against).
func ParseMatrix(rows [][]string, roster []string, strict bool) *Plan {
	plan := &Plan{}
	if len(rows) == 0 {
		return plan
	}
	knownPods := map[string]bool{}
	for _, r := range roster {
		knownPods[strings.ToLower(strings.TrimSpace(r))] = true
	}
	strict = strict && len(knownPods) > 0
	hdr := rows[0]
	initIdx := -1
	leads := map[int]string{}
	teamStart := -1
	for i, h := range hdr {
		l := strings.ToLower(strings.TrimSpace(h))
		switch {
		case initIdx < 0 && strings.Contains(l, "initiative"):
			initIdx = i
		case strings.Contains(l, "lead"):
			if k := leadKey(h); k != "" {
				leads[i] = k
			}
		case teamStart < 0 && strings.Contains(l, "estimate") && strings.Contains(l, "full kit"):
			teamStart = i + 1 // team columns begin right after the derived total
		}
	}
	if initIdx < 0 {
		initIdx = 1
	}
	if teamStart < 0 {
		teamStart = 7 // sane default for this sheet
	}

	// pair up the team columns: "<Team> Sequence" or "<Team> Dependencies" (deps,
	// two spellings seen across FullKit sheets in the wild) then "<Team>" (estimate)
	var cols []teamCol
	for i := teamStart; i < len(hdr); {
		h := strings.TrimSpace(hdr[i])
		if h == "" {
			i++
			continue
		}
		lh := strings.ToLower(h)
		if (strings.HasSuffix(lh, "sequence") || strings.HasSuffix(lh, "dependencies")) && i+1 < len(hdr) {
			cols = append(cols, teamCol{team: strings.TrimSpace(hdr[i+1]), seq: i, est: i + 1})
			i += 2
		} else {
			cols = append(cols, teamCol{team: h, seq: -1, est: i})
			i++
		}
	}
	for _, c := range cols {
		plan.Teams = append(plan.Teams, c.team)
	}

	at := func(row []string, i int) string {
		if i >= 0 && i < len(row) {
			return row[i]
		}
		return ""
	}
	for _, row := range rows[1:] {
		raw := strings.TrimSpace(at(row, initIdx))
		if raw == "" {
			continue
		}
		name, desc, _ := strings.Cut(raw, "\n")
		init := Initiative{Name: strings.TrimSpace(name), Description: strings.TrimSpace(desc), Work: map[string]TeamWork{}}
		for idx, key := range leads {
			if v := strings.TrimSpace(at(row, idx)); v != "" {
				if init.Leads == nil {
					init.Leads = map[string]string{}
				}
				init.Leads[key] = v
			}
		}
		for _, c := range cols {
			estCell := strings.TrimSpace(at(row, c.est))
			weeks, estimated := parseWeeks(estCell)
			deps := cleanDeps(at(row, c.seq))
			if strict {
				deps = filterKnownDeps(deps, knownPods)
			}
			notInPath := strings.EqualFold(estCell, "no dependency")
			inPath := estimated || (len(deps) > 0 && !notInPath)
			if !estimated && len(deps) == 0 {
				continue // nothing recorded for this team on this initiative
			}
			init.Work[c.team] = TeamWork{Weeks: weeks, Estimated: estimated, InPath: inPath, DependsOn: deps}
		}
		plan.Initiatives = append(plan.Initiatives, init)
	}
	return plan
}
