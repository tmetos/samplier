// Package ogc implements the OGC API — Features endpoints (Part 1: Core,
// GeoJSON encoding). M0 scope: landing page, conformance declaration,
// collections, collection metadata, and feature/feature-collection queries with
// bbox / datetime / limit / offset over PostGIS.
package ogc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/ogcquery"
)

// Handler serves OGC API — Features resources.
type Handler struct {
	pool *pgxpool.Pool
}

// New constructs an OGC handler backed by the given connection pool.
func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// Prop describes a feature property exposed in GeoJSON responses.
type Prop struct {
	Name string // JSON property name
	Expr string // SQL expression (qualified column)
}

// Collection describes one OGC feature collection and how to read it.
type Collection struct {
	ID             string
	Title          string
	Description    string
	FromSQL        string // FROM clause (table or join)
	IDColumn       string // qualified id column
	GeomColumn     string // qualified geometry column
	DatetimeColumn string // optional; may be empty
	Props          []Prop
}

var collections = []Collection{
	{
		ID: "projects", Title: "Projects", Description: "Project (site) boundaries and metadata",
		FromSQL: "project p", IDColumn: "p.id", GeomColumn: "p.geometry", DatetimeColumn: "p.start_time",
		Props: []Prop{{"code", "p.code"}, {"name", "p.name"}, {"srid", "p.srid"},
			{"start_time", "p.start_time"}, {"end_time", "p.end_time"}},
	},
	{
		ID: "tasks", Title: "Tasks", Description: "Sampling events / field programs",
		FromSQL: "task t", IDColumn: "t.id", GeomColumn: "t.geometry", DatetimeColumn: "t.start_date",
		Props: []Prop{{"code", "t.code"}, {"name", "t.name"}, {"start_date", "t.start_date"},
			{"end_date", "t.end_date"}, {"project_id", "t.project_id::text"}},
	},
	{
		ID: "locations", Title: "Locations", Description: "Sampling locations (wells, borings, stations)",
		FromSQL: "location l", IDColumn: "l.id", GeomColumn: "l.geometry", DatetimeColumn: "",
		Props: []Prop{{"sys_loc_code", "l.sys_loc_code"}, {"name", "l.name"},
			{"loc_type", "l.loc_type"}, {"status", "l.status"}, {"project_id", "l.project_id::text"}},
	},
	{
		ID: "samples", Title: "Samples", Description: "Samples with geometry inherited from their location",
		FromSQL: "sample s JOIN location l ON l.id = s.location_id", IDColumn: "s.id",
		GeomColumn: "l.geometry", DatetimeColumn: "s.sample_datetime",
		Props: []Prop{{"sys_sample_code", "s.sys_sample_code"}, {"name", "s.name"},
			{"sample_type", "s.sample_type"}, {"matrix", "s.matrix"},
			{"sample_datetime", "s.sample_datetime"}, {"task_id", "s.task_id::text"},
			{"location_id", "s.location_id::text"}},
	},
}

var conformanceClasses = []string{
	"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core",
	"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson",
	"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/oas30",
}

const openAPIDoc = `{
  "openapi": "3.0.3",
  "info": {"title": "Samplier OGC API — Features", "version": "0.1.0"},
  "paths": {
    "/ogc/collections/{collectionId}/items": {
      "get": {
        "summary": "Fetch features",
        "parameters": [
          {"name": "collectionId", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "bbox", "in": "query", "schema": {"type": "string"}},
          {"name": "datetime", "in": "query", "schema": {"type": "string"}},
          {"name": "limit", "in": "query", "schema": {"type": "integer"}},
          {"name": "offset", "in": "query", "schema": {"type": "integer"}}
        ],
        "responses": {"200": {"description": "GeoJSON FeatureCollection"}}
      }
    }
  }
}`

// Routes returns the OGC subrouter.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", h.landing)
	r.Get("/api", h.apiDefinition)
	r.Get("/conformance", h.conformance)
	r.Get("/collections", h.collections)
	r.Get("/collections/{collectionId}", h.collection)
	r.Get("/collections/{collectionId}/items", h.items)
	r.Get("/collections/{collectionId}/items/{featureId}", h.item)

	return r
}

func (h *Handler) landing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"title":       "Samplier OGC API — Features",
		"description": "Geospatial feature access for environmental sample data",
		"links": []map[string]string{
			{"href": "/ogc/conformance", "rel": "conformance", "type": "application/json", "title": "Conformance classes"},
			{"href": "/ogc/collections", "rel": "data", "type": "application/json", "title": "Feature collections"},
			{"href": "/ogc/api", "rel": "service-desc", "type": "application/vnd.oai.openapi+json;version=3.0", "title": "API definition"},
		},
	})
}

func (h *Handler) apiDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.0")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openAPIDoc))
}

func (h *Handler) conformance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"conformsTo": conformanceClasses})
}

func (h *Handler) collections(w http.ResponseWriter, r *http.Request) {
	list := make([]map[string]any, 0, len(collections))
	for _, c := range collections {
		list = append(list, collectionDoc(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"collections": list,
		"links":       []map[string]string{{"href": "/ogc/collections", "rel": "self", "type": "application/json"}},
	})
}

func (h *Handler) collection(w http.ResponseWriter, r *http.Request) {
	c, ok := findCollection(chi.URLParam(r, "collectionId"))
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, collectionDoc(c))
}

func collectionDoc(c Collection) map[string]any {
	return map[string]any{
		"id":          c.ID,
		"title":       c.Title,
		"description": c.Description,
		"itemType":    "feature",
		"crs":         []string{"http://www.opengis.net/def/crs/OGC/1.3/CRS84"},
		"links": []map[string]string{
			{"href": "/ogc/collections/" + c.ID + "/items", "rel": "items", "type": "application/geo+json"},
			{"href": "/ogc/collections/" + c.ID, "rel": "self", "type": "application/json"},
		},
	}
}

func (h *Handler) items(w http.ResponseWriter, r *http.Request) {
	c, ok := findCollection(chi.URLParam(r, "collectionId"))
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}

	q := r.URL.Query()
	p := ogcquery.Parse(q.Get("limit"), q.Get("offset"), q.Get("bbox"), q.Get("datetime"))
	limit, offset := p.Limit, p.Offset

	where := []string{c.GeomColumn + " IS NOT NULL"}
	args := []any{}
	argN := 0
	// add appends v to args and returns the next positional placeholder.
	add := func(v any) string {
		argN++
		args = append(args, v)
		return "$" + strconv.Itoa(argN)
	}

	if b := p.BBox; b != nil {
		where = append(where, fmt.Sprintf("%s && ST_MakeEnvelope(%s,%s,%s,%s,4326)",
			c.GeomColumn, add(b.MinLon), add(b.MinLat), add(b.MaxLon), add(b.MaxLat)))
	}

	if d := p.Datetime; d != nil && c.DatetimeColumn != "" {
		switch {
		case d.Start != nil && d.End != nil:
			where = append(where, fmt.Sprintf("%s >= %s AND %s < %s",
				c.DatetimeColumn, add(*d.Start), c.DatetimeColumn, add(*d.End)))
		case d.Start != nil:
			where = append(where, fmt.Sprintf("%s >= %s", c.DatetimeColumn, add(*d.Start)))
		case d.End != nil:
			where = append(where, fmt.Sprintf("%s < %s", c.DatetimeColumn, add(*d.End)))
		}
	}

	whereSQL := strings.Join(where, " AND ")

	// Count matching features (for numberMatched) using only the WHERE args.
	countQuery := "SELECT count(*) FROM " + c.FromSQL + " WHERE " + whereSQL
	var numberMatched int
	if err := h.pool.QueryRow(r.Context(), countQuery, args...).Scan(&numberMatched); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cols := selectColumns(c)
	limitPH := add(limit)
	offsetPH := add(offset)
	query := "SELECT " + strings.Join(cols, ", ") + " FROM " + c.FromSQL +
		" WHERE " + whereSQL +
		" ORDER BY " + c.IDColumn + " LIMIT " + limitPH + " OFFSET " + offsetPH

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	features, err := scanFeatures(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	next := ""
	if len(features) == limit {
		next = fmt.Sprintf("/ogc/collections/%s/items?limit=%d&offset=%d", c.ID, limit, offset+limit)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"type":           "FeatureCollection",
		"numberMatched":  numberMatched,
		"numberReturned": len(features),
		"features":       features,
		"links":          linksFor(next),
	})
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	c, ok := findCollection(chi.URLParam(r, "collectionId"))
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	id := chi.URLParam(r, "featureId")

	cols := selectColumns(c)
	query := "SELECT " + strings.Join(cols, ", ") + " FROM " + c.FromSQL +
		" WHERE " + c.IDColumn + "::text = $1 LIMIT 1"

	rows, err := h.pool.Query(r.Context(), query, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	features, err := scanFeatures(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(features) == 0 {
		writeError(w, http.StatusNotFound, "feature not found")
		return
	}
	writeJSON(w, http.StatusOK, features[0])
}

// --- helpers ---

func selectColumns(c Collection) []string {
	cols := []string{c.IDColumn + "::text AS id", "ST_AsGeoJSON(" + c.GeomColumn + ")::text AS geom"}
	for _, p := range c.Props {
		cols = append(cols, p.Expr+" AS "+quoteIdent(p.Name))
	}
	return cols
}

func scanFeatures(rows pgx.Rows) ([]map[string]any, error) {
	features := make([]map[string]any, 0)
	fds := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		feature := map[string]any{"type": "Feature"}
		props := map[string]any{}
		for i, fd := range fds {
			name := string(fd.Name)
			switch name {
			case "id":
				feature["id"] = toJSON(vals[i])
			case "geom":
				if s, ok := vals[i].(string); ok && s != "" {
					feature["geometry"] = json.RawMessage(s)
				} else {
					feature["geometry"] = nil
				}
			default:
				props[name] = toJSON(vals[i])
			}
		}
		feature["properties"] = props
		features = append(features, feature)
	}
	return features, rows.Err()
}

func toJSON(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case pgtype.Numeric:
		f, err := t.Float64Value()
		if err == nil && f.Valid {
			return f.Float64
		}
		return nil
	case int16, int32, int64, float32, float64, bool:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func findCollection(id string) (Collection, bool) {
	for _, c := range collections {
		if c.ID == id {
			return c, true
		}
	}
	return Collection{}, false
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func linksFor(next string) []map[string]string {
	links := []map[string]string{
		{"href": "", "rel": "self", "type": "application/geo+json"},
	}
	if next != "" {
		links = append(links, map[string]string{"href": next, "rel": "next", "type": "application/geo+json"})
	}
	return links
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
