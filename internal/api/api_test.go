package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/store"
	"github.com/samplier/samplier/internal/testsupport"
)

func testAPI(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	h := New(store.New(pool), nil)
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)
	// Remove test projects and their data once the test finishes.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM project WHERE code LIKE $1`, "API-%")
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM action_level WHERE code LIKE $1`, "API-%")
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn LIKE $1 AND NOT EXISTS (
			   SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`, "API-%")
	})
	// Analytes have a globally unique CAS RN; clear leftovers from prior runs.
	for _, prefix := range []string{"API-%"} {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn LIKE $1`, prefix)
	}
	return ts, pool
}

func call(t *testing.T, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestProjectEndpoints(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-PROJ"
	testsupport.Cleanup(t, pool, code)

	status, created := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{
		"code": code, "name": "API Test",
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, created)
	}
	id := created["id"].(string)

	status, got := call(t, http.MethodGet, ts.URL+"/projects/"+id, nil)
	if status != http.StatusOK || got["code"] != code {
		t.Fatalf("get: %d %v", status, got)
	}

	status, list := call(t, http.MethodGet, ts.URL+"/projects", nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	if _, ok := list["items"]; !ok {
		t.Fatalf("expected items: %v", list)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/projects/"+id, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}

	status, _ = call(t, http.MethodGet, ts.URL+"/projects/"+id, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404: %d", status)
	}
}

func TestTaskAndLocationValidation(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-VAL"
	testsupport.Cleanup(t, pool, code)

	// Missing fields.
	status, _ := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{"project_id": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for task: %d", status)
	}
	status, _ = call(t, http.MethodPost, ts.URL+"/locations", map[string]any{"project_id": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for location: %d", status)
	}

	// Create real ones.
	_, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{"code": code, "name": "V"})
	pid := p["id"].(string)

	status, task := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{
		"project_id": pid, "code": "T1",
	})
	if status != http.StatusCreated {
		t.Fatalf("task create: %d %v", status, task)
	}

	status, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": pid, "name": "MW-1", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})
	if status != http.StatusCreated {
		t.Fatalf("location create: %d %v", status, loc)
	}
	if loc["status"] != "active" {
		t.Fatalf("default status not applied: %v", loc["status"])
	}

	// Get task and location.
	status, _ = call(t, http.MethodGet, ts.URL+"/tasks/"+task["id"].(string), nil)
	if status != http.StatusOK {
		t.Fatalf("get task: %d", status)
	}
	status, _ = call(t, http.MethodGet, ts.URL+"/locations/"+loc["id"].(string), nil)
	if status != http.StatusOK {
		t.Fatalf("get location: %d", status)
	}
}

func TestSampleRequiresDatetime(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-SMP"
	testsupport.Cleanup(t, pool, code)

	_, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{"code": code, "name": "S"})
	pid := p["id"].(string)
	_, task := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{"project_id": pid, "code": "T1"})
	_, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": pid, "name": "MW-1", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})

	status, body := call(t, http.MethodPost, ts.URL+"/samples", map[string]any{
		"task_id": task["id"], "location_id": loc["id"],
		"name": "S-1", "matrix": "WATER",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing datetime: %d %v", status, body)
	}

	status, created := call(t, http.MethodPost, ts.URL+"/samples", map[string]any{
		"task_id": task["id"], "location_id": loc["id"],
		"name": "S-1", "matrix": "WATER", "sample_datetime": "2024-03-01T10:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("sample create: %d %v", status, created)
	}
}

func TestTestAndResultRequireFields(t *testing.T) {
	ts, _ := testAPI(t)

	status, _ := call(t, http.MethodPost, ts.URL+"/tests", map[string]any{"sample_id": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for test: %d", status)
	}
	status, _ = call(t, http.MethodPost, ts.URL+"/results", map[string]any{"test_id": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for result: %d", status)
	}
	status, _ = call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for analyte: %d", status)
	}
}

func TestAnalyteAndUnitsEndpoints(t *testing.T) {
	ts, _ := testAPI(t)

	status, created := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-" + t.Name(), "chemical_name": "Testium",
	})
	if status != http.StatusCreated {
		t.Fatalf("create analyte: %d %v", status, created)
	}

	status, _ = call(t, http.MethodGet, ts.URL+"/analytes/"+created["id"].(string), nil)
	if status != http.StatusOK {
		t.Fatalf("get analyte: %d", status)
	}

	status, list := call(t, http.MethodGet, ts.URL+"/analytes", nil)
	if status != http.StatusOK || len(list["items"].([]any)) == 0 {
		t.Fatalf("list analytes: %d", status)
	}

	status, units := call(t, http.MethodGet, ts.URL+"/units", nil)
	if status != http.StatusOK || len(units["items"].([]any)) == 0 {
		t.Fatalf("units: %d", status)
	}

	status, conv := call(t, http.MethodGet, ts.URL+"/unit-conversions", nil)
	if status != http.StatusOK || len(conv["items"].([]any)) == 0 {
		t.Fatalf("conversions: %d", status)
	}
}

func TestActionLevelEndpoints(t *testing.T) {
	ts, pool := testAPI(t)
	ctx := context.Background()

	status, an := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-AL-" + t.Name(), "chemical_name": "Actionium",
	})
	if status != http.StatusCreated {
		t.Fatalf("analyte: %d %v", status, an)
	}

	code := "API-AL-" + t.Name()
	_, _ = pool.Exec(ctx, `DELETE FROM action_level WHERE code=$1`, code)

	status, group := call(t, http.MethodPost, ts.URL+"/action-level-groups", map[string]any{
		"type_code": "EPA", "code": code, "name": "API AL",
	})
	if status != http.StatusCreated {
		t.Fatalf("group: %d %v", status, group)
	}
	groupID := group["id"].(string)

	status, param := call(t, http.MethodPost,
		ts.URL+"/action-level-groups/"+groupID+"/parameters", map[string]any{
			"analyte_id": an["id"], "action_level": 10, "unit": "mg/L",
		})
	if status != http.StatusCreated {
		t.Fatalf("param: %d %v", status, param)
	}

	status, list := call(t, http.MethodGet, ts.URL+"/action-levels?type=EPA", nil)
	if status != http.StatusOK || len(list["items"].([]any)) == 0 {
		t.Fatalf("list: %d", status)
	}

	// Validation.
	status, _ = call(t, http.MethodPost, ts.URL+"/action-level-groups", map[string]any{"type_code": "EPA"})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400: %d", status)
	}
	status, _ = call(t, http.MethodPost, ts.URL+"/action-level-groups/"+groupID+"/parameters",
		map[string]any{"analyte_id": an["id"]})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing unit: %d", status)
	}
}

func TestWellSegmentEndpointsAPI(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-WS"
	testsupport.Cleanup(t, pool, code)

	_, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{"code": code, "name": "WS"})
	_, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": p["id"], "name": "MW-1", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})
	locID := loc["id"].(string)

	status, seg := call(t, http.MethodPost, ts.URL+"/locations/"+locID+"/well-segments",
		map[string]any{"segment_type": "SCREEN", "start_depth": 5, "end_depth": 15, "depth_unit": "ft"})
	if status != http.StatusCreated {
		t.Fatalf("create segment: %d %v", status, seg)
	}

	status, list := call(t, http.MethodGet, ts.URL+"/locations/"+locID+"/well-segments", nil)
	if status != http.StatusOK || len(list["items"].([]any)) != 1 {
		t.Fatalf("list segments: %d %v", status, list)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/well-segments/"+seg["id"].(string), nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete segment: %d", status)
	}

	// Missing required field.
	status, _ = call(t, http.MethodPost, ts.URL+"/locations/"+locID+"/well-segments",
		map[string]any{"start_depth": 1})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400: %d", status)
	}
}

func TestPaginationHelper(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?limit=25&offset=50", nil)
	limit, offset := pagination(r)
	if limit != 25 || offset != 50 {
		t.Fatalf("got %d %d", limit, offset)
	}

	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	limit, offset = pagination(r)
	if limit != 100 || offset != 0 {
		t.Fatalf("defaults: %d %d", limit, offset)
	}

	r = httptest.NewRequest(http.MethodGet, "/x?limit=99999&offset=-5", nil)
	limit, offset = pagination(r)
	if limit != 100 || offset != 0 {
		t.Fatalf("clamped: %d %d", limit, offset)
	}
}
