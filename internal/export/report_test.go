package export

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestEncodeReportXLSXMultiTab(t *testing.T) {
	level := 0.015
	strict := 0.005
	data := ReportData{
		Title:             "Metals + VOCs vs EPA & State",
		ProjectCode:       "P-001",
		ProjectName:       "Riverside Site",
		GeneratedAt:       time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		ActionLevelGroups: []string{"EPA-MCL", "STATE-SL"},
		Tabs: []ReportTab{
			{Key: "metals", Label: "Metals", Rows: []Row{
				{
					ProjectID: "p", ProjectCode: "P-001", ProjectName: "Riverside Site",
					LocationName: "MW-02", SampleName: "S-001", Matrix: "WATER",
					SampleDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
					CASRN:      "7439-92-1", AnalyteName: "Lead",
					ResultNumeric: f(0.0503), DetectFlag: "Y", Units: "mg/L",
					Exceeded: true, ActionLevelGroup: "EPA-MCL", ActionLevelType: "EPA",
					ActionLevel: &level,
					Exceedances: []Exceedance{
						{Group: "EPA-MCL", Type: "EPA", Level: &level},
						{Group: "STATE-SL", Type: "STATE", Level: &strict},
					},
				},
			}},
			{Key: "vocs", Label: "VOCs", Rows: []Row{
				{
					ProjectID: "p", ProjectCode: "P-001", ProjectName: "Riverside Site",
					LocationName: "MW-01", SampleName: "S-001", Matrix: "WATER",
					SampleDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
					CASRN:      "71-43-2", AnalyteName: "Benzene",
					DetectFlag: "N", Units: "mg/L", RDL: f(0.001), LimitUnit: "mg/L",
				},
			}},
		},
	}

	out, err := EncodeReportXLSX(data, Options{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected bytes")
	}

	xlsx, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer xlsx.Close()

	sheets := xlsx.GetSheetList()
	want := []string{"Summary", "Metals", "VOCs", "Exceedances"}
	if len(sheets) != len(want) {
		t.Fatalf("sheets = %v, want %v", sheets, want)
	}
	for i, s := range want {
		if sheets[i] != s {
			t.Fatalf("sheets[%d] = %q, want %q", i, sheets[i], s)
		}
	}

	// Exceedances sheet must list both limit sets for the one Lead breach.
	rows, err := xlsx.GetRows("Exceedances")
	if err != nil {
		t.Fatalf("exceedances rows: %v", err)
	}
	if len(rows) != 3 { // header + 2 limit sets
		t.Fatalf("exceedance rows = %d, want 3", len(rows))
	}
	if rows[1][9] != "EPA-MCL" || rows[2][9] != "STATE-SL" {
		t.Fatalf("exceedance groups = %q, %q", rows[1][9], rows[2][9])
	}

	// The VOCs tab must include the non-detect rendering.
	vocRows, err := xlsx.GetRows("VOCs")
	if err != nil {
		t.Fatalf("vocs rows: %v", err)
	}
	found := false
	for _, r := range vocRows {
		if len(r) > 0 && r[0] == "Benzene" {
			found = true
		}
	}
	if !found {
		t.Fatal("Benzene row not found in VOCs sheet")
	}
}

func TestUniqueSheetName(t *testing.T) {
	used := map[string]bool{}
	if got := uniqueSheetName("Metals", used); got != "Metals" {
		t.Fatalf("got %q", got)
	}
	used["Metals"] = true
	if got := uniqueSheetName("Metals", used); got != "Metals-2" {
		t.Fatalf("got %q", got)
	}
	if got := uniqueSheetName("Lead (total, dissolved)", map[string]bool{}); len(got) > 31 {
		t.Fatalf("sheet name too long: %q", got)
	}
}
