package export

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func f(v float64) *float64 { return &v }
func tm(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func sampleDataset() Dataset {
	return Dataset{
		Title:             "Spring 2024 Groundwater",
		ProjectCode:       "P-001",
		ProjectName:       "Riverside Site",
		GeneratedAt:       time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		ActionLevelGroups: []string{"EPA-RSL"},
		Rows: []Row{
			{
				ProjectCode: "P-001", ProjectName: "Riverside Site",
				LocationName: "MW-01", LocationType: "WELL",
				Longitude: -122.4194, Latitude: 47.6062,
				SampleName: "S-001", SampleType: "N", Matrix: "WATER",
				SampleDate: *tm("2024-03-01"), TaskCode: "T-001",
				TestID: "TST-1", Method: "EPA 6020B", Fraction: "TOTAL",
				LabSDG: "SDG-1", Lab: "ACME",
				CASRN: "7439-92-1", AnalyteName: "Lead",
				ResultNumeric: f(0.0052), DetectFlag: "Y", Units: "mg/L",
				ActionLevel: f(0.015), ActionLevelGroup: "EPA-RSL",
				ActionLevelType: "EPA", Exceeded: false,
			},
			{
				ProjectCode: "P-001", ProjectName: "Riverside Site",
				LocationName: "MW-01", LocationType: "WELL",
				Longitude: -122.4194, Latitude: 47.6062,
				SampleName: "S-001", SampleType: "N", Matrix: "WATER",
				SampleDate: *tm("2024-03-01"), TaskCode: "T-001",
				TestID: "TST-1", Method: "EPA 6020B", Fraction: "TOTAL",
				LabSDG: "SDG-1", Lab: "ACME",
				CASRN: "7440-38-2", AnalyteName: "Arsenic",
				DetectFlag: "N", Units: "mg/L", RDL: f(0.001), LimitUnit: "mg/L",
				LabQualifier: "U",
			},
			{
				ProjectCode: "P-001", ProjectName: "Riverside Site",
				LocationName: "MW-02", LocationType: "WELL",
				Longitude: -122.4180, Latitude: 47.6065,
				SampleName: "S-002", SampleType: "N", Matrix: "WATER",
				SampleDate: *tm("2024-03-01"), TaskCode: "T-001",
				TestID: "TST-2", Method: "EPA 6020B", Fraction: "TOTAL",
				LabSDG: "SDG-1", Lab: "ACME",
				CASRN: "7439-92-1", AnalyteName: "Lead",
				ResultNumeric: f(0.0503), DetectFlag: "Y", Units: "mg/L",
				ActionLevel: f(0.015), ActionLevelGroup: "EPA-RSL",
				ActionLevelType: "EPA", Exceeded: true,
			},
		},
	}
}

func TestAdapterRegistry(t *testing.T) {
	for _, code := range []string{"CROSSTAB", "WQX", "AQS", "EIM", "CSV"} {
		a, err := Get(code)
		if err != nil {
			t.Fatalf("Get(%q): %v", code, err)
		}
		if a.Code() == "" {
			t.Errorf("%q has empty code", code)
		}
	}
	if _, err := Get("NOPE"); err == nil {
		t.Fatal("expected error for unknown adapter")
	}
	if !equalFold("wqx", "WQX") {
		t.Fatal("equalFold failed")
	}
	if len(Codes()) < 5 {
		t.Fatalf("expected 5 adapters, got %v", Codes())
	}
}

func TestReportedValue(t *testing.T) {
	cases := []struct {
		row  Row
		want string
	}{
		{Row{ResultNumeric: f(50.3), DetectFlag: "Y"}, "50.3"},
		{Row{DetectFlag: "N", RDL: f(5)}, "< 5"},
		{Row{DetectFlag: "N"}, "ND"},
		{Row{DetectFlag: "N", MDL: f(0.5)}, "< 0.5"},
		{Row{ResultText: "PRESENT"}, "PRESENT"},
		{Row{}, ""},
	}
	for _, tc := range cases {
		if got := tc.row.ReportedValue(); got != tc.want {
			t.Errorf("ReportedValue() = %q, want %q", got, tc.want)
		}
	}
}

func TestDetectionLimitPrecedence(t *testing.T) {
	r := Row{RDL: f(1), MDL: f(2)}
	if got := *r.DetectionLimit(); got != 1 {
		t.Fatalf("got %v, want RDL 1", got)
	}
	r = Row{MDL: f(2)}
	if got := *r.DetectionLimit(); got != 2 {
		t.Fatalf("got %v, want MDL 2", got)
	}
	if (Row{}).DetectionLimit() != nil {
		t.Fatal("expected nil detection limit")
	}
}

func TestCrosstabEncode(t *testing.T) {
	a, _ := Get("CROSSTAB")
	data, err := a.Encode(sampleDataset(), Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected bytes")
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()

	if got := f.GetSheetList(); len(got) < 2 {
		t.Fatalf("expected Results + Exceedances sheets, got %v", got)
	}

	rows, err := f.GetRows("Results")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) < 8 {
		t.Fatalf("too few rows: %d", len(rows))
	}
	if rows[0][0] != "Spring 2024 Groundwater" {
		t.Fatalf("title row = %q", rows[0][0])
	}

	// Exceedance sheet must list the one exceedance.
	exRows, err := f.GetRows("Exceedances")
	if err != nil {
		t.Fatalf("get exceedance rows: %v", err)
	}
	if len(exRows) != 2 {
		t.Fatalf("expected header + 1 exceedance, got %d rows", len(exRows))
	}
	if exRows[1][0] != "MW-02" {
		t.Fatalf("exceedance location = %q", exRows[1][0])
	}
}

func TestCrosstabAnalytesAndNonDetect(t *testing.T) {
	a := CrosstabAdapter{}
	data, err := a.Encode(sampleDataset(), Options{AnalyteOrder: []string{"Lead", "Arsenic"}})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	f, _ := excelize.OpenReader(bytes.NewReader(data))
	defer f.Close()

	rows, _ := f.GetRows("Results")
	var found bool
	for _, r := range rows {
		if len(r) > 2 && r[0] == "Arsenic" {
			found = true
			// Non-detect renders as "< 0.001" somewhere in the row.
			joined := strings.Join(r, "|")
			if !strings.Contains(joined, "< 0.001") {
				t.Errorf("expected non-detect rendering, got %v", r)
			}
		}
	}
	if !found {
		t.Fatal("Arsenic row not found")
	}
}

func TestCSVEncode(t *testing.T) {
	a, _ := Get("CSV")
	data, err := a.Encode(sampleDataset(), Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 4 { // header + 3 rows
		t.Fatalf("got %d records", len(records))
	}
	if records[0][0] != "project_code" {
		t.Fatalf("header = %q", records[0][0])
	}

	// Find the Arsenic non-detect row and check the reported value column.
	header := records[0]
	idxValue := indexOf(header, "result")
	idxDetect := indexOf(header, "detect_flag")
	for _, rec := range records[1:] {
		if rec[indexOf(header, "analyte")] == "Arsenic" {
			if rec[idxValue] != "< 0.001" {
				t.Errorf("reported value = %q", rec[idxValue])
			}
			if rec[idxDetect] != "N" {
				t.Errorf("detect flag = %q", rec[idxDetect])
			}
		}
	}
}

func indexOf(xs []string, target string) int {
	for i, x := range xs {
		if x == target {
			return i
		}
	}
	return -1
}

func TestWQXEncode(t *testing.T) {
	a, _ := Get("WQX")
	data, err := a.Encode(sampleDataset(), Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Must be well-formed XML.
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid XML: %v", err)
		}
		depth++
	}
	if depth == 0 {
		t.Fatal("no XML tokens")
	}

	s := string(data)
	for _, want := range []string{
		"<WQX", "<Organization", "OrganizationIdentifier",
		"<Project>", "ProjectIdentifier",
		"<MonitoringLocation>", "MonitoringLocationIdentifier",
		"<Activity>", "ActivityIdentifier",
		"<ResultDescription>", "CharacteristicName", "Lead",
		"<ResultMeasure>", "MeasureUnitCode",
		"<DetectionQuantitationLimit>",
		"Not Detected",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("WQX output missing %q", want)
		}
	}
	// Coordinates present.
	if !strings.Contains(s, "47.6062") || !strings.Contains(s, "-122.4194") {
		t.Error("WQX output missing coordinates")
	}
}

func TestWQXExplicitOptions(t *testing.T) {
	ds := sampleDataset()
	wo := WQXOptions{
		OrganizationID:         "WA_ECOLOGY",
		OrganizationFormalName: "Washington State Dept of Ecology",
		ElectronicAddress:      "https://ecology.wa.gov",
		MonitoringLocationID:   "EIM-12345",
		MonitoringLocationType: "Well",
	}
	data, err := EncodeWQX(ds, wo, Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "WA_ECOLOGY") {
		t.Error("expected organization id")
	}
	if !strings.Contains(s, "EIM-12345") {
		t.Error("expected explicit location id")
	}
}

func TestWQXLimitToDetects(t *testing.T) {
	ds := sampleDataset()
	data, err := EncodeWQX(ds, DefaultWQXOptions(ds), Options{LimitToDetects: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(data), "Arsenic") {
		t.Error("Arsenic is a non-detect and should be excluded")
	}
	if !strings.Contains(string(data), "Lead") {
		t.Error("Lead detect should be retained")
	}
}

func TestAQSPipeEncode(t *testing.T) {
	a, _ := Get("AQS")
	data, err := a.Encode(sampleDataset(), Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 { // comment + 3 records
		t.Fatalf("got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "#") {
		t.Fatalf("expected comment header, got %q", lines[0])
	}

	// First data record must be an RD/I transaction.
	fields := strings.Split(lines[1], "|")
	if fields[0] != "RD" || fields[1] != "I" {
		t.Fatalf("unexpected transaction: %v", fields[:2])
	}
	if fields[2] != "MW-01" {
		t.Fatalf("site = %q", fields[2])
	}
	if fields[5] != "20240301" {
		t.Fatalf("date = %q", fields[5])
	}

	// The non-detect record must carry the ND null code.
	var foundND bool
	for _, line := range lines[1:] {
		fs := strings.Split(line, "|")
		if len(fs) > 3 && fs[3] == "7440-38-2" {
			if fs[len(fs)-1] != "ND" {
				t.Errorf("expected ND null code, got %q", fs[len(fs)-1])
			}
			foundND = true
		}
	}
	if !foundND {
		t.Error("non-detect record not found")
	}
}

func TestAQSXMLEncode(t *testing.T) {
	ds := sampleDataset()
	ao := DefaultAQSOptions()
	ao.Format = "xml"
	ao.SiteID = "530330001"
	ao.ParameterCode = map[string]string{"7439-92-1": "12128"}

	data, err := EncodeAQS(ds, ao, Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s := string(data)
	for _, want := range []string{"<AQData", "<RawData>", "SiteIdentifier", "530330001", "12128", "<DetectionLimit>"} {
		if !strings.Contains(s, want) {
			t.Errorf("AQS XML missing %q", want)
		}
	}
}

func TestEIMCSVEncode(t *testing.T) {
	a, _ := Get("EIM")
	data, err := a.Encode(sampleDataset(), Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("got %d records", len(records))
	}
	if records[0][0] != "Study ID" {
		t.Fatalf("header = %q", records[0][0])
	}
	if records[1][0] != "P-001" {
		t.Fatalf("study = %q", records[1][0])
	}
	// Non-detect gets a "U" qualifier by default.
	idxQual := indexOf(records[0], "Qualifier")
	idxParam := indexOf(records[0], "Parameter Name")
	for _, rec := range records[1:] {
		if rec[idxParam] == "Arsenic" {
			if rec[idxQual] != "U" {
				t.Errorf("expected U qualifier, got %q", rec[idxQual])
			}
		}
	}
}

func TestEIMXMLEencode(t *testing.T) {
	ds := sampleDataset()
	eo := DefaultEIMOptions(ds)
	eo.Format = "xml"
	eo.LocationID = map[string]string{"MW-01": "EIM-LOC-1"}

	data, err := EncodeEIM(ds, eo, Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s := string(data)
	for _, want := range []string{"<EIMSubmission>", "<StudyID>P-001</StudyID>", "EIM-LOC-1", "<ReportingLimit>"} {
		if !strings.Contains(s, want) {
			t.Errorf("EIM XML missing %q", want)
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatNumber(50.3); got != "50.3" {
		t.Errorf("formatNumber = %q", got)
	}
	if got := formatNumber(100); got != "100" {
		t.Errorf("formatNumber = %q", got)
	}
	if got := xmlText(`a<b&c"d'e`); got != "a&lt;b&amp;c&quot;d&apos;e" {
		t.Errorf("xmlText = %q", got)
	}
	// Control characters stripped.
	if got := xmlText("a\x00b"); got != "ab" {
		t.Errorf("xmlText control chars = %q", got)
	}
	if got := uniqueStrings([]string{"a", "b", "a", ""}); len(got) != 2 {
		t.Errorf("uniqueStrings = %v", got)
	}
	if got := csvEscape(`a,b`); got != `"a,b"` {
		t.Errorf("csvEscape = %q", got)
	}
	if got := csvEscape("plain"); got != "plain" {
		t.Errorf("csvEscape = %q", got)
	}
	if got := csvEscape(`has"quote`); got != `"has""quote"` {
		t.Errorf("csvEscape = %q", got)
	}
}

func TestWQXMediaAndSpeciation(t *testing.T) {
	cases := map[string]string{
		"WATER": "Water", "SOIL": "Soil", "SEDIMENT": "Sediment",
		"AIR": "Air", "BIOTA": "Biological Tissue",
	}
	for in, want := range cases {
		if got := wqxMedia(in); got != want {
			t.Errorf("wqxMedia(%q) = %q, want %q", in, got, want)
		}
	}
	if got := wqxSpeciation("DISSOLVED"); got != "Dissolved" {
		t.Errorf("got %q", got)
	}
	if got := wqxSpeciation("TOTAL"); got != "Total" {
		t.Errorf("got %q", got)
	}
}

func TestAQSUnitMapping(t *testing.T) {
	if got := aqsUnit("ug/m3"); got != "008" {
		t.Errorf("got %q", got)
	}
	if got := aqsUnit("mg/l"); got != "mg/L" {
		t.Errorf("got %q", got)
	}
}

func TestAQSNullCode(t *testing.T) {
	if got := aqsNullCode(Row{DetectFlag: "N"}); got != "ND" {
		t.Errorf("got %q", got)
	}
	if got := aqsNullCode(Row{DetectFlag: "Y"}); got != "" {
		t.Errorf("got %q", got)
	}
}
