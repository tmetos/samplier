package edd

import (
	"bytes"
	"strings"
	"testing"
)

func TestLookupFormats(t *testing.T) {
	d, err := Lookup("efwedd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Code != "EFWEDD" {
		t.Fatalf("got %q", d.Code)
	}
	if _, err := Lookup("NOPE"); err == nil {
		t.Fatal("expected error for unknown format")
	}
	if len(Formats()) == 0 {
		t.Fatal("expected at least one registered format")
	}
}

func TestSectionLookupAndOrder(t *testing.T) {
	d := EFWEDD()
	s, ok := d.Section("efw2labres")
	if !ok || s.TargetTable != "result" {
		t.Fatalf("unexpected section: %+v %v", s, ok)
	}
	names := d.SectionNames()
	if len(names) != len(d.Sections) {
		t.Fatalf("got %d names", len(names))
	}
	if names[0] != "EFW2FSample" {
		t.Fatalf("unexpected first section %q", names[0])
	}
}

// buildCSV constructs a CSV EDD from header/rows.
func buildCSV(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(strings.Join(headers, ","))
	b.WriteString("\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, ","))
		b.WriteString("\n")
	}
	return b.String()
}

func readFromString(t *testing.T, name, content string, def Definition) *Workbook {
	t.Helper()
	data := []byte(content)
	wb, err := Read(bytes.NewReader(data), int64(len(data)), name, def)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return wb
}

func TestReadCSVSection(t *testing.T) {
	def := EFWEDD()
	csvData := buildCSV(
		[]string{"SAMPLE_ID", "SYS_LOC_CODE", "MATRIX_CODE", "SAMPLE_DATE", "SAMPLE_TYPE_CODE"},
		[][]string{
			{"S-001", "1", "WATER", "2024-03-01", "N"},
			{"S-002", "1", "WATER", "2024-03-01", "FD"},
		})

	wb := readFromString(t, "EFW2FSample.csv", csvData, def)
	tbl, ok := wb.Get("EFW2FSample")
	if !ok {
		t.Fatal("expected EFW2FSample section")
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("got %d rows", len(tbl.Rows))
	}
	idx := tbl.ColumnIndex()
	if v, _ := tbl.Rows[0].Value(idx, "SAMPLE_ID"); v != "S-001" {
		t.Fatalf("got %q", v)
	}
}

func TestReadCSVStripsCommentHeader(t *testing.T) {
	def := EFWEDD()
	content := "#SAMPLE_ID,SYS_LOC_CODE,MATRIX_CODE,SAMPLE_DATE\n" +
		"#text,number,text,date\n" +
		"S-1,1,WATER,2024-01-02\n"

	wb := readFromString(t, "EFW2FSample.csv", content, def)
	tbl, ok := wb.Get("EFW2FSample")
	if !ok {
		t.Fatal("expected section")
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("got %d rows, want 1 (type row must be skipped)", len(tbl.Rows))
	}
	if tbl.Headers[0] != "SAMPLE_ID" {
		t.Fatalf("header not stripped: %q", tbl.Headers[0])
	}
}

func TestReadTSV(t *testing.T) {
	def := EFWEDD()
	content := "SAMPLE_ID\tSYS_LOC_CODE\tMATRIX_CODE\tSAMPLE_DATE\nS-1\t1\tWATER\t2024-01-02\n"
	wb := readFromString(t, "EFW2FSample.tsv", content, def)
	tbl, ok := wb.Get("EFW2FSample")
	if !ok || len(tbl.Rows) != 1 {
		t.Fatalf("unexpected table: %+v", tbl)
	}
}

func TestInferSectionFromFilename(t *testing.T) {
	def := EFWEDD()
	// A filename with a prefix still resolves to the section.
	wb := readFromString(t, "labdata_EFW2LabRES.csv",
		"SAMPLE_ID,TEST_ID,CAS_RN,RESULT_VALUE\nS-1,T-1,7439-92-1,5\n", def)
	if _, ok := wb.Get("EFW2LabRES"); !ok {
		t.Fatalf("expected EFW2LabRES, got %v", wb.Order)
	}
}

func TestIsHeaderRow(t *testing.T) {
	if !IsHeaderRow([]string{"#SAMPLE_ID"}) {
		t.Fatal("expected header row")
	}
	if IsHeaderRow([]string{"SAMPLE_ID"}) {
		t.Fatal("did not expect header row")
	}
	if !IsHeaderRow(nil) {
		t.Fatal("nil row should be header")
	}
}

func TestTableColumnIndexAndValue(t *testing.T) {
	tbl := Table{Headers: []string{"A", "B"}}
	idx := tbl.ColumnIndex()
	row := Row{"1", "2"}
	if v, ok := row.Value(idx, "a"); !ok || v != "1" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := row.Value(idx, "C"); ok {
		t.Fatal("expected miss for unknown header")
	}
	// Out-of-range index.
	if _, ok := row.Value(map[string]int{"D": 9}, "D"); ok {
		t.Fatal("expected miss for out-of-range")
	}
}

func TestValidateHappyPath(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, ReferenceSet{
		"MATRIX":      {"WATER", "SOIL"},
		"SAMPLE_TYPE": {"N", "FD"},
		"UNIT":        {"mg/L", "ug/L"},
		"METHOD":      {"EPA 8260D"},
		"FRACTION":    {"TOTAL"},
	})

	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Section: "EFW2FSample",
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows:    []Row{{"1", "S-1", "WATER", "2024-01-02"}},
		},
		"EFW2LabTST": {
			Section: "EFW2LabTST",
			Headers: []string{"SAMPLE_ID", "TEST_ID", "ANALYTIC_METHOD", "FRACTION"},
			Rows:    []Row{{"S-1", "T-1", "EPA 8260D", "TOTAL"}},
		},
		"EFW2LabRES": {
			Section: "EFW2LabRES",
			Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN", "RESULT_VALUE", "RESULT_UNIT", "DETECT_FLAG"},
			Rows:    []Row{{"S-1", "T-1", "7439-92-1", "5.2", "mg/L", "Y"}},
		},
	}}

	report := v.Validate(wb)
	if !report.Valid() {
		t.Fatalf("expected valid, got errors: %+v", report.Errors())
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v", report.Findings)
	}
}

func TestValidateMissingRequiredColumn(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Section: "EFW2FSample",
			Headers: []string{"SAMPLE_ID"}, // missing SYS_LOC_CODE, MATRIX_CODE, SAMPLE_DATE
			Rows:    []Row{{"S-1"}},
		},
		"EFW2LabTST": {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
		"EFW2LabRES": {Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"}},
	}}

	report := v.Validate(wb)
	if report.Valid() {
		t.Fatal("expected invalid")
	}
	missing := 0
	for _, f := range report.Errors() {
		if strings.Contains(f.Message, "required column is missing") {
			missing++
		}
	}
	if missing < 2 {
		t.Fatalf("expected at least 2 missing-column errors, got %d: %+v", missing, report.Errors())
	}
}

func TestValidateEmptyRequiredValue(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows:    []Row{{"1", "", "WATER", "2024-01-02"}},
		},
		"EFW2LabTST": {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
		"EFW2LabRES": {Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"}},
	}}

	report := v.Validate(wb)
	found := false
	for _, f := range report.Errors() {
		if f.Field == "name" && strings.Contains(f.Message, "required value is empty") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected empty-required-value error, got %+v", report.Errors())
	}
}

func TestValidateTypeErrors(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows: []Row{
				{"not-a-number", "S-1", "WATER", "2024-01-02"},
				{"1", "S-2", "WATER", "not-a-date"},
			},
		},
		"EFW2LabTST": {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
		"EFW2LabRES": {Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"}},
	}}

	report := v.Validate(wb)
	if len(report.Errors()) < 2 {
		t.Fatalf("expected type errors, got %+v", report.Findings)
	}
}

func TestValidateReferenceValue(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, ReferenceSet{"MATRIX": {"WATER"}})
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows:    []Row{{"1", "S-1", "LAVA", "2024-01-02"}},
		},
		"EFW2LabTST": {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
		"EFW2LabRES": {Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"}},
	}}

	report := v.Validate(wb)
	found := false
	for _, f := range report.Errors() {
		if strings.Contains(f.Message, "not in reference table") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected reference error, got %+v", report.Findings)
	}
}

func TestValidateOrphanTestAndResult(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows:    []Row{{"1", "S-1", "WATER", "2024-01-02"}},
		},
		"EFW2LabTST": {
			Headers: []string{"SAMPLE_ID", "TEST_ID"},
			Rows:    []Row{{"S-UNKNOWN", "T-1"}},
		},
		"EFW2LabRES": {
			Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"},
			Rows:    []Row{{"S-1", "T-NOTEXIST", "7439-92-1"}},
		},
	}}

	report := v.Validate(wb)
	if report.Valid() {
		t.Fatal("expected orphan errors")
	}
	var orphanSample, orphanTest bool
	for _, f := range report.Errors() {
		if strings.Contains(f.Message, "does not exist in the EDD") {
			orphanSample = true
		}
		if strings.Contains(f.Message, "does not exist in the EDD") && f.Section == "EFW2LabRES" {
			orphanTest = true
		}
	}
	if !orphanSample {
		t.Error("expected orphan sample error")
	}
	if !orphanTest {
		t.Error("expected orphan test error")
	}
}

func TestValidateNonDetectWarning(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {
			Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"},
			Rows:    []Row{{"1", "S-1", "WATER", "2024-01-02"}},
		},
		"EFW2LabTST": {
			Headers: []string{"SAMPLE_ID", "TEST_ID"},
			Rows:    []Row{{"S-1", "T-1"}},
		},
		"EFW2LabRES": {
			Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN", "RESULT_VALUE", "DETECT_FLAG"},
			Rows:    []Row{{"S-1", "T-1", "7439-92-1", "5", "N"}},
		},
	}}

	report := v.Validate(wb)
	if !report.Valid() {
		t.Fatalf("expected valid (warning only): %+v", report.Errors())
	}
	if len(report.Warnings()) == 0 {
		t.Fatal("expected a non-detect warning")
	}
}

func TestValidateMissingSampleSection(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	wb := &Workbook{Tables: map[string]Table{
		"EFW2LabTST": {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
	}}
	report := v.Validate(wb)
	found := false
	for _, f := range report.Errors() {
		if strings.Contains(f.Message, "one of EFW2FSample or EFW2LabSMP is required") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing-sample-section error, got %+v", report.Findings)
	}
}

func TestValidateBothSampleSectionsWarning(t *testing.T) {
	def := EFWEDD()
	v := NewValidator(def, nil)
	hdrs := []string{"SYS_LOC_CODE", "SAMPLE_ID", "MATRIX_CODE", "SAMPLE_DATE"}
	wb := &Workbook{Tables: map[string]Table{
		"EFW2FSample": {Headers: hdrs, Rows: []Row{{"1", "S-1", "WATER", "2024-01-02"}}},
		"EFW2LabSMP":  {Headers: hdrs, Rows: []Row{{"1", "S-1", "WATER", "2024-01-02"}}},
		"EFW2LabTST":  {Headers: []string{"SAMPLE_ID", "TEST_ID"}},
		"EFW2LabRES":  {Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN"}},
	}}
	report := v.Validate(wb)
	warned := false
	for _, f := range report.Warnings() {
		if strings.Contains(f.Message, "only one is used") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected both-sections warning, got %+v", report.Findings)
	}
}

func TestParseDateFormats(t *testing.T) {
	good := []string{
		"2024-01-02", "2024-01-02 15:04:05", "2024-01-02T15:04:05",
		"01/02/2024", "1/2/2024", "01/02/06", "2024/01/02", "02-Jan-2024",
	}
	for _, s := range good {
		if _, err := ParseDate(s); err != nil {
			t.Errorf("ParseDate(%q) unexpected error: %v", s, err)
		}
	}
	for _, s := range []string{"", "garbage", "2024-99-99"} {
		if _, err := ParseDate(s); err == nil {
			t.Errorf("ParseDate(%q) expected error", s)
		}
	}
}

func TestValidateBoolType(t *testing.T) {
	for _, ok := range []string{"y", "N", "yes", "no", "true", "FALSE", "1", "0"} {
		if err := validateType(TypeBool, ok); err != nil {
			t.Errorf("validateType(bool, %q) unexpected error: %v", ok, err)
		}
	}
	if err := validateType(TypeBool, "maybe"); err == nil {
		t.Error("expected error for invalid boolean")
	}
}

func TestCommitModeValidation(t *testing.T) {
	for _, m := range []CommitMode{InsertOnly, InsertUpdate, InsertMerge} {
		if !ValidCommitMode(m) {
			t.Errorf("%q should be valid", m)
		}
	}
	if ValidCommitMode("NONSENSE") {
		t.Error("expected invalid commit mode")
	}
}

func TestReferenceSetUnknownTableAllows(t *testing.T) {
	rs := ReferenceSet{}
	if !rs.Contains("MATRIX", "anything") {
		t.Fatal("unknown reference table should not block")
	}
}
