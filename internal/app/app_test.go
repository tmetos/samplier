package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/testsupport"
)

// testServer spins up the full HTTP handler against the test database.
func testServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	srv := New(pool, Config{AuthEnabled: false, Version: "test"}, nil)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	// Keep a shared development database clean for the next test (and for any
	// human looking at the demo data afterwards).
	t.Cleanup(func() {
		cleanupTestData(t, pool, "APP-%")
		// EDD imports are not project-scoped, so remove the audit rows too.
		_, _ = pool.Exec(context.Background(), `DELETE FROM edd_batch`)
	})
	return ts, pool
}

// cleanupTestData removes projects (cascading to samples/tests/results) whose
// code matches a SQL LIKE pattern, plus the analytes the tests create.
func cleanupTestData(t *testing.T, pool *pgxpool.Pool, pattern string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM project WHERE code LIKE $1`, pattern); err != nil {
		t.Logf("cleanup projects %s: %v", pattern, err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM analyte WHERE cas_rn LIKE $1 AND NOT EXISTS (
		   SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`, pattern); err != nil {
		t.Logf("cleanup analytes %s: %v", pattern, err)
	}
}

// fixture creates a project/task/location and returns their ids.
type fixtureIDs struct {
	ProjectID  string
	TaskID     string
	LocCode    string
	AnalyteID  string
	TestUUID   string
	SampleUUID string
}

func makeFixture(t *testing.T, pool *pgxpool.Pool, code string) fixtureIDs {
	t.Helper()
	ctx := context.Background()
	testsupport.Cleanup(t, pool, code)

	var f fixtureIDs
	if err := pool.QueryRow(ctx,
		`INSERT INTO project (code, name, srid, elev_unit) VALUES ($1,'App Test',4326,'ft')
		 RETURNING id`, code).Scan(&f.ProjectID); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO task (project_id, code, name) VALUES ($1,'T1','Task 1') RETURNING id`,
		f.ProjectID).Scan(&f.TaskID); err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO location (project_id, name, loc_type, geometry)
		 VALUES ($1,'MW-01','WELL', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))
		 RETURNING sys_loc_code::text`, f.ProjectID).Scan(&f.LocCode); err != nil {
		t.Fatalf("location: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO sample (task_id, location_id, name, matrix, sample_datetime)
		 SELECT $1, l.id, 'S-001', 'WATER', '2024-03-01'
		 FROM location l WHERE l.project_id=$2 AND l.name='MW-01'
		 RETURNING id`, f.TaskID, f.ProjectID).Scan(&f.SampleUUID); err != nil {
		t.Fatalf("sample: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO test (sample_id, test_id, analytic_method, fraction)
		 VALUES ($1,'TST-1','EPA 6020B','TOTAL') RETURNING id`,
		f.SampleUUID).Scan(&f.TestUUID); err != nil {
		t.Fatalf("test: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO analyte (cas_rn, chemical_name) VALUES ('7439-92-1','Lead')
		 ON CONFLICT (cas_rn) DO UPDATE SET chemical_name=EXCLUDED.chemical_name
		 RETURNING id`).Scan(&f.AnalyteID); err != nil {
		t.Fatalf("analyte: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units)
		 VALUES ($1,$2,0.0503,'Y','mg/L')`, f.TestUUID, f.AnalyteID); err != nil {
		t.Fatalf("result: %v", err)
	}
	return f
}

func doJSON(t *testing.T, method, url string, body any) (int, map[string]any, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
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
	return resp.StatusCode, out, raw
}

func TestHealthz(t *testing.T) {
	ts, _ := testServer(t)
	status, body, _ := doJSON(t, http.MethodGet, ts.URL+"/healthz", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["status"] != "ok" {
		t.Fatalf("body %v", body)
	}
}

func TestProjectCRUD(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-CRUD"
	t.Cleanup(func() { testsupport.Cleanup(t, pool, code) })

	// Create.
	status, created, _ := doJSON(t, http.MethodPost, ts.URL+"/api/v1/projects", map[string]any{
		"code": code, "name": "CRUD Project",
	})
	if status != http.StatusCreated {
		t.Fatalf("create status %d: %v", status, created)
	}
	id := created["id"].(string)
	if created["srid"].(float64) != 4326 {
		t.Fatalf("default srid not applied: %v", created["srid"])
	}

	// Read.
	status, got, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/projects/"+id, nil)
	if status != http.StatusOK || got["code"] != code {
		t.Fatalf("get: %d %v", status, got)
	}

	// List.
	status, list, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/projects?limit=10", nil)
	if status != http.StatusOK {
		t.Fatalf("list status %d", status)
	}
	if _, ok := list["items"]; !ok {
		t.Fatalf("expected items: %v", list)
	}

	// Update.
	status, updated, _ := doJSON(t, http.MethodPut, ts.URL+"/api/v1/projects/"+id, map[string]any{
		"code": code, "name": "Renamed", "srid": 4326, "elev_unit": "ft",
	})
	if status != http.StatusOK || updated["name"] != "Renamed" {
		t.Fatalf("update: %d %v", status, updated)
	}

	// Delete.
	status, _, _ = doJSON(t, http.MethodDelete, ts.URL+"/api/v1/projects/"+id, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete status %d", status)
	}

	// Gone.
	status, _, _ = doJSON(t, http.MethodGet, ts.URL+"/api/v1/projects/"+id, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestProjectValidation(t *testing.T) {
	ts, _ := testServer(t)
	status, body, _ := doJSON(t, http.MethodPost, ts.URL+"/api/v1/projects", map[string]any{
		"code": "", "name": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", status, body)
	}

	// Invalid JSON.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects",
		strings.NewReader("{bad"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLocationAndSampleFlow(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-FLOW"
	f := makeFixture(t, pool, code)

	status, locs, _ := doJSON(t, http.MethodGet,
		ts.URL+"/api/v1/locations?project_id="+f.ProjectID, nil)
	if status != http.StatusOK {
		t.Fatalf("locations status %d", status)
	}
	items := locs["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d locations", len(items))
	}
	loc := items[0].(map[string]any)
	if loc["name"] != "MW-01" {
		t.Fatalf("unexpected location: %v", loc)
	}
	// Coordinates must be populated.
	if loc["lon"].(float64) == 0 || loc["lat"].(float64) == 0 {
		t.Fatalf("expected coordinates: %v", loc)
	}

	status, samples, _ := doJSON(t, http.MethodGet,
		ts.URL+"/api/v1/samples?task_id="+f.TaskID, nil)
	if status != http.StatusOK {
		t.Fatalf("samples status %d", status)
	}
	if len(samples["items"].([]any)) != 1 {
		t.Fatalf("expected 1 sample: %v", samples)
	}

	status, tests, _ := doJSON(t, http.MethodGet,
		ts.URL+"/api/v1/samples/"+f.SampleUUID+"/tests", nil)
	if status != http.StatusOK {
		t.Fatalf("tests status %d", status)
	}
	if len(tests["items"].([]any)) != 1 {
		t.Fatalf("expected 1 test: %v", tests)
	}

	status, results, _ := doJSON(t, http.MethodGet,
		ts.URL+"/api/v1/tests/"+f.TestUUID+"/results", nil)
	if status != http.StatusOK {
		t.Fatalf("results status %d", status)
	}
	ritems := results["items"].([]any)
	if len(ritems) != 1 {
		t.Fatalf("expected 1 result: %v", results)
	}
	res := ritems[0].(map[string]any)
	if res["analyte_name"] != "Lead" {
		t.Fatalf("analyte join missing: %v", res)
	}
}

func TestResultNonDetectNullsNumeric(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-ND"
	f := makeFixture(t, pool, code)

	status, created, _ := doJSON(t, http.MethodPost, ts.URL+"/api/v1/results", map[string]any{
		"test_id": f.TestUUID, "analyte_id": f.AnalyteID,
		"result_numeric": 5, "detect_flag": "N", "units": "mg/L",
		"reporting_detection_limit": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("create status %d: %v", status, created)
	}
	if created["result_numeric"] != nil {
		t.Fatalf("non-detect must null the numeric value: %v", created["result_numeric"])
	}
}

func TestWellSegmentEndpoints(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-WELL"
	f := makeFixture(t, pool, code)

	var locID string
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM location WHERE project_id=$1`, f.ProjectID).Scan(&locID); err != nil {
		t.Fatalf("get location: %v", err)
	}

	status, created, _ := doJSON(t, http.MethodPost,
		fmt.Sprintf("%s/api/v1/locations/%s/well-segments", ts.URL, locID),
		map[string]any{
			"segment_type": "SCREEN", "start_depth": 5, "end_depth": 15, "depth_unit": "ft",
		})
	if status != http.StatusCreated {
		t.Fatalf("create segment %d: %v", status, created)
	}

	status, list, _ := doJSON(t, http.MethodGet,
		fmt.Sprintf("%s/api/v1/locations/%s/well-segments", ts.URL, locID), nil)
	if status != http.StatusOK {
		t.Fatalf("list segments %d", status)
	}
	if len(list["items"].([]any)) != 1 {
		t.Fatalf("expected 1 segment: %v", list)
	}
}

func TestUnitsAndConversions(t *testing.T) {
	ts, _ := testServer(t)

	status, body, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/units", nil)
	if status != http.StatusOK {
		t.Fatalf("units status %d", status)
	}
	if len(body["items"].([]any)) == 0 {
		t.Fatal("expected seeded units")
	}

	status, conv, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/unit-conversions", nil)
	if status != http.StatusOK {
		t.Fatalf("conversions status %d", status)
	}
	if len(conv["items"].([]any)) == 0 {
		t.Fatal("expected seeded conversions")
	}
}

func TestExceedanceEndpoint(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-EXC"
	f := makeFixture(t, pool, code)

	// Create a limit of 0.015 mg/L; the result is 0.0503 mg/L -> exceedance.
	const lvlCode = "APP-EXC-LVL"
	// Clean any previous run's group (code is globally unique), and again
	// afterwards so a shared development database stays demo-clean.
	del := func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM action_level WHERE code = $1`, lvlCode)
	}
	del()
	t.Cleanup(del)

	var groupID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO action_level (type_id, code, name, matrix)
		 SELECT t.id, $1, 'App Test Limit', 'WATER'
		 FROM action_level_type t WHERE t.code='EPA' RETURNING id`, lvlCode).Scan(&groupID)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level, unit)
		 VALUES ($1,$2,0.015,'mg/L')`, groupID, f.AnalyteID); err != nil {
		t.Fatalf("create parameter: %v", err)
	}

	status, body, raw := doJSON(t, http.MethodGet,
		ts.URL+"/api/v1/exceedances?project_id="+f.ProjectID+"&type=EPA", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, raw)
	}
	if body["count"].(float64) < 1 {
		t.Fatalf("expected at least one exceedance: %s", raw)
	}
}

func TestExportCSVDownload(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-EXP"
	f := makeFixture(t, pool, code)

	resp, err := http.Get(ts.URL + "/api/v1/exports/CSV?project_id=" + f.ProjectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "csv") {
		t.Fatalf("content type %q", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("content disposition %q", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Lead") {
		t.Fatalf("expected Lead in export: %s", body)
	}
}

func TestExportXLSXDownload(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-XLSX"
	f := makeFixture(t, pool, code)

	resp, err := http.Get(ts.URL + "/api/v1/exports/CROSSTAB?project_id=" + f.ProjectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	// XLSX is a ZIP: starts with "PK".
	if len(body) < 2 || body[0] != 'P' || body[1] != 'K' {
		t.Fatalf("expected xlsx zip magic, got %q", body[:2])
	}
}

func TestExportRequiresProject(t *testing.T) {
	ts, _ := testServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/exports/CSV")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestExportUnknownFormat(t *testing.T) {
	ts, _ := testServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/exports/NOPE?project_id=x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// uploadEDD posts a multipart CSV EDD.
func uploadEDD(t *testing.T, ts *httptest.Server, path, filename, content string, fields map[string]string) (int, map[string]any) {
	t.Helper()
	return uploadEDDBytes(t, ts, path, filename, []byte(content), fields)
}

// uploadEDDBytes posts a multipart EDD from raw bytes.
func uploadEDDBytes(t *testing.T, ts *httptest.Server, path, filename string, content []byte, fields map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
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

func TestEDDValidateAndLoad(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-EDD"
	f := makeFixture(t, pool, code)

	zipData := buildEDDZip(t, map[string]string{
		"EFW2FSample.csv": fmt.Sprintf(
			"SYS_LOC_CODE,SAMPLE_ID,MATRIX_CODE,SAMPLE_DATE,SAMPLE_TYPE_CODE\n%s,EDD-S-1,WATER,2024-04-01,N\n", f.LocCode),
		"EFW2LabTST.csv": "SAMPLE_ID,TEST_ID,ANALYTIC_METHOD,FRACTION\n" +
			"EDD-S-1,EDD-T-1,EPA 6020B,TOTAL\n",
		"EFW2LabRES.csv": "SAMPLE_ID,TEST_ID,CAS_RN,CHEMICAL_NAME,RESULT_VALUE,RESULT_UNIT,DETECT_FLAG\n" +
			"EDD-S-1,EDD-T-1,7439-92-1,Lead,0.004,mg/L,Y\n",
	})
	fields := map[string]string{
		"project_id": f.ProjectID, "task_id": f.TaskID,
		"commit_mode": "INSERT_UPDATE", "file_name": "package.zip",
	}

	// Validate only.
	status, body := uploadEDDBytes(t, ts, "/api/v1/edd/validate?format=EFWEDD",
		"package.zip", zipData, nil)
	if status != http.StatusOK {
		t.Fatalf("validate status %d: %v", status, body)
	}
	if body["valid"] != true {
		t.Fatalf("expected valid: %v", body)
	}

	// Load.
	status, body = uploadEDDBytes(t, ts, "/api/v1/edd/load?format=EFWEDD",
		"package.zip", zipData, fields)
	if status != http.StatusCreated {
		t.Fatalf("load status %d: %v", status, body)
	}
	loaded := body["loaded"].(map[string]any)
	if loaded["samples"].(float64) != 1 {
		t.Fatalf("expected 1 sample loaded: %v", loaded)
	}
	if loaded["results"].(float64) != 1 {
		t.Fatalf("expected 1 result loaded: %v", loaded)
	}
}

// buildEDDZip creates a ZIP archive from name->content pairs.
func buildEDDZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestEDDLoadRejectsInvalid(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-EDD-BAD"
	f := makeFixture(t, pool, code)

	// Missing required MATRIX_CODE column.
	csv := "SYS_LOC_CODE,SAMPLE_ID,SAMPLE_DATE\n" + f.LocCode + ",S-1,2024-04-01\n"
	status, body := uploadEDD(t, ts, "/api/v1/edd/load?format=EFWEDD",
		"EFW2FSample.csv", csv, map[string]string{
			"project_id": f.ProjectID, "task_id": f.TaskID,
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %v", status, body)
	}
}

func TestEDDUnknownFormat(t *testing.T) {
	ts, _ := testServer(t)
	status, _ := uploadEDD(t, ts, "/api/v1/edd/validate?format=NOPE",
		"x.csv", "a,b\n1,2\n", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
}

func TestEDDFormatsAndBatches(t *testing.T) {
	ts, _ := testServer(t)
	status, body, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/edd/formats", nil)
	if status != http.StatusOK {
		t.Fatalf("formats status %d", status)
	}
	if len(body["items"].([]any)) == 0 {
		t.Fatal("expected formats")
	}

	status, body, _ = doJSON(t, http.MethodGet, ts.URL+"/api/v1/edd/batches", nil)
	if status != http.StatusOK {
		t.Fatalf("batches status %d", status)
	}
	if _, ok := body["items"]; !ok {
		t.Fatalf("expected items: %v", body)
	}
}

func TestExportFormatsList(t *testing.T) {
	ts, _ := testServer(t)
	status, body, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/exports/formats", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	items := body["items"].([]any)
	if len(items) < 5 {
		t.Fatalf("expected 5 formats, got %v", items)
	}
}

func TestAuthDisabledAllowsAnonymous(t *testing.T) {
	ts, _ := testServer(t)
	status, _, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 with auth disabled, got %d", status)
	}
}

func TestAuthEnabledRejects(t *testing.T) {
	pool := testsupport.Pool(t)
	srv := New(pool, Config{
		AuthEnabled:  true,
		OIDCIssuer:   "https://issuer.invalid/realms/test",
		OIDCClientID: "samplier",
	}, nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// OGC stays public.
	resp2, err := http.Get(ts.URL + "/ogc/collections")
	if err != nil {
		t.Fatalf("ogc get: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for OGC, got %d", resp2.StatusCode)
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitize("P-001/a b"); got != "P-001_a_b" {
		t.Fatalf("got %q", got)
	}
	if got := sanitize(""); got != "samplier" {
		t.Fatalf("got %q", got)
	}
}

func TestParseResultFilters(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?project_id=abc&location=MW-1", nil)
	f, err := parseResultFilters(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ProjectID != "abc" || f.Location != "MW-1" {
		t.Fatalf("unexpected filters: %+v", f)
	}

	q, _ := url.Parse("/x")
	r = httptest.NewRequest(http.MethodGet, q.String(), nil)
	if _, err := parseResultFilters(r); err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestBasemapEndpointsArePublicAndSelfHosted guards the routing of the
// self-hosted MapLibre basemap through the full server stack, and asserts the
// style has no external dependencies (no API keys).
func TestBasemapEndpointsArePublicAndSelfHosted(t *testing.T) {
	ts, _ := testServer(t)

	for _, path := range []string{"/ogc/basemap/style.json", "/ogc/basemap/land", "/ogc/basemap/boundaries"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		if len(body) == 0 {
			t.Fatalf("%s: empty body", path)
		}
		if strings.Contains(string(body), "http://") || strings.Contains(string(body), "https://") {
			t.Errorf("%s: must not reference external URLs", path)
		}
	}

	// The style must reference this server's own basemap and collections.
	resp, err := http.Get(ts.URL + "/ogc/basemap/style.json")
	if err != nil {
		t.Fatalf("style: %v", err)
	}
	defer resp.Body.Close()
	var style map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&style); err != nil {
		t.Fatalf("parse style: %v", err)
	}
	sources := style["sources"].(map[string]any)
	if sources["ne_land"].(map[string]any)["data"] != "/ogc/basemap/land" {
		t.Errorf("land source = %v", sources["ne_land"])
	}
	if _, ok := sources["samplier-locations"]; !ok {
		t.Error("missing locations source")
	}
}

// TestBasemapWorksWithAuthEnabled confirms the map loads for unauthenticated
// users even when OIDC is enforced for the REST API.
func TestBasemapWorksWithAuthEnabled(t *testing.T) {
	pool := testsupport.Pool(t)
	srv := New(pool, Config{
		AuthEnabled:  true,
		OIDCIssuer:   "https://issuer.invalid/realms/test",
		OIDCClientID: "samplier",
	}, nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ogc/basemap/style.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("basemap should be public, got %d", resp.StatusCode)
	}
}
