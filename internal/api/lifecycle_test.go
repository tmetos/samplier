package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/samplier/samplier/internal/testsupport"
)

// TestFullLifecycle exercises every CRUD route.
func TestFullLifecycle(t *testing.T) {
	ts, pool := testAPI(t)
	ctx := context.Background()
	const code = "API-LIFE"
	testsupport.Cleanup(t, pool, code)

	// Project
	status, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{
		"code": code, "name": "Lifecycle",
	})
	if status != http.StatusCreated {
		t.Fatalf("project: %d %v", status, p)
	}
	pid := p["id"].(string)

	// Update project
	status, upd := call(t, http.MethodPut, ts.URL+"/projects/"+pid, map[string]any{
		"code": code, "name": "Lifecycle Updated", "srid": 4326, "elev_unit": "ft",
	})
	if status != http.StatusOK || upd["name"] != "Lifecycle Updated" {
		t.Fatalf("project update: %d %v", status, upd)
	}

	// Task
	status, task := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{
		"project_id": pid, "code": "T-1", "name": "Task",
	})
	if status != http.StatusCreated {
		t.Fatalf("task: %d %v", status, task)
	}
	tid := task["id"].(string)

	status, tasks := call(t, http.MethodGet, ts.URL+"/tasks?project_id="+pid, nil)
	if status != http.StatusOK || len(tasks["items"].([]any)) != 1 {
		t.Fatalf("list tasks: %d %v", status, tasks)
	}

	// Location
	status, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": pid, "name": "MW-1", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})
	if status != http.StatusCreated {
		t.Fatalf("location: %d %v", status, loc)
	}
	lid := loc["id"].(string)

	status, locs := call(t, http.MethodGet, ts.URL+"/locations?project_id="+pid, nil)
	if status != http.StatusOK || len(locs["items"].([]any)) != 1 {
		t.Fatalf("list locations: %d %v", status, locs)
	}

	// Analyte
	status, an := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-LIFE-" + t.Name(), "chemical_name": "Lifecycleium",
		"aliases": []string{"lc"},
	})
	if status != http.StatusCreated {
		t.Fatalf("analyte: %d %v", status, an)
	}
	aid := an["id"].(string)

	// Sample
	status, smp := call(t, http.MethodPost, ts.URL+"/samples", map[string]any{
		"task_id": tid, "location_id": lid, "name": "S-1", "matrix": "WATER",
		"sample_datetime": "2024-03-01T10:00:00Z", "start_depth": 5, "end_depth": 5,
		"depth_unit": "ft",
	})
	if status != http.StatusCreated {
		t.Fatalf("sample: %d %v", status, smp)
	}
	sid := smp["id"].(string)

	status, gotSample := call(t, http.MethodGet, ts.URL+"/samples/"+sid, nil)
	if status != http.StatusOK || gotSample["name"] != "S-1" {
		t.Fatalf("get sample: %d %v", status, gotSample)
	}

	status, samples := call(t, http.MethodGet, ts.URL+"/samples?task_id="+tid, nil)
	if status != http.StatusOK || len(samples["items"].([]any)) != 1 {
		t.Fatalf("list samples: %d %v", status, samples)
	}
	// Also by location.
	status, _ = call(t, http.MethodGet, ts.URL+"/samples?location_id="+lid, nil)
	if status != http.StatusOK {
		t.Fatalf("list samples by location: %d", status)
	}

	// Test
	status, tst := call(t, http.MethodPost, ts.URL+"/tests", map[string]any{
		"sample_id": sid, "test_id": "TST-1", "analytic_method": "EPA 6020B",
		"fraction": "TOTAL", "analysis_date": "2024-03-02",
	})
	if status != http.StatusCreated {
		t.Fatalf("test: %d %v", status, tst)
	}
	tstID := tst["id"].(string)

	status, tests := call(t, http.MethodGet, ts.URL+"/samples/"+sid+"/tests", nil)
	if status != http.StatusOK || len(tests["items"].([]any)) != 1 {
		t.Fatalf("list tests: %d %v", status, tests)
	}
	status, gotTest := call(t, http.MethodGet, ts.URL+"/tests/"+tstID, nil)
	if status != http.StatusOK || gotTest["test_id"] != "TST-1" {
		t.Fatalf("get test: %d %v", status, gotTest)
	}

	// Result
	status, res := call(t, http.MethodPost, ts.URL+"/results", map[string]any{
		"test_id": tstID, "analyte_id": aid, "result_numeric": 0.05,
		"detect_flag": "Y", "units": "mg/L", "reporting_detection_limit": 0.001,
	})
	if status != http.StatusCreated {
		t.Fatalf("result: %d %v", status, res)
	}
	resID := res["id"].(string)

	status, results := call(t, http.MethodGet, ts.URL+"/tests/"+tstID+"/results", nil)
	if status != http.StatusOK || len(results["items"].([]any)) != 1 {
		t.Fatalf("list results: %d %v", status, results)
	}
	status, gotResult := call(t, http.MethodGet, ts.URL+"/results/"+resID, nil)
	if status != http.StatusOK || gotResult["analyte_name"] != "Lifecycleium" {
		t.Fatalf("get result: %d %v", status, gotResult)
	}

	// Delete everything bottom-up.
	for _, tc := range []struct{ path, id string }{
		{"/results/", resID},
		{"/tests/", tstID},
		{"/samples/", sid},
		{"/locations/", lid},
		{"/tasks/", tid},
		{"/projects/", pid},
	} {
		status, _ := call(t, http.MethodDelete, ts.URL+tc.path+tc.id, nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete %s%s: %d", tc.path, tc.id, status)
		}
	}

	// Verify cascaded.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sample WHERE id=$1`, sid).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected sample deleted, got %d", n)
	}

	// Re-delete returns 404.
	status, _ = call(t, http.MethodDelete, ts.URL+"/tasks/"+tid, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 on re-delete, got %d", status)
	}
}

func TestGetMissingResources(t *testing.T) {
	ts, _ := testAPI(t)
	missing := "00000000-0000-0000-0000-000000000000"

	for _, path := range []string{
		"/projects/", "/tasks/", "/locations/", "/samples/", "/tests/", "/results/", "/analytes/",
	} {
		status, _ := call(t, http.MethodGet, ts.URL+path+missing, nil)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: expected 404, got %d", path, status)
		}
	}

	for _, path := range []string{
		"/projects/", "/tasks/", "/locations/", "/samples/", "/tests/", "/results/",
		"/well-segments/",
	} {
		status, _ := call(t, http.MethodDelete, ts.URL+path+missing, nil)
		if status != http.StatusNotFound {
			t.Errorf("DELETE %s: expected 404, got %d", path, status)
		}
	}
}

func TestStatsAndActivityEndpoints(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-DASH"
	testsupport.Cleanup(t, pool, code)

	_, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{
		"code": code, "name": "Dash Test",
	})
	pid := p["id"].(string)
	_, task := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{
		"project_id": pid, "code": "T-DASH",
	})
	_, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": pid, "name": "MW-DASH", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})
	if _, err := call(t, http.MethodPost, ts.URL+"/samples", map[string]any{
		"task_id": task["id"], "location_id": loc["id"], "name": "S-DASH",
		"matrix": "WATER", "sample_datetime": "2024-05-01T09:00:00Z",
	}); err == nil {
		// status checked below via list
	}

	status, stats := call(t, http.MethodGet, ts.URL+"/stats", nil)
	if status != http.StatusOK {
		t.Fatalf("stats status %d", status)
	}
	if stats["projects"].(float64) < 1 {
		t.Fatalf("unexpected stats: %v", stats)
	}
	detail, ok := stats["project_detail"].([]any)
	if !ok || len(detail) == 0 {
		t.Fatalf("expected project_detail, got %v", stats["project_detail"])
	}
	found := false
	for _, d := range detail {
		m := d.(map[string]any)
		if m["code"] == code {
			found = true
			if m["locations"].(float64) != 1 {
				t.Errorf("expected 1 location, got %v", m["locations"])
			}
			if m["samples"].(float64) != 1 {
				t.Errorf("expected 1 sample, got %v", m["samples"])
			}
		}
	}
	if !found {
		t.Fatalf("project %s not in rollup", code)
	}

	status, act := call(t, http.MethodGet, ts.URL+"/activity?limit=10", nil)
	if status != http.StatusOK {
		t.Fatalf("activity status %d", status)
	}
	items, ok := act["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected activity items, got %v", act)
	}
	first := items[0].(map[string]any)
	for _, k := range []string{"kind", "id", "title", "at"} {
		if _, ok := first[k]; !ok {
			t.Errorf("activity entry missing %q: %v", k, first)
		}
	}
}

func TestSamplesFilterByProject(t *testing.T) {
	ts, pool := testAPI(t)
	const code = "API-SP"
	testsupport.Cleanup(t, pool, code)

	_, p := call(t, http.MethodPost, ts.URL+"/projects", map[string]any{
		"code": code, "name": "SP",
	})
	pid := p["id"].(string)
	_, task := call(t, http.MethodPost, ts.URL+"/tasks", map[string]any{
		"project_id": pid, "code": "T-SP",
	})
	_, loc := call(t, http.MethodPost, ts.URL+"/locations", map[string]any{
		"project_id": pid, "name": "MW-SP", "loc_type": "WELL", "lon": -122.4, "lat": 47.6,
	})
	status, _ := call(t, http.MethodPost, ts.URL+"/samples", map[string]any{
		"task_id": task["id"], "location_id": loc["id"], "name": "S-SP",
		"matrix": "WATER", "sample_datetime": "2024-05-02T09:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("sample create status %d", status)
	}

	status, list := call(t, http.MethodGet, ts.URL+"/samples?project_id="+pid, nil)
	if status != http.StatusOK {
		t.Fatalf("list status %d", status)
	}
	if len(list["items"].([]any)) != 1 {
		t.Fatalf("expected 1 sample, got %v", list["items"])
	}

	// An unknown project yields nothing rather than everything.
	status, empty := call(t, http.MethodGet,
		ts.URL+"/samples?project_id=00000000-0000-0000-0000-000000000000", nil)
	if status != http.StatusOK || len(empty["items"].([]any)) != 0 {
		t.Fatalf("expected empty result: %d %v", status, empty["items"])
	}
}
