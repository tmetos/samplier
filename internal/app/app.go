// Package app assembles the Samplier HTTP server: OGC API Features, the REST
// API, EDD import/export endpoints, and auth middleware.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/samplier/samplier/internal/api"
	"github.com/samplier/samplier/internal/auth"
	"github.com/samplier/samplier/internal/basemap"
	"github.com/samplier/samplier/internal/edd"
	"github.com/samplier/samplier/internal/export"
	"github.com/samplier/samplier/internal/ogc"
	"github.com/samplier/samplier/internal/store"
)

// Config holds HTTP-server configuration.
type Config struct {
	// AuthEnabled toggles OIDC enforcement.
	AuthEnabled bool
	// OIDCIssuer and OIDCClientID configure the verifier.
	OIDCIssuer   string
	OIDCClientID string
	// MaxUploadBytes caps EDD uploads.
	MaxUploadBytes int64
	// Version is reported by /healthz and the OGC landing page.
	Version string
	// Basemap selects the self-hosted basemap provider and endpoints.
	Basemap basemap.Config
}

// Server bundles the assembled HTTP handler and its dependencies.
type Server struct {
	Handler http.Handler
	Pool    *pgxpool.Pool
	Store   *store.Store
	Logger  *slog.Logger
}

// New builds the HTTP server.
func New(pool *pgxpool.Pool, cfg Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxUploadBytes == 0 {
		cfg.MaxUploadBytes = 32 << 20 // 32 MiB
	}

	st := store.New(pool)
	verifier := auth.NewVerifier(cfg.OIDCIssuer, cfg.OIDCClientID)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if err := pool.Ping(req.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "version": cfg.Version,
		})
	})

	// Self-hosted MapLibre basemap (Natural Earth or OpenMapTiles/Dark Matter).
	// Registered at the top level so StripPrefix sees the absolute path.
	basemapHandler := http.StripPrefix("/ogc/basemap", basemap.HandlerFor(cfg.Basemap))
	r.Handle("/ogc/basemap", basemapHandler)
	r.Handle("/ogc/basemap/*", basemapHandler)

	// OGC API — Features is public by design: it is the map-serving surface.
	r.Mount("/ogc", ogc.New(pool).Routes())

	// REST API is protected.
	apiHandler := api.New(st, logger)
	r.Group(func(gr chi.Router) {
		gr.Use(verifier.Middleware(cfg.AuthEnabled))
		gr.Mount("/api/v1", apiHandler.Routes())

		ed := &eddHandlers{pool: pool, store: st, logger: logger, maxUpload: cfg.MaxUploadBytes}
		gr.Post("/api/v1/edd/validate", ed.validate)
		gr.Post("/api/v1/edd/load", ed.load)
		gr.Get("/api/v1/edd/formats", ed.formats)
		gr.Get("/api/v1/edd/batches", ed.batches)

		ex := &exportHandlers{pool: pool, store: st, logger: logger}
		gr.Get("/api/v1/exports/formats", ex.formats)
		gr.Get("/api/v1/exports/{format}", ex.download)
		gr.Get("/api/v1/exceedances", ex.exceedances)

		rp := &reportHandlers{pool: pool, store: st, logger: logger}
		gr.Post("/api/v1/reports/preview", rp.preview)
		gr.Post("/api/v1/reports/export", rp.export)
		gr.Get("/api/v1/reports", rp.list)
		gr.Post("/api/v1/reports", rp.create)
		gr.Get("/api/v1/reports/{id}", rp.get)
		gr.Put("/api/v1/reports/{id}", rp.update)
		gr.Delete("/api/v1/reports/{id}", rp.delete)
		gr.Get("/api/v1/reports/{id}/preview", rp.previewSaved)
		gr.Get("/api/v1/reports/{id}/export", rp.exportSaved)
	})

	// Wrap for tracing/metrics (RED).
	handler := otelhttp.NewHandler(r, "samplier-api")

	return &Server{Handler: handler, Pool: pool, Store: st, Logger: logger}
}

// requestLogger logs one line per request with trace correlation.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
			)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// EDD handlers
// ---------------------------------------------------------------------------

type eddHandlers struct {
	pool      *pgxpool.Pool
	store     *store.Store
	logger    *slog.Logger
	maxUpload int64
}

// referenceSet loads valid reference codes for EDD validation.
func (h *eddHandlers) referenceSet(req *http.Request) edd.ReferenceSet {
	refs := edd.ReferenceSet{}
	queries := map[string]string{
		"MATRIX":      "SELECT code FROM matrix",
		"SAMPLE_TYPE": "SELECT code FROM sample_type",
		"UNIT":        "SELECT code FROM unit",
		"METHOD":      "SELECT code FROM method",
		"FRACTION":    "SELECT code FROM fraction",
		"TEST_TYPE":   "SELECT code FROM test_type",
		"LAB":         "SELECT code FROM lab",
	}
	for table, q := range queries {
		rows, err := h.pool.Query(req.Context(), q)
		if err != nil {
			continue
		}
		var codes []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err == nil {
				codes = append(codes, c)
			}
		}
		rows.Close()
		refs[table] = codes
	}
	return refs
}

// readWorkbook parses an uploaded EDD file.
func (h *eddHandlers) readWorkbook(req *http.Request, def edd.Definition) (*edd.Workbook, error) {
	if err := req.ParseMultipartForm(h.maxUpload); err != nil {
		return nil, fmt.Errorf("parse upload: %w", err)
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing 'file' field: %w", err)
	}
	defer file.Close()

	if header.Size > h.maxUpload {
		return nil, fmt.Errorf("file exceeds %d bytes", h.maxUpload)
	}

	// excelize and archive/zip need random access, so buffer.
	buf, err := io.ReadAll(io.LimitReader(file, h.maxUpload))
	if err != nil {
		return nil, err
	}
	return edd.Read(bytes.NewReader(buf), int64(len(buf)), header.Filename, def)
}

func (h *eddHandlers) formats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": edd.Formats()})
}

func (h *eddHandlers) validate(w http.ResponseWriter, r *http.Request) {
	formatCode := r.URL.Query().Get("format")
	if formatCode == "" {
		formatCode = "EFWEDD"
	}
	def, err := edd.Lookup(formatCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	wb, err := h.readWorkbook(r, def)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	report := edd.NewValidator(def, h.referenceSet(r)).Validate(wb)
	writeJSON(w, http.StatusOK, map[string]any{
		"format":   def.Code,
		"valid":    report.Valid(),
		"errors":   len(report.Errors()),
		"warnings": len(report.Warnings()),
		"findings": report.Findings,
		"sections": wb.Order,
	})
}

func (h *eddHandlers) load(w http.ResponseWriter, r *http.Request) {
	formatCode := r.URL.Query().Get("format")
	if formatCode == "" {
		formatCode = "EFWEDD"
	}
	def, err := edd.Lookup(formatCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	wb, err := h.readWorkbook(r, def)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	report := edd.NewValidator(def, h.referenceSet(r)).Validate(wb)
	if !report.Valid() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "validation failed",
			"errors":   len(report.Errors()),
			"findings": report.Errors(),
		})
		return
	}

	opts := edd.LoadOptions{
		ProjectID:  r.FormValue("project_id"),
		TaskID:     r.FormValue("task_id"),
		CommitMode: edd.CommitMode(strings.ToUpper(r.FormValue("commit_mode"))),
	}
	if opts.CommitMode == "" {
		opts.CommitMode = edd.InsertUpdate
	}
	if opts.ProjectID == "" || opts.TaskID == "" {
		writeError(w, http.StatusBadRequest, "project_id and task_id are required")
		return
	}

	// Record the batch for audit.
	var batchID string
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO edd_batch (format_code, file_name, commit_mode, status, error_count, warning_count)
		 VALUES ($1,$2,$3,'VALIDATED',$4,$5) RETURNING id`,
		def.Code, r.FormValue("file_name"), opts.CommitMode,
		len(report.Errors()), len(report.Warnings())).Scan(&batchID)
	if err != nil {
		// Non-fatal: continue the load even if auditing fails.
		h.logger.Warn("could not record EDD batch", "error", err)
	}
	opts.EDDBatchID = batchID

	res, err := edd.NewLoader(h.pool).Load(r.Context(), wb, def, opts)
	if err != nil {
		if batchID != "" {
			_, _ = h.pool.Exec(r.Context(),
				`UPDATE edd_batch SET status='ERROR' WHERE id=$1`, batchID)
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if batchID != "" {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE edd_batch SET status='COMMITTED', committed_at=now() WHERE id=$1`, batchID)
		// Persist findings.
		for _, f := range report.Findings {
			_, _ = h.pool.Exec(r.Context(),
				`INSERT INTO edd_validation_error (edd_batch_id, section, row_number, field, message, severity)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				batchID, f.Section, f.Row, f.Field, f.Message, string(f.Severity))
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id": batchID,
		"loaded":   res,
		"warnings": len(report.Warnings()),
	})
}

func (h *eddHandlers) batches(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, format_code, file_name, commit_mode, status, error_count,
		        warning_count, created_at, committed_at
		 FROM edd_batch ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, format, name, mode, status string
		var errCount, warnCount int
		var created any
		var committed any
		if err := rows.Scan(&id, &format, &name, &mode, &status, &errCount,
			&warnCount, &created, &committed); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "format": format, "file_name": name, "commit_mode": mode,
			"status": status, "errors": errCount, "warnings": warnCount,
			"created_at": created, "committed_at": committed,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ---------------------------------------------------------------------------
// Export handlers
// ---------------------------------------------------------------------------

type exportHandlers struct {
	pool   *pgxpool.Pool
	store  *store.Store
	logger *slog.Logger
}

func (h *exportHandlers) formats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": export.Codes()})
}

// download builds a dataset from query filters and streams the export.
func (h *exportHandlers) download(w http.ResponseWriter, r *http.Request) {
	formatCode := chi.URLParam(r, "format")
	adapter, err := export.Get(formatCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	filters, err := parseResultFilters(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.loadRows(r, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "no results matched the query")
		return
	}

	ds := buildDataset(rows, r.URL.Query().Get("title"))
	data, err := adapter.Encode(ds, export.Options{
		LimitToDetects: r.URL.Query().Get("detects_only") == "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := sanitize(ds.ProjectCode) + "_" + strings.ToLower(adapter.Code()) + adapter.FileExtension()
	w.Header().Set("Content-Type", adapter.ContentType())
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *exportHandlers) exceedances(w http.ResponseWriter, r *http.Request) {
	filters, err := parseResultFilters(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := h.loadRows(r, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var out []map[string]any
	for _, row := range rows {
		if !row.Exceeded {
			continue
		}
		out = append(out, map[string]any{
			"location": row.LocationName, "sample": row.SampleName,
			"sample_date": exportDay(row.SampleDate),
			"analyte":     row.AnalyteName, "cas_rn": row.CASRN,
			// Human-readable value plus the raw fields, so clients can render
			// non-detects and qualifiers themselves.
			"result":                    row.ReportedValue(),
			"result_numeric":            row.ResultNumeric,
			"detect_flag":               row.DetectFlag,
			"reporting_detection_limit": row.RDL,
			"unit":                      row.Units,
			"action_level":              row.ActionLevel, "group": row.ActionLevelGroup,
			"type": row.ActionLevelType,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "items": out})
}

// resultFilters are query parameters shared by export endpoints.
type resultFilters struct {
	ProjectID string
	Location  string
	CASRN     string
	StartDate string
	EndDate   string
	Type      string
	Limit     int
}

func parseResultFilters(r *http.Request) (resultFilters, error) {
	q := r.URL.Query()
	f := resultFilters{
		ProjectID: q.Get("project_id"),
		Location:  q.Get("location"),
		CASRN:     q.Get("cas_rn"),
		StartDate: q.Get("start_date"),
		EndDate:   q.Get("end_date"),
		Type:      q.Get("type"),
	}
	if f.ProjectID == "" {
		return f, errors.New("project_id is required")
	}
	return f, nil
}

// loadRows queries denormalised results and evaluates action levels.
func (h *exportHandlers) loadRows(r *http.Request, f resultFilters) ([]export.Row, error) {
	conds := []string{"p.id = $1"}
	args := []any{f.ProjectID}
	add := func(v any, cond string) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.Location != "" {
		add(f.Location, "l.name ILIKE $%d")
	}
	if f.CASRN != "" {
		add(f.CASRN, "a.cas_rn = $%d")
	}
	if f.StartDate != "" {
		add(f.StartDate, "s.sample_datetime >= $%d::date")
	}
	if f.EndDate != "" {
		add(f.EndDate, "s.sample_datetime < ($%d::date + interval '1 day')")
	}
	args = append(args, 20000)

	query := `
SELECT p.id::text, p.code, p.name, l.name, COALESCE(l.loc_type,''), ST_X(l.geometry), ST_Y(l.geometry),
       s.name, COALESCE(s.sample_type,''), COALESCE(s.matrix,''), s.sample_datetime,
       s.start_depth, s.end_depth, COALESCE(s.depth_unit,''), COALESCE(tk.code,''),
       ts.test_id, COALESCE(ts.analytic_method,''), COALESCE(ts.fraction,''),
       ts.analysis_date, COALESCE(b.name,''), COALESCE(ts.lab,''),
       COALESCE(a.cas_rn,''), a.chemical_name, r.analyte_id::text,
       r.result_numeric, COALESCE(r.result_text,''), COALESCE(r.detect_flag,'Y'),
       COALESCE(r.units,''), r.reporting_detection_limit, r.method_detection_limit,
       COALESCE(r.detection_limit_unit,''), COALESCE(r.lab_qualifiers,''),
       COALESCE(r.validator_qualifiers,'')
FROM result r
JOIN test ts ON ts.id = r.test_id
JOIN sample s ON s.id = ts.sample_id
JOIN task tk ON tk.id = s.task_id
JOIN project p ON p.id = tk.project_id
JOIN location l ON l.id = s.location_id
JOIN analyte a ON a.id = r.analyte_id
LEFT JOIN batch b ON b.id = ts.batch_id
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY l.name, s.sample_datetime, a.chemical_name
LIMIT $` + fmt.Sprint(len(args))

	dbRows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()

	var rows []export.Row
	for dbRows.Next() {
		var row export.Row
		if err := dbRows.Scan(
			&row.ProjectID, &row.ProjectCode, &row.ProjectName, &row.LocationName, &row.LocationType,
			&row.Longitude, &row.Latitude, &row.SampleName, &row.SampleType,
			&row.Matrix, &row.SampleDate, &row.StartDepth, &row.EndDepth,
			&row.DepthUnit, &row.TaskCode, &row.TestID, &row.Method, &row.Fraction,
			&row.AnalysisDate, &row.LabSDG, &row.Lab, &row.CASRN, &row.AnalyteName,
			&row.AnalyteID, &row.ResultNumeric, &row.ResultText, &row.DetectFlag,
			&row.Units, &row.RDL, &row.MDL, &row.LimitUnit, &row.LabQualifier,
			&row.ValQualifier,
		); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}

	if err := h.applyActionLevels(r, rows, f); err != nil {
		h.logger.Warn("action level evaluation failed", "error", err)
	}
	return rows, nil
}

func buildDataset(rows []export.Row, title string) export.Dataset {
	ds := export.Dataset{Title: title, Rows: rows, GeneratedAt: time.Now().UTC()}
	if len(rows) > 0 {
		ds.ProjectCode = rows[0].ProjectCode
		ds.ProjectName = rows[0].ProjectName
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ActionLevelGroup != "" && !seen[r.ActionLevelGroup] {
			seen[r.ActionLevelGroup] = true
			ds.ActionLevelGroups = append(ds.ActionLevelGroups, r.ActionLevelGroup)
		}
	}
	return ds
}

func sanitize(s string) string {
	if s == "" {
		return "samplier"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
