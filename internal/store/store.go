// Package store provides PostgreSQL access for Samplier's core entities.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("record not found")

// Store wraps a connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New constructs a Store.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool exposes the underlying pool (used by handlers needing raw SQL).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping verifies database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

// Project is a top-level site/project container.
type Project struct {
	ID        string     `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Notes     *string    `json:"notes,omitempty"`
	SRID      int        `json:"srid"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	ElevUnit  string     `json:"elev_unit"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

const projectColumns = `id, code, name, notes, srid, start_time, end_time, elev_unit, created_at, updated_at`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Notes, &p.SRID, &p.StartTime, &p.EndTime,
		&p.ElevUnit, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// CreateProject inserts a project.
func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO project (code, name, notes, srid, start_time, end_time, elev_unit)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+projectColumns,
		p.Code, p.Name, p.Notes, p.SRID, p.StartTime, p.EndTime, p.ElevUnit)
	return scanProject(row)
}

// GetProject fetches a project by id.
func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	p, err := scanProject(s.pool.QueryRow(ctx,
		`SELECT `+projectColumns+` FROM project WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// ListProjects returns projects ordered by code.
func (s *Store) ListProjects(ctx context.Context, limit, offset int) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+projectColumns+` FROM project ORDER BY code LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountProjects returns the total project count.
func (s *Store) CountProjects(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM project`).Scan(&n)
	return n, err
}

// UpdateProject applies non-nil field updates.
func (s *Store) UpdateProject(ctx context.Context, id string, p Project) (Project, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE project SET code=$2, name=$3, notes=$4, srid=$5, start_time=$6,
		        end_time=$7, elev_unit=$8
		 WHERE id=$1 RETURNING `+projectColumns,
		id, p.Code, p.Name, p.Notes, p.SRID, p.StartTime, p.EndTime, p.ElevUnit)
	out, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return out, err
}

// DeleteProject removes a project (cascades to children).
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM project WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

// Task is a sampling event / field program.
type Task struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Code      string    `json:"code"`
	Name      *string   `json:"name,omitempty"`
	Notes     *string   `json:"notes,omitempty"`
	StartDate *Date     `json:"start_date,omitempty"`
	EndDate   *Date     `json:"end_date,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const taskColumns = `id, project_id, code, name, notes, start_date, end_date, created_at, updated_at`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Code, &t.Name, &t.Notes,
		&t.StartDate, &t.EndDate, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// CreateTask inserts a task.
func (s *Store) CreateTask(ctx context.Context, t Task) (Task, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO task (project_id, code, name, notes, start_date, end_date)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+taskColumns,
		t.ProjectID, t.Code, t.Name, t.Notes, t.StartDate, t.EndDate)
	return scanTask(row)
}

// GetTask fetches a task by id.
func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	t, err := scanTask(s.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM task WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// ListTasks returns tasks, optionally filtered by project.
func (s *Store) ListTasks(ctx context.Context, projectID string, limit, offset int) ([]Task, error) {
	query := `SELECT ` + taskColumns + ` FROM task`
	args := []any{}
	if projectID != "" {
		query += ` WHERE project_id = $1`
		args = append(args, projectID)
	}
	query += fmt.Sprintf(` ORDER BY code LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTask removes a task.
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM task WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Locations
// ---------------------------------------------------------------------------

// Location is a sampling location.
type Location struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	SysLocCode int64     `json:"sys_loc_code"`
	Name       string    `json:"name"`
	LocType    string    `json:"loc_type"`
	Lon        float64   `json:"lon"`
	Lat        float64   `json:"lat"`
	Notes      *string   `json:"notes,omitempty"`
	TotalDepth *float64  `json:"total_depth,omitempty"`
	DepthUnit  *string   `json:"depth_unit,omitempty"`
	Elevation  *float64  `json:"elevation,omitempty"`
	ElevUnit   *string   `json:"elev_unit,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const locationColumns = `id, project_id, sys_loc_code, name, loc_type,
	ST_X(geometry), ST_Y(geometry), notes, total_depth, depth_unit, elevation,
	elev_unit, status, created_at, updated_at`

func scanLocation(row pgx.Row) (Location, error) {
	var l Location
	err := row.Scan(&l.ID, &l.ProjectID, &l.SysLocCode, &l.Name, &l.LocType, &l.Lon, &l.Lat,
		&l.Notes, &l.TotalDepth, &l.DepthUnit, &l.Elevation, &l.ElevUnit, &l.Status,
		&l.CreatedAt, &l.UpdatedAt)
	return l, err
}

// CreateLocation inserts a location at the given lon/lat (EPSG:4326).
func (s *Store) CreateLocation(ctx context.Context, l Location) (Location, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO location (project_id, name, loc_type, geometry, notes, total_depth,
		        depth_unit, elevation, elev_unit, status)
		 VALUES ($1,$2,$3, ST_SetSRID(ST_MakePoint($4,$5),4326), $6,$7,$8,$9,$10,$11)
		 RETURNING `+locationColumns,
		l.ProjectID, l.Name, l.LocType, l.Lon, l.Lat, l.Notes, l.TotalDepth,
		l.DepthUnit, l.Elevation, l.ElevUnit, l.Status)
	return scanLocation(row)
}

// GetLocation fetches a location by id.
func (s *Store) GetLocation(ctx context.Context, id string) (Location, error) {
	l, err := scanLocation(s.pool.QueryRow(ctx,
		`SELECT `+locationColumns+` FROM location WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, ErrNotFound
	}
	return l, err
}

// ListLocations returns locations, optionally filtered by project.
func (s *Store) ListLocations(ctx context.Context, projectID string, limit, offset int) ([]Location, error) {
	query := `SELECT ` + locationColumns + ` FROM location`
	args := []any{}
	if projectID != "" {
		query += ` WHERE project_id = $1`
		args = append(args, projectID)
	}
	query += fmt.Sprintf(` ORDER BY name LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Location, 0)
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteLocation removes a location.
func (s *Store) DeleteLocation(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM location WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Well construction
// ---------------------------------------------------------------------------

// WellSegment is one construction interval of a well.
type WellSegment struct {
	ID               string   `json:"id"`
	LocationID       string   `json:"location_id"`
	SegmentType      string   `json:"segment_type"`
	MaterialTypeCode *string  `json:"material_type_code,omitempty"`
	StartDepth       float64  `json:"start_depth"`
	EndDepth         float64  `json:"end_depth"`
	DepthUnit        string   `json:"depth_unit"`
	InnerDiameter    *float64 `json:"inner_diameter,omitempty"`
	OuterDiameter    *float64 `json:"outer_diameter,omitempty"`
	Remark           *string  `json:"remark,omitempty"`
}

const wellSegmentColumns = `id, location_id, segment_type, material_type_code,
	start_depth, end_depth, depth_unit, inner_diameter, outer_diameter, remark`

func scanWellSegment(row pgx.Row) (WellSegment, error) {
	var w WellSegment
	err := row.Scan(&w.ID, &w.LocationID, &w.SegmentType, &w.MaterialTypeCode,
		&w.StartDepth, &w.EndDepth, &w.DepthUnit, &w.InnerDiameter, &w.OuterDiameter, &w.Remark)
	return w, err
}

// CreateWellSegment inserts a well construction interval.
func (s *Store) CreateWellSegment(ctx context.Context, w WellSegment) (WellSegment, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO well_segment (location_id, segment_type, material_type_code,
		        start_depth, end_depth, depth_unit, inner_diameter, outer_diameter, remark)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+wellSegmentColumns,
		w.LocationID, w.SegmentType, w.MaterialTypeCode, w.StartDepth, w.EndDepth,
		w.DepthUnit, w.InnerDiameter, w.OuterDiameter, w.Remark)
	return scanWellSegment(row)
}

// ListWellSegments returns a location's construction intervals by depth.
func (s *Store) ListWellSegments(ctx context.Context, locationID string) ([]WellSegment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+wellSegmentColumns+` FROM well_segment
		 WHERE location_id=$1 ORDER BY start_depth`, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WellSegment, 0)
	for rows.Next() {
		w, err := scanWellSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteWellSegment removes a segment.
func (s *Store) DeleteWellSegment(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM well_segment WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Samples
// ---------------------------------------------------------------------------

// Sample is a collected sample.
type Sample struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"task_id"`
	LocationID     string    `json:"location_id"`
	SysSampleCode  int64     `json:"sys_sample_code"`
	Name           string    `json:"name"`
	SampleType     string    `json:"sample_type"`
	Matrix         string    `json:"matrix"`
	StartDepth     *float64  `json:"start_depth,omitempty"`
	EndDepth       *float64  `json:"end_depth,omitempty"`
	DepthUnit      *string   `json:"depth_unit,omitempty"`
	SampleDatetime time.Time `json:"sample_datetime"`
	ParentSampleID *string   `json:"parent_sample_id,omitempty"`
	Notes          *string   `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const sampleColumns = `id, task_id, location_id, sys_sample_code, name, sample_type,
	matrix, start_depth, end_depth, depth_unit, sample_datetime, parent_sample_id,
	notes, created_at, updated_at`

func scanSample(row pgx.Row) (Sample, error) {
	var s Sample
	err := row.Scan(&s.ID, &s.TaskID, &s.LocationID, &s.SysSampleCode, &s.Name,
		&s.SampleType, &s.Matrix, &s.StartDepth, &s.EndDepth, &s.DepthUnit,
		&s.SampleDatetime, &s.ParentSampleID, &s.Notes, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// CreateSample inserts a sample.
func (s *Store) CreateSample(ctx context.Context, sm Sample) (Sample, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
		        start_depth, end_depth, depth_unit, sample_datetime, parent_sample_id, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+sampleColumns,
		sm.TaskID, sm.LocationID, sm.Name, sm.SampleType, sm.Matrix, sm.StartDepth,
		sm.EndDepth, sm.DepthUnit, sm.SampleDatetime, sm.ParentSampleID, sm.Notes)
	return scanSample(row)
}

// GetSample fetches a sample by id.
func (s *Store) GetSample(ctx context.Context, id string) (Sample, error) {
	sm, err := scanSample(s.pool.QueryRow(ctx,
		`SELECT `+sampleColumns+` FROM sample WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Sample{}, ErrNotFound
	}
	return sm, err
}

// ListSamples returns samples, optionally filtered by task and/or location.
func (s *Store) ListSamples(ctx context.Context, projectID, taskID, locationID string, limit, offset int) ([]Sample, error) {
	conds := []string{}
	args := []any{}
	if projectID != "" {
		args = append(args, projectID)
		conds = append(conds, fmt.Sprintf(
			"task_id IN (SELECT id FROM task WHERE project_id=$%d)", len(args)))
	}
	if taskID != "" {
		args = append(args, taskID)
		conds = append(conds, fmt.Sprintf("task_id=$%d", len(args)))
	}
	if locationID != "" {
		args = append(args, locationID)
		conds = append(conds, fmt.Sprintf("location_id=$%d", len(args)))
	}
	query := `SELECT ` + sampleColumns + ` FROM sample`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(` ORDER BY sample_datetime DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Sample, 0)
	for rows.Next() {
		sm, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// DeleteSample removes a sample.
func (s *Store) DeleteSample(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sample WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Test is a laboratory analysis of a sample.
type Test struct {
	ID             string    `json:"id"`
	SampleID       string    `json:"sample_id"`
	TestID         string    `json:"test_id"`
	AnalyticMethod *string   `json:"analytic_method,omitempty"`
	Fraction       *string   `json:"fraction,omitempty"`
	AnalysisDate   *Date     `json:"analysis_date,omitempty"`
	BatchID        *string   `json:"batch_id,omitempty"`
	TestType       *string   `json:"test_type,omitempty"`
	Lab            *string   `json:"lab,omitempty"`
	ColumnNumber   *int      `json:"column_number,omitempty"`
	Notes          *string   `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const testColumns = `id, sample_id, test_id, analytic_method, fraction, analysis_date,
	batch_id, test_type, lab, column_number, notes, created_at, updated_at`

func scanTest(row pgx.Row) (Test, error) {
	var t Test
	err := row.Scan(&t.ID, &t.SampleID, &t.TestID, &t.AnalyticMethod, &t.Fraction,
		&t.AnalysisDate, &t.BatchID, &t.TestType, &t.Lab, &t.ColumnNumber, &t.Notes,
		&t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// CreateTest inserts a test.
func (s *Store) CreateTest(ctx context.Context, t Test) (Test, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
		        batch_id, test_type, lab, column_number, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+testColumns,
		t.SampleID, t.TestID, t.AnalyticMethod, t.Fraction, t.AnalysisDate,
		t.BatchID, t.TestType, t.Lab, t.ColumnNumber, t.Notes)
	return scanTest(row)
}

// GetTest fetches a test by id.
func (s *Store) GetTest(ctx context.Context, id string) (Test, error) {
	t, err := scanTest(s.pool.QueryRow(ctx, `SELECT `+testColumns+` FROM test WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Test{}, ErrNotFound
	}
	return t, err
}

// ListTests returns tests for a sample.
func (s *Store) ListTests(ctx context.Context, sampleID string, limit, offset int) ([]Test, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+testColumns+` FROM test WHERE sample_id=$1
		 ORDER BY test_id LIMIT $2 OFFSET $3`, sampleID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Test, 0)
	for rows.Next() {
		t, err := scanTest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTest removes a test (cascades to results).
func (s *Store) DeleteTest(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM test WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

// Result is one analyte measurement from a test.
type Result struct {
	ID                      string   `json:"id"`
	TestID                  string   `json:"test_id"`
	AnalyteID               string   `json:"analyte_id"`
	CASRN                   *string  `json:"cas_rn,omitempty"`
	AnalyteName             *string  `json:"analyte_name,omitempty"`
	ResultNumeric           *float64 `json:"result_numeric,omitempty"`
	ResultText              *string  `json:"result_text,omitempty"`
	DetectFlag              *string  `json:"detect_flag,omitempty"`
	Units                   *string  `json:"units,omitempty"`
	ReportingDetectionLimit *float64 `json:"reporting_detection_limit,omitempty"`
	MethodDetectionLimit    *float64 `json:"method_detection_limit,omitempty"`
	QuantitationLimit       *float64 `json:"quantitation_limit,omitempty"`
	DetectionLimitUnit      *string  `json:"detection_limit_unit,omitempty"`
	LabQualifiers           *string  `json:"lab_qualifiers,omitempty"`
	ValidatorQualifiers     *string  `json:"validator_qualifiers,omitempty"`
	InterpretedQualifiers   *string  `json:"interpreted_qualifiers,omitempty"`
	ReportableResult        bool     `json:"reportable_result"`
	ReportedName            *string  `json:"reported_name,omitempty"`
	Notes                   *string  `json:"notes,omitempty"`
}

const resultColumns = `r.id, r.test_id, r.analyte_id, a.cas_rn, a.chemical_name,
	r.result_numeric, r.result_text, r.detect_flag, r.units,
	r.reporting_detection_limit, r.method_detection_limit, r.quantitation_limit,
	r.detection_limit_unit, r.lab_qualifiers, r.validator_qualifiers,
	r.interpreted_qualifiers, r.reportable_result, r.reported_name, r.notes`

func scanResult(row pgx.Row) (Result, error) {
	var r Result
	err := row.Scan(&r.ID, &r.TestID, &r.AnalyteID, &r.CASRN, &r.AnalyteName,
		&r.ResultNumeric, &r.ResultText, &r.DetectFlag, &r.Units,
		&r.ReportingDetectionLimit, &r.MethodDetectionLimit, &r.QuantitationLimit,
		&r.DetectionLimitUnit, &r.LabQualifiers, &r.ValidatorQualifiers,
		&r.InterpretedQualifiers, &r.ReportableResult, &r.ReportedName, &r.Notes)
	return r, err
}

// CreateResult inserts a result.
func (s *Store) CreateResult(ctx context.Context, r Result) (Result, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO result (test_id, analyte_id, result_numeric, result_text, detect_flag,
		        units, reporting_detection_limit, method_detection_limit, quantitation_limit,
		        detection_limit_unit, lab_qualifiers, validator_qualifiers,
		        interpreted_qualifiers, reportable_result, reported_name, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id`,
		r.TestID, r.AnalyteID, r.ResultNumeric, r.ResultText, r.DetectFlag, r.Units,
		r.ReportingDetectionLimit, r.MethodDetectionLimit, r.QuantitationLimit,
		r.DetectionLimitUnit, r.LabQualifiers, r.ValidatorQualifiers,
		r.InterpretedQualifiers, r.ReportableResult, r.ReportedName, r.Notes)
	var id string
	if err := row.Scan(&id); err != nil {
		return Result{}, err
	}
	return s.GetResult(ctx, id)
}

// GetResult fetches a result by id (with analyte info joined).
func (s *Store) GetResult(ctx context.Context, id string) (Result, error) {
	r, err := scanResult(s.pool.QueryRow(ctx,
		`SELECT `+resultColumns+` FROM result r JOIN analyte a ON a.id=r.analyte_id
		 WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	return r, err
}

// ListResults returns results for a test.
func (s *Store) ListResults(ctx context.Context, testID string, limit, offset int) ([]Result, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+resultColumns+` FROM result r JOIN analyte a ON a.id=r.analyte_id
		 WHERE r.test_id=$1 ORDER BY a.chemical_name LIMIT $2 OFFSET $3`,
		testID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Result, 0)
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteResult removes a result.
func (s *Store) DeleteResult(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM result WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Analytes & units
// ---------------------------------------------------------------------------

// Analyte is a chemical or field parameter.
type Analyte struct {
	ID           string   `json:"id"`
	CASRN        *string  `json:"cas_rn,omitempty"`
	ChemicalName string   `json:"chemical_name"`
	ShortName    *string  `json:"short_name,omitempty"`
	Aliases      []string `json:"aliases"`
}

// CASRNValue returns the CAS RN or an empty string when unset.
func (a Analyte) CASRNValue() string {
	if a.CASRN == nil {
		return ""
	}
	return *a.CASRN
}

// CreateAnalyte inserts an analyte.
func (s *Store) CreateAnalyte(ctx context.Context, a Analyte) (Analyte, error) {
	if a.Aliases == nil {
		a.Aliases = []string{}
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO analyte (cas_rn, chemical_name, short_name, aliases)
		 VALUES ($1,$2,$3,$4) RETURNING id, cas_rn, chemical_name, short_name, aliases`,
		a.CASRN, a.ChemicalName, a.ShortName, a.Aliases)
	err := row.Scan(&a.ID, &a.CASRN, &a.ChemicalName, &a.ShortName, &a.Aliases)
	return a, err
}

// GetAnalyte fetches an analyte by id.
func (s *Store) GetAnalyte(ctx context.Context, id string) (Analyte, error) {
	var a Analyte
	err := s.pool.QueryRow(ctx,
		`SELECT id, cas_rn, chemical_name, short_name, aliases FROM analyte WHERE id=$1`, id).
		Scan(&a.ID, &a.CASRN, &a.ChemicalName, &a.ShortName, &a.Aliases)
	if errors.Is(err, pgx.ErrNoRows) {
		return Analyte{}, ErrNotFound
	}
	return a, err
}

// ListAnalytes returns analytes ordered by chemical name.
func (s *Store) ListAnalytes(ctx context.Context, limit, offset int) ([]Analyte, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, cas_rn, chemical_name, short_name, aliases FROM analyte
		 ORDER BY chemical_name LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Analyte, 0)
	for rows.Next() {
		var a Analyte
		if err := rows.Scan(&a.ID, &a.CASRN, &a.ChemicalName, &a.ShortName, &a.Aliases); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindAnalyteByCAS looks up an analyte by CAS RN (case-insensitive).
func (s *Store) FindAnalyteByCAS(ctx context.Context, cas string) (Analyte, error) {
	var a Analyte
	err := s.pool.QueryRow(ctx,
		`SELECT id, cas_rn, chemical_name, short_name, aliases FROM analyte
		 WHERE upper(cas_rn)=upper($1)`, cas).
		Scan(&a.ID, &a.CASRN, &a.ChemicalName, &a.ShortName, &a.Aliases)
	if errors.Is(err, pgx.ErrNoRows) {
		return Analyte{}, ErrNotFound
	}
	return a, err
}

// UpdateAnalyte replaces an analyte's fields.
func (s *Store) UpdateAnalyte(ctx context.Context, id string, a Analyte) (Analyte, error) {
	if a.Aliases == nil {
		a.Aliases = []string{}
	}
	err := s.pool.QueryRow(ctx,
		`UPDATE analyte SET cas_rn=$2, chemical_name=$3, short_name=$4, aliases=$5
		 WHERE id=$1 RETURNING id, cas_rn, chemical_name, short_name, aliases`,
		id, a.CASRN, a.ChemicalName, a.ShortName, a.Aliases).
		Scan(&a.ID, &a.CASRN, &a.ChemicalName, &a.ShortName, &a.Aliases)
	if errors.Is(err, pgx.ErrNoRows) {
		return Analyte{}, ErrNotFound
	}
	return a, err
}

// DeleteAnalyte removes an analyte.
func (s *Store) DeleteAnalyte(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM analyte WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Unit is a measurement unit.
type Unit struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Quantity *string `json:"quantity,omitempty"`
}

// ListUnits returns all units.
func (s *Store) ListUnits(ctx context.Context) ([]Unit, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name, quantity FROM unit ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Unit, 0)
	for rows.Next() {
		var u Unit
		if err := rows.Scan(&u.Code, &u.Name, &u.Quantity); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Conversion is a unit conversion rule row.
type Conversion struct {
	ReportedUnit     string  `json:"reported_unit"`
	TargetUnit       string  `json:"target_unit"`
	ConversionFactor float64 `json:"conversion_factor"`
	Delta            float64 `json:"delta"`
	Formula          *string `json:"formula,omitempty"`
}

// ListConversions returns all unit conversion rules.
func (s *Store) ListConversions(ctx context.Context) ([]Conversion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT reported_unit, target_unit, conversion_factor, delta, formula
		 FROM unit_conversion ORDER BY reported_unit, target_unit`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Conversion, 0)
	for rows.Next() {
		var c Conversion
		if err := rows.Scan(&c.ReportedUnit, &c.TargetUnit, &c.ConversionFactor,
			&c.Delta, &c.Formula); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Action levels
// ---------------------------------------------------------------------------

// ActionLevelGroup is a group of limits with matching criteria.
type ActionLevelGroup struct {
	ID         string                 `json:"id"`
	TypeCode   string                 `json:"type_code"`
	Code       string                 `json:"code"`
	Name       string                 `json:"name"`
	Matrix     *string                `json:"matrix,omitempty"`
	Fraction   *string                `json:"fraction,omitempty"`
	ProjectID  *string                `json:"project_id,omitempty"`
	StartDate  *string                `json:"start_date,omitempty"`
	EndDate    *string                `json:"end_date,omitempty"`
	Parameters []ActionLevelParameter `json:"parameters,omitempty"`
}

// ActionLevelParameter is a per-analyte limit.
type ActionLevelParameter struct {
	ID             string   `json:"id"`
	AnalyteID      string   `json:"analyte_id"`
	AnalyteName    *string  `json:"analyte_name,omitempty"`
	CASRN          *string  `json:"cas_rn,omitempty"`
	ActionLevel    float64  `json:"action_level"`
	ActionLevelMin *float64 `json:"action_level_min,omitempty"`
	Unit           string   `json:"unit"`
	Fraction       *string  `json:"fraction,omitempty"`
	AnalyticMethod *string  `json:"analytic_method,omitempty"`
}

// ListActionLevelGroups returns groups with their parameters.
func (s *Store) ListActionLevelGroups(ctx context.Context, typeCode, projectID string) ([]ActionLevelGroup, error) {
	query := `SELECT g.id, t.code, g.code, g.name, g.matrix, g.fraction,
	                 g.project_id, g.start_date::text, g.end_date::text
	          FROM action_level g JOIN action_level_type t ON t.id = g.type_id`
	args := []any{}
	conds := []string{}
	if typeCode != "" {
		args = append(args, typeCode)
		conds = append(conds, fmt.Sprintf("t.code=$%d", len(args)))
	}
	if projectID != "" {
		args = append(args, projectID)
		conds = append(conds, fmt.Sprintf("(g.project_id IS NULL OR g.project_id=$%d)", len(args)))
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY t.code, g.code`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]ActionLevelGroup, 0)
	byID := map[string]int{}
	for rows.Next() {
		var g ActionLevelGroup
		if err := rows.Scan(&g.ID, &g.TypeCode, &g.Code, &g.Name, &g.Matrix,
			&g.Fraction, &g.ProjectID, &g.StartDate, &g.EndDate); err != nil {
			return nil, err
		}
		byID[g.ID] = len(groups)
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	// Load parameters for the groups just fetched.
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	prows, err := s.pool.Query(ctx,
		`SELECT p.id, p.action_level_id, p.analyte_id, a.chemical_name, a.cas_rn,
		        p.action_level, p.action_level_min, p.unit, p.fraction, p.analytic_method
		 FROM action_level_parameter p JOIN analyte a ON a.id = p.analyte_id
		 WHERE p.action_level_id = ANY($1) ORDER BY a.chemical_name`, ids)
	if err != nil {
		return nil, err
	}
	defer prows.Close()

	for prows.Next() {
		var p ActionLevelParameter
		var groupID string
		if err := prows.Scan(&p.ID, &groupID, &p.AnalyteID, &p.AnalyteName, &p.CASRN,
			&p.ActionLevel, &p.ActionLevelMin, &p.Unit, &p.Fraction, &p.AnalyticMethod); err != nil {
			return nil, err
		}
		if i, ok := byID[groupID]; ok {
			groups[i].Parameters = append(groups[i].Parameters, p)
		}
	}
	return groups, prows.Err()
}

// CreateActionLevelType inserts a jurisdiction type.
func (s *Store) CreateActionLevelType(ctx context.Context, code, name, description string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO action_level_type (code, name, description) VALUES ($1,$2,$3)`,
		code, name, description)
	return err
}

// CreateActionLevelGroup inserts a group and returns its id.
func (s *Store) CreateActionLevelGroup(ctx context.Context, typeCode string, g ActionLevelGroup) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO action_level (type_id, code, name, matrix, fraction, project_id,
		        start_date, end_date)
		 SELECT t.id, $2, $3, $4, $5, $6, $7::date, $8::date
		 FROM action_level_type t WHERE t.code = $1
		 RETURNING id`,
		typeCode, g.Code, g.Name, g.Matrix, g.Fraction, g.ProjectID, g.StartDate, g.EndDate).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("unknown action level type %q", typeCode)
	}
	return id, err
}

// CreateActionLevelParameter inserts a per-analyte limit.
func (s *Store) CreateActionLevelParameter(ctx context.Context, groupID string, p ActionLevelParameter) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level,
		        action_level_min, unit, fraction, analytic_method)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		groupID, p.AnalyteID, p.ActionLevel, p.ActionLevelMin, p.Unit,
		p.Fraction, p.AnalyticMethod).Scan(&id)
	return id, err
}

// ActionLevelType is a regulatory jurisdiction (EPA, STATE, LOCAL, CUSTOM).
type ActionLevelType struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ListActionLevelTypes returns all action level types.
func (s *Store) ListActionLevelTypes(ctx context.Context) ([]ActionLevelType, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT code, name, COALESCE(description, '') FROM action_level_type ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ActionLevelType, 0)
	for rows.Next() {
		var t ActionLevelType
		if err := rows.Scan(&t.Code, &t.Name, &t.Description); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetActionLevelGroup fetches one group with its parameters.
func (s *Store) GetActionLevelGroup(ctx context.Context, id string) (ActionLevelGroup, error) {
	groups, err := s.ListActionLevelGroupsByIDs(ctx, []string{id})
	if err != nil {
		return ActionLevelGroup{}, err
	}
	if len(groups) == 0 {
		return ActionLevelGroup{}, ErrNotFound
	}
	return groups[0], nil
}

// UpdateActionLevelGroup replaces a group's header fields.
func (s *Store) UpdateActionLevelGroup(ctx context.Context, id string, g ActionLevelGroup) (ActionLevelGroup, error) {
	var out ActionLevelGroup
	err := s.pool.QueryRow(ctx,
		`UPDATE action_level al SET
		   type_id = COALESCE((SELECT t.id FROM action_level_type t WHERE t.code = $2), al.type_id),
		   code = $3, name = $4, matrix = $5, fraction = $6,
		   project_id = $7, start_date = $8::date, end_date = $9::date
		 WHERE al.id = $1
		 RETURNING al.id, (SELECT t.code FROM action_level_type t WHERE t.id = al.type_id),
		           al.code, al.name, al.matrix, al.fraction, al.project_id,
		           al.start_date::text, al.end_date::text`,
		id, g.TypeCode, g.Code, g.Name, g.Matrix, g.Fraction, g.ProjectID,
		g.StartDate, g.EndDate).
		Scan(&out.ID, &out.TypeCode, &out.Code, &out.Name, &out.Matrix,
			&out.Fraction, &out.ProjectID, &out.StartDate, &out.EndDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActionLevelGroup{}, ErrNotFound
	}
	return out, err
}

// DeleteActionLevelGroup removes a group and its parameters.
func (s *Store) DeleteActionLevelGroup(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM action_level WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateActionLevelParameter replaces a per-analyte limit.
func (s *Store) UpdateActionLevelParameter(ctx context.Context, id string, p ActionLevelParameter) (ActionLevelParameter, error) {
	err := s.pool.QueryRow(ctx,
		`UPDATE action_level_parameter SET
		   analyte_id=$2, action_level=$3, action_level_min=$4, unit=$5,
		   fraction=$6, analytic_method=$7
		 WHERE id=$1
		 RETURNING id, analyte_id, action_level, action_level_min, unit, fraction, analytic_method`,
		id, p.AnalyteID, p.ActionLevel, p.ActionLevelMin, p.Unit,
		p.Fraction, p.AnalyticMethod).
		Scan(&p.ID, &p.AnalyteID, &p.ActionLevel, &p.ActionLevelMin, &p.Unit,
			&p.Fraction, &p.AnalyticMethod)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActionLevelParameter{}, ErrNotFound
	}
	return p, err
}

// DeleteActionLevelParameter removes a per-analyte limit.
func (s *Store) DeleteActionLevelParameter(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM action_level_parameter WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Company-wide statistics and activity (dashboard)
// ---------------------------------------------------------------------------

// CompanyStats is a cross-project snapshot for the dashboard.
type CompanyStats struct {
	Projects  int `json:"projects"`
	Tasks     int `json:"tasks"`
	Locations int `json:"locations"`
	Samples   int `json:"samples"`
	Tests     int `json:"tests"`
	Results   int `json:"results"`
	Analytes  int `json:"analytes"`
	Imports   int `json:"imports"`
	// Projects detail, one row per project.
	ProjectDetail []ProjectSummary `json:"project_detail"`
}

// ProjectSummary is a per-project rollup.
type ProjectSummary struct {
	ID             string  `json:"id"`
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Tasks          int     `json:"tasks"`
	Locations      int     `json:"locations"`
	Samples        int     `json:"samples"`
	Results        int     `json:"results"`
	LastSampleDate *string `json:"last_sample_date,omitempty"`
}

// Stats computes company-wide counts and per-project rollups in one round trip.
func (s *Store) Stats(ctx context.Context) (CompanyStats, error) {
	st := CompanyStats{ProjectDetail: make([]ProjectSummary, 0)}
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM project),
		       (SELECT count(*) FROM task),
		       (SELECT count(*) FROM location),
		       (SELECT count(*) FROM sample),
		       (SELECT count(*) FROM test),
		       (SELECT count(*) FROM result),
		       (SELECT count(*) FROM analyte),
		       (SELECT count(*) FROM edd_batch)`,
	).Scan(&st.Projects, &st.Tasks, &st.Locations, &st.Samples, &st.Tests,
		&st.Results, &st.Analytes, &st.Imports)
	if err != nil {
		return st, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.code, p.name,
		       (SELECT count(*) FROM task t WHERE t.project_id = p.id),
		       (SELECT count(*) FROM location l WHERE l.project_id = p.id),
		       (SELECT count(*) FROM sample sa
		          JOIN task t ON t.id = sa.task_id WHERE t.project_id = p.id),
		       (SELECT count(*) FROM result r
		          JOIN test te ON te.id = r.test_id
		          JOIN sample sa ON sa.id = te.sample_id
		          JOIN task t ON t.id = sa.task_id WHERE t.project_id = p.id),
		       (SELECT max(sa.sample_datetime)::date::text FROM sample sa
		          JOIN task t ON t.id = sa.task_id WHERE t.project_id = p.id)
		FROM project p ORDER BY p.code`)
	if err != nil {
		return st, err
	}
	defer rows.Close()

	for rows.Next() {
		var ps ProjectSummary
		if err := rows.Scan(&ps.ID, &ps.Code, &ps.Name, &ps.Tasks, &ps.Locations,
			&ps.Samples, &ps.Results, &ps.LastSampleDate); err != nil {
			return st, err
		}
		st.ProjectDetail = append(st.ProjectDetail, ps)
	}
	return st, rows.Err()
}

// ---------------------------------------------------------------------------
// Analyte groups (reporting tab sets)
// ---------------------------------------------------------------------------

// AnalyteGroup is a named set of analytes, used as a crosstab tab in reports.
type AnalyteGroup struct {
	ID          string               `json:"id"`
	Code        string               `json:"code"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Analytes    []AnalyteGroupMember `json:"analytes,omitempty"`
}

// AnalyteGroupMember is one analyte inside a group.
type AnalyteGroupMember struct {
	ID           string  `json:"id"`
	AnalyteID    string  `json:"analyte_id"`
	AnalyteName  string  `json:"analyte_name,omitempty"`
	CASRN        string  `json:"cas_rn,omitempty"`
	Matrix       *string `json:"matrix,omitempty"`
	Fraction     *string `json:"fraction,omitempty"`
	DefaultUnits *string `json:"default_units,omitempty"`
	DisplayOrder int     `json:"display_order"`
}

// ListAnalyteGroups returns every analyte group with its ordered members.
func (s *Store) ListAnalyteGroups(ctx context.Context) ([]AnalyteGroup, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, COALESCE(description, '') FROM analyte_group ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]AnalyteGroup, 0)
	byID := map[string]int{}
	for rows.Next() {
		var g AnalyteGroup
		if err := rows.Scan(&g.ID, &g.Code, &g.Name, &g.Description); err != nil {
			return nil, err
		}
		byID[g.ID] = len(groups)
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	mrows, err := s.pool.Query(ctx,
		`SELECT m.id, m.group_id, m.analyte_id, a.chemical_name, COALESCE(a.cas_rn, ''),
		        m.matrix, m.fraction, m.default_units, m.display_order
		 FROM analyte_group_member m JOIN analyte a ON a.id = m.analyte_id
		 WHERE m.group_id = ANY($1)
		 ORDER BY m.display_order, a.chemical_name`, ids)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()

	for mrows.Next() {
		var m AnalyteGroupMember
		var groupID string
		if err := mrows.Scan(&m.ID, &groupID, &m.AnalyteID, &m.AnalyteName, &m.CASRN,
			&m.Matrix, &m.Fraction, &m.DefaultUnits, &m.DisplayOrder); err != nil {
			return nil, err
		}
		if i, ok := byID[groupID]; ok {
			groups[i].Analytes = append(groups[i].Analytes, m)
		}
	}
	return groups, mrows.Err()
}

// CreateAnalyteGroup inserts an analyte group (without members).
func (s *Store) CreateAnalyteGroup(ctx context.Context, g AnalyteGroup) (AnalyteGroup, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO analyte_group (code, name, description)
		 VALUES ($1,$2,$3) RETURNING id, code, name, COALESCE(description, '')`,
		g.Code, g.Name, g.Description).Scan(&g.ID, &g.Code, &g.Name, &g.Description)
	return g, err
}

// UpdateAnalyteGroup replaces an analyte group's header fields.
func (s *Store) UpdateAnalyteGroup(ctx context.Context, id string, g AnalyteGroup) (AnalyteGroup, error) {
	err := s.pool.QueryRow(ctx,
		`UPDATE analyte_group SET code=$2, name=$3, description=$4
		 WHERE id=$1 RETURNING id, code, name, COALESCE(description, '')`,
		id, g.Code, g.Name, g.Description).Scan(&g.ID, &g.Code, &g.Name, &g.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return AnalyteGroup{}, ErrNotFound
	}
	return g, err
}

// DeleteAnalyteGroup removes an analyte group and its members.
func (s *Store) DeleteAnalyteGroup(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM analyte_group WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddAnalyteGroupMember adds one analyte to a group.
func (s *Store) AddAnalyteGroupMember(ctx context.Context, groupID string, m AnalyteGroupMember) (AnalyteGroupMember, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO analyte_group_member (group_id, analyte_id, matrix, fraction, default_units, display_order)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, group_id, analyte_id, matrix, fraction, default_units, display_order`,
		groupID, m.AnalyteID, m.Matrix, m.Fraction, m.DefaultUnits, m.DisplayOrder).
		Scan(&m.ID, &groupID, &m.AnalyteID, &m.Matrix, &m.Fraction, &m.DefaultUnits, &m.DisplayOrder)
	return m, err
}

// DeleteAnalyteGroupMember removes one member by member id.
func (s *Store) DeleteAnalyteGroupMember(ctx context.Context, memberID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM analyte_group_member WHERE id=$1`, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Saved report views
// ---------------------------------------------------------------------------

// ReportView is a saved custom crosstab report.
type ReportView struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description,omitempty"`
	Title               string    `json:"title,omitempty"`
	ProjectID           *string   `json:"project_id,omitempty"`
	AnalyteIDs          []string  `json:"analyte_ids,omitempty"`
	AnalyteGroupIDs     []string  `json:"analyte_group_ids,omitempty"`
	ActionLevelGroupIDs []string  `json:"action_level_group_ids,omitempty"`
	IncludeNonDetects   bool      `json:"include_non_detects"`
	DetectsOnly         bool      `json:"detects_only"`
	LocationFilter      string    `json:"location_filter,omitempty"`
	StartDate           *string   `json:"start_date,omitempty"`
	EndDate             *string   `json:"end_date,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

const reportViewColumns = `id, name, COALESCE(description,''), COALESCE(title,''),
	project_id, analyte_ids::text[], analyte_group_ids::text[],
	action_level_group_ids::text[], include_non_detects, detects_only,
	COALESCE(location_filter,''), start_date::text, end_date::text, created_at, updated_at`

func scanReportView(row pgx.Row) (ReportView, error) {
	var v ReportView
	err := row.Scan(&v.ID, &v.Name, &v.Description, &v.Title, &v.ProjectID,
		&v.AnalyteIDs, &v.AnalyteGroupIDs, &v.ActionLevelGroupIDs,
		&v.IncludeNonDetects, &v.DetectsOnly, &v.LocationFilter,
		&v.StartDate, &v.EndDate, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// ListReportViews returns saved report views, newest first.
func (s *Store) ListReportViews(ctx context.Context) ([]ReportView, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+reportViewColumns+` FROM report_view ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ReportView, 0)
	for rows.Next() {
		v, err := scanReportView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetReportView fetches a saved report view by id.
func (s *Store) GetReportView(ctx context.Context, id string) (ReportView, error) {
	v, err := scanReportView(s.pool.QueryRow(ctx,
		`SELECT `+reportViewColumns+` FROM report_view WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportView{}, ErrNotFound
	}
	return v, err
}

// CreateReportView inserts a saved report view.
func (s *Store) CreateReportView(ctx context.Context, v ReportView) (ReportView, error) {
	if v.AnalyteIDs == nil {
		v.AnalyteIDs = []string{}
	}
	if v.AnalyteGroupIDs == nil {
		v.AnalyteGroupIDs = []string{}
	}
	if v.ActionLevelGroupIDs == nil {
		v.ActionLevelGroupIDs = []string{}
	}
	out, err := scanReportView(s.pool.QueryRow(ctx,
		`INSERT INTO report_view (name, description, title, project_id,
		        analyte_ids, analyte_group_ids, action_level_group_ids,
		        include_non_detects, detects_only, location_filter, start_date, end_date)
		 VALUES ($1,$2,$3,$4,$5::uuid[],$6::uuid[],$7::uuid[],$8,$9,$10,$11::date,$12::date)
		 RETURNING `+reportViewColumns,
		v.Name, v.Description, v.Title, v.ProjectID, v.AnalyteIDs, v.AnalyteGroupIDs,
		v.ActionLevelGroupIDs, v.IncludeNonDetects, v.DetectsOnly, v.LocationFilter,
		v.StartDate, v.EndDate))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportView{}, ErrNotFound
	}
	return out, err
}

// UpdateReportView replaces a saved report view.
func (s *Store) UpdateReportView(ctx context.Context, id string, v ReportView) (ReportView, error) {
	if v.AnalyteIDs == nil {
		v.AnalyteIDs = []string{}
	}
	if v.AnalyteGroupIDs == nil {
		v.AnalyteGroupIDs = []string{}
	}
	if v.ActionLevelGroupIDs == nil {
		v.ActionLevelGroupIDs = []string{}
	}
	out, err := scanReportView(s.pool.QueryRow(ctx,
		`UPDATE report_view SET
		   name=$2, description=$3, title=$4, project_id=$5,
		   analyte_ids=$6::uuid[], analyte_group_ids=$7::uuid[],
		   action_level_group_ids=$8::uuid[], include_non_detects=$9,
		   detects_only=$10, location_filter=$11, start_date=$12::date, end_date=$13::date
		 WHERE id=$1 RETURNING `+reportViewColumns,
		id, v.Name, v.Description, v.Title, v.ProjectID, v.AnalyteIDs, v.AnalyteGroupIDs,
		v.ActionLevelGroupIDs, v.IncludeNonDetects, v.DetectsOnly, v.LocationFilter,
		v.StartDate, v.EndDate))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportView{}, ErrNotFound
	}
	return out, err
}

// DeleteReportView removes a saved report view.
func (s *Store) DeleteReportView(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM report_view WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListActionLevelGroupsByIDs returns specific action level groups with parameters.
func (s *Store) ListActionLevelGroupsByIDs(ctx context.Context, ids []string) ([]ActionLevelGroup, error) {
	if len(ids) == 0 {
		return []ActionLevelGroup{}, nil
	}
	query := `SELECT g.id, t.code, g.code, g.name, g.matrix, g.fraction,
	                 g.project_id, g.start_date::text, g.end_date::text
	          FROM action_level g JOIN action_level_type t ON t.id = g.type_id
	          WHERE g.id = ANY($1) ORDER BY t.code, g.code`
	rows, err := s.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]ActionLevelGroup, 0)
	byID := map[string]int{}
	for rows.Next() {
		var g ActionLevelGroup
		if err := rows.Scan(&g.ID, &g.TypeCode, &g.Code, &g.Name, &g.Matrix,
			&g.Fraction, &g.ProjectID, &g.StartDate, &g.EndDate); err != nil {
			return nil, err
		}
		byID[g.ID] = len(groups)
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachActionLevelParameters(ctx, groups, byID)
}

// attachActionLevelParameters loads parameters for the given groups in place.
func (s *Store) attachActionLevelParameters(ctx context.Context, groups []ActionLevelGroup, byID map[string]int) ([]ActionLevelGroup, error) {
	if len(groups) == 0 {
		return groups, nil
	}
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	prows, err := s.pool.Query(ctx,
		`SELECT p.id, p.action_level_id, p.analyte_id, a.chemical_name, a.cas_rn,
		        p.action_level, p.action_level_min, p.unit, p.fraction, p.analytic_method
		 FROM action_level_parameter p JOIN analyte a ON a.id = p.analyte_id
		 WHERE p.action_level_id = ANY($1) ORDER BY a.chemical_name`, ids)
	if err != nil {
		return nil, err
	}
	defer prows.Close()

	for prows.Next() {
		var p ActionLevelParameter
		var groupID string
		if err := prows.Scan(&p.ID, &groupID, &p.AnalyteID, &p.AnalyteName, &p.CASRN,
			&p.ActionLevel, &p.ActionLevelMin, &p.Unit, &p.Fraction, &p.AnalyticMethod); err != nil {
			return nil, err
		}
		if i, ok := byID[groupID]; ok {
			groups[i].Parameters = append(groups[i].Parameters, p)
		}
	}
	return groups, prows.Err()
}

// ActivityEvent is one entry in the recent-activity feed.
type ActivityEvent struct {
	Kind      string    `json:"kind"` // import | sample | project | task
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	ProjectID string    `json:"project_id,omitempty"`
	At        time.Time `json:"at"`
}

// RecentActivity returns the most recent imports and record changes across the
// whole database, newest first.
func (s *Store) RecentActivity(ctx context.Context, limit int) ([]ActivityEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `
		SELECT kind, id, title, detail, COALESCE(project_id, ''), at FROM (
			SELECT 'import' AS kind, eb.id::text AS id,
			       eb.file_name AS title,
			       eb.format_code || ' · ' || eb.status || ' · ' ||
			       eb.error_count || ' err / ' || eb.warning_count || ' warn' AS detail,
			       NULL::text AS project_id,
			       COALESCE(eb.committed_at, eb.created_at) AS at
			FROM edd_batch eb
			UNION ALL
			SELECT 'sample', sa.id::text, sa.name,
			       l.name || ' · ' || sa.matrix || ' · ' || t.code,
			       t.project_id::text, sa.created_at
			FROM sample sa
			JOIN task t ON t.id = sa.task_id
			JOIN location l ON l.id = sa.location_id
			UNION ALL
			SELECT 'project', p.id::text, p.name, p.code, p.id::text, p.updated_at
			FROM project p
			UNION ALL
			SELECT 'task', t.id::text, t.code, COALESCE(t.name, ''),
			       t.project_id::text, t.updated_at
			FROM task t
		) ev
		ORDER BY at DESC NULLS LAST
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ActivityEvent, 0, limit)
	for rows.Next() {
		var e ActivityEvent
		if err := rows.Scan(&e.Kind, &e.ID, &e.Title, &e.Detail, &e.ProjectID, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
