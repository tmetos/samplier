package edd

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Row is a single data row: header name -> raw string value.
type Row []string

// Table is a parsed section: ordered headers and rows.
type Table struct {
	Section string
	Headers []string
	Rows    []Row
}

// Workbook is a parsed EDD file: section name -> Table.
type Workbook struct {
	Tables map[string]Table
	// Order preserves the order sections were encountered.
	Order []string
}

// Get returns a section table by name (case-insensitive).
func (wb *Workbook) Get(name string) (Table, bool) {
	if t, ok := wb.Tables[name]; ok {
		return t, true
	}
	for k, t := range wb.Tables {
		if strings.EqualFold(k, name) {
			return t, true
		}
	}
	return Table{}, false
}

// IsHeaderRow reports whether a row is an EDD comment/header row. EDD files
// conventionally prefix header rows with '#'. The first row of a section is always treated as
// the header row by the reader.
func IsHeaderRow(row []string) bool {
	if len(row) == 0 {
		return true
	}
	first := strings.TrimSpace(row[0])
	return strings.HasPrefix(first, "#")
}

// stripComment removes a leading '#' from a header cell.
func stripComment(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSpace(strings.TrimPrefix(s, "#"))
}

// inferSection maps a worksheet or file name onto a known section name.
// Names need not match exactly: the section name may be preceded by other text.
func inferSection(name string, def Definition) (string, bool) {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	for _, s := range def.Sections {
		if strings.EqualFold(base, s.Name) {
			return s.Name, true
		}
		if strings.HasSuffix(strings.ToLower(base), strings.ToLower(s.Name)) {
			return s.Name, true
		}
	}
	return base, false
}

// Read parses an EDD file (XLSX, CSV/TSV/TXT, or ZIP of those) into a Workbook.
func Read(r io.ReaderAt, size int64, filename string, def Definition) (*Workbook, error) {
	ext := strings.ToLower(path.Ext(filename))
	raw, err := io.ReadAll(io.NewSectionReader(r, 0, size))
	if err != nil {
		return nil, err
	}

	switch ext {
	case ".xlsx", ".xlsm":
		return readXLSX(raw, def)
	case ".zip":
		return readZip(raw, def)
	case ".csv", ".tsv", ".txt":
		return readDelimited(nameOrDefault(filename), raw, def)
	default:
		// Fall back: try to sniff content.
		if len(raw) > 4 && bytes.HasPrefix(raw, []byte("PK")) {
			return readXLSX(raw, def)
		}
		return readDelimited(nameOrDefault(filename), raw, def)
	}
}

func nameOrDefault(name string) string { return name }

func readXLSX(raw []byte, def Definition) (*Workbook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	wb := &Workbook{Tables: map[string]Table{}}
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
		}
		if len(rows) == 0 {
			continue
		}
		sectionName, _ := inferSection(sheet, def)
		tbl := buildTable(sectionName, rows)
		if len(tbl.Headers) == 0 {
			continue
		}
		if _, exists := wb.Tables[sectionName]; !exists {
			wb.Order = append(wb.Order, sectionName)
		}
		wb.Tables[sectionName] = tbl
	}
	return wb, nil
}

func readDelimited(filename string, raw []byte, def Definition) (*Workbook, error) {
	delim := ','
	if strings.HasSuffix(strings.ToLower(filename), ".tsv") ||
		strings.HasSuffix(strings.ToLower(filename), ".txt") {
		delim = '\t'
	}
	secName, _ := inferSection(filename, def)

	cr := csv.NewReader(bytes.NewReader(raw))
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse delimited: %w", err)
	}
	wb := &Workbook{Tables: map[string]Table{}}
	tbl := buildTable(secName, records)
	if len(tbl.Headers) > 0 {
		wb.Tables[secName] = tbl
		wb.Order = append(wb.Order, secName)
	}
	return wb, nil
}

func readZip(raw []byte, def Definition) (*Workbook, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	wb := &Workbook{Tables: map[string]Table{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(f.Name))
		if ext != ".csv" && ext != ".tsv" && ext != ".txt" && ext != ".xlsx" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}

		var sub *Workbook
		if ext == ".xlsx" {
			sub, err = readXLSX(data, def)
		} else {
			sub, err = readDelimited(f.Name, data, def)
		}
		if err != nil {
			return nil, err
		}
		for name, tbl := range sub.Tables {
			if _, exists := wb.Tables[name]; !exists {
				wb.Order = append(wb.Order, name)
			}
			wb.Tables[name] = tbl
		}
	}
	return wb, nil
}

// buildTable converts raw grid rows into a Table, skipping '#' comment rows.
func buildTable(section string, rows [][]string) Table {
	tbl := Table{Section: section}
	if len(rows) == 0 {
		return tbl
	}

	// First non-comment row provides headers.
	headerIdx := -1
	for i, r := range rows {
		if len(r) == 0 {
			continue
		}
		if IsHeaderRow(r) && headerIdx == -1 {
			// A '#' row may be either a header or a comment. If it contains a
			// known-looking set of names, treat it as the header.
			tbl.Headers = cleanHeaders(r)
			headerIdx = i
			continue
		}
		if headerIdx == -1 {
			tbl.Headers = cleanHeaders(r)
			headerIdx = i
			continue
		}
	}

	if headerIdx == -1 {
		return tbl
	}
	for _, r := range rows[headerIdx+1:] {
		if isBlank(r) || IsHeaderRow(r) {
			continue
		}
		tbl.Rows = append(tbl.Rows, trimRow(r, len(tbl.Headers)))
	}
	return tbl
}

func cleanHeaders(r []string) []string {
	out := make([]string, len(r))
	for i, h := range r {
		out[i] = strings.ToUpper(stripComment(h))
	}
	return out
}

func trimRow(r []string, n int) Row {
	out := make(Row, n)
	copy(out, r)
	return out
}

func isBlank(r []string) bool {
	for _, v := range r {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// ColumnIndex builds a header->index map for a table.
func (t Table) ColumnIndex() map[string]int {
	idx := make(map[string]int, len(t.Headers))
	for i, h := range t.Headers {
		idx[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	return idx
}

// Value returns the cell value for a header name.
func (r Row) Value(idx map[string]int, header string) (string, bool) {
	i, ok := idx[strings.ToUpper(strings.TrimSpace(header))]
	if !ok || i >= len(r) {
		return "", false
	}
	return strings.TrimSpace(r[i]), true
}
