package edd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Severity is the severity of a validation finding.
type Severity string

const (
	// SeverityError blocks a commit.
	SeverityError Severity = "ERROR"
	// SeverityWarning allows a commit but flags the row.
	SeverityWarning Severity = "WARNING"
)

// Finding is one validation problem.
type Finding struct {
	Section  string   `json:"section"`
	Row      int      `json:"row"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

// Report is the outcome of validating a workbook.
type Report struct {
	Findings []Finding `json:"findings"`
}

// Errors returns the error-level findings.
func (r *Report) Errors() []Finding {
	return r.filter(SeverityError)
}

// Warnings returns the warning-level findings.
func (r *Report) Warnings() []Finding {
	return r.filter(SeverityWarning)
}

func (r *Report) filter(s Severity) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == s {
			out = append(out, f)
		}
	}
	return out
}

// Valid reports whether the workbook may be committed.
func (r *Report) Valid() bool { return len(r.Errors()) == 0 }

func (r *Report) add(f Finding) { r.Findings = append(r.Findings, f) }

// ReferenceSet holds the valid codes for a reference table.
type ReferenceSet map[string][]string

// Contains reports whether the reference table contains value (case-insensitive).
func (rs ReferenceSet) Contains(table, value string) bool {
	codes, ok := rs[strings.ToUpper(table)]
	if !ok {
		return true // unknown table: do not block
	}
	for _, c := range codes {
		if strings.EqualFold(c, value) {
			return true
		}
	}
	return false
}

// Validator validates a workbook against a format definition.
type Validator struct {
	Definition Definition
	References ReferenceSet
	// Now supplies the current time (injectable for tests).
	Now func() time.Time
}

// NewValidator constructs a validator.
func NewValidator(def Definition, refs ReferenceSet) *Validator {
	if refs == nil {
		refs = ReferenceSet{}
	}
	return &Validator{Definition: def, References: refs, Now: time.Now}
}

// Validate checks structural rules, types, reference values, and parent/child
// integrity. It never mutates the workbook.
func (v *Validator) Validate(wb *Workbook) *Report {
	report := &Report{}

	present := map[string]bool{}
	for name := range wb.Tables {
		present[strings.ToUpper(name)] = true
	}

	// 1. Required sections present.
	for _, s := range v.Definition.Sections {
		if !s.Optional && len(s.Columns) > 0 && !isSampleSection(s) {
			if !present[strings.ToUpper(s.Name)] {
				report.add(Finding{
					Section: s.Name, Severity: SeverityError,
					Message: "required section is missing",
				})
			}
		}
	}

	// Field/lab sample sections: exactly one is required.
	if v.Definition.Code == "EFWEDD" {
		field := present["EFW2FSAMPLE"]
		lab := present["EFW2LABSMP"]
		switch {
		case !field && !lab:
			report.add(Finding{
				Section: "EFW2FSample", Severity: SeverityError,
				Message: "one of EFW2FSample or EFW2LabSMP is required",
			})
		case field && lab:
			report.add(Finding{
				Section: "EFW2FSample", Severity: SeverityWarning,
				Message: "both EFW2FSample and EFW2LabSMP present; only one is used",
			})
		}
	}

	// 2. Column-level checks per section.
	for _, s := range v.Definition.Sections {
		tbl, ok := wb.Get(s.Name)
		if !ok {
			continue
		}
		idx := tbl.ColumnIndex()

		// Required columns present in the header.
		for _, c := range s.Columns {
			if c.Required {
				if _, ok := idx[strings.ToUpper(c.Header)]; !ok {
					report.add(Finding{
						Section: s.Name, Row: 0, Field: c.Header, Severity: SeverityError,
						Message: "required column is missing",
					})
				}
			}
		}

		// Row-level checks.
		for i, row := range tbl.Rows {
			rowNum := i + 2 // header is row 1
			for _, c := range s.Columns {
				raw, has := row.Value(idx, c.Header)
				if !has {
					continue
				}
				if raw == "" {
					if c.Required {
						report.add(Finding{
							Section: s.Name, Row: rowNum, Field: c.Field,
							Severity: SeverityError,
							Message:  "required value is empty",
						})
					}
					continue
				}
				if err := validateType(c.Type, raw); err != nil {
					report.add(Finding{
						Section: s.Name, Row: rowNum, Field: c.Field,
						Severity: SeverityError,
						Message:  fmt.Sprintf("%v (value %q)", err, raw),
					})
					continue
				}
				if c.ReferenceTable != "" && !v.References.Contains(c.ReferenceTable, raw) {
					report.add(Finding{
						Section: s.Name, Row: rowNum, Field: c.Field,
						Severity: SeverityError,
						Message:  fmt.Sprintf("value %q is not in reference table %s", raw, c.ReferenceTable),
					})
				}
			}
		}
	}

	// 3. Parent/child integrity.
	v.checkIntegrity(wb, report)
	return report
}

func isSampleSection(s Section) bool {
	return s.TargetTable == "sample"
}

// checkIntegrity verifies that child rows reference existing parent rows.
func (v *Validator) checkIntegrity(wb *Workbook, report *Report) {
	sampleIDs := collectKeys(wb, "EFW2FSample", "SAMPLE_ID")
	for id := range collectKeys(wb, "EFW2LabSMP", "SAMPLE_ID") {
		sampleIDs[id] = true
	}

	// Tests must reference a known sample.
	if tst, ok := wb.Get("EFW2LabTST"); ok {
		idx := tst.ColumnIndex()
		for i, row := range tst.Rows {
			id, _ := row.Value(idx, "SAMPLE_ID")
			if id == "" {
				continue
			}
			if !sampleIDs[strings.ToUpper(id)] {
				report.add(Finding{
					Section: "EFW2LabTST", Row: i + 2, Field: "sample_id",
					Severity: SeverityError,
					Message:  fmt.Sprintf("sample %q does not exist in the EDD", id),
				})
			}
		}
	}

	// Results must reference a known test (sample_id + test_id).
	testKeys := collectTestKeys(wb)
	if res, ok := wb.Get("EFW2LabRES"); ok {
		idx := res.ColumnIndex()
		for i, row := range res.Rows {
			sid, _ := row.Value(idx, "SAMPLE_ID")
			tid, _ := row.Value(idx, "TEST_ID")
			key := strings.ToUpper(sid + "\x00" + tid)
			if key == "\x00" || !testKeys[key] {
				report.add(Finding{
					Section: "EFW2LabRES", Row: i + 2, Field: "test_id",
					Severity: SeverityError,
					Message: fmt.Sprintf("test %q for sample %q does not exist in the EDD",
						tid, sid),
				})
			}

			// Non-detect consistency.
			detect, _ := row.Value(idx, "DETECT_FLAG")
			value, _ := row.Value(idx, "RESULT_VALUE")
			if strings.EqualFold(detect, "N") && value != "" {
				report.add(Finding{
					Section: "EFW2LabRES", Row: i + 2, Field: "result_numeric",
					Severity: SeverityWarning,
					Message:  "non-detect has a numeric result; value will be ignored",
				})
			}
			if detect == "" {
				report.add(Finding{
					Section: "EFW2LabRES", Row: i + 2, Field: "detect_flag",
					Severity: SeverityWarning,
					Message:  "detect_flag is empty; treated as detect",
				})
			}
		}
	}
}

func collectKeys(wb *Workbook, section, header string) map[string]bool {
	out := map[string]bool{}
	tbl, ok := wb.Get(section)
	if !ok {
		return out
	}
	idx := tbl.ColumnIndex()
	for _, row := range tbl.Rows {
		if v, ok := row.Value(idx, header); ok && v != "" {
			out[strings.ToUpper(v)] = true
		}
	}
	return out
}

func collectTestKeys(wb *Workbook) map[string]bool {
	out := map[string]bool{}
	tbl, ok := wb.Get("EFW2LabTST")
	if !ok {
		return out
	}
	idx := tbl.ColumnIndex()
	for _, row := range tbl.Rows {
		sid, _ := row.Value(idx, "SAMPLE_ID")
		tid, _ := row.Value(idx, "TEST_ID")
		out[strings.ToUpper(sid+"\x00"+tid)] = true
	}
	return out
}

// validateType checks that raw parses as the declared type.
func validateType(t ColumnType, raw string) error {
	switch t {
	case TypeNumber:
		if _, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err != nil {
			return fmt.Errorf("expected a number")
		}
	case TypeDate:
		if _, err := ParseDate(raw); err != nil {
			return fmt.Errorf("expected a date (YYYY-MM-DD or MM/DD/YYYY)")
		}
	case TypeBool:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "y", "n", "yes", "no", "true", "false", "1", "0":
		default:
			return fmt.Errorf("expected a boolean")
		}
	}
	return nil
}

// dateLayouts are accepted by ParseDate.
var dateLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	time.RFC3339,
	"01/02/2006",
	"01/02/2006 15:04",
	"1/2/2006",
	"01/02/06",
	"2006/01/02",
	"02-Jan-2006",
}

// ParseDate parses common environmental lab date formats.
func ParseDate(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", raw)
}
