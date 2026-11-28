package api

import (
	"context"
	"net/http"
	"testing"
)

func TestAnalyteCRUD(t *testing.T) {
	ts, _ := testAPI(t)

	status, created := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-REF-AN-" + t.Name(), "chemical_name": "Referencine",
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, created)
	}
	id := created["id"].(string)

	status, updated := call(t, http.MethodPut, ts.URL+"/analytes/"+id, map[string]any{
		"cas_rn": "API-REF-AN-" + t.Name(), "chemical_name": "Referencine II",
		"aliases": []string{"ref"},
	})
	if status != http.StatusOK || updated["chemical_name"] != "Referencine II" {
		t.Fatalf("update: %d %v", status, updated)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/analytes/"+id, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}

	status, _ = call(t, http.MethodGet, ts.URL+"/analytes/"+id, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestAnalyteGroupCRUD(t *testing.T) {
	ts, pool := testAPI(t)
	ctx := context.Background()
	code := "API-REF-GRP-" + t.Name()
	_, _ = pool.Exec(ctx, `DELETE FROM analyte_group WHERE code=$1`, code)

	status, group := call(t, http.MethodPost, ts.URL+"/analyte-groups", map[string]any{
		"code": code, "name": "Reference Group",
	})
	if status != http.StatusCreated {
		t.Fatalf("create group: %d %v", status, group)
	}
	groupID := group["id"].(string)

	// An analyte to put into the group.
	status, an := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-REF-AN-" + t.Name(), "chemical_name": "Group Analyte",
	})
	if status != http.StatusCreated {
		t.Fatalf("create analyte: %d %v", status, an)
	}

	status, member := call(t, http.MethodPost, ts.URL+"/analyte-groups/"+groupID+"/members", map[string]any{
		"analyte_id": an["id"], "matrix": "WATER",
	})
	if status != http.StatusCreated {
		t.Fatalf("add member: %d %v", status, member)
	}
	memberID := member["id"].(string)

	status, updated := call(t, http.MethodPut, ts.URL+"/analyte-groups/"+groupID, map[string]any{
		"code": code, "name": "Reference Group Renamed",
	})
	if status != http.StatusOK || updated["name"] != "Reference Group Renamed" {
		t.Fatalf("update group: %d %v", status, updated)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/analyte-groups/"+groupID+"/members/"+memberID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete member: %d", status)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/analyte-groups/"+groupID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete group: %d", status)
	}
}

func TestActionLevelCRUD(t *testing.T) {
	ts, pool := testAPI(t)
	ctx := context.Background()

	// Types list + create.
	status, types := call(t, http.MethodGet, ts.URL+"/action-level-types", nil)
	if status != http.StatusOK || len(types["items"].([]any)) == 0 {
		t.Fatalf("types: %d", status)
	}

	status, an := call(t, http.MethodPost, ts.URL+"/analytes", map[string]any{
		"cas_rn": "API-REF-AL-" + t.Name(), "chemical_name": "Limit Analyte",
	})
	if status != http.StatusCreated {
		t.Fatalf("analyte: %d %v", status, an)
	}

	code := "API-REF-AL-" + t.Name()
	_, _ = pool.Exec(ctx, `DELETE FROM action_level WHERE code=$1`, code)

	status, group := call(t, http.MethodPost, ts.URL+"/action-level-groups", map[string]any{
		"type_code": "EPA", "code": code, "name": "Reference Limit",
	})
	if status != http.StatusCreated {
		t.Fatalf("create group: %d %v", status, group)
	}
	groupID := group["id"].(string)

	status, param := call(t, http.MethodPost,
		ts.URL+"/action-level-groups/"+groupID+"/parameters", map[string]any{
			"analyte_id": an["id"], "action_level": 10, "unit": "mg/L",
		})
	if status != http.StatusCreated {
		t.Fatalf("create param: %d %v", status, param)
	}
	paramID := param["id"].(string)

	status, updated := call(t, http.MethodPut, ts.URL+"/action-level-groups/"+groupID, map[string]any{
		"type_code": "EPA", "code": code, "name": "Reference Limit Renamed",
	})
	if status != http.StatusOK || updated["name"] != "Reference Limit Renamed" {
		t.Fatalf("update group: %d %v", status, updated)
	}

	status, updatedParam := call(t, http.MethodPut,
		ts.URL+"/action-level-parameters/"+paramID, map[string]any{
			"analyte_id": an["id"], "action_level": 20, "unit": "mg/L",
		})
	if status != http.StatusOK || updatedParam["action_level"].(float64) != 20 {
		t.Fatalf("update param: %d %v", status, updatedParam)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/action-level-parameters/"+paramID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete param: %d", status)
	}

	status, _ = call(t, http.MethodDelete, ts.URL+"/action-level-groups/"+groupID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete group: %d", status)
	}
}
