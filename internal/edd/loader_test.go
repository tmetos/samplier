package edd

import (
	"context"
	"testing"

	"github.com/samplier/samplier/internal/testsupport"
)

// fixture creates a project, task and location for loader tests.
func fixture(t *testing.T) (projectID, taskID, locCode string) {
	t.Helper()
	pool := testsupport.Pool(t)
	ctx := context.Background()

	const code = "EDD-TEST"
	testsupport.Cleanup(t, pool, code)
	t.Cleanup(func() {
		testsupport.Cleanup(t, pool, code)
		// Remove analytes created by the loader once their results are gone.
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn IN ('7439-92-1','7440-38-2')
			   AND NOT EXISTS (SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO project (code, name, srid, elev_unit) VALUES ($1,'EDD Loader Test',4326,'ft')
		 RETURNING id`, code).Scan(&projectID); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO task (project_id, code, name) VALUES ($1,'T-EDD','EDD Task') RETURNING id`,
		projectID).Scan(&taskID); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO location (project_id, name, loc_type, geometry)
		 VALUES ($1,'MW-EDD-01','WELL', ST_SetSRID(ST_MakePoint(-122.4,47.6),4326))
		 RETURNING sys_loc_code::text`, projectID).Scan(&locCode); err != nil {
		t.Fatalf("create location: %v", err)
	}
	return projectID, taskID, locCode
}

var refs = ReferenceSet{
	"MATRIX":      {"WATER", "SOIL"},
	"SAMPLE_TYPE": {"N", "FD", "FB"},
	"UNIT":        {"mg/L", "ug/L", "mg/kg"},
	"METHOD":      {"EPA 8260D", "EPA 6020B"},
	"FRACTION":    {"TOTAL", "DISSOLVED"},
	"TEST_TYPE":   {"INITIAL", "DILUTION"},
}

func goodWorkbook(locCode string) *Workbook {
	return &Workbook{
		Order: []string{"EFW2FSample", "EFW2LabTST", "EFW2LabRES"},
		Tables: map[string]Table{
			"EFW2FSample": {
				Section: "EFW2FSample",
				Headers: []string{"SYS_LOC_CODE", "SAMPLE_ID", "SAMPLE_TYPE_CODE",
					"MATRIX_CODE", "SAMPLE_DATE", "START_DEPTH", "END_DEPTH", "DEPTH_UNIT"},
				Rows: []Row{
					{locCode, "EDD-S-001", "N", "WATER", "2024-03-01", "", "", ""},
					{locCode, "EDD-S-002", "FD", "WATER", "2024-03-01", "", "", ""},
				},
			},
			"EFW2LabTST": {
				Section: "EFW2LabTST",
				Headers: []string{"SAMPLE_ID", "TEST_ID", "ANALYTIC_METHOD", "FRACTION",
					"ANALYSIS_DATE", "TEST_TYPE", "LAB_SDG"},
				Rows: []Row{
					{"EDD-S-001", "EDD-T-001", "EPA 8260D", "TOTAL", "2024-03-05", "INITIAL", "EDD-SDG-1"},
					{"EDD-S-002", "EDD-T-002", "EPA 8260D", "TOTAL", "2024-03-05", "INITIAL", "EDD-SDG-1"},
				},
			},
			"EFW2LabRES": {
				Section: "EFW2LabRES",
				Headers: []string{"SAMPLE_ID", "TEST_ID", "CAS_RN", "CHEMICAL_NAME",
					"RESULT_VALUE", "RESULT_UNIT", "DETECT_FLAG",
					"REPORTING_DETECTION_LIMIT", "DETECTION_LIMIT_UNIT", "LAB_QUALIFIERS"},
				Rows: []Row{
					{"EDD-S-001", "EDD-T-001", "7439-92-1", "Lead", "0.0052", "mg/L", "Y", "", "", ""},
					{"EDD-S-001", "EDD-T-001", "7440-38-2", "Arsenic", "", "mg/L", "N", "0.001", "mg/L", "U"},
					{"EDD-S-002", "EDD-T-002", "7439-92-1", "Lead", "0.0048", "mg/L", "Y", "", "", ""},
				},
			},
			"EFW2Batch": {
				Section: "EFW2Batch",
				Headers: []string{"LAB_SDG", "RECEIVED_DATE"},
				Rows:    []Row{{"EDD-SDG-1", "2024-03-02"}},
			},
		},
	}
}

func TestLoadInsertsData(t *testing.T) {
	pool := testsupport.Pool(t)
	projectID, taskID, locCode := fixture(t)
	ctx := context.Background()

	def := EFWEDD()
	wb := goodWorkbook(locCode)

	// Validate first, as the API does.
	if report := NewValidator(def, refs).Validate(wb); !report.Valid() {
		t.Fatalf("fixture should validate: %+v", report.Errors())
	}

	loader := NewLoader(pool)
	res, err := loader.Load(ctx, wb, def, LoadOptions{
		ProjectID: projectID, TaskID: taskID, CommitMode: InsertOnly,
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Samples != 2 || res.Tests != 2 || res.Results != 3 || res.Batches != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Verify samples landed.
	var sampleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sample WHERE task_id=$1`, taskID).Scan(&sampleCount); err != nil {
		t.Fatalf("count samples: %v", err)
	}
	if sampleCount != 2 {
		t.Fatalf("got %d samples", sampleCount)
	}

	// Verify non-detect stored with NULL numeric and a detection limit.
	var numericNull bool
	var rdl float64
	err = pool.QueryRow(ctx,
		`SELECT r.result_numeric IS NULL, r.reporting_detection_limit
		 FROM result r
		 JOIN test ts ON ts.id = r.test_id
		 JOIN sample s ON s.id = ts.sample_id
		 JOIN analyte a ON a.id = r.analyte_id
		 WHERE s.name='EDD-S-001' AND a.cas_rn='7440-38-2'`).Scan(&numericNull, &rdl)
	if err != nil {
		t.Fatalf("query non-detect: %v", err)
	}
	if !numericNull {
		t.Fatal("non-detect must have NULL result_numeric")
	}
	if rdl != 0.001 {
		t.Fatalf("got rdl %v", rdl)
	}
}

func TestLoadIsIdempotentWithInsertUpdate(t *testing.T) {
	pool := testsupport.Pool(t)
	projectID, taskID, locCode := fixture(t)
	ctx := context.Background()

	def := EFWEDD()
	wb := goodWorkbook(locCode)
	loader := NewLoader(pool)
	opts := LoadOptions{ProjectID: projectID, TaskID: taskID, CommitMode: InsertUpdate}

	if _, err := loader.Load(ctx, wb, def, opts); err != nil {
		t.Fatalf("first load: %v", err)
	}
	// Re-load the same workbook: samples/tests upsert, results append.
	if _, err := loader.Load(ctx, wb, def, opts); err != nil {
		t.Fatalf("second load: %v", err)
	}

	var sampleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sample WHERE task_id=$1`, taskID).Scan(&sampleCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if sampleCount != 2 {
		t.Fatalf("samples should not duplicate, got %d", sampleCount)
	}
}

func TestLoadInsertOnlyRejectsDuplicate(t *testing.T) {
	pool := testsupport.Pool(t)
	projectID, taskID, locCode := fixture(t)
	ctx := context.Background()

	def := EFWEDD()
	wb := goodWorkbook(locCode)
	loader := NewLoader(pool)
	opts := LoadOptions{ProjectID: projectID, TaskID: taskID, CommitMode: InsertOnly}

	if _, err := loader.Load(ctx, wb, def, opts); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, err := loader.Load(ctx, wb, def, opts); err == nil {
		t.Fatal("expected duplicate-key error on second InsertOnly load")
	}
}

func TestLoadRejectsUnknownLocation(t *testing.T) {
	pool := testsupport.Pool(t)
	projectID, taskID, _ := fixture(t)
	ctx := context.Background()

	wb := goodWorkbook("999999") // nonexistent sys_loc_code
	if _, err := NewLoader(pool).Load(ctx, wb, EFWEDD(), LoadOptions{
		ProjectID: projectID, TaskID: taskID, CommitMode: InsertOnly,
	}); err == nil {
		t.Fatal("expected error for unknown location")
	}

	// The transaction must roll back entirely.
	var sampleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sample WHERE task_id=$1`, taskID).Scan(&sampleCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if sampleCount != 0 {
		t.Fatalf("expected rollback, found %d samples", sampleCount)
	}
}

func TestLoadRequiresOptions(t *testing.T) {
	pool := testsupport.Pool(t)
	wb := &Workbook{Tables: map[string]Table{}}
	loader := NewLoader(pool)

	if _, err := loader.Load(context.Background(), wb, EFWEDD(),
		LoadOptions{TaskID: "x", CommitMode: InsertOnly}); err == nil {
		t.Fatal("expected error for missing project")
	}
	if _, err := loader.Load(context.Background(), wb, EFWEDD(),
		LoadOptions{ProjectID: "x", CommitMode: InsertOnly}); err == nil {
		t.Fatal("expected error for missing task")
	}
	if _, err := loader.Load(context.Background(), wb, EFWEDD(),
		LoadOptions{ProjectID: "x", TaskID: "y", CommitMode: "BAD"}); err == nil {
		t.Fatal("expected error for invalid commit mode")
	}
}
