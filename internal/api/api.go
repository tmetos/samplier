// Package api implements the Samplier REST API under /api/v1.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/samplier/samplier/internal/store"
)

// Handler serves REST resources.
type Handler struct {
	store  *store.Store
	logger *slog.Logger
}

// New constructs an API handler.
func New(st *store.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: st, logger: logger}
}

// Routes returns the /api/v1 subrouter.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/projects", h.listProjects)
	r.Post("/projects", h.createProject)
	r.Get("/projects/{id}", h.getProject)
	r.Put("/projects/{id}", h.updateProject)
	r.Delete("/projects/{id}", h.deleteProject)

	r.Get("/tasks", h.listTasks)
	r.Post("/tasks", h.createTask)
	r.Get("/tasks/{id}", h.getTask)
	r.Delete("/tasks/{id}", h.deleteTask)

	r.Get("/locations", h.listLocations)
	r.Post("/locations", h.createLocation)
	r.Get("/locations/{id}", h.getLocation)
	r.Delete("/locations/{id}", h.deleteLocation)
	r.Get("/locations/{id}/well-segments", h.listWellSegments)
	r.Post("/locations/{id}/well-segments", h.createWellSegment)
	r.Delete("/well-segments/{id}", h.deleteWellSegment)

	r.Get("/samples", h.listSamples)
	r.Post("/samples", h.createSample)
	r.Get("/samples/{id}", h.getSample)
	r.Delete("/samples/{id}", h.deleteSample)
	r.Get("/samples/{id}/tests", h.listTests)

	r.Post("/tests", h.createTest)
	r.Get("/tests/{id}", h.getTest)
	r.Delete("/tests/{id}", h.deleteTest)
	r.Get("/tests/{id}/results", h.listResults)

	r.Post("/results", h.createResult)
	r.Get("/results/{id}", h.getResult)
	r.Delete("/results/{id}", h.deleteResult)

	r.Get("/analytes", h.listAnalytes)
	r.Post("/analytes", h.createAnalyte)
	r.Get("/analytes/{id}", h.getAnalyte)
	r.Put("/analytes/{id}", h.updateAnalyte)
	r.Delete("/analytes/{id}", h.deleteAnalyte)
	r.Get("/analyte-groups", h.listAnalyteGroups)
	r.Post("/analyte-groups", h.createAnalyteGroup)
	r.Put("/analyte-groups/{id}", h.updateAnalyteGroup)
	r.Delete("/analyte-groups/{id}", h.deleteAnalyteGroup)
	r.Post("/analyte-groups/{id}/members", h.addAnalyteGroupMember)
	r.Delete("/analyte-groups/{id}/members/{memberId}", h.deleteAnalyteGroupMember)

	r.Get("/units", h.listUnits)
	r.Get("/unit-conversions", h.listConversions)

	r.Get("/action-level-types", h.listActionLevelTypes)
	r.Post("/action-level-types", h.createActionLevelType)
	r.Get("/action-levels", h.listActionLevels)
	r.Get("/action-levels/{id}", h.getActionLevelGroup)

	r.Get("/stats", h.stats)
	r.Get("/activity", h.activity)
	r.Post("/action-level-groups", h.createActionLevelGroup)
	r.Put("/action-level-groups/{id}", h.updateActionLevelGroup)
	r.Delete("/action-level-groups/{id}", h.deleteActionLevelGroup)
	r.Post("/action-level-groups/{id}/parameters", h.createActionLevelParameter)
	r.Put("/action-level-parameters/{id}", h.updateActionLevelParameter)
	r.Delete("/action-level-parameters/{id}", h.deleteActionLevelParameter)

	return r
}

// --- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeStoreErr maps store errors onto HTTP status codes.
func (h *Handler) writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// Translate common PostgreSQL constraint outcomes into client errors.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			writeErr(w, http.StatusConflict, "duplicate record: "+pgErr.ConstraintName)
			return
		case "23503": // foreign_key_violation
			writeErr(w, http.StatusUnprocessableEntity, "referenced record does not exist")
			return
		case "23514": // check_violation
			writeErr(w, http.StatusUnprocessableEntity, "value violates a constraint")
			return
		}
	}
	h.logger.Error("request failed", "error", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func pagination(r *http.Request) (limit, offset int) {
	limit, offset = 100, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

type listResponse struct {
	Items  any `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// --- projects --------------------------------------------------------------

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListProjects(r.Context(), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	total, err := h.store.CountProjects(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if p.Code == "" || p.Name == "" {
		writeErr(w, http.StatusBadRequest, "code and name are required")
		return
	}
	if p.SRID == 0 {
		p.SRID = 4326
	}
	if p.ElevUnit == "" {
		p.ElevUnit = "ft"
	}
	out, err := h.store.CreateProject(r.Context(), p)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	out, err := h.store.UpdateProject(r.Context(), chi.URLParam(r, "id"), p)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteProject(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- tasks -----------------------------------------------------------------

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListTasks(r.Context(), r.URL.Query().Get("project_id"), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var t store.Task
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if t.ProjectID == "" || t.Code == "" {
		writeErr(w, http.StatusBadRequest, "project_id and code are required")
		return
	}
	out, err := h.store.CreateTask(r.Context(), t)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := h.store.GetTask(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteTask(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- locations -------------------------------------------------------------

func (h *Handler) listLocations(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListLocations(r.Context(), r.URL.Query().Get("project_id"), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	var l store.Location
	if err := decode(r, &l); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if l.ProjectID == "" || l.Name == "" || l.LocType == "" {
		writeErr(w, http.StatusBadRequest, "project_id, name and loc_type are required")
		return
	}
	if l.Status == "" {
		l.Status = "active"
	}
	out, err := h.store.CreateLocation(r.Context(), l)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getLocation(w http.ResponseWriter, r *http.Request) {
	l, err := h.store.GetLocation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Handler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteLocation(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listWellSegments(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListWellSegments(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWellSegment(w http.ResponseWriter, r *http.Request) {
	var s store.WellSegment
	if err := decode(r, &s); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	s.LocationID = chi.URLParam(r, "id")
	if s.SegmentType == "" || s.DepthUnit == "" {
		writeErr(w, http.StatusBadRequest, "segment_type and depth_unit are required")
		return
	}
	out, err := h.store.CreateWellSegment(r.Context(), s)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) deleteWellSegment(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteWellSegment(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- samples ---------------------------------------------------------------

func (h *Handler) listSamples(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	q := r.URL.Query()
	items, err := h.store.ListSamples(r.Context(), q.Get("project_id"),
		q.Get("task_id"), q.Get("location_id"), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

type sampleRequest struct {
	store.Sample
	SampleDatetimeStr string `json:"sample_datetime_rfc3339,omitempty"`
}

func (h *Handler) createSample(w http.ResponseWriter, r *http.Request) {
	var req sampleRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sm := req.Sample
	if sm.Name == "" || sm.TaskID == "" || sm.LocationID == "" || sm.Matrix == "" {
		writeErr(w, http.StatusBadRequest, "name, task_id, location_id and matrix are required")
		return
	}
	if sm.SampleType == "" {
		sm.SampleType = "N"
	}
	if sm.SampleDatetime.IsZero() {
		writeErr(w, http.StatusBadRequest, "sample_datetime is required (RFC3339)")
		return
	}
	out, err := h.store.CreateSample(r.Context(), sm)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getSample(w http.ResponseWriter, r *http.Request) {
	sm, err := h.store.GetSample(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sm)
}

func (h *Handler) deleteSample(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteSample(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- tests -----------------------------------------------------------------

func (h *Handler) listTests(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListTests(r.Context(), chi.URLParam(r, "id"), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) createTest(w http.ResponseWriter, r *http.Request) {
	var t store.Test
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if t.SampleID == "" || t.TestID == "" {
		writeErr(w, http.StatusBadRequest, "sample_id and test_id are required")
		return
	}
	out, err := h.store.CreateTest(r.Context(), t)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getTest(w http.ResponseWriter, r *http.Request) {
	t, err := h.store.GetTest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteTest(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteTest(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- results ---------------------------------------------------------------

func (h *Handler) listResults(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListResults(r.Context(), chi.URLParam(r, "id"), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) createResult(w http.ResponseWriter, r *http.Request) {
	var res store.Result
	if err := decode(r, &res); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if res.TestID == "" || res.AnalyteID == "" {
		writeErr(w, http.StatusBadRequest, "test_id and analyte_id are required")
		return
	}
	// Non-detect must not carry a numeric value.
	if res.DetectFlag != nil && *res.DetectFlag == "N" {
		res.ResultNumeric = nil
	}
	out, err := h.store.CreateResult(r.Context(), res)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getResult(w http.ResponseWriter, r *http.Request) {
	res, err := h.store.GetResult(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) deleteResult(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteResult(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- analytes, units -------------------------------------------------------

func (h *Handler) listAnalytes(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	items, err := h.store.ListAnalytes(r.Context(), limit, offset)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Limit: limit, Offset: offset})
}

func (h *Handler) listAnalyteGroups(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListAnalyteGroups(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createAnalyteGroup(w http.ResponseWriter, r *http.Request) {
	var g store.AnalyteGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if g.Code == "" || g.Name == "" {
		writeErr(w, http.StatusBadRequest, "code and name are required")
		return
	}
	out, err := h.store.CreateAnalyteGroup(r.Context(), g)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) updateAnalyteGroup(w http.ResponseWriter, r *http.Request) {
	var g store.AnalyteGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if g.Code == "" || g.Name == "" {
		writeErr(w, http.StatusBadRequest, "code and name are required")
		return
	}
	out, err := h.store.UpdateAnalyteGroup(r.Context(), chi.URLParam(r, "id"), g)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteAnalyteGroup(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAnalyteGroup(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) addAnalyteGroupMember(w http.ResponseWriter, r *http.Request) {
	var m store.AnalyteGroupMember
	if err := decode(r, &m); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if m.AnalyteID == "" {
		writeErr(w, http.StatusBadRequest, "analyte_id is required")
		return
	}
	out, err := h.store.AddAnalyteGroupMember(r.Context(), chi.URLParam(r, "id"), m)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) deleteAnalyteGroupMember(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAnalyteGroupMember(r.Context(), chi.URLParam(r, "memberId")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createAnalyte(w http.ResponseWriter, r *http.Request) {
	var a store.Analyte
	if err := decode(r, &a); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if a.ChemicalName == "" {
		writeErr(w, http.StatusBadRequest, "chemical_name is required")
		return
	}
	out, err := h.store.CreateAnalyte(r.Context(), a)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getAnalyte(w http.ResponseWriter, r *http.Request) {
	a, err := h.store.GetAnalyte(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) updateAnalyte(w http.ResponseWriter, r *http.Request) {
	var a store.Analyte
	if err := decode(r, &a); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if a.ChemicalName == "" {
		writeErr(w, http.StatusBadRequest, "chemical_name is required")
		return
	}
	out, err := h.store.UpdateAnalyte(r.Context(), chi.URLParam(r, "id"), a)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteAnalyte(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAnalyte(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listUnits(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListUnits(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listConversions(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListConversions(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// --- action levels ---------------------------------------------------------

// stats returns company-wide counts plus per-project rollups.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.Stats(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// activity returns the recent-activity feed.
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.RecentActivity(r.Context(), limit)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listActionLevels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := h.store.ListActionLevelGroups(r.Context(), q.Get("type"), q.Get("project_id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getActionLevelGroup(w http.ResponseWriter, r *http.Request) {
	g, err := h.store.GetActionLevelGroup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) listActionLevelTypes(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListActionLevelTypes(r.Context())
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createActionLevelType(w http.ResponseWriter, r *http.Request) {
	var t store.ActionLevelType
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if t.Code == "" || t.Name == "" {
		writeErr(w, http.StatusBadRequest, "code and name are required")
		return
	}
	if err := h.store.CreateActionLevelType(r.Context(), t.Code, t.Name, t.Description); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

type createGroupRequest struct {
	store.ActionLevelGroup
	TypeCode string `json:"type_code"`
}

func (h *Handler) createActionLevelGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.TypeCode == "" || req.Code == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "type_code, code and name are required")
		return
	}
	id, err := h.store.CreateActionLevelGroup(r.Context(), req.TypeCode, req.ActionLevelGroup)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) updateActionLevelGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.TypeCode == "" || req.Code == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "type_code, code and name are required")
		return
	}
	out, err := h.store.UpdateActionLevelGroup(r.Context(), chi.URLParam(r, "id"), req.ActionLevelGroup)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteActionLevelGroup(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteActionLevelGroup(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createActionLevelParameter(w http.ResponseWriter, r *http.Request) {
	var p store.ActionLevelParameter
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if p.AnalyteID == "" || p.Unit == "" {
		writeErr(w, http.StatusBadRequest, "analyte_id and unit are required")
		return
	}
	id, err := h.store.CreateActionLevelParameter(r.Context(), chi.URLParam(r, "id"), p)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) updateActionLevelParameter(w http.ResponseWriter, r *http.Request) {
	var p store.ActionLevelParameter
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if p.AnalyteID == "" || p.Unit == "" {
		writeErr(w, http.StatusBadRequest, "analyte_id and unit are required")
		return
	}
	out, err := h.store.UpdateActionLevelParameter(r.Context(), chi.URLParam(r, "id"), p)
	if err != nil {
		h.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteActionLevelParameter(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteActionLevelParameter(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unused guard to keep time import meaningful for future timestamp handling.
var _ = time.Time{}
