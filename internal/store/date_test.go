package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDateLayouts(t *testing.T) {
	good := []string{
		"2024-01-02", "2024-01-02T15:04:05Z", "2024-01-02T15:04:05",
		"2024-01-02 15:04:05", "01/02/2024", "01/02/24", "2024/01/02", "02-Jan-2024",
	}
	for _, s := range good {
		d, err := ParseDate(s)
		if err != nil {
			t.Errorf("ParseDate(%q): %v", s, err)
			continue
		}
		if !d.Valid() {
			t.Errorf("ParseDate(%q) returned invalid date", s)
		}
	}
	if _, err := ParseDate("garbage"); err == nil {
		t.Error("expected error for garbage date")
	}
	// Empty string yields the zero value without error.
	d, err := ParseDate("")
	if err != nil || d.Valid() {
		t.Errorf("empty parse: %v %v", err, d.Valid())
	}
}

func TestDateJSONRoundTrip(t *testing.T) {
	var d Date
	if err := json.Unmarshal([]byte(`"2024-03-15"`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Format("2006-01-02") != "2024-03-15" {
		t.Fatalf("got %v", d)
	}

	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `"2024-03-15"` {
		t.Fatalf("got %s", out)
	}

	// Null handling.
	var zero Date
	if err := json.Unmarshal([]byte(`null`), &zero); err != nil {
		t.Fatalf("null: %v", err)
	}
	if zero.Valid() {
		t.Fatal("null should produce invalid date")
	}
	out, _ = json.Marshal(zero)
	if string(out) != "null" {
		t.Fatalf("zero marshal = %s", out)
	}

	// Empty string also clears.
	one := NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := json.Unmarshal([]byte(`""`), &one); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if one.Valid() {
		t.Fatal("empty string should clear date")
	}

	// Numeric JSON is rejected.
	if err := json.Unmarshal([]byte(`20240101`), &d); err == nil {
		t.Fatal("expected error for numeric date")
	}
}

func TestDateValueScan(t *testing.T) {
	d := NewDate(time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC))

	v, err := d.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if _, ok := v.(time.Time); !ok {
		t.Fatalf("expected time.Time, got %T", v)
	}

	var zero Date
	v, err = zero.Value()
	if err != nil || v != nil {
		t.Fatalf("zero value = %v, %v", v, err)
	}

	// Scan variants.
	var scanned Date
	if err := scanned.Scan(time.Date(2024, 7, 8, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("scan time: %v", err)
	}
	if scanned.Format("2006-01-02") != "2024-07-08" {
		t.Fatalf("got %v", scanned)
	}

	if err := scanned.Scan("2024-09-10"); err != nil {
		t.Fatalf("scan string: %v", err)
	}
	if scanned.Format("2006-01-02") != "2024-09-10" {
		t.Fatalf("got %v", scanned)
	}

	if err := scanned.Scan([]byte("2024-11-12")); err != nil {
		t.Fatalf("scan bytes: %v", err)
	}
	if scanned.Format("2006-01-02") != "2024-11-12" {
		t.Fatalf("got %v", scanned)
	}

	if err := scanned.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if scanned.Valid() {
		t.Fatal("nil scan should clear")
	}

	if err := scanned.Scan(12345); err == nil {
		t.Fatal("expected error for unsupported scan type")
	}
}

func TestDateHelpers(t *testing.T) {
	d := NewDate(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if d.String() != "2024-01-02" {
		t.Errorf("String = %q", d.String())
	}
	if p := d.Ptr(); p == nil || p.String() != "2024-01-02" {
		t.Errorf("Ptr = %v", p)
	}
	if (Date{}).Ptr() != nil {
		t.Error("zero Ptr should be nil")
	}
	if (Date{}).String() != "" {
		t.Error("zero String should be empty")
	}
}
