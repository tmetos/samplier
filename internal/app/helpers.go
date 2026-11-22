package app

import (
	"context"
	"net/http"
	"time"

	"github.com/samplier/samplier/internal/actionlevel"
	"github.com/samplier/samplier/internal/export"
	"github.com/samplier/samplier/internal/units"
)

// applyActionLevels evaluates action levels for the given rows in place.
func (h *exportHandlers) applyActionLevels(r *http.Request, rows []export.Row, f resultFilters) error {
	if len(rows) == 0 {
		return nil
	}

	groups, err := h.store.ListActionLevelGroups(r.Context(), f.Type, f.ProjectID)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	conv, err := h.unitTable(r.Context())
	if err != nil {
		return err
	}

	agroups := make([]actionlevel.Group, 0, len(groups))
	for _, g := range groups {
		ag := actionlevel.Group{ID: g.ID, TypeCode: g.TypeCode, Code: g.Code, Name: g.Name}
		if g.Matrix != nil {
			ag.Matrix = *g.Matrix
		}
		if g.Fraction != nil {
			ag.Fraction = *g.Fraction
		}
		if g.ProjectID != nil {
			ag.ProjectID = *g.ProjectID
		}
		ag.StartDate = parseDay(g.StartDate)
		ag.EndDate = parseDay(g.EndDate)
		for _, p := range g.Parameters {
			param := actionlevel.Parameter{
				AnalyteID:      p.AnalyteID,
				ActionLevel:    p.ActionLevel,
				ActionLevelMin: p.ActionLevelMin,
				Unit:           p.Unit,
			}
			if p.Fraction != nil {
				param.Fraction = *p.Fraction
			}
			if p.AnalyticMethod != nil {
				param.AnalyticMethod = *p.AnalyticMethod
			}
			ag.Parameters = append(ag.Parameters, param)
		}
		agroups = append(agroups, ag)
	}

	for i := range rows {
		row := &rows[i]
		res := actionlevel.Result{
			AnalyteID:      row.AnalyteID,
			Value:          row.ResultNumeric,
			DetectFlag:     row.DetectFlag,
			Unit:           row.Units,
			DetectionLimit: row.DetectionLimit(),
			DetectionUnit:  row.LimitUnit,
			Fraction:       row.Fraction,
			AnalyticMethod: row.Method,
			Matrix:         row.Matrix,
			ProjectID:      f.ProjectID,
			SampleDate:     row.SampleDate,
			Depth:          row.StartDepth,
		}
		cmps, cmpErr := actionlevel.Evaluate(res, agroups, conv)
		if cmpErr != nil {
			continue // incomparable units must not fail the report
		}
		for _, c := range cmps {
			if c.Verdict == actionlevel.VerdictExceed ||
				c.Verdict == actionlevel.VerdictNonDetectExceed {
				row.Exceeded = true
				row.ActionLevelGroup = c.GroupCode
				row.ActionLevelType = c.TypeCode
				limit := c.Limit
				row.ActionLevel = &limit
				break
			}
		}
	}
	return nil
}

// unitTable builds a conversion table from the database.
func (h *exportHandlers) unitTable(ctx context.Context) (*units.Table, error) {
	rows, err := h.store.ListConversions(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]units.Conversion, 0, len(rows))
	for _, r := range rows {
		c := units.Conversion{
			ReportedUnit:     r.ReportedUnit,
			TargetUnit:       r.TargetUnit,
			ConversionFactor: r.ConversionFactor,
			Delta:            r.Delta,
		}
		if r.Formula != nil {
			c.Formula = *r.Formula
		}
		rules = append(rules, c)
	}
	return units.NewTable(rules), nil
}

// exportDay renders a sample timestamp as a UTC calendar date.
func exportDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func parseDay(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", *s); err == nil {
		return &t
	}
	return nil
}
