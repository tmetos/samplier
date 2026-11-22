// Package ogcquery parses OGC API — Features query parameters.
package ogcquery

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BBox is a bounding box in CRS84 (minLon, minLat, maxLon, maxLat).
type BBox struct {
	MinLon, MinLat, MaxLon, MaxLat float64
}

// Params holds parsed feature-collection query parameters.
type Params struct {
	Limit    int
	Offset   int
	BBox     *BBox
	Datetime *DatetimeRange
}

// DatetimeRange is a parsed `datetime` parameter. Nil bounds are open.
type DatetimeRange struct {
	Start *time.Time
	End   *time.Time
}

// Defaults for paging.
const (
	DefaultLimit = 10
	MaxLimit     = 1000
)

// ParseLimit reads a limit parameter, clamped to [1, MaxLimit].
func ParseLimit(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return DefaultLimit
	}
	if v < 1 {
		return 1
	}
	if v > MaxLimit {
		return MaxLimit
	}
	return v
}

// ParseOffset reads an offset parameter; invalid or negative values become 0.
func ParseOffset(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// ParseBBox parses "minx,miny,maxx,maxy". OGC also allows 3D/4D boxes
// (minx,miny,minz,maxx,maxy,maxz and ...), where the horizontal bounds are at
// indices 0,1 and the ones at len-4,len-3.
func ParseBBox(s string) (*BBox, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty bbox")
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	var idx [4]int
	switch len(parts) {
	case 4:
		idx = [4]int{0, 1, 2, 3}
	case 6: // 3D
		idx = [4]int{0, 1, 3, 4}
	case 8: // 4D
		idx = [4]int{0, 1, 4, 5}
	default:
		return nil, fmt.Errorf("bbox requires 4, 6, or 8 values, got %d", len(parts))
	}

	var vals [4]float64
	for i, at := range idx {
		v, err := strconv.ParseFloat(parts[at], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid bbox value %q", parts[at])
		}
		vals[i] = v
	}
	if vals[0] > vals[2] {
		return nil, fmt.Errorf("bbox minimum longitude exceeds maximum")
	}
	if vals[1] > vals[3] {
		return nil, fmt.Errorf("bbox minimum latitude exceeds maximum")
	}
	return &BBox{MinLon: vals[0], MinLat: vals[1], MaxLon: vals[2], MaxLat: vals[3]}, nil
}

// ParseDatetime parses an RFC3339 instant, a date, or an interval ("a/b").
// Open intervals use an empty side, e.g. "../2024-01-01" or "2024-01-01/..".
func ParseDatetime(s string) (*DatetimeRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty datetime")
	}
	if !strings.Contains(s, "/") {
		t, err := parseTime(s)
		if err != nil {
			return nil, err
		}
		return &DatetimeRange{Start: &t, End: &t}, nil
	}

	parts := strings.SplitN(s, "/", 2)
	open := func(v string) bool {
		v = strings.TrimSpace(v)
		return v == "" || v == ".."
	}

	r := &DatetimeRange{}
	if !open(parts[0]) {
		t, err := parseTime(parts[0])
		if err != nil {
			return nil, err
		}
		r.Start = &t
	}
	if !open(parts[1]) {
		t, err := parseTime(parts[1])
		if err != nil {
			return nil, err
		}
		r.End = &t
	}
	if r.Start == nil && r.End == nil {
		return nil, fmt.Errorf("datetime interval has no bounds")
	}
	return r, nil
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime %q", s)
}

// Parse parses the full parameter set, ignoring invalid bbox/datetime rather
// than failing the request (matching lenient OGC server behaviour).
func Parse(limit, offset, bbox, datetime string) Params {
	p := Params{Limit: ParseLimit(limit), Offset: ParseOffset(offset)}
	if b, err := ParseBBox(bbox); err == nil {
		p.BBox = b
	}
	if d, err := ParseDatetime(datetime); err == nil {
		p.Datetime = d
	}
	return p
}
