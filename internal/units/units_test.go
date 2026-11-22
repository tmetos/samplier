package units

import (
	"errors"
	"math"
	"testing"
)

func TestConvertDirect(t *testing.T) {
	table := NewTable([]Conversion{
		{ReportedUnit: "mg/L", TargetUnit: "ug/L", ConversionFactor: 1000},
		{ReportedUnit: "ug/L", TargetUnit: "mg/L", ConversionFactor: 0.001},
		{ReportedUnit: "ft", TargetUnit: "m", ConversionFactor: 0.3048},
	})

	got, err := table.Convert(2, "mg/L", "ug/L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2000 {
		t.Fatalf("got %v, want 2000", got)
	}

	got, err = table.Convert(10, "ft", "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-3.048) > 1e-9 {
		t.Fatalf("got %v, want 3.048", got)
	}
}

func TestConvertIdentity(t *testing.T) {
	table := NewTable(nil)
	got, err := table.Convert(5, "mg/L", "MG/L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5 {
		t.Fatalf("got %v, want 5", got)
	}
}

func TestConvertDelta(t *testing.T) {
	// deg F -> deg C: (v - 32) * 5/9 expressed as factor 0.5555... with delta.
	table := NewTable([]Conversion{
		{ReportedUnit: "deg F", TargetUnit: "deg C", ConversionFactor: 5.0 / 9.0, Delta: -32.0 * 5.0 / 9.0},
	})
	got, err := table.Convert(212, "deg F", "deg C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-100) > 1e-9 {
		t.Fatalf("got %v, want 100", got)
	}
}

func TestConvertBridged(t *testing.T) {
	table := NewTable([]Conversion{
		{ReportedUnit: "mg/kg", TargetUnit: "ug/kg", ConversionFactor: 1000},
		{ReportedUnit: "ug/kg", TargetUnit: "ng/kg", ConversionFactor: 1000},
	})
	got, err := table.Convert(2, "mg/kg", "ng/kg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2_000_000 {
		t.Fatalf("got %v, want 2000000", got)
	}
}

func TestConvertUnavailable(t *testing.T) {
	table := NewTable([]Conversion{
		{ReportedUnit: "mg/L", TargetUnit: "ug/L", ConversionFactor: 1000},
	})
	if _, err := table.Convert(1, "mg/L", "ft"); !errors.Is(err, ErrNoConversion) {
		t.Fatalf("got %v, want ErrNoConversion", err)
	}
	if table.CanConvert("mg/L", "ft") {
		t.Fatal("CanConvert should be false")
	}
	if !table.CanConvert("mg/L", "ug/L") {
		t.Fatal("CanConvert should be true")
	}
}

func TestConvertFormula(t *testing.T) {
	table := NewTable([]Conversion{
		{
			ReportedUnit: "deg F",
			TargetUnit:   "deg C",
			Formula:      "({original_value} - 32) * 5 / 9",
		},
		{
			ReportedUnit: "raw",
			TargetUnit:   "scaled",
			Formula:      "{original_value} * {a} + {b}",
			A:            2,
			B:            3,
		},
	})

	got, err := table.Convert(212, "deg F", "deg C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-100) > 1e-9 {
		t.Fatalf("got %v, want 100", got)
	}

	got, err = table.Convert(4, "raw", "scaled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 11 {
		t.Fatalf("got %v, want 11", got)
	}
}

func TestEvalFormulaEdgeCases(t *testing.T) {
	cases := []struct {
		formula string
		want    float64
		wantErr bool
	}{
		{"1 + 2 * 3", 7, false},
		{"(1 + 2) * 3", 9, false},
		{"-{original_value}", -5, false},
		{"10 / 2", 5, false},
		{"1 / 0", 0, true},
		{"{unknown}", 0, true},
		{"1 +", 0, true},
		{"(1 + 2", 0, true},
		{"1e3", 1000, false},
		{"", 0, true},
	}
	for _, tc := range cases {
		got, err := evalFormula(tc.formula, map[string]float64{"original_value": 5})
		if tc.wantErr {
			if err == nil {
				t.Errorf("formula %q: expected error, got %v", tc.formula, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("formula %q: unexpected error %v", tc.formula, err)
			continue
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("formula %q: got %v, want %v", tc.formula, got, tc.want)
		}
	}
}
