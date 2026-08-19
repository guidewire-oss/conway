package planning

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// WriteXLSX builds a minimal, Excel-valid single-sheet workbook: numeric-looking
// cells become numbers, everything else inline strings.
func WriteXLSX(sheetName string, rows [][]string) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&sb, `<row r="%d">`, r+1)
		for c, v := range row {
			if v == "" {
				continue
			}
			ref := colLetters(c) + strconv.Itoa(r+1)
			if isNumeric(v) {
				fmt.Fprintf(&sb, `<c r="%s"><v>%s</v></c>`, ref, v)
			} else {
				var e bytes.Buffer
				xml.EscapeText(&e, []byte(v))
				fmt.Fprintf(&sb, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, e.String())
			}
		}
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)

	var name bytes.Buffer
	xml.EscapeText(&name, []byte(sheetName))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(p, content string) { w, _ := zw.Create(p); w.Write([]byte(content)) }
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`+
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets><sheet name="`+name.String()+`" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/worksheets/sheet1.xml", sb.String())
	zw.Close()
	return buf.Bytes()
}

func colLetters(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}

// WriteInitiativesXLSX emits a v2 FullKit matrix from a plan: paired
// "<Team> Sequence" (deps) + "<Team>" (weeks) columns. Non-involved teams are
// marked "No Dependency".
func WriteInitiativesXLSX(teams []Team, inits []Initiative) []byte {
	var names []string
	for _, t := range teams {
		names = append(names, t.Name)
	}
	// The optional sequencing columns sit between the leads and the derived total,
	// which is where spec 001 §8 puts them and where initiativeAttrCols looks. They
	// are written even when every value is blank: this workbook is what
	// /api/sample/initiatives.xlsx serves, so the header row is how a planner finds
	// out the columns exist at all.
	hdr := []string{"S. No", "Initiative ", "PM Lead", "Engg Lead", "Architect lead", "PgM Lead",
		"Priority", "Priority Fixed", "Target Date", "Date Fixed", "Tier", "Cost of Delay",
		"Earliest Start", "Depends on Initiative", "Kit %", "In Flight", "% Complete",
		"Estimate in Weeks across teams required for Full Kit"}
	teamCols := len(hdr) // team columns begin right after the derived total
	for _, n := range names {
		hdr = append(hdr, n+" Sequence", n)
	}
	rows := [][]string{hdr}
	for idx, it := range inits {
		row := make([]string, len(hdr))
		row[0] = strconv.Itoa(idx + 1)
		row[1] = it.Name
		if it.Leads != nil {
			row[2], row[3], row[4], row[5] = it.Leads["pm"], it.Leads["eng"], it.Leads["architect"], it.Leads["pgm"]
		}
		row[6] = intCell(it.StatedPriority)
		row[7] = boolCell(it.PriorityLocked)
		row[8] = it.TargetDate
		row[9] = boolCell(it.DateLocked)
		row[10] = intCell(it.Tier)
		row[11] = numCell(it.CostOfDelayPerWeek)
		row[12] = it.EarliestStart
		row[13] = joinInitiativeNames(it.AfterInitiatives)
		row[14] = numCell(it.KitPct)
		row[15] = boolCell(it.InFlight)
		row[16] = numCell(it.ProgressPct)
		row[17] = "0" // derived total in the real sheet
		for j, n := range names {
			seq, est := teamCols+j*2, teamCols+j*2+1
			if wk, ok := it.Work[n]; ok && wk.InPath {
				if len(wk.DependsOn) > 0 {
					row[seq] = strings.Join(wk.DependsOn, ", ")
				} else {
					row[seq] = "NONE"
				}
				row[est] = strconv.FormatFloat(wk.Weeks, 'f', -1, 64)
			} else {
				row[seq] = "NONE"
				row[est] = "No Dependency"
			}
		}
		rows = append(rows, row)
	}
	return WriteXLSX("FullKit exercise", rows)
}

// WriteTeamsCSV emits a roster matching the sample initiatives (explicit Tracks).
func WriteTeamsCSV(teams []Team) []byte {
	var b bytes.Buffer
	wr := csv.NewWriter(&b)
	wr.Write([]string{"Pod Name", "Tracks", "Location", "Pairs"})
	for _, t := range teams {
		pairs := "no"
		if t.Pairs {
			pairs = "yes"
		}
		wr.Write([]string{t.Name, strconv.Itoa(t.EffectiveTracks()), t.Site, pairs})
	}
	wr.Flush()
	return b.Bytes()
}

// Cell writers for the optional attributes. Zero means absent for all three, so
// they emit an empty cell rather than a "0" or a "no" that would read as a
// deliberate choice a planner never made.
func intCell(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func numCell(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func boolCell(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// joinInitiativeNames writes the predecessor cell, quoting any name that contains
// a comma so splitInitiativeList reads it back as one name rather than two.
func joinInitiativeNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, quoteInitiativeName(n))
	}
	return strings.Join(quoted, ", ")
}
