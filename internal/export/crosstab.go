package export

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// CrosstabAdapter produces a formatted multi-sheet XLSX crosstab: analytes as
// columns, locations as rows, one column group per sample date.
type CrosstabAdapter struct{}

// Code implements Adapter.
func (CrosstabAdapter) Code() string { return "CROSSTAB" }

// ContentType implements Adapter.
func (CrosstabAdapter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

// FileExtension implements Adapter.
func (CrosstabAdapter) FileExtension() string { return ".xlsx" }

// crosstabCell is one analyte result at a location/date.
type crosstabCell struct {
	value     string
	flags     []string
	exceeded  bool
	nonDetect bool
}

// crosstabKey identifies a location + sample date column.
type crosstabKey struct {
	location string
	date     string
}

// Encode implements Adapter.
func (a CrosstabAdapter) Encode(ds Dataset, opts Options) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	title := opts.Title
	if title == "" {
		title = ds.Title
	}
	if title == "" {
		title = "Analytical Results"
	}

	resultsSheet := "Results"
	if err := f.SetSheetName("Sheet1", resultsSheet); err != nil {
		return nil, err
	}

	rows := filterRows(ds.Rows, opts)

	// Determine analyte columns.
	analytes := opts.AnalyteOrder
	if len(analytes) == 0 {
		for _, r := range rows {
			analytes = append(analytes, analyteLabel(r))
		}
		analytes = uniqueStrings(analytes)
		sort.Strings(analytes)
	}

	// Determine sample columns (location + date), in first-seen order.
	var sampleCols []crosstabKey
	seen := map[crosstabKey]bool{}
	for _, r := range rows {
		k := crosstabKey{location: r.LocationName, date: formatDay(r.SampleDate)}
		if !seen[k] {
			seen[k] = true
			sampleCols = append(sampleCols, k)
		}
	}
	// Group by location, then date.
	sort.SliceStable(sampleCols, func(i, j int) bool {
		if sampleCols[i].location != sampleCols[j].location {
			return sampleCols[i].location < sampleCols[j].location
		}
		return sampleCols[i].date < sampleCols[j].date
	})

	// Header block.
	mustSet(f, resultsSheet, "A1", title)
	mustSet(f, resultsSheet, "A2", "Project: "+ds.ProjectName+" ("+ds.ProjectCode+")")
	if len(ds.ActionLevelGroups) > 0 {
		mustSet(f, resultsSheet, "A3", "Action levels: "+strings.Join(ds.ActionLevelGroups, ", "))
	}
	mustSet(f, resultsSheet, "A4", "Generated: "+formatDateTime(ds.GeneratedAt))

	headerRow := 6
	// Column A: analyte; column B: unit.
	mustSet(f, resultsSheet, cellName(1, headerRow), "Analyte")
	mustSet(f, resultsSheet, cellName(2, headerRow), "Unit")

	// Two columns per sample (value + flag).
	col := 3
	type sampleSpan struct{ start, end int }
	spans := make([]sampleSpan, len(sampleCols))
	for i, sc := range sampleCols {
		spans[i] = sampleSpan{start: col, end: col + 1}
		mustSet(f, resultsSheet, cellName(col, headerRow), sc.location)
		mustSet(f, resultsSheet, cellName(col+1, headerRow), "")
		sub := headerRow + 1
		mustSet(f, resultsSheet, cellName(col, sub), "Result")
		mustSet(f, resultsSheet, cellName(col+1, sub), "Flags")
		col += 2
	}
	lastCol := col - 1

	// Merge the sample header cells across their two columns.
	for _, sp := range spans {
		if sp.end > sp.start {
			start := cellName(sp.start, headerRow)
			end := cellName(sp.end, headerRow)
			_ = f.MergeCell(resultsSheet, start, end)
		}
	}
	_ = f.MergeCell(resultsSheet, "A1", cellName(lastCol, 1))
	_ = f.MergeCell(resultsSheet, "A2", cellName(lastCol, 2))

	// Build a lookup of cell values.
	cells := map[string]map[string]crosstabCell{} // analyte -> "loc|date" -> cell
	for _, r := range rows {
		label := analyteLabel(r)
		k := crosstabKey{location: r.LocationName, date: formatDay(r.SampleDate)}.encoded()
		if cells[label] == nil {
			cells[label] = map[string]crosstabCell{}
		}
		c := cells[label][k]
		// A replicate: append rather than overwrite.
		v := r.ReportedValue()
		if c.value != "" && v != "" && c.value != v {
			c.value += " / " + v
		} else if c.value == "" {
			c.value = v
		}
		if f := rowFlag(r); f != "" {
			c.flags = append(c.flags, f)
		}
		if r.Exceeded {
			c.exceeded = true
		}
		if r.IsNonDetect() {
			c.nonDetect = true
		}
		cells[label][k] = c
	}

	// Data rows.
	dataRow := headerRow + 2
	for _, label := range analytes {
		mustSet(f, resultsSheet, cellName(1, dataRow), label)
		mustSet(f, resultsSheet, cellName(2, dataRow), unitFor(rows, label))

		c := 3
		for _, sc := range sampleCols {
			cell := cells[label][sc.encoded()]
			mustSet(f, resultsSheet, cellName(c, dataRow), cell.value)
			mustSet(f, resultsSheet, cellName(c+1, dataRow), strings.Join(uniqueStrings(cell.flags), ","))

			style, err := cellStyle(f, cell)
			if err != nil {
				return nil, err
			}
			if style != 0 {
				_ = f.SetCellStyle(resultsSheet, cellName(c, dataRow), cellName(c+1, dataRow), style)
			}
			c += 2
		}
		dataRow++
	}

	// Column widths for readability.
	_ = f.SetColWidth(resultsSheet, "A", "A", 28)
	_ = f.SetColWidth(resultsSheet, "B", "B", 10)
	if lastCol >= 3 {
		_ = f.SetColWidth(resultsSheet, "C", cellName(lastCol, 1), 14)
	}

	// Second sheet: exceedances.
	if err := writeExceedanceSheet(f, ds, rows); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeExceedanceSheet adds a sheet listing only exceedances.
func writeExceedanceSheet(f *excelize.File, ds Dataset, rows []Row) error {
	const sheet = "Exceedances"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	headers := []string{"Location", "Sample", "Date", "Analyte", "CAS RN", "Result",
		"Unit", "Action Level", "Group", "Type"}
	for i, h := range headers {
		mustSet(f, sheet, cellName(i+1, 1), h)
	}

	n := 2
	for _, r := range rows {
		if !r.Exceeded {
			continue
		}
		limit := ""
		if r.ActionLevel != nil {
			limit = formatNumber(*r.ActionLevel)
		}
		vals := []string{
			r.LocationName, r.SampleName, formatDay(r.SampleDate),
			r.AnalyteName, r.CASRN, r.ReportedValue(), r.Units, limit,
			r.ActionLevelGroup, r.ActionLevelType,
		}
		for i, v := range vals {
			mustSet(f, sheet, cellName(i+1, n), v)
		}
		n++
	}
	_ = f.SetColWidth(sheet, "A", "J", 18)
	return nil
}

// cellStyle returns a style id for a crosstab cell.
func cellStyle(f *excelize.File, c crosstabCell) (int, error) {
	var styleID int
	var err error
	switch {
	case c.exceeded:
		// Brand "exceedance" pink, chosen for contrast on white Excel sheets.
		styleID, err = f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Color: "B3245A"},
			Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFE3EC"}},
		})
	case c.nonDetect:
		styleID, err = f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Color: "666666", Italic: true},
		})
	}
	return styleID, err
}

func mustSet(f *excelize.File, sheet, cell, value string) {
	if value == "" {
		return
	}
	_ = f.SetCellValue(sheet, cell, value)
}

// cellName converts 1-based column/row to an A1 reference.
func cellName(col, row int) string {
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return "A1"
	}
	return name
}

// analyteLabel prefers chemical name, falling back to CAS RN.
func analyteLabel(r Row) string {
	if r.AnalyteName != "" {
		return r.AnalyteName
	}
	if r.CASRN != "" {
		return r.CASRN
	}
	return "unknown"
}

// unitFor finds the unit used for an analyte across rows.
func unitFor(rows []Row, label string) string {
	for _, r := range rows {
		if analyteLabel(r) == label && r.Units != "" {
			return r.Units
		}
	}
	return ""
}

// rowFlag builds a compact flag string for a result.
func rowFlag(r Row) string {
	var parts []string
	if r.LabQualifier != "" {
		parts = append(parts, r.LabQualifier)
	}
	if r.ValQualifier != "" {
		parts = append(parts, r.ValQualifier)
	}
	if r.Exceeded {
		groups := r.ExceededGroups()
		if len(groups) > 0 {
			parts = append(parts, "EXCEED:"+strings.Join(groups, "+"))
		} else {
			parts = append(parts, "EXCEED")
		}
	}
	return strings.Join(parts, ",")
}

func (k crosstabKey) encoded() string { return k.location + "\x00" + k.date }

// filterRows applies IncludeNonDetects / LimitToDetects options.
func filterRows(rows []Row, opts Options) []Row {
	if opts.LimitToDetects {
		out := make([]Row, 0, len(rows))
		for _, r := range rows {
			if !r.IsNonDetect() {
				out = append(out, r)
			}
		}
		return out
	}
	if !opts.IncludeNonDetects {
		return rows
	}
	return rows
}

// CSVAdapter produces a flat CSV of results.
type CSVAdapter struct{}

// Code implements Adapter.
func (CSVAdapter) Code() string { return "CSV" }

// ContentType implements Adapter.
func (CSVAdapter) ContentType() string { return "text/csv" }

// FileExtension implements Adapter.
func (CSVAdapter) FileExtension() string { return ".csv" }

var csvHeaders = []string{
	"project_code", "location", "location_type", "longitude", "latitude",
	"sample", "sample_type", "matrix", "sample_date", "start_depth", "end_depth",
	"depth_unit", "task", "test_id", "method", "fraction", "analysis_date",
	"lab_sdg", "lab", "cas_rn", "analyte", "result", "result_numeric",
	"detect_flag", "units", "rdl", "mdl", "lab_qualifier", "validator_qualifier",
	"action_level", "action_level_group", "action_level_type", "exceeded",
}

// Encode implements Adapter.
func (CSVAdapter) Encode(ds Dataset, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(strings.Join(csvHeaders, ","))
	buf.WriteString("\n")

	for _, r := range filterRows(ds.Rows, opts) {
		vals := []string{
			r.ProjectCode, r.LocationName, r.LocationType,
			formatNumber(r.Longitude), formatNumber(r.Latitude),
			r.SampleName, r.SampleType, r.Matrix, formatDay(r.SampleDate),
			ptrNum(r.StartDepth), ptrNum(r.EndDepth), r.DepthUnit,
			r.TaskCode, r.TestID, r.Method, r.Fraction, formatDate(r.AnalysisDate),
			r.LabSDG, r.Lab, r.CASRN, r.AnalyteName, r.ReportedValue(),
			ptrNum(r.ResultNumeric), detectFlagCode(r), r.Units,
			ptrNum(r.RDL), ptrNum(r.MDL), r.LabQualifier, r.ValQualifier,
			ptrNum(r.ActionLevel), r.ActionLevelGroup, r.ActionLevelType,
			boolString(r.Exceeded),
		}
		for i, v := range vals {
			vals[i] = csvEscape(v)
		}
		buf.WriteString(strings.Join(vals, ","))
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

func ptrNum(v *float64) string {
	if v == nil {
		return ""
	}
	return formatNumber(*v)
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

var _ = fmt.Sprintf
