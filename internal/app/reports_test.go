package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCustomReportPreviewAndExport(t *testing.T) {
	ts, pool := testServer(t)
	code := "APP-RPT"
	f := makeFixture(t, pool, code)

	// Two regulatory limit sets so we can prove multiple sets are evaluated.
	const (
		epaCode   = "APP-RPT-EPA"
		stateCode = "APP-RPT-STATE"
		grpCode   = "APP-RPT-GRP"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM action_level WHERE code IN ($1,$2)`, epaCode, stateCode)
		_, _ = pool.Exec(context.Background(), `DELETE FROM analyte_group WHERE code = $1`, grpCode)
	}
	cleanup()
	t.Cleanup(cleanup)

	var epaID, stateID, grpID string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO action_level (type_id, code, name, matrix)
		 SELECT t.id, $1, 'EPA Limit', 'WATER' FROM action_level_type t WHERE t.code='EPA'
		 RETURNING id`, epaCode).Scan(&epaID)
	if err != nil {
		t.Fatalf("epa group: %v", err)
	}
	err = pool.QueryRow(context.Background(),
		`INSERT INTO action_level (type_id, code, name, matrix)
		 SELECT t.id, $1, 'State Limit', 'WATER' FROM action_level_type t WHERE t.code='STATE'
		 RETURNING id`, stateCode).Scan(&stateID)
	if err != nil {
		t.Fatalf("state group: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level, unit) VALUES
		 ($1,$3,0.015,'mg/L'),($2,$3,0.005,'mg/L')`, epaID, stateID, f.AnalyteID); err != nil {
		t.Fatalf("parameters: %v", err)
	}
	err = pool.QueryRow(context.Background(),
		`INSERT INTO analyte_group (code, name) VALUES ($1,'App Group') RETURNING id`, grpCode).Scan(&grpID)
	if err != nil {
		t.Fatalf("analyte group: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO analyte_group_member (group_id, analyte_id, matrix) VALUES ($1,$2,'WATER')`,
		grpID, f.AnalyteID); err != nil {
		t.Fatalf("group member: %v", err)
	}

	spec := map[string]any{
		"name":                   "App Report",
		"project_id":             f.ProjectID,
		"analyte_group_ids":      []string{grpID},
		"action_level_group_ids": []string{epaID, stateID},
		"include_non_detects":    true,
	}

	// Preview.
	status, preview, raw := doJSON(t, http.MethodPost, ts.URL+"/api/v1/reports/preview", spec)
	if status != http.StatusOK {
		t.Fatalf("preview status %d: %s", status, raw)
	}
	tabs, ok := preview["tabs"].([]any)
	if !ok || len(tabs) != 1 {
		t.Fatalf("expected 1 tab, got %v", preview["tabs"])
	}
	tab := tabs[0].(map[string]any)
	if tab["label"] != "App Group" {
		t.Fatalf("tab label = %v", tab["label"])
	}
	if tab["exceedance_count"].(float64) != 1 {
		t.Fatalf("exceedance_count = %v", tab["exceedance_count"])
	}
	rows := tab["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	cells := rows[0].(map[string]any)["cells"].([]any)
	cell := cells[0].(map[string]any)
	if cell["exceeded"] != true {
		t.Fatalf("expected exceeded cell: %v", cell)
	}
	if flags := cell["flags"].(string); !strings.Contains(flags, epaCode) || !strings.Contains(flags, stateCode) {
		t.Fatalf("expected both limit groups in flags, got %q", flags)
	}

	// Export.
	resp, err := http.Post(ts.URL+"/api/v1/reports/export", "application/json",
		bytes.NewReader(mustMarshal(t, spec)))
	if err != nil {
		t.Fatalf("export post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("export status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content type %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 2 || body[0] != 'P' || body[1] != 'K' {
		t.Fatalf("expected xlsx zip magic")
	}

	// Save, list, delete.
	status, saved, _ := doJSON(t, http.MethodPost, ts.URL+"/api/v1/reports", spec)
	if status != http.StatusCreated {
		t.Fatalf("save status %d", status)
	}
	id := saved["id"].(string)

	status, list, _ := doJSON(t, http.MethodGet, ts.URL+"/api/v1/reports", nil)
	if status != http.StatusOK {
		t.Fatalf("list status %d", status)
	}
	if len(list["items"].([]any)) == 0 {
		t.Fatal("expected saved report in list")
	}

	// Saved preview works too.
	status, _, raw = doJSON(t, http.MethodGet, ts.URL+"/api/v1/reports/"+id+"/preview", nil)
	if status != http.StatusOK {
		t.Fatalf("saved preview status %d: %s", status, raw)
	}

	status, _, _ = doJSON(t, http.MethodDelete, ts.URL+"/api/v1/reports/"+id, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete status %d", status)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
