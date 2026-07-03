package planning

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// ReadXLSX extracts a worksheet (matched by name, case-insensitive, trimmed)
// from .xlsx bytes into a dense [][]string grid for ParseMatrix. It handles
// shared strings and inline strings; numbers come through as their text. The
// parser is namespace-agnostic (matches local element names) so it tolerates the
// various namespace prefixes Excel/Sheets emit.
func ReadXLSX(data []byte, sheetMatch string) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = b
	}
	shared := parseSharedStrings(files["xl/sharedStrings.xml"])
	target, err := pickSheetPath(files, sheetMatch)
	if err != nil {
		return nil, err
	}
	return parseSheet(files[target], shared)
}

func parseSharedStrings(b []byte) []string {
	var out []string
	if len(b) == 0 {
		return out
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	var cur strings.Builder
	inSI, inT := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				if inSI {
					inT = true
				}
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inT = false
			case "si":
				if inSI {
					out = append(out, cur.String())
					inSI = false
					cur.Reset()
				}
			}
		}
	}
	return out
}

func pickSheetPath(files map[string][]byte, match string) (string, error) {
	type sheet struct{ name, rid string }
	var sheets []sheet
	dec := xml.NewDecoder(bytes.NewReader(files["xl/workbook.xml"]))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sheet" {
			var s sheet
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "name":
					s.name = a.Value
				case "id": // r:id -> local name "id"
					s.rid = a.Value
				}
			}
			sheets = append(sheets, s)
		}
	}
	rid2t := map[string]string{}
	dec = xml.NewDecoder(bytes.NewReader(files["xl/_rels/workbook.xml.rels"]))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
			var id, tgt string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "Id":
					id = a.Value
				case "Target":
					tgt = a.Value
				}
			}
			rid2t[id] = tgt
		}
	}
	m := strings.ToLower(strings.TrimSpace(match))
	var chosen sheet
	for _, s := range sheets {
		if strings.ToLower(strings.TrimSpace(s.name)) == m {
			chosen = s
			break
		}
	}
	if chosen.rid == "" {
		for _, s := range sheets {
			if m != "" && strings.Contains(strings.ToLower(s.name), m) {
				chosen = s
				break
			}
		}
	}
	if chosen.rid == "" && len(sheets) > 0 {
		chosen = sheets[0]
	}
	if chosen.rid == "" {
		return "", fmt.Errorf("no worksheet matching %q", match)
	}
	tgt := strings.TrimPrefix(rid2t[chosen.rid], "/")
	if tgt == "" {
		return "", fmt.Errorf("worksheet relationship %q not found", chosen.rid)
	}
	if !strings.HasPrefix(tgt, "xl/") {
		tgt = "xl/" + tgt
	}
	if _, ok := files[tgt]; !ok {
		return "", fmt.Errorf("worksheet file %q missing in archive", tgt)
	}
	return tgt, nil
}

var refLetters = regexp.MustCompile(`^[A-Za-z]+`)

func colIndex(ref string) int {
	letters := refLetters.FindString(ref)
	if letters == "" {
		return -1
	}
	n := 0
	for _, c := range strings.ToUpper(letters) {
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

func parseSheet(b []byte, shared []string) ([][]string, error) {
	var rows [][]string
	dec := xml.NewDecoder(bytes.NewReader(b))
	var row map[int]string
	maxCol := -1
	var curRef, curType string
	var val strings.Builder
	inV, inT := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = map[int]string{}
				maxCol = -1
			case "c":
				curRef, curType = "", ""
				val.Reset()
				inV, inT = false, false
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "r":
						curRef = a.Value
					case "t":
						curType = a.Value
					}
				}
			case "v":
				inV = true
				val.Reset()
			case "t": // inline string text (inside <is>)
				inT = true
			}
		case xml.CharData:
			if inV || inT {
				val.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
			case "t":
				inT = false
			case "c":
				ci := colIndex(curRef)
				cell := val.String()
				if curType == "s" { // shared-string index
					if idx, err := strconv.Atoi(strings.TrimSpace(cell)); err == nil && idx >= 0 && idx < len(shared) {
						cell = shared[idx]
					}
				}
				if ci >= 0 && row != nil {
					row[ci] = cell
					if ci > maxCol {
						maxCol = ci
					}
				}
			case "row":
				out := make([]string, maxCol+1)
				for i := 0; i <= maxCol; i++ {
					out[i] = row[i]
				}
				rows = append(rows, out)
			}
		}
	}
	return rows, nil
}
