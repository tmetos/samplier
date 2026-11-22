package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// dateLayouts are accepted when parsing incoming date strings. Environmental
// data commonly arrives as bare dates (YYYY-MM-DD) rather than timestamps.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"01/02/2006",
	"01/02/06",
	"2006/01/02",
	"02-Jan-2006",
}

// Date is a calendar date that round-trips JSON as "YYYY-MM-DD" and accepts
// several common input formats.
type Date struct {
	time.Time
}

// ParseDate parses a date string using the accepted layouts.
func ParseDate(s string) (Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Date{}, nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return Date{Time: t}, nil
		}
	}
	return Date{}, fmt.Errorf("unrecognized date %q", s)
}

// NewDate builds a Date from a time.Time.
func NewDate(t time.Time) Date { return Date{Time: t} }

// Valid reports whether the date holds a non-zero value.
func (d Date) Valid() bool { return !d.Time.IsZero() }

// Ptr returns a pointer to the date when valid, else nil.
func (d Date) Ptr() *Date {
	if !d.Valid() {
		return nil
	}
	c := d
	return &c
}

// UnmarshalJSON accepts "YYYY-MM-DD", RFC3339, or null.
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` {
		d.Time = time.Time{}
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return fmt.Errorf("date must be a string: %w", err)
	}
	parsed, err := ParseDate(str)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// MarshalJSON renders "YYYY-MM-DD", or null for the zero value.
func (d Date) MarshalJSON() ([]byte, error) {
	if !d.Valid() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}

// String renders the date.
func (d Date) String() string {
	if !d.Valid() {
		return ""
	}
	return d.Format("2006-01-02")
}

// Value implements driver.Valuer for database writes.
func (d Date) Value() (driver.Value, error) {
	if !d.Valid() {
		return nil, nil
	}
	return d.Time, nil
}

// Scan implements sql.Scanner for database reads.
func (d *Date) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		d.Time = time.Time{}
		return nil
	case time.Time:
		d.Time = t
		return nil
	case string:
		parsed, err := ParseDate(t)
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	case []byte:
		parsed, err := ParseDate(string(t))
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	default:
		return fmt.Errorf("cannot scan %T into Date", v)
	}
}
