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
	"math"
	"strconv"
	"strings"
	"time"
)

// TeamWork is one team's involvement in one initiative.
type TeamWork struct {
	Weeks     float64  `json:"weeks"`               // estimated weeks (0 when unestimated)
	Estimated bool     `json:"estimated"`           // a number was entered in the estimate cell
	InPath    bool     `json:"inPath"`              // team participates in this initiative
	DependsOn []string `json:"dependsOn,omitempty"` // pods this team waits on (dep -> team)
}

// Initiative is one planned feature/project for the year.
//
// The sequencing attributes below are all optional, and a plan carrying none of
// them schedules exactly as it behaves today (spec 001 FR-002).
//
// ParseMatrix reads them from §8's optional columns, so an uploaded sheet can
// carry them, and the draft override on POST /api/plan/{id}/schedule can supply
// them for an unsaved plan. In-app editing (PATCH /api/plan/{id}/initiatives) is
// the entry point still to be built; §10 Q9 resolved that both exist, with an
// uploaded sheet winning for the initiatives it names.
type Initiative struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Leads       map[string]string   `json:"leads,omitempty"`
	Work        map[string]TeamWork `json:"work"` // team name -> involvement

	StatedPriority     int      `json:"statedPriority,omitempty"`     // planner's rank, 1 = highest; 0 = unranked
	PriorityLocked     bool     `json:"priorityLocked,omitempty"`     // stated priority is non-negotiable
	TargetDate         string   `json:"targetDate,omitempty"`         // ISO date this is wanted by (one per initiative)
	DateLocked         bool     `json:"dateLocked,omitempty"`         // the date is a commitment, not an aspiration
	Tier               int      `json:"tier,omitempty"`               // requester tier 1-4; T1 contractual .. T4 aspirational
	CostOfDelayPerWeek float64  `json:"costOfDelayPerWeek,omitempty"` // unitless points, 1-10; only ratios are used
	EarliestStart      string   `json:"earliestStart,omitempty"`      // ISO date: not before, for funding/hiring/upstream events
	AfterInitiatives   []string `json:"afterInitiatives,omitempty"`   // cross-initiative precedence, by name
	KitPct             float64  `json:"kitPct,omitempty"`             // full-kit readiness at period start, 0..1
	InFlight           bool     `json:"inFlight,omitempty"`           // carryover already running at period start
	ProgressPct        float64  `json:"progressPct,omitempty"`        // how much of it is already done, 0..1
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

// attrKey maps a header to one of the optional sequencing columns spec 001 §8
// names, or "" for a header this parser does not recognise — which §8 says to
// ignore, exactly as it always has.
//
// Order matters. "Priority Fixed" has to be tested before "Priority", and "Date
// Fixed" before "Target Date", or the lock columns would be read as the values
// they lock. Matching is on substrings because these headers are typed by hand and
// arrive spelled several ways across real sheets.
func attrKey(h string) string {
	l := strings.ToLower(strings.TrimSpace(h))
	fixed := strings.Contains(l, "fix") || strings.Contains(l, "lock")
	switch {
	case l == "":
		return ""
	case strings.Contains(l, "priority") && fixed:
		return "priorityLocked"
	case strings.Contains(l, "priority"):
		return "statedPriority"
	case strings.Contains(l, "date") && fixed:
		return "dateLocked"
	case strings.Contains(l, "target"):
		return "targetDate"
	case strings.Contains(l, "tier"):
		return "tier"
	case strings.Contains(l, "cost of delay") || l == "cod":
		return "costOfDelay"
	case strings.Contains(l, "earliest"):
		return "earliestStart"
	case strings.Contains(l, "initiative") && (strings.Contains(l, "depends") || strings.Contains(l, "after")):
		return "afterInitiatives"
	case strings.Contains(l, "kit") && (strings.Contains(l, "%") || strings.Contains(l, "pct") || strings.Contains(l, "readiness")):
		return "kitPct"
	case strings.Contains(l, "in flight") || strings.Contains(l, "in-flight"):
		return "inFlight"
	case strings.Contains(l, "complete"):
		return "progressPct"
	}
	return ""
}

// splitInitiativeList parses the predecessor cell. It is comma-separated like a
// pod dependency cell, but an initiative name is prose a person typed and may well
// contain a comma ("Payments, phase 2"), so a name may be double-quoted to hold
// itself together — the same convention CSV uses, and the one WriteInitiativesXLSX
// emits. Without it, one comma'd predecessor silently becomes two names that match
// no initiative, and an unmatched predecessor is ignored, so the precedence would
// be lost without a word.
func splitInitiativeList(cell string) []string {
	s := strings.TrimSpace(cell)
	if s == "" || strings.HasPrefix(strings.ToLower(s), "replace this with") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var cur strings.Builder
	inQuotes := false
	flush := func() {
		name := strings.TrimSpace(cur.String())
		cur.Reset()
		if name == "" || strings.EqualFold(name, "none") {
			return
		}
		key := strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, name)
	}
	// Byte-wise, with a one-byte lookahead for the doubled quote. Safe for UTF-8:
	// every byte of a multi-byte rune is >= 0x80, so none can be mistaken for a
	// quote or a comma, and copying bytes through leaves the rune intact.
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' && inQuotes && i+1 < len(s) && s[i+1] == '"':
			cur.WriteByte('"') // "" inside quotes is one literal quote, as in CSV
			i++
		case c == '"':
			inQuotes = !inQuotes
		case c == ',' && !inQuotes:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// quoteInitiativeName wraps a name containing a comma or a quote so
// splitInitiativeList reads it back as one predecessor, doubling any embedded quote
// the way CSV does. Kept next to the parser it has to agree with: stripping the
// quote instead would rename the initiative, which loses the precedence edge just
// as silently as splitting on the comma did.
func quoteInitiativeName(name string) string {
	if !strings.ContainsAny(name, `",`) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// initiativeAttrCols locates the sequencing columns, searching only to the left of
// limit — the full-kit total column. §8 places them there, and the bound is what
// keeps a pod legitimately named "Tier" or "Priority" from being read as one: team
// columns live to the right of that total. First header wins for each attribute.
func initiativeAttrCols(hdr []string, limit int) map[string]int {
	cols := map[string]int{}
	for i := 0; i < limit && i < len(hdr); i++ {
		if k := attrKey(hdr[i]); k != "" {
			if _, seen := cols[k]; !seen {
				cols[k] = i
			}
		}
	}
	return cols
}

// minExcelSerial is 1954-10-03. Below it, a bare number in a date column is far
// likelier to be a typo than a date — no plan targets the 1950s — and reading "5"
// as 1900-01-05 would turn a mistyped cell into a confident, wildly wrong verdict.
const minExcelSerial = 20000

// excelEpoch is 1899-12-30, not 1899-12-31, because Excel's serial numbering
// includes a 1900-02-29 that never existed. The offset that bug creates is baked
// into this epoch, which is correct for every serial above minExcelSerial.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// dateLayouts are the text forms accepted in a date cell: ISO, and the two
// month-name forms. Purely numeric forms like 30/03/2026 are deliberately absent —
// they cannot be told apart from 03/30/2026, and a silently wrong date is worse
// than an unset one, which simply reads as "no date".
var dateLayouts = []string{"2006-01-02", "2006/01/02", "02-Jan-2006", "Jan 2, 2006"}

// parseSheetDate normalises a date cell to ISO yyyy-mm-dd, returning "" for blank,
// "TBD" or anything it cannot read without guessing.
//
// Both forms have to be handled because ReadXLSX renders every cell as its text
// and never reads the number format: a date a planner typed as text arrives as
// that text, while one they formatted as a Date arrives as an Excel serial.
//
// Known limitation: workbooks using the 1904 date system (legacy Mac Excel) would
// be read four years early. Detecting it means parsing xl/workbook.xml's
// workbookPr, which the grid reader does not surface — see spec 001 §10 Q16.
func parseSheetDate(cell string) string {
	s := strings.TrimSpace(cell)
	if s == "" {
		return ""
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	serial, err := strconv.ParseFloat(s, 64)
	if err == nil && !math.IsInf(serial, 0) && serial >= minExcelSerial {
		return excelEpoch.AddDate(0, 0, int(serial)).Format("2006-01-02")
	}
	return ""
}

// parseSheetFraction reads a 0..1 fraction a planner may have written as "0.75",
// "75" or "75%". Anything above 1 is taken as a percentage, since a fraction
// cannot exceed 1, and the result is clamped so a stray "500" cannot escape.
func parseSheetFraction(cell string) float64 {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cell), "%"))
	if s == "" {
		return 0
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	if n > 1 {
		n /= 100
	}
	return clampFrac(n)
}

// parseSheetInt reads a whole number, returning 0 for blank or non-numeric — the
// same "absent" both statedPriority and tier already use.
func parseSheetInt(cell string) int {
	n, err := strconv.Atoi(strings.TrimSpace(cell))
	if err != nil {
		return 0
	}
	return n
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
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
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
	fullKitIdx := -1
	for i, h := range hdr {
		l := strings.ToLower(strings.TrimSpace(h))
		switch {
		// A header the attribute scan claims cannot also be the name column:
		// "Depends on Initiative" contains "initiative", so without this guard it would
		// take the name whenever it happened to sit further left. The guard is scoped to
		// this case on purpose — a full-kit total column named "Full Kit % Estimate"
		// also matches an attribute, and it still has to be found as the total.
		case initIdx < 0 && strings.Contains(l, "initiative") && attrKey(h) == "":
			initIdx = i
		case strings.Contains(l, "lead"):
			if k := leadKey(h); k != "" {
				leads[i] = k
			}
		case teamStart < 0 && strings.Contains(l, "estimate") && strings.Contains(l, "full kit"):
			fullKitIdx = i
			teamStart = i + 1 // team columns begin right after the derived total
		}
	}
	if initIdx < 0 {
		initIdx = 1
	}
	if teamStart < 0 {
		teamStart = 7 // sane default for this sheet
	}
	// §8's sequencing columns sit to the left of the full-kit total. With no such
	// column to anchor on, the default team start is the only bound available.
	attrLimit := teamStart
	if fullKitIdx >= 0 {
		attrLimit = fullKitIdx
	}
	attrs := initiativeAttrCols(hdr, attrLimit)

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
		readInitiativeAttrs(&init, row, attrs, at)
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

// readInitiativeAttrs fills the optional sequencing attributes from the columns
// initiativeAttrCols found. Every one is absent-by-default: a blank or unreadable
// cell leaves the field zero, so a sheet without these columns produces exactly
// the Initiative it produced before they existed (FR-002).
func readInitiativeAttrs(init *Initiative, row []string, attrs map[string]int, at func([]string, int) string) {
	cell := func(key string) string {
		if i, ok := attrs[key]; ok {
			return at(row, i)
		}
		return ""
	}
	init.StatedPriority = parseSheetInt(cell("statedPriority"))
	init.PriorityLocked = truthy(cell("priorityLocked"))
	init.TargetDate = parseSheetDate(cell("targetDate"))
	init.DateLocked = truthy(cell("dateLocked"))
	init.Tier = parseSheetInt(cell("tier"))
	if n, ok := parseWeeks(cell("costOfDelay")); ok {
		init.CostOfDelayPerWeek = n
	}
	init.EarliestStart = parseSheetDate(cell("earliestStart"))
	init.AfterInitiatives = splitInitiativeList(cell("afterInitiatives"))
	init.KitPct = parseSheetFraction(cell("kitPct"))
	init.InFlight = truthy(cell("inFlight"))
	init.ProgressPct = parseSheetFraction(cell("progressPct"))
}
