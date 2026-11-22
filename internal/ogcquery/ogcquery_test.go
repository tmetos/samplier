package ogcquery

import (
	"testing"
	"time"
)

func TestParseLimit(t *testing.T) {
	cases := map[string]int{
		"":     DefaultLimit,
		"50":   50,
		"0":    1,
		"-5":   1,
		"5000": MaxLimit,
		"abc":  DefaultLimit,
	}
	for in, want := range cases {
		if got := ParseLimit(in); got != want {
			t.Errorf("ParseLimit(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseOffset(t *testing.T) {
	cases := map[string]int{"": 0, "10": 10, "-3": 0, "x": 0}
	for in, want := range cases {
		if got := ParseOffset(in); got != want {
			t.Errorf("ParseOffset(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseBBox(t *testing.T) {
	b, err := ParseBBox("-123.5,47.1,-122.2,48.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.MinLon != -123.5 || b.MinLat != 47.1 || b.MaxLon != -122.2 || b.MaxLat != 48.0 {
		t.Fatalf("unexpected bbox: %+v", b)
	}

	// 6-value (3D) bbox: extra values ignored.
	if _, err := ParseBBox("-123,47,0,-122,48,100"); err != nil {
		t.Fatalf("3D bbox should parse: %v", err)
	}

	for _, bad := range []string{"", "1,2,3", "a,b,c,d", "10,0,-10,5"} {
		if _, err := ParseBBox(bad); err == nil {
			t.Errorf("ParseBBox(%q) expected error", bad)
		}
	}
}

func TestParseDatetimeInstant(t *testing.T) {
	r, err := ParseDatetime("2024-05-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start == nil || r.End == nil || !r.Start.Equal(*r.End) {
		t.Fatalf("expected instant range, got %+v", r)
	}
	// Both bounds pin the day: start == end.
	if r.Start.Day() != 1 || r.Start.Month() != time.May {
		t.Fatalf("unexpected date: %v", r.Start)
	}
}

func TestParseDatetimeInterval(t *testing.T) {
	r, err := ParseDatetime("2024-01-01/2024-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start == nil || r.End == nil {
		t.Fatalf("expected closed range")
	}

	r, err = ParseDatetime("../2024-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != nil || r.End == nil {
		t.Fatalf("expected open start")
	}

	r, err = ParseDatetime("2024-01-01/..")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start == nil || r.End != nil {
		t.Fatalf("expected open end")
	}

	r, err = ParseDatetime("2024-01-01/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start == nil || r.End != nil {
		t.Fatalf("expected open end for trailing slash")
	}
}

func TestParseDatetimeInvalid(t *testing.T) {
	for _, bad := range []string{"", "not-a-date", "../..", "2024-13-45"} {
		if _, err := ParseDatetime(bad); err == nil {
			t.Errorf("ParseDatetime(%q) expected error", bad)
		}
	}
}

func TestParseFull(t *testing.T) {
	p := Parse("25", "50", "-123,47,-122,48", "2024-01-01/2024-12-31")
	if p.Limit != 25 || p.Offset != 50 || p.BBox == nil || p.Datetime == nil {
		t.Fatalf("unexpected params: %+v", p)
	}

	// Invalid bbox/datetime are dropped, paging still parsed.
	p = Parse("5", "0", "garbage", "garbage")
	if p.Limit != 5 || p.BBox != nil || p.Datetime != nil {
		t.Fatalf("unexpected params: %+v", p)
	}
}
