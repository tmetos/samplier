// Package actionlevel compares analytical results against regulatory limits.
//
// The model groups limits by jurisdiction and matching criteria:
// action_level_type (jurisdiction) -> action_level (group with matching
// criteria) -> action_level_parameter (per-analyte limit).
package actionlevel

import (
	"errors"
	"fmt"
	"time"

	"github.com/samplier/samplier/internal/units"
)

// ErrIncompatibleUnits is returned when a result and limit cannot be compared.
var ErrIncompatibleUnits = errors.New("result and action level units are not convertible")

// Verdict describes the outcome of comparing one result to one limit.
type Verdict string

const (
	// VerdictNone means no limit applied to this result.
	VerdictNone Verdict = "none"
	// VerdictPass means the result is within the limit.
	VerdictPass Verdict = "pass"
	// VerdictExceed means the result breaches the limit.
	VerdictExceed Verdict = "exceed"
	// VerdictNonDetectExceed means the detection limit exceeds the action level.
	VerdictNonDetectExceed Verdict = "nondetect_exceed"
)

// Parameter is one analyte limit inside a group.
type Parameter struct {
	AnalyteID      string
	ActionLevel    float64  // upper bound: exceed when result > this
	ActionLevelMin *float64 // lower bound (screening range): exceed when result < this
	Unit           string
	Fraction       string
	AnalyticMethod string
}

// Group is an action level group with its matching criteria.
type Group struct {
	ID         string
	TypeCode   string
	Code       string
	Name       string
	Matrix     string
	Fraction   string
	ProjectID  string // empty = global
	StartDate  *time.Time
	EndDate    *time.Time
	LandUse    string
	DepthMin   *float64
	DepthMax   *float64
	Parameters []Parameter
}

// Result is the measurement being compared.
type Result struct {
	AnalyteID      string
	Value          *float64 // nil when non-detect
	DetectFlag     string   // "Y" or "N"
	Unit           string
	DetectionLimit *float64
	DetectionUnit  string
	Fraction       string
	AnalyticMethod string
	Matrix         string
	ProjectID      string
	SampleDate     time.Time
	Depth          *float64
	LandUse        string
}

// Match reports whether the group's criteria apply to the result.
func (g Group) Match(r Result) bool {
	if g.Matrix != "" && !equalFold(g.Matrix, r.Matrix) {
		return false
	}
	if g.Fraction != "" && !equalFold(g.Fraction, r.Fraction) {
		return false
	}
	if g.ProjectID != "" && g.ProjectID != r.ProjectID {
		return false
	}
	if g.LandUse != "" && !equalFold(g.LandUse, r.LandUse) {
		return false
	}
	if g.StartDate != nil && r.SampleDate.Before(*g.StartDate) {
		return false
	}
	if g.EndDate != nil && r.SampleDate.After(*g.EndDate) {
		return false
	}
	if g.DepthMin != nil && (r.Depth == nil || *r.Depth < *g.DepthMin) {
		return false
	}
	if g.DepthMax != nil && (r.Depth == nil || *r.Depth > *g.DepthMax) {
		return false
	}
	return true
}

// Comparison records the verdict for one result against one parameter.
type Comparison struct {
	GroupCode string
	GroupName string
	TypeCode  string
	Verdict   Verdict
	Limit     float64
	LimitUnit string
	// ComparableValue is the result value expressed in LimitUnit.
	ComparableValue *float64
	// DetectionLimitInLimitUnit is the detection limit in LimitUnit (non-detects).
	DetectionLimitInLimitUnit *float64
}

// Exceeds compares a single result against a single parameter. A nil parameter
// unit means the result unit is used as-is.
func Exceeds(r Result, p Parameter, table *units.Table) (Comparison, error) {
	cmp := Comparison{Limit: p.ActionLevel, LimitUnit: p.Unit}

	limitUnit := p.Unit
	if limitUnit == "" {
		limitUnit = r.Unit
		cmp.LimitUnit = limitUnit
	}

	// Non-detect: compare the detection limit against the action level.
	if r.Value == nil || equalFold(r.DetectFlag, "N") {
		if r.DetectionLimit == nil {
			cmp.Verdict = VerdictNone
			return cmp, nil
		}
		dlUnit := r.DetectionUnit
		if dlUnit == "" {
			dlUnit = r.Unit
		}
		dl, err := convert(table, *r.DetectionLimit, dlUnit, limitUnit)
		if err != nil {
			return cmp, fmt.Errorf("%w: %v", ErrIncompatibleUnits, err)
		}
		cmp.DetectionLimitInLimitUnit = &dl
		if dl > p.ActionLevel {
			cmp.Verdict = VerdictNonDetectExceed
		} else {
			cmp.Verdict = VerdictPass
		}
		return cmp, nil
	}

	value, err := convert(table, *r.Value, r.Unit, limitUnit)
	if err != nil {
		return cmp, fmt.Errorf("%w: %v", ErrIncompatibleUnits, err)
	}
	cmp.ComparableValue = &value

	switch {
	case p.ActionLevelMin != nil && value < *p.ActionLevelMin:
		cmp.Verdict = VerdictExceed
	case value > p.ActionLevel:
		cmp.Verdict = VerdictExceed
	default:
		cmp.Verdict = VerdictPass
	}
	return cmp, nil
}

// Evaluate compares a result against every matching parameter across groups.
// Ordering follows the input order so callers control precedence.
func Evaluate(r Result, groups []Group, table *units.Table) ([]Comparison, error) {
	var out []Comparison
	for _, g := range groups {
		if !g.Match(r) {
			continue
		}
		for _, p := range g.Parameters {
			if p.AnalyteID != r.AnalyteID {
				continue
			}
			if p.Fraction != "" && !equalFold(p.Fraction, r.Fraction) {
				continue
			}
			if p.AnalyticMethod != "" && !equalFold(p.AnalyticMethod, r.AnalyticMethod) {
				continue
			}
			cmp, err := Exceeds(r, p, table)
			if err != nil {
				return out, err
			}
			cmp.GroupCode = g.Code
			cmp.GroupName = g.Name
			cmp.TypeCode = g.TypeCode
			out = append(out, cmp)
		}
	}
	return out, nil
}

// Exceeded reports whether any comparison in the slice is an exceedance.
func Exceeded(cmps []Comparison) bool {
	for _, c := range cmps {
		if c.Verdict == VerdictExceed || c.Verdict == VerdictNonDetectExceed {
			return true
		}
	}
	return false
}

func convert(table *units.Table, v float64, from, to string) (float64, error) {
	if table == nil {
		if equalFold(from, to) {
			return v, nil
		}
		return 0, fmt.Errorf("%w: %s -> %s", ErrIncompatibleUnits, from, to)
	}
	return table.Convert(v, from, to)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
