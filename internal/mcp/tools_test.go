package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/store"
	"github.com/samplier/samplier/internal/testsupport"
)

// toolsFixture creates a project with a full data chain and an action level.
func toolsFixture(t *testing.T, pool *pgxpool.Pool, code string) (projectID, analyteID string) {
	t.Helper()
	ctx := context.Background()
	testsupport.Cleanup(t, pool, code)
	_, _ = pool.Exec(ctx, `DELETE FROM action_level WHERE code=$1`, code+"-AL")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM project WHERE code LIKE $1`, "MCP-%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM action_level WHERE code LIKE $1`, "MCP-%")
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn LIKE $1 AND NOT EXISTS (
			   SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`, "MCP%")
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO project (code, name, srid, elev_unit) VALUES ($1,'MCP Test',4326,'ft')
		 RETURNING id`, code).Scan(&projectID); err != nil {
		t.Fatalf("project: %v", err)
	}

	var taskID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO task (project_id, code, name) VALUES ($1,'T1','Task') RETURNING id`,
		projectID).Scan(&taskID); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO location (project_id, name, loc_type, geometry)
		 VALUES ($1,'MW-01','WELL', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))`,
		projectID); err != nil {
		t.Fatalf("location: %v", err)
	}

	var sampleID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sample (task_id, location_id, name, matrix, sample_datetime)
		 SELECT $1, l.id, 'S-001', 'WATER', '2024-03-01'
		 FROM location l WHERE l.project_id=$2 AND l.name='MW-01'
		 RETURNING id`, taskID, projectID).Scan(&sampleID); err != nil {
		t.Fatalf("sample: %v", err)
	}

	var testID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO test (sample_id, test_id, analytic_method, fraction)
		 VALUES ($1,'TST-1','EPA 6020B','TOTAL') RETURNING id`, sampleID).Scan(&testID); err != nil {
		t.Fatalf("test: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO analyte (cas_rn, chemical_name) VALUES ($1,'Lead')
		 ON CONFLICT (cas_rn) DO UPDATE SET chemical_name=EXCLUDED.chemical_name
		 RETURNING id`, "MCP-"+code).Scan(&analyteID); err != nil {
		t.Fatalf("analyte: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
		                     reporting_detection_limit, detection_limit_unit)
		 VALUES ($1,$2,0.0503,'Y','mg/L',0.001,'mg/L')`, testID, analyteID); err != nil {
		t.Fatalf("result: %v", err)
	}

	// Non-detect row for a second analyte.
	var an2 string
	if err := pool.QueryRow(ctx,
		`INSERT INTO analyte (cas_rn, chemical_name) VALUES ($1,'Arsenic')
		 ON CONFLICT (cas_rn) DO UPDATE SET chemical_name=EXCLUDED.chemical_name
		 RETURNING id`, "MCP2-"+code).Scan(&an2); err != nil {
		t.Fatalf("analyte2: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
		                     reporting_detection_limit, detection_limit_unit)
		 VALUES ($1,$2,NULL,'N','mg/L',0.002,'mg/L')`, testID, an2); err != nil {
		t.Fatalf("nondetect: %v", err)
	}

	// Action level that the Lead result exceeds.
	var groupID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO action_level (type_id, code, name, matrix)
		 SELECT t.id, $1, 'MCP Limit', 'WATER' FROM action_level_type t WHERE t.code='EPA'
		 RETURNING id`, code+"-AL").Scan(&groupID); err != nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level, unit)
		 VALUES ($1,$2,0.015,'mg/L')`, groupID, analyteID); err != nil {
		t.Fatalf("param: %v", err)
	}
	return projectID, analyteID
}

func newTools(t *testing.T, pool *pgxpool.Pool) (*Server, *Tools) {
	t.Helper()
	srv := NewServer("samplier", "test", nil)
	tl := NewTools(pool, store.New(pool))
	tl.Register(srv)
	return srv, tl
}

// invoke calls a tool directly and returns the raw result.
func invoke(t *testing.T, s *Tools, name string, args map[string]any) (any, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	var handler func(context.Context, json.RawMessage) (any, error)
	for _, tool := range s.registeredTools() {
		if tool.Name == name {
			handler = tool.Handler
		}
	}
	if handler == nil {
		t.Fatalf("tool %q not registered", name)
	}
	return handler(context.Background(), raw)
}

func TestToolsRegistration(t *testing.T) {
	pool := testsupport.Pool(t)
	srv, _ := newTools(t, pool)

	names := map[string]bool{}
	for _, tool := range srv.Tools() {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Errorf("tool %q has invalid schema: %v", tool.Name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q schema is not an object: %v", tool.Name, schema)
		}
	}

	for _, want := range []string{
		"list_projects", "list_locations", "list_analytes", "list_action_levels",
		"query_results", "get_exceedances", "generate_crosstab", "export_data",
		"list_export_formats",
	} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestListProjectsTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	toolsFixture(t, pool, "MCP-LISTP")

	res, err := invoke(t, tl, "list_projects", map[string]any{"limit": 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	projects, ok := res.([]map[string]any)
	if !ok {
		t.Fatalf("unexpected type %T", res)
	}
	if len(projects) == 0 {
		t.Fatal("expected projects")
	}
	if projects[0]["code"] == nil {
		t.Fatalf("missing code: %v", projects[0])
	}
}

func TestListLocationsRequiresProject(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)

	if _, err := invoke(t, tl, "list_locations", map[string]any{}); err == nil {
		t.Fatal("expected error for missing project_id")
	}

	projectID, _ := toolsFixture(t, pool, "MCP-LISTL")
	res, err := invoke(t, tl, "list_locations", map[string]any{"project_id": projectID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	locs := res.([]map[string]any)
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if locs[0]["name"] != "MW-01" {
		t.Fatalf("unexpected location: %v", locs[0])
	}
}

func TestListAnalytesAndActionLevelsTools(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-LISTA")

	res, err := invoke(t, tl, "list_analytes", map[string]any{"limit": 50})
	if err != nil {
		t.Fatalf("analytes: %v", err)
	}
	if len(res.([]store.Analyte)) == 0 {
		t.Fatal("expected analytes")
	}

	res, err = invoke(t, tl, "list_action_levels", map[string]any{
		"type": "EPA", "project_id": projectID,
	})
	if err != nil {
		t.Fatalf("action levels: %v", err)
	}
	groups := res.([]store.ActionLevelGroup)
	if len(groups) == 0 {
		t.Fatal("expected action level groups")
	}
}

func TestQueryResultsTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-QUERY")

	if _, err := invoke(t, tl, "query_results", map[string]any{}); err == nil {
		t.Fatal("expected error for missing project_id")
	}

	res, err := invoke(t, tl, "query_results", map[string]any{
		"project_id": projectID, "type": "EPA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := res.([]map[string]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 results, got %d", len(rows))
	}

	// Filter by analyte.
	res, err = invoke(t, tl, "query_results", map[string]any{
		"project_id": projectID, "analyte": "Lead", "type": "EPA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.([]map[string]any)) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(res.([]map[string]any)))
	}

	// Filter by date range that excludes everything.
	res, err = invoke(t, tl, "query_results", map[string]any{
		"project_id": projectID, "start_date": "2030-01-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.([]map[string]any)) != 0 {
		t.Fatalf("expected no rows, got %d", len(res.([]map[string]any)))
	}

	// Filter by CAS RN.
	res, err = invoke(t, tl, "query_results", map[string]any{
		"project_id": projectID, "cas_rn": "MCP-MCP-QUERY",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.([]map[string]any)) != 1 {
		t.Fatalf("expected 1 CAS-matched row, got %d", len(res.([]map[string]any)))
	}
}

func TestExceedancesTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-EXC")

	res, err := invoke(t, tl, "get_exceedances", map[string]any{
		"project_id": projectID, "type": "EPA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := res.(map[string]any)
	count := m["count"].(int)
	if count != 1 {
		t.Fatalf("expected 1 exceedance, got %d (%v)", count, m)
	}
	exc := m["exceedances"].([]map[string]any)
	if exc[0]["analyte"] != "Lead" {
		t.Fatalf("unexpected exceedance: %v", exc[0])
	}
	if exc[0]["exceeded"] != true {
		t.Fatalf("expected exceeded=true: %v", exc[0])
	}
}

func TestGenerateCrosstabTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-XTAB")

	res, err := invoke(t, tl, "generate_crosstab", map[string]any{
		"project_id": projectID, "type": "EPA", "title": "MCP Crosstab",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := res.(map[string]any)
	if m["encoding"] != "base64" {
		t.Fatalf("unexpected encoding: %v", m["encoding"])
	}
	data, err := base64.StdEncoding.DecodeString(m["data"].(string))
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(data) < 2 || data[0] != 'P' || data[1] != 'K' {
		t.Fatalf("expected xlsx zip magic, got %v", data[:2])
	}
	if m["filename"] == "" {
		t.Fatal("expected filename")
	}
}

func TestExportDataTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-EXP")

	for _, format := range []string{"CSV", "WQX", "AQS", "EIM"} {
		res, err := invoke(t, tl, "export_data", map[string]any{
			"project_id": projectID, "format": format, "type": "EPA",
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", format, err)
		}
		m := res.(map[string]any)
		data, err := base64.StdEncoding.DecodeString(m["data"].(string))
		if err != nil {
			t.Fatalf("%s: decode: %v", format, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s: empty export", format)
		}
	}

	// Unknown format.
	if _, err := invoke(t, tl, "export_data", map[string]any{
		"project_id": projectID, "format": "NOPE",
	}); err == nil {
		t.Fatal("expected error for unknown format")
	}

	// Missing format.
	if _, err := invoke(t, tl, "export_data", map[string]any{
		"project_id": projectID,
	}); err == nil {
		t.Fatal("expected error for missing format")
	}
}

func TestListFormatsTool(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)

	res, err := invoke(t, tl, "list_export_formats", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := res.(map[string]any)
	if len(m["exports"].([]string)) < 5 {
		t.Fatalf("expected export formats, got %v", m["exports"])
	}
	if len(m["edd"].([]string)) == 0 {
		t.Fatal("expected EDD formats")
	}
}

func TestEmptyDatasetErrors(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-EMPTY")

	if _, err := invoke(t, tl, "generate_crosstab", map[string]any{
		"project_id": projectID, "start_date": "2030-01-01",
	}); err == nil {
		t.Fatal("expected error for empty dataset")
	}
	if _, err := invoke(t, tl, "export_data", map[string]any{
		"project_id": projectID, "format": "CSV", "start_date": "2030-01-01",
	}); err == nil {
		t.Fatal("expected error for empty dataset")
	}
}

func TestToolsCallThroughServer(t *testing.T) {
	pool := testsupport.Pool(t)
	srv, _ := newTools(t, pool)
	toolsFixture(t, pool, "MCP-SRV")

	resp := srv.Handle(context.Background(), &Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"list_export_formats","arguments":{}}`),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] != false {
		t.Fatalf("expected success: %v", res)
	}
}

func TestDatasetAndRowMapHelpers(t *testing.T) {
	pool := testsupport.Pool(t)
	_, tl := newTools(t, pool)
	projectID, _ := toolsFixture(t, pool, "MCP-HELP")

	rows, err := tl.fetchRows(context.Background(), resultFilter{
		ProjectID: projectID, Type: "EPA",
	})
	if err != nil {
		t.Fatalf("fetchRows: %v", err)
	}
	ds := tl.dataset(rows, "Title")
	if ds.ProjectCode == "" {
		t.Fatal("expected project code")
	}
	if len(ds.ActionLevelGroups) == 0 {
		t.Fatal("expected action level groups in dataset")
	}

	m := rowMap(rows[0])
	for _, key := range []string{"location", "sample", "analyte", "result", "exceeded"} {
		if _, ok := m[key]; !ok {
			t.Errorf("rowMap missing %q", key)
		}
	}

	// Decode args tolerates empty input.
	var args struct{ X string }
	if err := decodeArgs(nil, &args); err != nil {
		t.Fatalf("decodeArgs(nil): %v", err)
	}
	if err := decodeArgs(json.RawMessage(`{"X":"y"}`), &args); err != nil || args.X != "y" {
		t.Fatalf("decodeArgs: %v %v", err, args.X)
	}
}
