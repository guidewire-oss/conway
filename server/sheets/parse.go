package sheets

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"conway/server/planning"
)

type Candidate struct {
	Teams       []planning.Team       `json:"teams"`
	Initiatives []planning.Initiative `json:"initiatives"`
	Errors      []string              `json:"errors"`
	Warnings    []string              `json:"warnings"`
	Removals    []string              `json:"removals"`
	Count       int                   `json:"count"`
}

func (c Candidate) Valid() bool { return len(c.Errors) == 0 && c.Count > 0 }
func key(s string) string       { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func cell(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}
func meaningful(row []string) bool {
	for _, s := range row {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}
func (c *Candidate) fail(format string, args ...any) {
	c.Errors = append(c.Errors, fmt.Sprintf(format, args...))
}

// Parse refuses data the permissive file importer would discard, while retaining
// its supported fields (specs/023-linked-google-sheets.md:220).
func Parse(kind string, rows [][]string, current planning.BaselineInputs) Candidate {
	in := planning.NewBaselineInputs(current.Teams, current.Initiatives, current.Params, current.Scheduling)
	c := Candidate{Teams: in.Teams, Initiatives: in.Initiatives, Errors: []string{}, Warnings: []string{}, Removals: []string{}}
	if len(rows) < 2 || len(rows) > MaxRows {
		c.fail("the capture needs a header and at least one data row, within %d rows", MaxRows)
		return c
	}
	for _, r := range rows {
		if len(r) > MaxColumns {
			c.fail("the capture exceeds %d columns", MaxColumns)
			return c
		}
	}
	switch kind {
	case "teams":
		c.parseTeams(rows)
	case "initiatives":
		c.parseInitiatives(rows)
	default:
		c.fail("source kind must be teams or initiatives")
		return c
	}
	if c.Count == 0 {
		c.fail("an empty capture cannot replace planning inputs")
	}
	known := map[string]string{}
	for _, t := range c.Teams {
		known[key(t.Name)] = t.Name
	}
	for _, it := range c.Initiatives {
		for pod, w := range it.Work {
			if w.InPath && known[key(pod)] == "" {
				c.fail("%s requires team %s, which is missing from the roster", it.Name, pod)
			}
			for _, dep := range w.DependsOn {
				if known[key(dep)] == "" {
					c.fail("%s / %s requires dependency team %s, which is missing from the roster", it.Name, pod, dep)
				}
			}
		}
	}
	if err := planning.ValidateInitiativeNames(c.Initiatives); err != nil {
		c.fail("%s", err)
	}
	oldNames, newNames := map[string]bool{}, map[string]bool{}
	if kind == "teams" {
		for _, t := range current.Teams {
			oldNames[t.Name] = true
		}
		for _, t := range c.Teams {
			newNames[t.Name] = true
		}
	} else {
		for _, it := range current.Initiatives {
			oldNames[it.Name] = true
		}
		for _, it := range c.Initiatives {
			newNames[it.Name] = true
		}
		byName := map[string]planning.Initiative{}
		for _, it := range c.Initiatives {
			byName[it.Name] = it
		}
		for _, old := range current.Initiatives {
			if next, ok := byName[old.Name]; ok {
				for pod, w := range old.Work {
					if w.InPath && !next.Work[pod].InPath {
						c.Removals = append(c.Removals, old.Name+" / "+pod+" assigned work")
					}
				}
			}
		}
	}
	for name := range oldNames {
		if !newNames[name] {
			c.Removals = append(c.Removals, name)
		}
	}
	sort.Strings(c.Removals)
	if len(c.Removals) > 0 {
		c.Warnings = append(c.Warnings, "Scope removal requires explicit review and acknowledgement; it cannot apply automatically.")
	}
	if c.Valid() {
		if msg := planning.ValidateLanePinsWithParams(c.Initiatives, current.Scheduling, current.Params, c.Teams); msg != "" {
			c.fail("saved placement is incompatible: %s", msg)
		}
	}
	return c
}

func (c *Candidate) parseTeams(rows [][]string) {
	grid := make([][]string, len(rows))
	for i, r := range rows {
		grid[i] = append([]string(nil), r...)
	}
	rows = grid
	nameCol := -1
	seenHeaders := map[string]bool{}
	seenFields := map[string]bool{}
	for i, h := range rows[0] {
		k := key(h)
		if k == "" {
			for _, r := range rows[1:] {
				if cell(r, i) != "" {
					c.fail("column %d has values without a roster header", i+1)
					break
				}
			}
			continue
		}
		if seenHeaders[k] {
			c.fail("duplicate roster column %q", h)
		}
		seenHeaders[k] = true
		field := rosterField(k)
		if field != "" && seenFields[field] {
			c.fail("multiple roster columns supply %s", field)
		}
		seenFields[field] = true
		if k == "name" || k == "team" || k == "pod" || k == "pod name" {
			if nameCol >= 0 {
				c.fail("multiple roster name columns")
			}
			nameCol = i
		}
		known := k == "name" || k == "team" || k == "pod" || k == "pod name" || strings.HasPrefix(k, "developer") || k == "location" || k == "site" || strings.HasPrefix(k, "pair") || k == "tracks" || k == "capacity" || strings.Contains(k, "stream") || strings.Contains(k, "parallel") || strings.Contains(k, "lane") || strings.Contains(k, "loss") || strings.Contains(k, "timezone")
		if !known {
			for _, r := range rows[1:] {
				if cell(r, i) != "" {
					c.fail("unrecognized populated roster column %q", h)
					break
				}
			}
		}
		for n, r := range rows[1:] {
			v := cell(r, i)
			if v == "" {
				continue
			}
			if strings.HasPrefix(k, "developer") {
				if count, ok := number(v); ok {
					if count < 0 || math.Trunc(count) != count || count > 10000 {
						c.fail("row %d: developer count must be a nonnegative whole number", n+2)
					} else {
						r[i] = strconv.FormatFloat(count, 'f', -1, 64)
					}
				}
			}
			if k == "tracks" || k == "capacity" || strings.Contains(k, "stream") || strings.Contains(k, "parallel") || strings.Contains(k, "lane") {
				num, err := strconv.Atoi(v)
				if err != nil || num < 0 || num > 1000 {
					c.fail("row %d: tracks must be a whole number from 0 to 1000", n+2)
				}
			}
			if strings.HasPrefix(k, "pair") && !boolCell(v) {
				c.fail("row %d: pairing must be yes or no", n+2)
			}
		}
	}
	if nameCol < 0 {
		c.fail("the roster needs a Name or Team column")
		return
	}
	seen := map[string]bool{}
	for i, r := range rows[1:] {
		if !meaningful(r) {
			continue
		}
		name := cell(r, nameCol)
		if name == "" {
			c.fail("row %d has values but no team name", i+2)
		}
		if seen[key(name)] {
			c.fail("duplicate team name %q", name)
		}
		seen[key(name)] = true
		if len(r) > len(rows[0]) {
			c.fail("row %d has values beyond its headers", i+2)
		}
	}
	parsed, err := planning.ParseTeamsRows(rows)
	if err != nil {
		c.fail("%s", err)
		return
	}
	// Matching identities retain the plan's spelling, so case-only source edits
	// cannot disconnect existing work or calendars.
	canonical := map[string]string{}
	for _, t := range c.Teams {
		canonical[key(t.Name)] = t.Name
	}
	for i := range parsed {
		if name := canonical[key(parsed[i].Name)]; name != "" {
			parsed[i].Name = name
		}
	}
	c.Teams = parsed
	c.Count = len(parsed)
}

func boolCell(v string) bool {
	switch key(v) {
	case "y", "yes", "true", "1", "pairs", "pairing", "n", "no", "false", "0":
		return true
	}
	return false
}
func number(v string) (float64, bool) {
	n, e := strconv.ParseFloat(v, 64)
	return n, e == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}

type teamColumn struct {
	name     string
	est, dep int
}

func rosterField(k string) string {
	switch {
	case k == "name" || k == "team" || k == "pod" || k == "pod name":
		return "name"
	case strings.HasPrefix(k, "developer"):
		return "developers"
	case k == "location" || k == "site":
		return "site"
	case strings.HasPrefix(k, "pair"):
		return "pairs"
	case k == "tracks" || k == "capacity" || strings.Contains(k, "stream") || strings.Contains(k, "parallel") || strings.Contains(k, "lane"):
		return "tracks"
	case strings.Contains(k, "loss"):
		return "loss"
	case strings.Contains(k, "timezone"):
		return "timezone"
	}
	return ""
}

func matrixField(k string) string {
	fixed := strings.Contains(k, "fix") || strings.Contains(k, "lock")
	switch {
	case k == "epics" || k == "epic keys":
		return "epics"
	case strings.Contains(k, "priority") && fixed:
		return "priority lock"
	case strings.Contains(k, "priority"):
		return "priority"
	case strings.Contains(k, "date") && fixed:
		return "date lock"
	case strings.Contains(k, "target"):
		return "target"
	case strings.Contains(k, "tier"):
		return "tier"
	case strings.Contains(k, "cost of delay") || k == "cod":
		return "cost"
	case strings.Contains(k, "earliest"):
		return "earliest"
	case strings.Contains(k, "initiative") && (strings.Contains(k, "depends") || strings.Contains(k, "after")):
		return "predecessor"
	case strings.Contains(k, "kit") && (strings.Contains(k, "%") || strings.Contains(k, "pct") || strings.Contains(k, "readiness")):
		return "kit"
	case strings.Contains(k, "in flight") || strings.Contains(k, "in-flight"):
		return "flight"
	case strings.Contains(k, "complete"):
		return "progress"
	case strings.Contains(k, "lead"):
		for _, role := range []string{"pm", "eng", "arch", "pgm", "program"} {
			if strings.Contains(k, role) {
				if role == "arch" {
					role = "architect"
				}
				if role == "program" {
					role = "pgm"
				}
				return "lead " + role
			}
		}
	}
	return ""
}

func validMetadata(field, v string) bool {
	switch field {
	case "priority lock", "date lock", "flight":
		return boolCell(v)
	case "priority", "tier":
		n, ok := number(v)
		return ok && n >= 0 && math.Trunc(n) == n && n <= 1000000
	case "cost":
		n, ok := number(v)
		return ok && n >= 0 && n <= 1000000
	case "kit", "progress":
		n, ok := number(strings.TrimSpace(strings.TrimSuffix(v, "%")))
		return ok && n >= 0 && n <= 100
	case "target", "earliest":
		for _, layout := range []string{"2006-01-02", "2006/01/02", "02-Jan-2006", "Jan 2, 2006"} {
			if _, err := time.Parse(layout, v); err == nil {
				return true
			}
		}
		n, ok := number(v)
		return ok && n >= 20000 && n <= 2958465
	}
	return true
}

func (c *Candidate) parseInitiatives(rows [][]string) {
	grid := make([][]string, len(rows))
	for i, r := range rows {
		grid[i] = append([]string(nil), r...)
	}
	hdr := grid[0]
	anchor, initCol := -1, -1
	epicCol := false
	for i, h := range hdr {
		k := key(h)
		if strings.Contains(k, "full kit") && strings.Contains(k, "estimate") {
			anchor = i
		}
		if strings.Contains(k, "initiative") && !strings.Contains(k, "depends") && !strings.Contains(k, "after") {
			if initCol >= 0 {
				c.fail("multiple initiative name columns")
			}
			initCol = i
		}
		if k == "epics" || k == "epic keys" {
			epicCol = true
		}
	}
	if anchor < 0 || initCol < 0 {
		c.fail("the matrix needs Initiative and Full Kit Estimate headers before its team columns")
		return
	}
	roster := map[string]string{}
	names := []string{}
	for _, t := range c.Teams {
		roster[key(t.Name)] = t.Name
		names = append(names, t.Name)
	}
	if len(roster) == 0 {
		c.fail("link or upload a team roster before linking initiatives")
		return
	}
	cols := []teamColumn{}
	seen := map[string]bool{}
	for i := anchor + 1; i < len(hdr); i++ {
		if cell(hdr, i) == "" {
			for _, r := range grid[1:] {
				if cell(r, i) != "" {
					c.fail("column %d has values without a team header", i+1)
					break
				}
			}
			continue
		}
		dep := -1
		k := key(hdr[i])
		if strings.HasSuffix(k, "sequence") || strings.HasSuffix(k, "dependencies") {
			dep = i
			i++
			if i >= len(hdr) {
				c.fail("a team dependency column has no following estimate column")
				break
			}
		}
		name := roster[key(hdr[i])]
		if name == "" {
			c.fail("unknown team column %q", hdr[i])
			continue
		}
		if seen[name] {
			c.fail("duplicate team estimate column %q", name)
		}
		seen[name] = true
		hdr[i] = name
		if dep >= 0 {
			prefix := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(key(hdr[dep]), "dependencies"), "sequence"))
			if roster[prefix] != name {
				c.fail("dependency column %q does not match the following team %s", hdr[dep], name)
			}
			hdr[dep] = name + " Dependencies"
		}
		cols = append(cols, teamColumn{name, i, dep})
	}
	if len(cols) == 0 {
		c.fail("the matrix has no recognized team estimate columns")
	}
	seenAttrs := map[string]bool{}
	for i, h := range hdr[:anchor] {
		if i == initCol {
			continue
		}
		k := key(h)
		attr := matrixField(k)
		known := k == "s.no" || k == "s.no." || k == "#" || k == "no" || k == "no." || attr != ""
		if attr != "" && seenAttrs[attr] {
			c.fail("multiple matrix columns supply %s", attr)
		}
		seenAttrs[attr] = true
		if !known {
			for _, r := range grid[1:] {
				if cell(r, i) != "" {
					c.fail("unrecognized populated initiative column %q", h)
					break
				}
			}
		}
		for n, r := range grid[1:] {
			if v := cell(r, i); v != "" && !validMetadata(attr, v) {
				c.fail("row %d: %s value %q is not a supported planning value", n+2, h, v)
			}
		}
	}
	for n, r := range grid[1:] {
		if !meaningful(r) {
			continue
		}
		if cell(r, initCol) == "" {
			c.fail("row %d has values but no initiative name", n+2)
		}
		if len(r) > len(hdr) {
			c.fail("row %d has values beyond its headers", n+2)
		}
		for _, col := range cols {
			v := cell(r, col.est)
			clean := strings.NewReplacer(",", "", " ", "", "wks", "", "wk", "", "w", "", "W", "").Replace(v)
			if v != "" && !strings.EqualFold(v, "tbd") && !strings.EqualFold(v, "no dependency") {
				f, ok := number(clean)
				if !ok || f < 0 || f > 10000 {
					c.fail("row %d / %s: estimate %q must be nonnegative weeks, TBD, or No Dependency", n+2, col.name, v)
				} else {
					r[col.est] = strconv.FormatFloat(f, 'f', -1, 64)
				}
			}
			if col.dep >= 0 {
				d := cell(r, col.dep)
				if strings.EqualFold(d, "none") || d == "" {
					continue
				}
				if strings.HasPrefix(key(d), "replace this with") {
					c.Warnings = append(c.Warnings, fmt.Sprintf("Row %d / %s has an unfilled dependency placeholder.", n+2, col.name))
					continue
				}
				parts := strings.Split(d, ",")
				for j, p := range parts {
					canonical := roster[key(p)]
					if canonical == "" {
						c.fail("row %d / %s: unknown team dependency %q", n+2, col.name, p)
					} else {
						parts[j] = canonical
					}
				}
				r[col.dep] = strings.Join(parts, ",")
			}
		}
	}
	parsed := planning.ParseMatrix(grid, names, false)
	old := map[string]planning.Initiative{}
	for _, it := range c.Initiatives {
		old[key(it.Name)] = it
	}
	rowIndex := 0
	for i := range parsed.Initiatives {
		it := &parsed.Initiatives[i]
		for rowIndex < len(grid)-1 && cell(grid[rowIndex+1], initCol) == "" {
			rowIndex++
		}
		r := grid[rowIndex+1]
		rowIndex++
		if prior, ok := old[key(it.Name)]; ok {
			it.Name = prior.Name
			it.PinnedStarts = prior.PinnedStarts
			it.PinnedLanes = prior.PinnedLanes
			if !epicCol {
				it.EpicKeys = prior.EpicKeys
			}
		}
		for _, col := range cols {
			if strings.EqualFold(cell(r, col.est), "tbd") {
				w := it.Work[col.name]
				w.InPath = true
				w.Estimated = false
				it.Work[col.name] = w
				c.Warnings = append(c.Warnings, it.Name+" / "+col.name+" has an unknown estimate (TBD).")
			}
		}
		keys, err := planning.NormalizeEpicKeys(it.EpicKeys)
		if err != nil {
			c.fail("%s: %s", it.Name, err)
		} else {
			it.EpicKeys = keys
		}
	}
	byName := map[string]string{}
	for _, it := range parsed.Initiatives {
		byName[key(it.Name)] = it.Name
	}
	for i := range parsed.Initiatives {
		it := &parsed.Initiatives[i]
		for j, dep := range it.AfterInitiatives {
			canonical := byName[key(dep)]
			if canonical == "" {
				c.fail("%s: unknown predecessor initiative %q", it.Name, dep)
			} else {
				it.AfterInitiatives[j] = canonical
			}
		}
	}
	c.Initiatives = parsed.Initiatives
	c.Count = len(c.Initiatives)
}
