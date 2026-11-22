package actionlevel

import (
	"errors"
	"testing"
	"time"

	"github.com/samplier/samplier/internal/units"
)

func floatPtr(v float64) *float64    { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func testTable() *units.Table {
	return units.NewTable([]units.Conversion{
		{ReportedUnit: "mg/L", TargetUnit: "ug/L", ConversionFactor: 1000},
		{ReportedUnit: "ug/L", TargetUnit: "mg/L", ConversionFactor: 0.001},
	})
}

func TestExceedsDetectOverLimit(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: floatPtr(50.3), DetectFlag: "Y", Unit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictExceed {
		t.Fatalf("got %v, want exceed", cmp.Verdict)
	}
}

func TestExceedsDetectUnderLimit(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: floatPtr(5), DetectFlag: "Y", Unit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictPass {
		t.Fatalf("got %v, want pass", cmp.Verdict)
	}
}

func TestExceedsUnitConversion(t *testing.T) {
	// Result in mg/L, limit in ug/L: 0.05 mg/L == 50 ug/L > 15 ug/L.
	r := Result{AnalyteID: "pb", Value: floatPtr(0.05), DetectFlag: "Y", Unit: "mg/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictExceed {
		t.Fatalf("got %v, want exceed", cmp.Verdict)
	}
	if cmp.ComparableValue == nil || *cmp.ComparableValue != 50 {
		t.Fatalf("comparable value = %v, want 50", cmp.ComparableValue)
	}
}

func TestExceedsNonDetect(t *testing.T) {
	// Non-detect with DL above the limit: flag it.
	r := Result{AnalyteID: "pb", Value: nil, DetectFlag: "N", Unit: "ug/L",
		DetectionLimit: floatPtr(20), DetectionUnit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictNonDetectExceed {
		t.Fatalf("got %v, want nondetect_exceed", cmp.Verdict)
	}
}

func TestExceedsNonDetectBelowLimit(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: nil, DetectFlag: "N", Unit: "ug/L",
		DetectionLimit: floatPtr(2), DetectionUnit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictPass {
		t.Fatalf("got %v, want pass", cmp.Verdict)
	}
}

func TestExceedsNonDetectNoLimit(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: nil, DetectFlag: "N", Unit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictNone {
		t.Fatalf("got %v, want none", cmp.Verdict)
	}
}

func TestExceedsScreeningMinimum(t *testing.T) {
	// pH-style range: exceed when below minimum.
	r := Result{AnalyteID: "ph", Value: floatPtr(5.5), DetectFlag: "Y", Unit: "su"}
	p := Parameter{AnalyteID: "ph", ActionLevel: 8.5, ActionLevelMin: floatPtr(6.5), Unit: "su"}

	cmp, err := Exceeds(r, p, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictExceed {
		t.Fatalf("got %v, want exceed", cmp.Verdict)
	}
}

func TestExceedsIncompatibleUnits(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: floatPtr(1), DetectFlag: "Y", Unit: "mg/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 1, Unit: "ft"}

	if _, err := Exceeds(r, p, testTable()); !errors.Is(err, ErrIncompatibleUnits) {
		t.Fatalf("got %v, want ErrIncompatibleUnits", err)
	}
}

func TestExceedsNilTableSameUnit(t *testing.T) {
	r := Result{AnalyteID: "pb", Value: floatPtr(5), DetectFlag: "Y", Unit: "ug/L"}
	p := Parameter{AnalyteID: "pb", ActionLevel: 10, Unit: "ug/L"}
	cmp, err := Exceeds(r, p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp.Verdict != VerdictPass {
		t.Fatalf("got %v, want pass", cmp.Verdict)
	}

	if _, err := Exceeds(r, p, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Nil table with differing units must fail.
	r2 := Result{AnalyteID: "pb", Value: floatPtr(5), DetectFlag: "Y", Unit: "mg/L"}
	if _, err := Exceeds(r2, p, nil); !errors.Is(err, ErrIncompatibleUnits) {
		t.Fatalf("got %v, want ErrIncompatibleUnits", err)
	}
}

func TestGroupMatchCriteria(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	depthMin, depthMax := 0.0, 10.0

	g := Group{
		Matrix: "SOIL", Fraction: "TOTAL", ProjectID: "proj-1",
		LandUse: "residential", StartDate: timePtr(start), EndDate: timePtr(end),
		DepthMin: &depthMin, DepthMax: &depthMax,
	}

	r := Result{
		Matrix: "SOIL", Fraction: "TOTAL", ProjectID: "proj-1", LandUse: "residential",
		SampleDate: time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC), Depth: floatPtr(5),
	}
	if !g.Match(r) {
		t.Fatal("expected match")
	}

	cases := []struct {
		name   string
		mutate func(*Result)
	}{
		{"matrix", func(r *Result) { r.Matrix = "WATER" }},
		{"fraction", func(r *Result) { r.Fraction = "DISSOLVED" }},
		{"project", func(r *Result) { r.ProjectID = "other" }},
		{"landuse", func(r *Result) { r.LandUse = "industrial" }},
		{"before start", func(r *Result) { r.SampleDate = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"after end", func(r *Result) { r.SampleDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"no depth", func(r *Result) { r.Depth = nil }},
		{"too deep", func(r *Result) { r.Depth = floatPtr(50) }},
	}
	for _, tc := range cases {
		mutated := r
		tc.mutate(&mutated)
		if g.Match(mutated) {
			t.Errorf("%s: expected no match", tc.name)
		}
	}
}

func TestEvaluateMultipleGroups(t *testing.T) {
	groups := []Group{
		{
			ID: "g1", TypeCode: "EPA", Code: "EPA-RSL", Name: "EPA Residential Soil",
			Matrix: "SOIL", Parameters: []Parameter{
				{AnalyteID: "pb", ActionLevel: 400, Unit: "mg/kg"},
			},
		},
		{
			ID: "g2", TypeCode: "STATE", Code: "WA-MTCA", Name: "WA Model Toxics",
			Matrix: "SOIL", Parameters: []Parameter{
				{AnalyteID: "pb", ActionLevel: 250, Unit: "mg/kg"},
			},
		},
		{
			ID: "g3", TypeCode: "EPA", Code: "EPA-WATER", Name: "Water",
			Matrix: "WATER", Parameters: []Parameter{
				{AnalyteID: "pb", ActionLevel: 15, Unit: "ug/L"},
			},
		},
	}

	r := Result{
		AnalyteID: "pb", Value: floatPtr(300), DetectFlag: "Y", Unit: "mg/kg",
		Matrix: "SOIL",
	}

	cmps, err := Evaluate(r, groups, testTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmps) != 2 {
		t.Fatalf("got %d comparisons, want 2", len(cmps))
	}

	byCode := map[string]Comparison{}
	for _, c := range cmps {
		byCode[c.GroupCode] = c
	}
	if byCode["EPA-RSL"].Verdict != VerdictPass {
		t.Errorf("EPA-RSL got %v, want pass", byCode["EPA-RSL"].Verdict)
	}
	if byCode["WA-MTCA"].Verdict != VerdictExceed {
		t.Errorf("WA-MTCA got %v, want exceed", byCode["WA-MTCA"].Verdict)
	}
	if !Exceeded(cmps) {
		t.Error("Exceeded should be true")
	}
}

func TestEvaluateFiltersByAnalyteAndMethod(t *testing.T) {
	groups := []Group{{
		Code: "G", Matrix: "SOIL",
		Parameters: []Parameter{
			{AnalyteID: "pb", ActionLevel: 1, Unit: "mg/kg"},
			{AnalyteID: "pb", ActionLevel: 2, Unit: "mg/kg", AnalyticMethod: "EPA 3050B"},
			{AnalyteID: "as", ActionLevel: 3, Unit: "mg/kg"},
		},
	}}

	r := Result{AnalyteID: "pb", Value: floatPtr(10), DetectFlag: "Y", Unit: "mg/kg", Matrix: "SOIL"}
	cmps, err := Evaluate(r, groups, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmps) != 1 {
		t.Fatalf("got %d, want 1 (method-scoped parameter excluded)", len(cmps))
	}
}

func TestExceededFalse(t *testing.T) {
	if Exceeded(nil) {
		t.Fatal("nil slice should not be exceeded")
	}
	if Exceeded([]Comparison{{Verdict: VerdictPass}}) {
		t.Fatal("pass should not be exceeded")
	}
}
