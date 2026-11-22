package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samplier/samplier/internal/testsupport"
)

// newStore returns a Store backed by the test database, or skips.
func newStore(t *testing.T) *Store {
	t.Helper()
	pool := testsupport.Pool(t)
	// Analytes have a globally unique CAS RN; clear leftovers from prior runs.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM project WHERE code LIKE $1`, "STORE-%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM action_level WHERE code LIKE $1`, "STORE-%")
		for _, prefix := range []string{"STORE-%"} {
			_, _ = pool.Exec(context.Background(),
				`DELETE FROM analyte WHERE cas_rn LIKE $1 AND NOT EXISTS (
				   SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`, prefix)
		}
	})
	for _, prefix := range []string{"STORE-%"} {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn LIKE $1 AND NOT EXISTS (
			   SELECT 1 FROM result r WHERE r.analyte_id = analyte.id)`, prefix)
	}
	return New(pool)
}

func TestPing(t *testing.T) {
	s := newStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if s.Pool() == nil {
		t.Fatal("expected pool")
	}
}

func TestProjectLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const code = "STORE-PROJ"
	testsupport.Cleanup(t, s.Pool(), code)

	p, err := s.CreateProject(ctx, Project{Code: code, Name: "Store Test", SRID: 4326, ElevUnit: "ft"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected id")
	}

	got, err := s.GetProject(ctx, p.ID)
	if err != nil || got.Code != code {
		t.Fatalf("get: %v %v", err, got.Code)
	}

	// Update.
	got.Name = "Renamed"
	upd, err := s.UpdateProject(ctx, p.ID, got)
	if err != nil || upd.Name != "Renamed" {
		t.Fatalf("update: %v %v", err, upd.Name)
	}

	// List and count.
	list, err := s.ListProjects(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected projects")
	}
	n, err := s.CountProjects(ctx)
	if err != nil || n == 0 {
		t.Fatalf("count: %v %d", err, n)
	}

	// Delete.
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetProject(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteProject(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on re-delete, got %v", err)
	}
}

func TestTaskLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const code = "STORE-TASK"
	testsupport.Cleanup(t, s.Pool(), code)

	p, err := s.CreateProject(ctx, Project{Code: code, Name: "Task Test", SRID: 4326, ElevUnit: "ft"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	tk, err := s.CreateTask(ctx, Task{ProjectID: p.ID, Code: "T1", Name: strPtr("Task 1")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetTask(ctx, tk.ID)
	if err != nil || got.Code != "T1" {
		t.Fatalf("get: %v", err)
	}

	list, err := s.ListTasks(ctx, p.ID, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	if err := s.DeleteTask(ctx, tk.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetTask(ctx, tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteTask(ctx, tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on re-delete, got %v", err)
	}
}

func TestLocationLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const code = "STORE-LOC"
	testsupport.Cleanup(t, s.Pool(), code)

	p, err := s.CreateProject(ctx, Project{Code: code, Name: "Loc Test", SRID: 4326, ElevUnit: "ft"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	loc, err := s.CreateLocation(ctx, Location{
		ProjectID: p.ID, Name: "MW-1", LocType: "WELL",
		Lon: -122.4, Lat: 47.6, Status: "active",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if loc.SysLocCode == 0 {
		t.Fatal("expected generated sys_loc_code")
	}
	if loc.Lon != -122.4 || loc.Lat != 47.6 {
		t.Fatalf("coordinates round-trip failed: %v %v", loc.Lon, loc.Lat)
	}

	got, err := s.GetLocation(ctx, loc.ID)
	if err != nil || got.Name != "MW-1" {
		t.Fatalf("get: %v", err)
	}

	list, err := s.ListLocations(ctx, p.ID, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	if err := s.DeleteLocation(ctx, loc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetLocation(ctx, loc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteLocation(ctx, loc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on re-delete, got %v", err)
	}
}

func TestWellSegments(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const code = "STORE-WELL"
	testsupport.Cleanup(t, s.Pool(), code)

	p, _ := s.CreateProject(ctx, Project{Code: code, Name: "Well", SRID: 4326, ElevUnit: "ft"})
	loc, err := s.CreateLocation(ctx, Location{
		ProjectID: p.ID, Name: "MW-W", LocType: "WELL",
		Lon: -122.4, Lat: 47.6, Status: "active",
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}

	seg, err := s.CreateWellSegment(ctx, WellSegment{
		LocationID: loc.ID, SegmentType: "SCREEN",
		StartDepth: 5, EndDepth: 15, DepthUnit: "ft",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if seg.ID == "" {
		t.Fatal("expected id")
	}

	segs, err := s.ListWellSegments(ctx, loc.ID)
	if err != nil || len(segs) != 1 {
		t.Fatalf("list: %v %d", err, len(segs))
	}

	if err := s.DeleteWellSegment(ctx, seg.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteWellSegment(ctx, seg.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// buildChain creates project/task/location/sample/test/analyte/result.
func buildChain(t *testing.T, s *Store, code string) (Project, Task, Location, Sample, Test, Analyte, Result) {
	t.Helper()
	ctx := context.Background()
	testsupport.Cleanup(t, s.Pool(), code)
	cleanupAnalyte(t, s, casFor(code))

	p, err := s.CreateProject(ctx, Project{Code: code, Name: code, SRID: 4326, ElevUnit: "ft"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	tk, err := s.CreateTask(ctx, Task{ProjectID: p.ID, Code: "T1"})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	loc, err := s.CreateLocation(ctx, Location{
		ProjectID: p.ID, Name: "MW-1", LocType: "WELL",
		Lon: -122.4, Lat: 47.6, Status: "active",
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	sm, err := s.CreateSample(ctx, Sample{
		TaskID: tk.ID, LocationID: loc.ID, Name: "S-1",
		SampleType: "N", Matrix: "WATER", SampleDatetime: mustTime(t, "2024-03-01"),
	})
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	ts, err := s.CreateTest(ctx, Test{SampleID: sm.ID, TestID: "TST-1"})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	an, err := s.CreateAnalyte(ctx, Analyte{CASRN: strPtr(casFor(code)), ChemicalName: "Lead"})
	if err != nil {
		t.Fatalf("analyte: %v", err)
	}
	res, err := s.CreateResult(ctx, Result{
		TestID: ts.ID, AnalyteID: an.ID,
		ResultNumeric: floatPtr(0.05), DetectFlag: strPtr("Y"), Units: strPtr("mg/L"),
		ReportableResult: true,
	})
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	return p, tk, loc, sm, ts, an, res
}

func TestSampleTestResultChain(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, tk, _, sm, ts, _, res := buildChain(t, s, "STORE-CHAIN")

	gotSample, err := s.GetSample(ctx, sm.ID)
	if err != nil || gotSample.Name != "S-1" {
		t.Fatalf("get sample: %v", err)
	}
	if gotSample.SysSampleCode == 0 {
		t.Fatal("expected sys_sample_code")
	}

	samples, err := s.ListSamples(ctx, "", tk.ID, "", 10, 0)
	if err != nil || len(samples) != 1 {
		t.Fatalf("list samples: %v %d", err, len(samples))
	}
	if _, err := s.ListSamples(ctx, "", "", sm.LocationID, 10, 0); err != nil {
		t.Fatalf("list by location: %v", err)
	}

	tests, err := s.ListTests(ctx, sm.ID, 10, 0)
	if err != nil || len(tests) != 1 {
		t.Fatalf("list tests: %v %d", err, len(tests))
	}
	if _, err := s.GetTest(ctx, ts.ID); err != nil {
		t.Fatalf("get test: %v", err)
	}

	results, err := s.ListResults(ctx, ts.ID, 10, 0)
	if err != nil || len(results) != 1 {
		t.Fatalf("list results: %v %d", err, len(results))
	}
	if results[0].AnalyteName == nil || *results[0].AnalyteName != "Lead" {
		t.Fatalf("analyte join missing: %+v", results[0])
	}
	if got, err := s.GetResult(ctx, res.ID); err != nil || got.ID != res.ID {
		t.Fatalf("get result: %v", err)
	}
}

func TestNotFoundPaths(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	missing := "00000000-0000-0000-0000-000000000000"

	if _, err := s.GetSample(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSample: %v", err)
	}
	if _, err := s.GetTest(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTest: %v", err)
	}
	if _, err := s.GetResult(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetResult: %v", err)
	}
	if _, err := s.GetAnalyte(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAnalyte: %v", err)
	}
	if _, err := s.FindAnalyteByCAS(ctx, "00-00-0"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindAnalyteByCAS: %v", err)
	}
	if err := s.DeleteSample(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteSample: %v", err)
	}
	if err := s.DeleteTest(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTest: %v", err)
	}
	if err := s.DeleteResult(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteResult: %v", err)
	}
	if _, err := s.UpdateProject(ctx, missing, Project{Code: "x", Name: "x", SRID: 4326, ElevUnit: "ft"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateProject: %v", err)
	}
}

func TestDeleteCascades(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, tk, _, sm, ts, _, res := buildChain(t, s, "STORE-CASCADE")

	if err := s.DeleteResult(ctx, res.ID); err != nil {
		t.Fatalf("delete result: %v", err)
	}
	if err := s.DeleteTest(ctx, ts.ID); err != nil {
		t.Fatalf("delete test: %v", err)
	}
	if err := s.DeleteSample(ctx, sm.ID); err != nil {
		t.Fatalf("delete sample: %v", err)
	}
	if err := s.DeleteTask(ctx, tk.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
}

func TestAnalytesAndUnits(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	cas := "STORE-" + t.Name()
	cleanupAnalyte(t, s, cas)
	a, err := s.CreateAnalyte(ctx, Analyte{
		CASRN: strPtr(cas), ChemicalName: "Testium",
		Aliases: []string{"alias1"},
	})
	if err != nil {
		t.Fatalf("create analyte: %v", err)
	}
	if len(a.Aliases) != 1 {
		t.Fatalf("aliases not persisted: %v", a.Aliases)
	}

	found, err := s.FindAnalyteByCAS(ctx, a.CASRNValue())
	if err != nil || found.ID != a.ID {
		t.Fatalf("find by cas: %v", err)
	}

	all, err := s.ListAnalytes(ctx, 100, 0)
	if err != nil || len(all) == 0 {
		t.Fatalf("list analytes: %v", err)
	}

	units, err := s.ListUnits(ctx)
	if err != nil || len(units) == 0 {
		t.Fatalf("list units: %v", err)
	}

	convs, err := s.ListConversions(ctx)
	if err != nil || len(convs) == 0 {
		t.Fatalf("list conversions: %v", err)
	}
}

func TestActionLevels(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	anCas := "STORE-AL-" + t.Name()
	cleanupAnalyte(t, s, anCas)
	an, err := s.CreateAnalyte(ctx, Analyte{
		CASRN: strPtr(anCas), ChemicalName: "Actionium",
	})
	if err != nil {
		t.Fatalf("analyte: %v", err)
	}

	code := "STORE-AL-" + t.Name()
	_, _ = s.Pool().Exec(ctx, `DELETE FROM action_level WHERE code=$1`, code)

	groupID, err := s.CreateActionLevelGroup(ctx, "EPA", ActionLevelGroup{
		Code: code, Name: "Store AL",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	min := 1.0
	if _, err := s.CreateActionLevelParameter(ctx, groupID, ActionLevelParameter{
		AnalyteID: an.ID, ActionLevel: 10, ActionLevelMin: &min, Unit: "mg/L",
	}); err != nil {
		t.Fatalf("create param: %v", err)
	}

	groups, err := s.ListActionLevelGroups(ctx, "EPA", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found bool
	for _, g := range groups {
		if g.ID == groupID {
			found = true
			if len(g.Parameters) != 1 {
				t.Fatalf("expected 1 parameter, got %d", len(g.Parameters))
			}
			if g.Parameters[0].AnalyteName == nil {
				t.Fatal("expected analyte name join")
			}
		}
	}
	if !found {
		t.Fatal("created group not returned")
	}

	// Unknown type must error.
	if _, err := s.CreateActionLevelGroup(ctx, "NOPE", ActionLevelGroup{Code: code + "-X", Name: "x"}); err == nil {
		t.Fatal("expected error for unknown type")
	}

	// CreateActionLevelType works (delete any prior run first).
	typeCode := "STORE-TYPE-" + t.Name()
	_, _ = s.Pool().Exec(ctx, `DELETE FROM action_level_type WHERE code=$1`, typeCode)
	if err := s.CreateActionLevelType(ctx, typeCode, "Store", "desc"); err != nil {
		t.Fatalf("create type: %v", err)
	}
}

func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

// casFor returns a unique CAS-like key per project code.
func casFor(code string) string { return code }

// cleanupAnalyte removes an analyte and re-runs at test end, after the owning
// project (and therefore every referencing result) has been deleted.
func cleanupAnalyte(t *testing.T, s *Store, cas string) {
	t.Helper()
	del := func() {
		_, _ = s.Pool().Exec(context.Background(),
			`DELETE FROM analyte WHERE cas_rn = $1`, cas)
	}
	del()
	t.Cleanup(del)
}
