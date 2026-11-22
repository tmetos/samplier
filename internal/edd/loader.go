package edd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CommitMode controls how existing records are treated on load.
type CommitMode string

const (
	// InsertOnly fails when a business key already exists.
	InsertOnly CommitMode = "INSERT_ONLY"
	// InsertUpdate overwrites existing records.
	InsertUpdate CommitMode = "INSERT_UPDATE"
	// InsertMerge fills blank fields on existing records, preserving populated ones.
	InsertMerge CommitMode = "INSERT_MERGE"
)

// ValidCommitMode reports whether m is a known commit mode.
func ValidCommitMode(m CommitMode) bool {
	switch m {
	case InsertOnly, InsertUpdate, InsertMerge:
		return true
	}
	return false
}

// LoadOptions configures a load.
type LoadOptions struct {
	ProjectID  string
	TaskID     string
	CommitMode CommitMode
	EDDBatchID string
}

// LoadResult summarises a committed load.
type LoadResult struct {
	Samples int `json:"samples"`
	Tests   int `json:"tests"`
	Results int `json:"results"`
	Batches int `json:"batches"`
}

// Loader writes validated EDD data into PostgreSQL.
type Loader struct {
	pool *pgxpool.Pool
}

// NewLoader constructs a Loader.
func NewLoader(pool *pgxpool.Pool) *Loader { return &Loader{pool: pool} }

// Load commits a validated workbook inside one transaction.
func (l *Loader) Load(ctx context.Context, wb *Workbook, def Definition, opts LoadOptions) (LoadResult, error) {
	if !ValidCommitMode(opts.CommitMode) {
		return LoadResult{}, fmt.Errorf("invalid commit mode %q", opts.CommitMode)
	}
	if opts.ProjectID == "" {
		return LoadResult{}, errors.New("project_id is required")
	}
	if opts.TaskID == "" {
		return LoadResult{}, errors.New("task_id is required")
	}

	var res LoadResult
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Batches first (tests reference them).
	if n, err := l.loadBatches(ctx, tx, wb, opts); err != nil {
		return res, err
	} else {
		res.Batches = n
	}

	if n, err := l.loadSamples(ctx, tx, wb, opts); err != nil {
		return res, err
	} else {
		res.Samples = n
	}

	if n, err := l.loadTests(ctx, tx, wb, opts); err != nil {
		return res, err
	} else {
		res.Tests = n
	}

	if n, err := l.loadResults(ctx, tx, wb, opts); err != nil {
		return res, err
	} else {
		res.Results = n
	}

	if err := tx.Commit(ctx); err != nil {
		return LoadResult{}, err
	}
	return res, nil
}

func (l *Loader) loadBatches(ctx context.Context, tx pgx.Tx, wb *Workbook, opts LoadOptions) (int, error) {
	tbl, ok := wb.Get("EFW2Batch")
	if !ok {
		return 0, nil
	}
	idx := tbl.ColumnIndex()
	count := 0
	for _, row := range tbl.Rows {
		name, _ := row.Value(idx, "LAB_SDG")
		if name == "" {
			continue
		}
		lab, _ := row.Value(idx, "LAB_NAME_CODE")
		received, _ := row.Value(idx, "RECEIVED_DATE")

		var receivedVal any
		if received != "" {
			if t, err := ParseDate(received); err == nil {
				receivedVal = t
			}
		}

		_, err := tx.Exec(ctx,
			`INSERT INTO batch (project_id, task_id, name, lab, received_date)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5)
			 ON CONFLICT (project_id, name) DO UPDATE
			   SET lab = COALESCE(EXCLUDED.lab, batch.lab),
			       received_date = COALESCE(EXCLUDED.received_date, batch.received_date)`,
			opts.ProjectID, opts.TaskID, name, lab, receivedVal)
		if err != nil {
			return count, fmt.Errorf("batch %q: %w", name, err)
		}
		count++
	}
	return count, nil
}

func (l *Loader) loadSamples(ctx context.Context, tx pgx.Tx, wb *Workbook, opts LoadOptions) (int, error) {
	tbl, ok := wb.Get("EFW2FSample")
	if !ok {
		tbl, ok = wb.Get("EFW2LabSMP")
	}
	if !ok {
		return 0, nil
	}
	idx := tbl.ColumnIndex()
	count := 0

	for _, row := range tbl.Rows {
		name, _ := row.Value(idx, "SAMPLE_ID")
		locCode, _ := row.Value(idx, "SYS_LOC_CODE")
		matrix, _ := row.Value(idx, "MATRIX_CODE")
		sampleType, _ := row.Value(idx, "SAMPLE_TYPE_CODE")
		dateRaw, _ := row.Value(idx, "SAMPLE_DATE")
		startDepth, _ := row.Value(idx, "START_DEPTH")
		endDepth, _ := row.Value(idx, "END_DEPTH")
		depthUnit, _ := row.Value(idx, "DEPTH_UNIT")

		if name == "" || locCode == "" {
			continue
		}
		if sampleType == "" {
			sampleType = "N"
		}

		sampleDate, err := ParseDate(dateRaw)
		if err != nil {
			return count, fmt.Errorf("sample %q: %w", name, err)
		}

		// Resolve the location by sys_loc_code within the project.
		var locationID int64
		var locUUID string
		err = tx.QueryRow(ctx,
			`SELECT id FROM location WHERE project_id=$1 AND sys_loc_code=$2`,
			opts.ProjectID, locCode).Scan(&locUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return count, fmt.Errorf("sample %q: location sys_loc_code %s not found in project",
				name, locCode)
		}
		if err != nil {
			return count, err
		}
		_ = locationID

		startVal := numericOrNil(startDepth)
		endVal := numericOrNil(endDepth)

		switch opts.CommitMode {
		case InsertOnly:
			_, err = tx.Exec(ctx,
				`INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
				        start_depth, end_depth, depth_unit, sample_datetime)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)`,
				opts.TaskID, locUUID, name, sampleType, matrix, startVal, endVal,
				depthUnit, sampleDate)
		case InsertUpdate:
			_, err = tx.Exec(ctx,
				`INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
				        start_depth, end_depth, depth_unit, sample_datetime)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)
				 ON CONFLICT (task_id, name) DO UPDATE
				   SET location_id = EXCLUDED.location_id,
				       sample_type = EXCLUDED.sample_type,
				       matrix = EXCLUDED.matrix,
				       start_depth = EXCLUDED.start_depth,
				       end_depth = EXCLUDED.end_depth,
				       depth_unit = EXCLUDED.depth_unit,
				       sample_datetime = EXCLUDED.sample_datetime`,
				opts.TaskID, locUUID, name, sampleType, matrix, startVal, endVal,
				depthUnit, sampleDate)
		case InsertMerge:
			_, err = tx.Exec(ctx,
				`INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
				        start_depth, end_depth, depth_unit, sample_datetime)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)
				 ON CONFLICT (task_id, name) DO UPDATE
				   SET sample_type = COALESCE(NULLIF(sample.sample_type,''), EXCLUDED.sample_type),
				       matrix = COALESCE(NULLIF(sample.matrix,''), EXCLUDED.matrix),
				       start_depth = COALESCE(sample.start_depth, EXCLUDED.start_depth),
				       end_depth = COALESCE(sample.end_depth, EXCLUDED.end_depth),
				       depth_unit = COALESCE(NULLIF(sample.depth_unit,''), EXCLUDED.depth_unit)`,
				opts.TaskID, locUUID, name, sampleType, matrix, startVal, endVal,
				depthUnit, sampleDate)
		}
		if err != nil {
			return count, fmt.Errorf("sample %q: %w", name, err)
		}
		count++
	}
	return count, nil
}

func (l *Loader) loadTests(ctx context.Context, tx pgx.Tx, wb *Workbook, opts LoadOptions) (int, error) {
	tbl, ok := wb.Get("EFW2LabTST")
	if !ok {
		return 0, nil
	}
	idx := tbl.ColumnIndex()
	count := 0

	for _, row := range tbl.Rows {
		sampleName, _ := row.Value(idx, "SAMPLE_ID")
		testID, _ := row.Value(idx, "TEST_ID")
		if sampleName == "" || testID == "" {
			continue
		}

		method, _ := row.Value(idx, "ANALYTIC_METHOD")
		fraction, _ := row.Value(idx, "FRACTION")
		analysisDate, _ := row.Value(idx, "ANALYSIS_DATE")
		testType, _ := row.Value(idx, "TEST_TYPE")
		sdg, _ := row.Value(idx, "LAB_SDG")
		lab, _ := row.Value(idx, "LAB_NAME_CODE")

		var sampleUUID string
		err := tx.QueryRow(ctx,
			`SELECT id FROM sample WHERE task_id=$1 AND name=$2`, opts.TaskID, sampleName).
			Scan(&sampleUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return count, fmt.Errorf("test %q: sample %q not found", testID, sampleName)
		}
		if err != nil {
			return count, err
		}

		var batchUUID any
		if sdg != "" {
			var id string
			err := tx.QueryRow(ctx,
				`SELECT id FROM batch WHERE project_id=$1 AND name=$2`, opts.ProjectID, sdg).Scan(&id)
			if err == nil {
				batchUUID = id
			}
		}

		var analysisVal any
		if analysisDate != "" {
			if t, err := ParseDate(analysisDate); err == nil {
				analysisVal = t
			}
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
			        batch_id, test_type, lab)
			 VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,NULLIF($7,''),NULLIF($8,''))
			 ON CONFLICT (sample_id, test_id) DO UPDATE
			   SET analytic_method = COALESCE(EXCLUDED.analytic_method, test.analytic_method),
			       fraction = COALESCE(EXCLUDED.fraction, test.fraction),
			       analysis_date = COALESCE(EXCLUDED.analysis_date, test.analysis_date),
			       batch_id = COALESCE(EXCLUDED.batch_id, test.batch_id),
			       test_type = COALESCE(EXCLUDED.test_type, test.test_type),
			       lab = COALESCE(EXCLUDED.lab, test.lab)`,
			sampleUUID, testID, method, fraction, analysisVal, batchUUID, testType, lab)
		if err != nil {
			return count, fmt.Errorf("test %q: %w", testID, err)
		}
		count++
	}
	return count, nil
}

func (l *Loader) loadResults(ctx context.Context, tx pgx.Tx, wb *Workbook, opts LoadOptions) (int, error) {
	tbl, ok := wb.Get("EFW2LabRES")
	if !ok {
		return 0, nil
	}
	idx := tbl.ColumnIndex()
	count := 0

	for _, row := range tbl.Rows {
		sampleName, _ := row.Value(idx, "SAMPLE_ID")
		testID, _ := row.Value(idx, "TEST_ID")
		casRN, _ := row.Value(idx, "CAS_RN")
		chemName, _ := row.Value(idx, "CHEMICAL_NAME")

		if sampleName == "" || testID == "" || (casRN == "" && chemName == "") {
			continue
		}

		// Resolve or create the analyte.
		analyteID, err := resolveAnalyte(ctx, tx, casRN, chemName)
		if err != nil {
			return count, err
		}

		// Resolve the test.
		var testUUID string
		err = tx.QueryRow(ctx,
			`SELECT t.id FROM test t JOIN sample s ON s.id = t.sample_id
			 WHERE s.task_id=$1 AND s.name=$2 AND t.test_id=$3`,
			opts.TaskID, sampleName, testID).Scan(&testUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return count, fmt.Errorf("result for %q/%q: test not found", sampleName, testID)
		}
		if err != nil {
			return count, err
		}

		valueRaw, _ := row.Value(idx, "RESULT_VALUE")
		units, _ := row.Value(idx, "RESULT_UNIT")
		detect, _ := row.Value(idx, "DETECT_FLAG")
		rdl, _ := row.Value(idx, "REPORTING_DETECTION_LIMIT")
		mdl, _ := row.Value(idx, "METHOD_DETECTION_LIMIT")
		dlUnit, _ := row.Value(idx, "DETECTION_LIMIT_UNIT")
		labQ, _ := row.Value(idx, "LAB_QUALIFIERS")
		valQ, _ := row.Value(idx, "VALIDATOR_QUALIFIERS")
		text, _ := row.Value(idx, "RESULT_TEXT")

		detectFlag := strings.ToUpper(strings.TrimSpace(detect))
		if detectFlag == "" {
			detectFlag = "Y"
		}

		var value any
		if detectFlag != "N" && valueRaw != "" {
			value = numericOrNil(valueRaw)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO result (test_id, analyte_id, result_numeric, result_text,
			        detect_flag, units, reporting_detection_limit, method_detection_limit,
			        detection_limit_unit, lab_qualifiers, validator_qualifiers, reported_name)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),$7,$8,NULLIF($9,''),
			         NULLIF($10,''),NULLIF($11,''),NULLIF($12,''))`,
			testUUID, analyteID, value, text, detectFlag, units,
			numericOrNil(rdl), numericOrNil(mdl), dlUnit, labQ, valQ, chemName)
		if err != nil {
			return count, fmt.Errorf("result %q/%q: %w", sampleName, testID, err)
		}
		count++
	}
	return count, nil
}

// resolveAnalyte finds an analyte by CAS RN (or name) or creates it.
func resolveAnalyte(ctx context.Context, tx pgx.Tx, casRN, name string) (string, error) {
	if casRN != "" {
		var id string
		err := tx.QueryRow(ctx,
			`SELECT id FROM analyte WHERE upper(cas_rn) = upper($1)`, casRN).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}

	if name != "" {
		var id string
		err := tx.QueryRow(ctx,
			`SELECT id FROM analyte WHERE lower(chemical_name) = lower($1)`, name).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}

	var id string
	var cas any
	if casRN != "" {
		cas = casRN
	}
	err := tx.QueryRow(ctx,
		`INSERT INTO analyte (cas_rn, chemical_name) VALUES ($1,$2) RETURNING id`,
		cas, coalesce(name, casRN)).Scan(&id)
	return id, err
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "unknown"
}

// numericOrNil returns nil for blank input, else the raw numeric string cast by
// PostgreSQL (NULLIF-free path avoids a second parse).
func numericOrNil(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
