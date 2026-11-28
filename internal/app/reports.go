package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/actionlevel"
	"github.com/samplier/samplier/internal/export"
	"github.com/samplier/samplier/internal/store"
)

// ReportSpec is the request payload for building (or saving) a custom report.
type ReportSpec struct {
	Name                string   `json:"name,omitempty"`
	Description         string   `json:"description,omitempty"`
	Title               string   `json:"title,omitempty"`
	ProjectID           string   `json:"project_id,omitempty"`
	AnalyteIDs          []string `json:"analyte_ids,omitempty"`
	AnalyteGroupIDs     []string `json:"analyte_group_ids,omitempty"`
	ActionLevelGroupIDs []string `json:"action_level_group_ids,omitempty"`
	IncludeNonDetects   *bool    `json:"include_non_detects,omitempty"`
	DetectsOnly         bool     `json:"detects_only,omitempty"`
	LocationFilter      string   `json:"location_filter,omitempty"`
	StartDate           string   `json:"start_date,omitempty"`
	EndDate             string   `json:"end_date,omitempty"`
}

type reportHandlers struct {
	pool   *pgxpool.Pool
	store  *store.Store
	logger *slog.Logger
}

// reportTab is a resolved analyte grouping set (one worksheet).
type reportTab struct {
	Key        string
	Label      string
	AnalyteIDs []string
}

// --- HTTP handlers ----------------------------------------------------------

func (h *reportHandlers) preview(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeReportSpec(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pv, err := h.buildPreview(r.Context(), spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (h *reportHandlers) export(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeReportSpec(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := h.buildReport(r.Context(), spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	xlsx, err := export.EncodeReportXLSX(data, export.Options{
		Title:          spec.Title,
		LimitToDetects: reportLimitToDetects(spec),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := "report"
	if data.ProjectCode != "" {
		filename = sanitize(data.ProjectCode)
	}
	filename += "_crosstab.xlsx"
	w.Header().Set("Content-Type", export.CrosstabAdapter{}.ContentType())
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xlsx)
}

func (h *reportHandlers) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListReportViews(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *reportHandlers) create(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeReportSpec(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if spec.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	v, err := h.store.CreateReportView(r.Context(), viewFromSpec(spec))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *reportHandlers) get(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.GetReportView(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *reportHandlers) update(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeReportSpec(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if spec.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	v, err := h.store.UpdateReportView(r.Context(), chi.URLParam(r, "id"), viewFromSpec(spec))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *reportHandlers) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteReportView(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *reportHandlers) previewSaved(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.GetReportView(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	pv, err := h.buildPreview(r.Context(), specFromView(v))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (h *reportHandlers) exportSaved(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.GetReportView(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	spec := specFromView(v)
	data, err := h.buildReport(r.Context(), spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	xlsx, err := export.EncodeReportXLSX(data, export.Options{
		Title:          spec.Title,
		LimitToDetects: reportLimitToDetects(spec),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := "report"
	if data.ProjectCode != "" {
		filename = sanitize(data.ProjectCode)
	}
	filename += "_crosstab.xlsx"
	w.Header().Set("Content-Type", export.CrosstabAdapter{}.ContentType())
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xlsx)
}

func (h *reportHandlers) writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// --- report building ---------------------------------------------------------

func decodeReportSpec(r *http.Request) (ReportSpec, error) {
	var spec ReportSpec
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return spec, fmt.Errorf("invalid JSON: %w", err)
	}
	return spec, nil
}

// buildReport loads, filters and evaluates the rows for a report spec.
func (h *reportHandlers) buildReport(ctx context.Context, spec ReportSpec) (export.ReportData, error) {
	tabs, analyteIDs, err := h.resolveTabs(ctx, spec)
	if err != nil {
		return export.ReportData{}, err
	}
	rows, err := h.loadReportRows(ctx, spec, analyteIDs)
	if err != nil {
		return export.ReportData{}, err
	}
	if len(rows) == 0 {
		return export.ReportData{}, errors.New("no results matched the report filters")
	}
	groups, err := h.evaluateReportRows(ctx, rows, spec.ActionLevelGroupIDs)
	if err != nil {
		return export.ReportData{}, err
	}
	return assembleReportData(spec, tabs, rows, groups), nil
}

// resolveTabs maps the selected analyte groups (plus any custom analyte list)
// into ordered tabs and returns the union of all analyte ids.
func (h *reportHandlers) resolveTabs(ctx context.Context, spec ReportSpec) ([]reportTab, []string, error) {
	var tabs []reportTab
	allIDs := make([]string, 0)
	seen := map[string]bool{}

	if len(spec.AnalyteGroupIDs) > 0 {
		groups, err := h.store.ListAnalyteGroups(ctx)
		if err != nil {
			return nil, nil, err
		}
		byID := map[string]store.AnalyteGroup{}
		for _, g := range groups {
			byID[g.ID] = g
		}
		for _, id := range spec.AnalyteGroupIDs {
			g, ok := byID[id]
			if !ok {
				return nil, nil, fmt.Errorf("unknown analyte group %q", id)
			}
			label := g.Name
			if label == "" {
				label = g.Code
			}
			tab := reportTab{Key: id, Label: label}
			for _, m := range g.Analytes {
				tab.AnalyteIDs = append(tab.AnalyteIDs, m.AnalyteID)
				if !seen[m.AnalyteID] {
					seen[m.AnalyteID] = true
					allIDs = append(allIDs, m.AnalyteID)
				}
			}
			tabs = append(tabs, tab)
		}
	}

	if len(spec.AnalyteIDs) > 0 {
		tab := reportTab{Key: "custom", Label: "Custom"}
		for _, id := range spec.AnalyteIDs {
			tab.AnalyteIDs = append(tab.AnalyteIDs, id)
			if !seen[id] {
				seen[id] = true
				allIDs = append(allIDs, id)
			}
		}
		tabs = append(tabs, tab)
	}

	if len(tabs) == 0 {
		return nil, nil, errors.New("select at least one analyte group or analyte")
	}
	return tabs, allIDs, nil
}

// loadReportRows queries denormalised results for the report's filters.
func (h *reportHandlers) loadReportRows(ctx context.Context, spec ReportSpec, analyteIDs []string) ([]export.Row, error) {
	conds := []string{}
	args := []any{}
	add := func(v any, cond string) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if spec.ProjectID != "" {
		add(spec.ProjectID, "p.id = $%d")
	}
	if spec.LocationFilter != "" {
		add("%"+spec.LocationFilter+"%", "l.name ILIKE $%d")
	}
	if spec.StartDate != "" {
		add(spec.StartDate, "s.sample_datetime >= $%d::date")
	}
	if spec.EndDate != "" {
		add(spec.EndDate, "s.sample_datetime < ($%d::date + interval '1 day')")
	}
	if len(analyteIDs) > 0 {
		add(analyteIDs, "r.analyte_id = ANY($%d)")
	}
	args = append(args, 50000)

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	query := `
SELECT p.id::text, p.code, p.name, l.name, COALESCE(l.loc_type,''), ST_X(l.geometry), ST_Y(l.geometry),
       s.name, COALESCE(s.sample_type,''), COALESCE(s.matrix,''), s.sample_datetime,
       s.start_depth, s.end_depth, COALESCE(s.depth_unit,''), COALESCE(tk.code,''),
       ts.test_id, COALESCE(ts.analytic_method,''), COALESCE(ts.fraction,''),
       ts.analysis_date, COALESCE(b.name,''), COALESCE(ts.lab,''),
       COALESCE(a.cas_rn,''), a.chemical_name, r.analyte_id::text,
       r.result_numeric, COALESCE(r.result_text,''), COALESCE(r.detect_flag,'Y'),
       COALESCE(r.units,''), r.reporting_detection_limit, r.method_detection_limit,
       COALESCE(r.detection_limit_unit,''), COALESCE(r.lab_qualifiers,''),
       COALESCE(r.validator_qualifiers,'')
FROM result r
JOIN test ts ON ts.id = r.test_id
JOIN sample s ON s.id = ts.sample_id
JOIN task tk ON tk.id = s.task_id
JOIN project p ON p.id = tk.project_id
JOIN location l ON l.id = s.location_id
JOIN analyte a ON a.id = r.analyte_id
LEFT JOIN batch b ON b.id = ts.batch_id
` + where + `
ORDER BY l.name, s.sample_datetime, a.chemical_name
LIMIT $` + fmt.Sprint(len(args))

	dbRows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()

	var rows []export.Row
	for dbRows.Next() {
		var row export.Row
		if err := dbRows.Scan(
			&row.ProjectID, &row.ProjectCode, &row.ProjectName, &row.LocationName,
			&row.LocationType, &row.Longitude, &row.Latitude, &row.SampleName,
			&row.SampleType, &row.Matrix, &row.SampleDate, &row.StartDepth,
			&row.EndDepth, &row.DepthUnit, &row.TaskCode, &row.TestID, &row.Method,
			&row.Fraction, &row.AnalysisDate, &row.LabSDG, &row.Lab, &row.CASRN,
			&row.AnalyteName, &row.AnalyteID, &row.ResultNumeric, &row.ResultText,
			&row.DetectFlag, &row.Units, &row.RDL, &row.MDL, &row.LimitUnit,
			&row.LabQualifier, &row.ValQualifier,
		); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// evaluateReportRows compares every row against the selected action level sets.
func (h *reportHandlers) evaluateReportRows(ctx context.Context, rows []export.Row, groupIDs []string) ([]store.ActionLevelGroup, error) {
	groups, err := h.store.ListActionLevelGroupsByIDs(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 || len(rows) == 0 {
		return groups, nil
	}

	conv, err := buildUnitTable(ctx, h.store)
	if err != nil {
		return nil, err
	}
	agroups := toActionLevelGroups(groups)

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
			ProjectID:      row.ProjectID,
			SampleDate:     row.SampleDate,
			Depth:          row.StartDepth,
		}
		cmps, cmpErr := actionlevel.Evaluate(res, agroups, conv)
		if cmpErr != nil {
			continue // incomparable units must not fail the report
		}
		seenGroup := map[string]bool{}
		for _, c := range cmps {
			if c.Verdict != actionlevel.VerdictExceed &&
				c.Verdict != actionlevel.VerdictNonDetectExceed {
				continue
			}
			if seenGroup[c.GroupCode] {
				continue
			}
			seenGroup[c.GroupCode] = true
			limit := c.Limit
			row.Exceedances = append(row.Exceedances, export.Exceedance{
				Group: c.GroupCode,
				Type:  c.TypeCode,
				Level: &limit,
			})
		}
		if len(row.Exceedances) > 0 {
			row.Exceeded = true
			row.ActionLevelGroup = row.Exceedances[0].Group
			row.ActionLevelType = row.Exceedances[0].Type
			row.ActionLevel = row.Exceedances[0].Level
		}
	}
	return groups, nil
}

// assembleReportData groups rows into tabs and records report metadata.
func assembleReportData(spec ReportSpec, tabs []reportTab, rows []export.Row, groups []store.ActionLevelGroup) export.ReportData {
	data := export.ReportData{Title: spec.Title, GeneratedAt: time.Now().UTC()}
	if len(rows) > 0 {
		data.ProjectCode = rows[0].ProjectCode
		data.ProjectName = rows[0].ProjectName
	}
	for _, g := range groups {
		data.ActionLevelGroups = append(data.ActionLevelGroups, g.Code)
	}

	for _, t := range tabs {
		set := map[string]bool{}
		for _, id := range t.AnalyteIDs {
			set[id] = true
		}
		tabRows := make([]export.Row, 0)
		for _, r := range rows {
			if set[r.AnalyteID] {
				tabRows = append(tabRows, r)
			}
		}
		data.Tabs = append(data.Tabs, export.ReportTab{Key: t.Key, Label: t.Label, Rows: tabRows})
	}
	return data
}

func reportIncludeNonDetects(spec ReportSpec) bool {
	if spec.IncludeNonDetects != nil {
		return *spec.IncludeNonDetects
	}
	return true
}

func reportLimitToDetects(spec ReportSpec) bool {
	return spec.DetectsOnly || !reportIncludeNonDetects(spec)
}

func viewFromSpec(spec ReportSpec) store.ReportView {
	v := store.ReportView{
		Name:                spec.Name,
		Description:         spec.Description,
		Title:               spec.Title,
		AnalyteIDs:          spec.AnalyteIDs,
		AnalyteGroupIDs:     spec.AnalyteGroupIDs,
		ActionLevelGroupIDs: spec.ActionLevelGroupIDs,
		IncludeNonDetects:   reportIncludeNonDetects(spec),
		DetectsOnly:         spec.DetectsOnly,
		LocationFilter:      spec.LocationFilter,
	}
	if spec.ProjectID != "" {
		v.ProjectID = &spec.ProjectID
	}
	if spec.StartDate != "" {
		v.StartDate = &spec.StartDate
	}
	if spec.EndDate != "" {
		v.EndDate = &spec.EndDate
	}
	return v
}

func specFromView(v store.ReportView) ReportSpec {
	spec := ReportSpec{
		Name:                v.Name,
		Description:         v.Description,
		Title:               v.Title,
		AnalyteIDs:          v.AnalyteIDs,
		AnalyteGroupIDs:     v.AnalyteGroupIDs,
		ActionLevelGroupIDs: v.ActionLevelGroupIDs,
		DetectsOnly:         v.DetectsOnly,
		LocationFilter:      v.LocationFilter,
		IncludeNonDetects:   &v.IncludeNonDetects,
	}
	if v.ProjectID != nil {
		spec.ProjectID = *v.ProjectID
	}
	if v.StartDate != nil {
		spec.StartDate = *v.StartDate
	}
	if v.EndDate != nil {
		spec.EndDate = *v.EndDate
	}
	return spec
}

// --- preview JSON ------------------------------------------------------------

type reportPreview struct {
	Title             string             `json:"title"`
	ProjectCode       string             `json:"project_code,omitempty"`
	ProjectName       string             `json:"project_name,omitempty"`
	ActionLevelGroups []string           `json:"action_level_groups"`
	ExceedanceCount   int                `json:"exceedance_count"`
	Tabs              []reportPreviewTab `json:"tabs"`
}

type reportPreviewTab struct {
	Key             string               `json:"key"`
	Label           string               `json:"label"`
	AnalyteCount    int                  `json:"analyte_count"`
	ResultCount     int                  `json:"result_count"`
	ExceedanceCount int                  `json:"exceedance_count"`
	SampleColumns   []reportSampleColumn `json:"sample_columns"`
	Rows            []reportPreviewRow   `json:"rows"`
}

type reportSampleColumn struct {
	Location string `json:"location"`
	Date     string `json:"date"`
	Sample   string `json:"sample"`
}

type reportPreviewRow struct {
	Analyte string              `json:"analyte"`
	CASRN   string              `json:"cas_rn,omitempty"`
	Unit    string              `json:"unit"`
	Cells   []reportPreviewCell `json:"cells"`
}

type reportPreviewCell struct {
	Value     string `json:"value"`
	Flags     string `json:"flags"`
	Exceeded  bool   `json:"exceeded"`
	NonDetect bool   `json:"non_detect"`
}

func (h *reportHandlers) buildPreview(ctx context.Context, spec ReportSpec) (reportPreview, error) {
	data, err := h.buildReport(ctx, spec)
	if err != nil {
		return reportPreview{}, err
	}

	pv := reportPreview{
		Title:             data.Title,
		ProjectCode:       data.ProjectCode,
		ProjectName:       data.ProjectName,
		ActionLevelGroups: data.ActionLevelGroups,
	}
	limitToDetects := reportLimitToDetects(spec)

	for _, tab := range data.Tabs {
		rows := filterReportRows(tab.Rows, limitToDetects)
		ptab := reportPreviewTab{Key: tab.Key, Label: tab.Label, ResultCount: len(rows)}

		analytes := make([]string, 0)
		for _, r := range rows {
			analytes = append(analytes, reportAnalyteLabel(r))
		}
		analytes = reportUniqueStrings(analytes)
		sort.Strings(analytes)
		ptab.AnalyteCount = len(analytes)

		// Sample columns (location + date), sorted like the Excel crosstab.
		type sampleKey struct{ location, date string }
		seen := map[sampleKey]bool{}
		for _, r := range rows {
			k := sampleKey{location: r.LocationName, date: reportDay(r.SampleDate)}
			if !seen[k] {
				seen[k] = true
				ptab.SampleColumns = append(ptab.SampleColumns, reportSampleColumn{
					Location: k.location, Date: k.date, Sample: r.SampleName,
				})
			}
		}
		sort.SliceStable(ptab.SampleColumns, func(i, j int) bool {
			if ptab.SampleColumns[i].Location != ptab.SampleColumns[j].Location {
				return ptab.SampleColumns[i].Location < ptab.SampleColumns[j].Location
			}
			return ptab.SampleColumns[i].Date < ptab.SampleColumns[j].Date
		})

		// Cells keyed by analyte label -> sample key.
		cells := map[string]map[sampleKey]reportPreviewCell{}
		for _, r := range rows {
			label := reportAnalyteLabel(r)
			k := sampleKey{location: r.LocationName, date: reportDay(r.SampleDate)}
			if cells[label] == nil {
				cells[label] = map[sampleKey]reportPreviewCell{}
			}
			c := cells[label][k]
			v := r.ReportedValue()
			if c.Value != "" && v != "" && c.Value != v {
				c.Value += " / " + v
			} else if c.Value == "" {
				c.Value = v
			}
			if f := reportRowFlag(r); f != "" {
				c.Flags = f
			}
			if r.Exceeded {
				c.Exceeded = true
				ptab.ExceedanceCount++
			}
			if r.IsNonDetect() {
				c.NonDetect = true
			}
			cells[label][k] = c
		}

		for _, label := range analytes {
			row := reportPreviewRow{Analyte: label, Unit: reportUnitFor(rows, label)}
			for _, r := range rows {
				if reportAnalyteLabel(r) == label && r.CASRN != "" {
					row.CASRN = r.CASRN
					break
				}
			}
			for _, sc := range ptab.SampleColumns {
				cell := cells[label][sampleKey{location: sc.Location, date: sc.Date}]
				row.Cells = append(row.Cells, cell)
			}
			ptab.Rows = append(ptab.Rows, row)
		}

		pv.ExceedanceCount += ptab.ExceedanceCount
		pv.Tabs = append(pv.Tabs, ptab)
	}
	return pv, nil
}

func filterReportRows(rows []export.Row, limitToDetects bool) []export.Row {
	if !limitToDetects {
		return rows
	}
	out := make([]export.Row, 0, len(rows))
	for _, r := range rows {
		if !r.IsNonDetect() {
			out = append(out, r)
		}
	}
	return out
}

func reportAnalyteLabel(r export.Row) string {
	if r.AnalyteName != "" {
		return r.AnalyteName
	}
	if r.CASRN != "" {
		return r.CASRN
	}
	return "unknown"
}

func reportUnitFor(rows []export.Row, label string) string {
	for _, r := range rows {
		if reportAnalyteLabel(r) == label && r.Units != "" {
			return r.Units
		}
	}
	return ""
}

func reportDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func reportRowFlag(r export.Row) string {
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

func reportUniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
