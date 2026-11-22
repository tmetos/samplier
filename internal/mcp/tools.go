package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/actionlevel"
	"github.com/samplier/samplier/internal/export"
	"github.com/samplier/samplier/internal/store"
	"github.com/samplier/samplier/internal/units"
)

// Tools wires Samplier capabilities into an MCP server.
type Tools struct {
	pool  *pgxpool.Pool
	store *store.Store
	conv  *units.Table
	tools []Tool
}

// registeredTools returns the tools this instance exposes (for introspection
// and testing).
func (t *Tools) registeredTools() []Tool { return t.tools }

// NewTools constructs the tool set.
func NewTools(pool *pgxpool.Pool, st *store.Store) *Tools {
	return &Tools{pool: pool, store: st, conv: units.NewTable(nil)}
}

// Register adds every Samplier tool to the server.
func (t *Tools) Register(s *Server) {
	t.add(s, Tool{
		Name:        "list_projects",
		Description: "List environmental projects (sites) with codes and names.",
		InputSchema: schema(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":200}}}`, nil),
		Handler:     t.listProjects,
	})
	t.add(s, Tool{
		Name:        "list_locations",
		Description: "List sampling locations for a project, including coordinates and type.",
		InputSchema: schema(`{"type":"object","properties":{"project_id":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":500}},"required":["project_id"]}`, []string{"project_id"}),
		Handler:     t.listLocations,
	})
	t.add(s, Tool{
		Name:        "list_analytes",
		Description: "List known analytes (chemicals and field parameters) with CAS registry numbers.",
		InputSchema: schema(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":1000}}}`, nil),
		Handler:     t.listAnalytes,
	})
	t.add(s, Tool{
		Name:        "list_action_levels",
		Description: "List regulatory action level groups (EPA, State, Local, Custom) with per-analyte limits.",
		InputSchema: schema(`{"type":"object","properties":{"type":{"type":"string"},"project_id":{"type":"string"}}}`, nil),
		Handler:     t.listActionLevels,
	})
	t.add(s, Tool{
		Name:        "query_results",
		Description: "Query analytical results, optionally filtered by project, location, analyte, and date range.",
		InputSchema: schema(`{"type":"object","properties":{
			"project_id":{"type":"string"},
			"location":{"type":"string"},
			"cas_rn":{"type":"string"},
			"analyte":{"type":"string"},
			"start_date":{"type":"string"},
			"end_date":{"type":"string"},
			"limit":{"type":"integer","minimum":1,"maximum":5000}},"required":["project_id"]}`, []string{"project_id"}),
		Handler: t.queryResults,
	})
	t.add(s, Tool{
		Name:        "get_exceedances",
		Description: "Compare results against action levels and return only exceedances.",
		InputSchema: schema(`{"type":"object","properties":{
			"project_id":{"type":"string"},
			"type":{"type":"string","description":"action level type code, e.g. EPA"},
			"start_date":{"type":"string"},
			"end_date":{"type":"string"}},"required":["project_id"]}`, []string{"project_id"}),
		Handler: t.getExceedances,
	})
	t.add(s, Tool{
		Name:        "generate_crosstab",
		Description: "Generate a formatted XLSX crosstab (analytes as columns, locations as rows) with exceedance highlighting. Returns base64-encoded XLSX.",
		InputSchema: schema(`{"type":"object","properties":{
			"project_id":{"type":"string"},
			"type":{"type":"string"},
			"title":{"type":"string"},
			"limit":{"type":"integer","minimum":1,"maximum":10000}},"required":["project_id"]}`, []string{"project_id"}),
		Handler: t.generateCrosstab,
	})
	t.add(s, Tool{
		Name:        "export_data",
		Description: "Export results in a regulatory or reporting format. Formats: CSV, WQX, AQS, EIM. Returns base64-encoded file plus filename.",
		InputSchema: schema(`{"type":"object","properties":{
			"project_id":{"type":"string"},
			"format":{"type":"string","enum":["CSV","WQX","AQS","EIM"]},
			"type":{"type":"string"},
			"limit":{"type":"integer","minimum":1,"maximum":10000}},"required":["project_id","format"]}`, []string{"project_id", "format"}),
		Handler: t.exportData,
	})
	t.add(s, Tool{
		Name:        "list_export_formats",
		Description: "List available export formats and EDD import formats.",
		InputSchema: schema(`{"type":"object","properties":{}}`, nil),
		Handler:     t.listFormats,
	})
	t.loadConv(context.Background(), s)
}

// add registers a tool with the server and records it locally.
func (t *Tools) add(s *Server, tool Tool) {
	t.tools = append(t.tools, tool)
	s.AddTool(tool)
}

// loadConv loads unit conversions from the database into the cache.
func (t *Tools) loadConv(ctx context.Context, s *Server) {
	if t.pool == nil {
		return
	}
	rows, err := t.store.ListConversions(ctx)
	if err != nil {
		s.logger.Warn("could not load unit conversions", "error", err)
		return
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
	t.conv = units.NewTable(rules)
}

func schema(props string, required []string) json.RawMessage {
	if len(required) == 0 {
		return json.RawMessage(props)
	}
	// Merge required into the provided object.
	obj := strings.TrimSuffix(strings.TrimSpace(props), "}")
	req, _ := json.Marshal(required)
	return json.RawMessage(obj + `,"required":` + string(req) + `}`)
}

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func (t *Tools) listProjects(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Limit int `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	limit := clamp(args.Limit, 1, 200, 100)

	projects, err := t.store.ListProjects(ctx, limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		out = append(out, map[string]any{
			"id": p.ID, "code": p.Code, "name": p.Name,
			"srid": p.SRID, "start_time": p.StartTime,
		})
	}
	return out, nil
}

func (t *Tools) listLocations(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ProjectID string `json:"project_id"`
		Limit     int    `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	limit := clamp(args.Limit, 1, 500, 200)

	locs, err := t.store.ListLocations(ctx, args.ProjectID, limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(locs))
	for _, l := range locs {
		out = append(out, map[string]any{
			"id": l.ID, "name": l.Name, "sys_loc_code": l.SysLocCode,
			"loc_type": l.LocType, "lon": l.Lon, "lat": l.Lat,
			"total_depth": l.TotalDepth, "depth_unit": l.DepthUnit,
		})
	}
	return out, nil
}

func (t *Tools) listAnalytes(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Limit int `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	limit := clamp(args.Limit, 1, 1000, 500)

	analytes, err := t.store.ListAnalytes(ctx, limit, 0)
	if err != nil {
		return nil, err
	}
	return analytes, nil
}

func (t *Tools) listActionLevels(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Type      string `json:"type"`
		ProjectID string `json:"project_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, err
	}
	groups, err := t.store.ListActionLevelGroups(ctx, args.Type, args.ProjectID)
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// resultFilter is the shared filter for query/export tools.
type resultFilter struct {
	ProjectID string `json:"project_id"`
	Location  string `json:"location"`
	CASRN     string `json:"cas_rn"`
	Analyte   string `json:"analyte"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Type      string `json:"type"`
	Limit     int    `json:"limit"`
}

func (t *Tools) queryResults(ctx context.Context, raw json.RawMessage) (any, error) {
	var f resultFilter
	if err := decodeArgs(raw, &f); err != nil {
		return nil, err
	}
	if f.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	rows, err := t.fetchRows(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowMap(r))
	}
	return out, nil
}

func (t *Tools) getExceedances(ctx context.Context, raw json.RawMessage) (any, error) {
	var f resultFilter
	if err := decodeArgs(raw, &f); err != nil {
		return nil, err
	}
	if f.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	rows, err := t.fetchRows(ctx, f)
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for _, r := range rows {
		if r.Exceeded {
			out = append(out, rowMap(r))
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	return map[string]any{
		"count":       len(out),
		"exceedances": out,
	}, nil
}

func (t *Tools) generateCrosstab(ctx context.Context, raw json.RawMessage) (any, error) {
	var f resultFilter
	if err := decodeArgs(raw, &f); err != nil {
		return nil, err
	}
	if f.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	var args struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(raw, &args)

	rows, err := t.fetchRows(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no results matched the query")
	}

	ds := t.dataset(rows, args.Title)
	a, err := export.Get("CROSSTAB")
	if err != nil {
		return nil, err
	}
	data, err := a.Encode(ds, export.Options{Title: ds.Title})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"filename":     sanitizeFilename(ds.ProjectCode) + "_crosstab.xlsx",
		"content_type": a.ContentType(),
		"encoding":     "base64",
		"data":         base64.StdEncoding.EncodeToString(data),
		"row_count":    len(rows),
	}, nil
}

func (t *Tools) exportData(ctx context.Context, raw json.RawMessage) (any, error) {
	var f resultFilter
	if err := decodeArgs(raw, &f); err != nil {
		return nil, err
	}
	var args struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if f.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if args.Format == "" {
		return nil, fmt.Errorf("format is required")
	}

	a, err := export.Get(args.Format)
	if err != nil {
		return nil, err
	}

	rows, err := t.fetchRows(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no results matched the query")
	}

	ds := t.dataset(rows, "")
	data, err := a.Encode(ds, export.Options{})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"filename":     sanitizeFilename(ds.ProjectCode) + "_" + strings.ToLower(a.Code()) + a.FileExtension(),
		"content_type": a.ContentType(),
		"encoding":     "base64",
		"data":         base64.StdEncoding.EncodeToString(data),
		"row_count":    len(rows),
	}, nil
}

func (t *Tools) listFormats(ctx context.Context, raw json.RawMessage) (any, error) {
	return map[string]any{
		"exports": export.Codes(),
		"edd":     []string{"EFWEDD"},
		"commit_modes": []string{
			"INSERT_ONLY", "INSERT_UPDATE", "INSERT_MERGE",
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Data access
// ---------------------------------------------------------------------------

// fetchRows loads denormalised result rows and evaluates action levels.
func (t *Tools) fetchRows(ctx context.Context, f resultFilter) ([]export.Row, error) {
	conds := []string{"p.id = $1"}
	args := []any{f.ProjectID}

	add := func(v any, cond string) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.Location != "" {
		add(f.Location, "l.name ILIKE $%d")
	}
	if f.CASRN != "" {
		add(f.CASRN, "a.cas_rn = $%d")
	}
	if f.Analyte != "" {
		add("%"+f.Analyte+"%", "a.chemical_name ILIKE $%d")
	}
	if f.StartDate != "" {
		add(f.StartDate, "s.sample_datetime >= $%d::date")
	}
	if f.EndDate != "" {
		add(f.EndDate, "s.sample_datetime < ($%d::date + interval '1 day')")
	}

	limit := clamp(f.Limit, 1, 10000, 2000)
	args = append(args, limit)

	query := `
SELECT p.code, p.name, l.name, l.loc_type, ST_X(l.geometry), ST_Y(l.geometry),
       s.name, s.sample_type, s.matrix, s.sample_datetime, s.start_depth, s.end_depth,
       COALESCE(s.depth_unit,''), tk.code,
       ts.test_id, COALESCE(ts.analytic_method,''), COALESCE(ts.fraction,''),
       ts.analysis_date, COALESCE(b.name,''), COALESCE(ts.lab,''),
       COALESCE(a.cas_rn,''), a.chemical_name,
       r.result_numeric, COALESCE(r.result_text,''), COALESCE(r.detect_flag,'Y'),
       COALESCE(r.units,''), r.reporting_detection_limit, r.method_detection_limit,
       COALESCE(r.detection_limit_unit,''), COALESCE(r.lab_qualifiers,''),
       COALESCE(r.validator_qualifiers,''), r.analyte_id::text
FROM result r
JOIN test ts ON ts.id = r.test_id
JOIN sample s ON s.id = ts.sample_id
JOIN task tk ON tk.id = s.task_id
JOIN project p ON p.id = tk.project_id
JOIN location l ON l.id = s.location_id
JOIN analyte a ON a.id = r.analyte_id
LEFT JOIN batch b ON b.id = ts.batch_id
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY l.name, s.sample_datetime, a.chemical_name
LIMIT $` + fmt.Sprint(len(args))

	dbRows, err := t.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()

	var rows []export.Row
	for dbRows.Next() {
		var r export.Row
		var analyteID string
		if err := dbRows.Scan(
			&r.ProjectCode, &r.ProjectName, &r.LocationName, &r.LocationType,
			&r.Longitude, &r.Latitude, &r.SampleName, &r.SampleType, &r.Matrix,
			&r.SampleDate, &r.StartDepth, &r.EndDepth, &r.DepthUnit, &r.TaskCode,
			&r.TestID, &r.Method, &r.Fraction, &r.AnalysisDate, &r.LabSDG, &r.Lab,
			&r.CASRN, &r.AnalyteName, &r.ResultNumeric, &r.ResultText, &r.DetectFlag,
			&r.Units, &r.RDL, &r.MDL, &r.LimitUnit, &r.LabQualifier, &r.ValQualifier,
			&analyteID,
		); err != nil {
			return nil, err
		}
		r.AnalyteID = analyteID
		rows = append(rows, r)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}

	// Evaluate action levels when requested.
	if err := t.evaluate(ctx, rows, f.ProjectID, f.Type); err != nil {
		return nil, err
	}
	return rows, nil
}

// evaluate applies action-level comparisons to rows in place.
func (t *Tools) evaluate(ctx context.Context, rows []export.Row, projectID, typeCode string) error {
	if len(rows) == 0 {
		return nil
	}
	groups, err := t.store.ListActionLevelGroups(ctx, typeCode, projectID)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	// Convert store groups into comparison groups.
	agroups := make([]actionlevel.Group, 0, len(groups))
	for _, g := range groups {
		ag := actionlevel.Group{
			ID: g.ID, TypeCode: g.TypeCode, Code: g.Code, Name: g.Name,
		}
		if g.Matrix != nil {
			ag.Matrix = *g.Matrix
		}
		if g.Fraction != nil {
			ag.Fraction = *g.Fraction
		}
		if g.ProjectID != nil {
			ag.ProjectID = *g.ProjectID
		}
		ag.StartDate = parseDatePtr(g.StartDate)
		ag.EndDate = parseDatePtr(g.EndDate)
		for _, p := range g.Parameters {
			ag.Parameters = append(ag.Parameters, actionlevel.Parameter{
				AnalyteID:      p.AnalyteID,
				ActionLevel:    p.ActionLevel,
				ActionLevelMin: p.ActionLevelMin,
				Unit:           p.Unit,
				Fraction:       deref(p.Fraction),
				AnalyticMethod: deref(p.AnalyticMethod),
			})
		}
		agroups = append(agroups, ag)
	}

	for i := range rows {
		r := &rows[i]
		res := actionlevel.Result{
			AnalyteID:      r.AnalyteID,
			Value:          r.ResultNumeric,
			DetectFlag:     r.DetectFlag,
			Unit:           r.Units,
			DetectionLimit: r.DetectionLimit(),
			DetectionUnit:  r.LimitUnit,
			Fraction:       r.Fraction,
			AnalyticMethod: r.Method,
			Matrix:         r.Matrix,
			ProjectID:      projectID,
			SampleDate:     r.SampleDate,
			Depth:          r.StartDepth,
		}
		cmps, err := actionlevel.Evaluate(res, agroups, t.conv)
		if err != nil {
			// Incompatible units should not fail the whole report.
			continue
		}
		for _, c := range cmps {
			if c.Verdict == actionlevel.VerdictExceed ||
				c.Verdict == actionlevel.VerdictNonDetectExceed {
				r.Exceeded = true
				r.ActionLevelGroup = c.GroupCode
				r.ActionLevelType = c.TypeCode
				limit := c.Limit
				r.ActionLevel = &limit
				r.ExceedanceNote = string(c.Verdict)
				break
			}
		}
	}
	return nil
}

func (t *Tools) dataset(rows []export.Row, title string) export.Dataset {
	ds := export.Dataset{
		Title:       title,
		GeneratedAt: time.Now().UTC(),
		Rows:        rows,
	}
	if len(rows) > 0 {
		ds.ProjectCode = rows[0].ProjectCode
		ds.ProjectName = rows[0].ProjectName
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ActionLevelGroup != "" && !seen[r.ActionLevelGroup] {
			seen[r.ActionLevelGroup] = true
			ds.ActionLevelGroups = append(ds.ActionLevelGroups, r.ActionLevelGroup)
		}
	}
	sort.Strings(ds.ActionLevelGroups)
	return ds
}

func rowMap(r export.Row) map[string]any {
	m := map[string]any{
		"location":     r.LocationName,
		"sample":       r.SampleName,
		"sample_date":  utcDay(r.SampleDate),
		"matrix":       r.Matrix,
		"analyte":      r.AnalyteName,
		"cas_rn":       r.CASRN,
		"result":       r.ReportedValue(),
		"detect":       r.DetectFlag,
		"unit":         r.Units,
		"method":       r.Method,
		"fraction":     r.Fraction,
		"exceeded":     r.Exceeded,
		"action_level": r.ActionLevel,
	}
	if r.ActionLevelGroup != "" {
		m["action_level_group"] = r.ActionLevelGroup
	}
	if r.ActionLevelType != "" {
		m["action_level_type"] = r.ActionLevelType
	}
	return m
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func decodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func clamp(v, min, max, def int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseDatePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", *s); err == nil {
		return &t
	}
	return nil
}

// utcDay renders a sample timestamp as a UTC calendar date.
func utcDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func sanitizeFilename(s string) string {
	if s == "" {
		return "samplier"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
