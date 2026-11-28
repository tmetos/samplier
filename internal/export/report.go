package export

import (
	"bytes"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// ReportTab is one worksheet in a multi-tab crosstab report.
type ReportTab struct {
	Key   string
	Label string
	Rows  []Row
}

// ReportData is a complete custom report: one or more analyte grouping sets,
// each rendered as its own worksheet.
type ReportData struct {
	Title             string
	ProjectCode       string
	ProjectName       string
	GeneratedAt       time.Time
	ActionLevelGroups []string
	Tabs              []ReportTab
}

// EncodeReportXLSX produces a multi-sheet XLSX workbook: Summary, one sheet per
// analyte grouping set, and an Exceedances sheet.
func EncodeReportXLSX(data ReportData, opts Options) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	title := opts.Title
	if title == "" {
		title = data.Title
	}
	if title == "" {
		title = "Analytical Results"
	}

	const summary = "Summary"
	if err := f.SetSheetName("Sheet1", summary); err != nil {
		return nil, err
	}

	used := map[string]bool{summary: true}
	for _, tab := range data.Tabs {
		name := uniqueSheetName(tab.Label, used)
		used[name] = true
		if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
		if err := writeReportResultsSheet(f, name, data, tab, opts); err != nil {
			return nil, err
		}
	}

	if err := writeReportExceedanceSheet(f, data); err != nil {
		return nil, err
	}
	if err := writeReportSummarySheet(f, summary, title, data, opts); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeReportResultsSheet writes a crosstab worksheet (analytes as rows,
// locations/dates as column groups) for one analyte grouping set.
func writeReportResultsSheet(f *excelize.File, sheet string, data ReportData, tab ReportTab, opts Options) error {
	rows := filterRows(tab.Rows, opts)

	analytes := opts.AnalyteOrder
	if len(analytes) == 0 {
		for _, r := range rows {
			analytes = append(analytes, analyteLabel(r))
		}
		analytes = uniqueStrings(analytes)
		sort.Strings(analytes)
	}

	var sampleCols []crosstabKey
	seen := map[crosstabKey]bool{}
	for _, r := range rows {
		k := crosstabKey{location: r.LocationName, date: formatDay(r.SampleDate)}
		if !seen[k] {
			seen[k] = true
			sampleCols = append(sampleCols, k)
		}
	}
	sort.SliceStable(sampleCols, func(i, j int) bool {
		if sampleCols[i].location != sampleCols[j].location {
			return sampleCols[i].location < sampleCols[j].location
		}
		return sampleCols[i].date < sampleCols[j].date
	})

	mustSet(f, sheet, "A1", tab.Label)
	mustSet(f, sheet, "A2", "Project: "+data.ProjectName+" ("+data.ProjectCode+")")
	if len(data.ActionLevelGroups) > 0 {
		mustSet(f, sheet, "A3", "Action levels: "+strings.Join(data.ActionLevelGroups, ", "))
	}
	mustSet(f, sheet, "A4", "Generated: "+formatDateTime(data.GeneratedAt))

	headerRow := 6
	mustSet(f, sheet, cellName(1, headerRow), "Analyte")
	mustSet(f, sheet, cellName(2, headerRow), "Unit")

	col := 3
	type sampleSpan struct{ start, end int }
	spans := make([]sampleSpan, len(sampleCols))
	for i, sc := range sampleCols {
		spans[i] = sampleSpan{start: col, end: col + 1}
		mustSet(f, sheet, cellName(col, headerRow), sc.location)
		mustSet(f, sheet, cellName(col+1, headerRow), "")
		mustSet(f, sheet, cellName(col, headerRow+1), "Result")
		mustSet(f, sheet, cellName(col+1, headerRow+1), "Flags")
		col += 2
	}
	lastCol := col - 1

	for _, sp := range spans {
		if sp.end > sp.start {
			_ = f.MergeCell(sheet, cellName(sp.start, headerRow), cellName(sp.end, headerRow))
		}
	}
	_ = f.MergeCell(sheet, "A1", cellName(lastCol, 1))
	_ = f.MergeCell(sheet, "A2", cellName(lastCol, 2))

	cells := map[string]map[string]crosstabCell{}
	for _, r := range rows {
		label := analyteLabel(r)
		k := crosstabKey{location: r.LocationName, date: formatDay(r.SampleDate)}.encoded()
		if cells[label] == nil {
			cells[label] = map[string]crosstabCell{}
		}
		c := cells[label][k]
		v := r.ReportedValue()
		if c.value != "" && v != "" && c.value != v {
			c.value += " / " + v
		} else if c.value == "" {
			c.value = v
		}
		if fl := rowFlag(r); fl != "" {
			c.flags = append(c.flags, fl)
		}
		if r.Exceeded {
			c.exceeded = true
		}
		if r.IsNonDetect() {
			c.nonDetect = true
		}
		cells[label][k] = c
	}

	dataRow := headerRow + 2
	for _, label := range analytes {
		mustSet(f, sheet, cellName(1, dataRow), label)
		mustSet(f, sheet, cellName(2, dataRow), unitFor(rows, label))

		c := 3
		for _, sc := range sampleCols {
			cell := cells[label][sc.encoded()]
			mustSet(f, sheet, cellName(c, dataRow), cell.value)
			mustSet(f, sheet, cellName(c+1, dataRow), strings.Join(uniqueStrings(cell.flags), ","))

			style, err := cellStyle(f, cell)
			if err != nil {
				return err
			}
			if style != 0 {
				_ = f.SetCellStyle(sheet, cellName(c, dataRow), cellName(c+1, dataRow), style)
			}
			c += 2
		}
		dataRow++
	}

	_ = f.SetColWidth(sheet, "A", "A", 28)
	_ = f.SetColWidth(sheet, "B", "B", 10)
	if lastCol >= 3 {
		_ = f.SetColWidth(sheet, "C", cellName(lastCol, 1), 14)
	}
	return nil
}

// writeReportSummarySheet writes the workbook front page.
func writeReportSummarySheet(f *excelize.File, sheet, title string, data ReportData, opts Options) error {
	mustSet(f, sheet, "A1", title)
	if data.ProjectName != "" || data.ProjectCode != "" {
		mustSet(f, sheet, "A2", "Project: "+data.ProjectName+" ("+data.ProjectCode+")")
	}
	if len(data.ActionLevelGroups) > 0 {
		mustSet(f, sheet, "A3", "Action levels: "+strings.Join(data.ActionLevelGroups, ", "))
	}
	mustSet(f, sheet, "A4", "Generated: "+formatDateTime(data.GeneratedAt))

	row := 6
	mustSet(f, sheet, cellName(1, row), "Tab")
	mustSet(f, sheet, cellName(2, row), "Analytes")
	mustSet(f, sheet, cellName(3, row), "Results")
	mustSet(f, sheet, cellName(4, row), "Exceedances")
	row++

	for _, tab := range data.Tabs {
		rows := filterRows(tab.Rows, opts)
		analyteCount := len(uniqueStrings(analyteNames(rows)))
		exceedances := 0
		for _, r := range rows {
			if r.Exceeded {
				exceedances++
			}
		}
		mustSet(f, sheet, cellName(1, row), tab.Label)
		mustSet(f, sheet, cellName(2, row), itoa(analyteCount))
		mustSet(f, sheet, cellName(3, row), itoa(len(rows)))
		mustSet(f, sheet, cellName(4, row), itoa(exceedances))
		row++
	}

	_ = f.SetColWidth(sheet, "A", "A", 30)
	_ = f.SetColWidth(sheet, "B", "D", 14)
	return nil
}

// writeReportExceedanceSheet lists every exceedance across every tab, including
// the analyte grouping set (tab) and the regulatory limit group it breached.
func writeReportExceedanceSheet(f *excelize.File, data ReportData) error {
	const sheet = "Exceedances"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}

	headers := []string{"Tab", "Location", "Sample", "Date", "Analyte", "CAS RN",
		"Result", "Unit", "Action Level", "Group", "Type"}
	for i, h := range headers {
		mustSet(f, sheet, cellName(i+1, 1), h)
	}

	n := 2
	for _, tab := range data.Tabs {
		for _, r := range tab.Rows {
			exceedances := r.Exceedances
			if len(exceedances) == 0 && r.Exceeded {
				exceedances = []Exceedance{{Group: r.ActionLevelGroup, Type: r.ActionLevelType, Level: r.ActionLevel}}
			}
			for _, e := range exceedances {
				limit := ""
				if e.Level != nil {
					limit = formatNumber(*e.Level)
				}
				vals := []string{
					tab.Label, r.LocationName, r.SampleName, formatDay(r.SampleDate),
					r.AnalyteName, r.CASRN, r.ReportedValue(), r.Units, limit,
					e.Group, e.Type,
				}
				for i, v := range vals {
					mustSet(f, sheet, cellName(i+1, n), v)
				}
				n++
			}
		}
	}
	_ = f.SetColWidth(sheet, "A", "K", 18)
	return nil
}

// uniqueSheetName returns a sanitised, unique, ≤31-character sheet name.
func uniqueSheetName(label string, used map[string]bool) string {
	if label == "" {
		label = "Results"
	}
	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.TrimSpace(b.String())
	if name == "" {
		name = "Results"
	}
	if len(name) > 31 {
		name = name[:31]
	}
	base := name
	for i := 2; used[name]; i++ {
		suffix := "-" + itoa(i)
		name = base[:minInt(len(base), 31-len(suffix))] + suffix
	}
	return name
}

// analyteNames returns the distinct analyte labels in a row set.
func analyteNames(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, analyteLabel(r))
	}
	return out
}

// itoa renders a small integer without importing strconv at every call site.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
