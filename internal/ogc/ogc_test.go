package ogc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/testsupport"
)

func testOGC(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	ts := httptest.NewServer(New(pool).Routes())
	t.Cleanup(ts.Close)
	return ts, pool
}

func getJSON(t *testing.T, url string) (int, map[string]any, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()

	var out map[string]any
	dec := json.NewDecoder(resp.Body)
	_ = dec.Decode(&out)
	return resp.StatusCode, out, resp.Header.Get("Content-Type")
}

// ogcFixture creates a project with task/location/sample/result.
func ogcFixture(t *testing.T, pool *pgxpool.Pool, code string) (projectID, taskID string) {
	t.Helper()
	ctx := context.Background()
	testsupport.Cleanup(t, pool, code)
	t.Cleanup(func() { testsupport.Cleanup(t, pool, code) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO project (code, name, srid, elev_unit, geometry)
		 VALUES ($1,$1,4326,'ft', ST_SetSRID(ST_MakeEnvelope(-123,47,-122,48),4326))
		 RETURNING id`, code).Scan(&projectID); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO task (project_id, code, name, start_date, geometry)
		 VALUES ($1,'T1','Task','2024-03-01', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))
		 RETURNING id`, projectID).Scan(&taskID); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO location (project_id, name, loc_type, geometry)
		 VALUES ($1,'MW-01','WELL', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))`,
		projectID); err != nil {
		t.Fatalf("location: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO sample (task_id, location_id, name, matrix, sample_datetime)
		 SELECT $1, l.id, 'S-001', 'WATER', '2024-03-15'
		 FROM location l WHERE l.project_id=$2 AND l.name='MW-01'`,
		taskID, projectID); err != nil {
		t.Fatalf("sample: %v", err)
	}
	return projectID, taskID
}

func TestLandingPage(t *testing.T) {
	ts, _ := testOGC(t)
	status, body, ct := getJSON(t, ts.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
	links := body["links"].([]any)
	if len(links) < 3 {
		t.Fatalf("expected >=3 links, got %d", len(links))
	}
	// Must expose service-desc, conformance, and data links.
	rels := map[string]bool{}
	for _, l := range links {
		rels[l.(map[string]any)["rel"].(string)] = true
	}
	for _, want := range []string{"service-desc", "conformance", "data"} {
		if !rels[want] {
			t.Errorf("missing link rel %q", want)
		}
	}
}

func TestConformance(t *testing.T) {
	ts, _ := testOGC(t)
	status, body, _ := getJSON(t, ts.URL+"/conformance")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	classes := body["conformsTo"].([]any)
	if len(classes) < 3 {
		t.Fatalf("expected conformance classes, got %v", classes)
	}
	want := map[string]bool{
		"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core":    false,
		"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson": false,
		"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/oas30":   false,
	}
	for _, c := range classes {
		if _, ok := want[c.(string)]; ok {
			want[c.(string)] = true
		}
	}
	for cls, found := range want {
		if !found {
			t.Errorf("missing conformance class %q", cls)
		}
	}
}

func TestAPIDefinition(t *testing.T) {
	ts, _ := testOGC(t)
	resp, err := http.Get(ts.URL + "/api")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.oai.openapi+json;version=3.0" {
		t.Fatalf("content type %q", ct)
	}
}

func TestCollectionsList(t *testing.T) {
	ts, _ := testOGC(t)
	status, body, _ := getJSON(t, ts.URL+"/collections")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	colls := body["collections"].([]any)
	ids := map[string]bool{}
	for _, c := range colls {
		m := c.(map[string]any)
		ids[m["id"].(string)] = true
		if m["itemType"] != "feature" {
			t.Errorf("itemType = %v", m["itemType"])
		}
	}
	for _, want := range []string{"projects", "tasks", "locations", "samples"} {
		if !ids[want] {
			t.Errorf("missing collection %q", want)
		}
	}
}

func TestSingleCollection(t *testing.T) {
	ts, _ := testOGC(t)
	status, body, _ := getJSON(t, ts.URL+"/collections/locations")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["id"] != "locations" {
		t.Fatalf("id = %v", body["id"])
	}
	if _, ok := body["links"]; !ok {
		t.Fatal("expected links")
	}

	status, _, _ = getJSON(t, ts.URL+"/collections/nope")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestFeatureCollectionWithGeometry(t *testing.T) {
	ts, pool := testOGC(t)
	code := "OGC-ITEMS"
	ogcFixture(t, pool, code)

	for _, collection := range []string{"projects", "tasks", "locations", "samples"} {
		status, body, _ := getJSON(t, ts.URL+"/collections/"+collection+"/items?limit=100")
		if status != http.StatusOK {
			t.Fatalf("%s status %d", collection, status)
		}
		if body["type"] != "FeatureCollection" {
			t.Fatalf("%s type = %v", collection, body["type"])
		}
		if _, ok := body["numberMatched"]; !ok {
			t.Fatalf("%s missing numberMatched", collection)
		}
		if _, ok := body["numberReturned"]; !ok {
			t.Fatalf("%s missing numberReturned", collection)
		}
		features := body["features"].([]any)
		if len(features) == 0 {
			t.Fatalf("%s returned no features", collection)
		}
		f := features[0].(map[string]any)
		if f["type"] != "Feature" {
			t.Fatalf("%s feature type = %v", collection, f["type"])
		}
		if f["id"] == nil || f["id"] == "" {
			t.Fatalf("%s feature missing id", collection)
		}
		geom, ok := f["geometry"].(map[string]any)
		if !ok {
			t.Fatalf("%s feature missing geometry: %v", collection, f["geometry"])
		}
		if geom["type"] == "" {
			t.Fatalf("%s geometry has no type", collection)
		}
		props := f["properties"].(map[string]any)
		if len(props) == 0 {
			t.Fatalf("%s feature has no properties", collection)
		}
	}
}

func TestFeatureById(t *testing.T) {
	ts, pool := testOGC(t)
	code := "OGC-BYID"
	projectID, _ := ogcFixture(t, pool, code)

	status, body, _ := getJSON(t, ts.URL+"/collections/projects/items/"+projectID)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["id"] != projectID {
		t.Fatalf("id = %v", body["id"])
	}

	status, _, _ = getJSON(t, ts.URL+"/collections/projects/items/00000000-0000-0000-0000-000000000000")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestBBoxFiltering(t *testing.T) {
	ts, pool := testOGC(t)
	code := "OGC-BBOX"
	ogcFixture(t, pool, code)

	// Envelope containing the fixture.
	status, body, _ := getJSON(t, ts.URL+"/collections/locations/items?bbox=-123,47,-122,48&limit=100")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["numberMatched"].(float64) < 1 {
		t.Fatalf("expected bbox match: %v", body["numberMatched"])
	}

	// Envelope far away.
	status, body, _ = getJSON(t, ts.URL+"/collections/locations/items?bbox=-80,30,-70,40")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["numberMatched"].(float64) != 0 {
		t.Fatalf("expected no match, got %v", body["numberMatched"])
	}
}

func TestDatetimeFiltering(t *testing.T) {
	ts, pool := testOGC(t)
	code := "OGC-DATE"
	ogcFixture(t, pool, code)

	status, body, _ := getJSON(t, ts.URL+"/collections/samples/items?datetime=2024-03-01/2024-04-01")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["numberMatched"].(float64) < 1 {
		t.Fatalf("expected datetime match, got %v", body["numberMatched"])
	}

	// Open-ended interval.
	status, body, _ = getJSON(t, ts.URL+"/collections/samples/items?datetime=2020-01-01/..")
	if status != http.StatusOK || body["numberMatched"].(float64) < 1 {
		t.Fatalf("open interval: %d %v", status, body["numberMatched"])
	}

	// No match window.
	status, body, _ = getJSON(t, ts.URL+"/collections/samples/items?datetime=2019-01-01/2019-02-01")
	if status != http.StatusOK || body["numberMatched"].(float64) != 0 {
		t.Fatalf("expected no match: %d %v", status, body["numberMatched"])
	}
}

func TestPaginationAndNextLink(t *testing.T) {
	ts, pool := testOGC(t)
	code := "OGC-PAGE"
	projectID, _ := ogcFixture(t, pool, code)

	// Add more locations to force pagination.
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO location (project_id, name, loc_type, geometry)
			 VALUES ($1,$2,'WELL', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))`,
			projectID, "MW-P"+string(rune('A'+i))); err != nil {
			t.Fatalf("location: %v", err)
		}
	}

	status, body, _ := getJSON(t, ts.URL+"/collections/locations/items?limit=2")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(body["features"].([]any)) != 2 {
		t.Fatalf("expected 2 features, got %d", len(body["features"].([]any)))
	}

	// A next link must be present when a full page was returned.
	var hasNext bool
	for _, l := range body["links"].([]any) {
		if l.(map[string]any)["rel"] == "next" {
			hasNext = true
		}
	}
	if !hasNext {
		t.Fatalf("expected next link: %v", body["links"])
	}

	// Offset beyond the end returns zero features.
	status, body, _ = getJSON(t, ts.URL+"/collections/locations/items?limit=2&offset=1000")
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	feats, ok := body["features"].([]any)
	if !ok {
		t.Fatalf("features must be an array, got %T (%v)", body["features"], body["features"])
	}
	if len(feats) != 0 {
		t.Fatalf("expected empty page, got %d features", len(feats))
	}
}

func TestInvalidCollection(t *testing.T) {
	ts, _ := testOGC(t)
	status, body, _ := getJSON(t, ts.URL+"/collections/bogus/items")
	if status != http.StatusNotFound {
		t.Fatalf("status %d", status)
	}
	if body["error"] == nil {
		t.Fatal("expected error message")
	}
}
