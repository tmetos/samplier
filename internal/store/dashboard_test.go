package store

import (
	"context"
	"testing"
	"time"
)

func TestStatsCountsAndRollup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	code := "STORE-STATS"
	_, _, loc, sm, ts, an, _ := buildChain(t, s, code)

	// Record an import so the imports counter moves.
	if _, err := s.Pool().Exec(ctx,
		`INSERT INTO edd_format (code, name) VALUES ('EFWEDD','EFWEDD')
		 ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatalf("format: %v", err)
	}
	if _, err := s.Pool().Exec(ctx,
		`INSERT INTO edd_batch (format_code, file_name, status, committed_at)
		 VALUES ('EFWEDD','stats-test.zip','COMMITTED', now())`); err != nil {
		t.Fatalf("batch: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.Pool().Exec(context.Background(),
			`DELETE FROM edd_batch WHERE file_name = 'stats-test.zip'`)
	})

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.Projects < 1 || st.Tasks < 1 || st.Locations < 1 || st.Samples < 1 ||
		st.Tests < 1 || st.Results < 1 || st.Analytes < 1 {
		t.Fatalf("counts look wrong: %+v", st)
	}
	if st.Imports < 1 {
		t.Fatalf("expected imports >= 1, got %d", st.Imports)
	}

	// The project must appear in the rollup with its own numbers.
	var found *ProjectSummary
	for i := range st.ProjectDetail {
		if st.ProjectDetail[i].Code == code {
			found = &st.ProjectDetail[i]
		}
	}
	if found == nil {
		t.Fatalf("project %s missing from rollup", code)
	}
	if found.Locations != 1 || found.Samples != 1 || found.Tasks != 1 || found.Results != 1 {
		t.Fatalf("unexpected rollup: %+v", *found)
	}
	if found.LastSampleDate == nil {
		t.Fatal("expected last_sample_date")
	}

	_ = loc
	_ = sm
	_ = ts
	_ = an
}

func TestRecentActivityOrderingAndKinds(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	code := "STORE-ACT"
	buildChain(t, s, code)

	events, err := s.RecentActivity(ctx, 20)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected activity events")
	}

	// Newest first.
	for i := 1; i < len(events); i++ {
		if events[i].At.After(events[i-1].At) {
			t.Fatalf("events not ordered newest-first at %d", i)
		}
	}

	kinds := map[string]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
		if e.Title == "" {
			t.Errorf("event %s has no title", e.ID)
		}
		if e.At.IsZero() {
			t.Errorf("event %s has no timestamp", e.ID)
		}
	}
	if !kinds["sample"] || !kinds["project"] || !kinds["task"] {
		t.Fatalf("expected sample/project/task events, got %v", kinds)
	}
}

func TestRecentActivityLimit(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	buildChain(t, s, "STORE-ACTL")

	events, err := s.RecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Invalid limits fall back to the default rather than erroring.
	if _, err := s.RecentActivity(ctx, 0); err != nil {
		t.Fatalf("limit 0: %v", err)
	}
	if _, err := s.RecentActivity(ctx, 10_000); err != nil {
		t.Fatalf("large limit: %v", err)
	}
}

func TestListSamplesByProject(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	p, tk, _, sm, _, _, _ := buildChain(t, s, "STORE-SP")

	byProject, err := s.ListSamples(ctx, p.ID, "", "", 50, 0)
	if err != nil {
		t.Fatalf("by project: %v", err)
	}
	if len(byProject) != 1 || byProject[0].ID != sm.ID {
		t.Fatalf("unexpected samples: %+v", byProject)
	}

	// A different project yields nothing.
	other, err := s.ListSamples(ctx, "00000000-0000-0000-0000-000000000000", "", "", 50, 0)
	if err != nil {
		t.Fatalf("other project: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("expected no samples, got %d", len(other))
	}

	// Combined filters still work.
	both, err := s.ListSamples(ctx, p.ID, tk.ID, "", 50, 0)
	if err != nil || len(both) != 1 {
		t.Fatalf("combined filter: %v %d", err, len(both))
	}

	_ = time.Now
}
