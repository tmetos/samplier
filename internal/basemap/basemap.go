// Package basemap serves a self-hosted MapLibre basemap so Samplier needs no
// third-party tile service (no API keys, no external requests from the browser).
//
// It embeds public-domain Natural Earth 1:110m land polygons and country
// boundary lines, then composes a MapLibre style that layers Samplier's own
// OGC API — Features collections on top.
package basemap

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed data/land.geojson
var landGeoJSON []byte

//go:embed data/boundaries.geojson
var boundariesGeoJSON []byte

//go:embed data/lakes.geojson
var lakesGeoJSON []byte

//go:embed data/rivers_lake_centerlines.geojson
var riversGeoJSON []byte

// Attribution for the embedded Natural Earth data (public domain).
const attribution = "Natural Earth (public domain) · Samplier OGC API"

// Land returns the embedded land polygons as GeoJSON.
func Land() []byte { return landGeoJSON }

// Boundaries returns the embedded country boundary lines as GeoJSON.
func Boundaries() []byte { return boundariesGeoJSON }

// Lakes returns the embedded major lake polygons as GeoJSON.
func Lakes() []byte { return lakesGeoJSON }

// Rivers returns the embedded major river centrelines as GeoJSON.
func Rivers() []byte { return riversGeoJSON }

// FeatureLayer describes a Samplier OGC collection rendered on the map.
type FeatureLayer struct {
	Collection string
	Color      string
	Radius     int
	MinZoom    int
	// Hollow draws a ring (transparent fill) instead of a filled dot.
	Hollow bool
	// Area declares that the collection may hold polygon geometry (site
	// boundaries, task extents) and should render as fill + outline.
	Area bool
	// Label is the human name shown in legend/UI.
	Label string
}

// Layers are the Samplier collections exposed in the default style. Order is
// draw order: boundaries underneath, points on top.
var Layers = []FeatureLayer{
	{Collection: "projects", Label: "Projects", Color: "#8fb3d9", Radius: 5, Area: true},
	{Collection: "tasks", Label: "Tasks", Color: "#e0a952", Radius: 5, Area: true},
	{Collection: "locations", Label: "Locations", Color: "#35b37e", Radius: 6},
	// Samples share their location's geometry, so they are drawn last as a
	// hollow ring: both layers remain visible when overlaid.
	{Collection: "samples", Label: "Samples", Color: "#6fa8dc", Radius: 9, Hollow: true},
}

// geometryFilter builds a MapLibre expression matching the given geometry types.
func geometryFilter(types ...string) []any {
	if len(types) == 1 {
		return []any{"==", []any{"geometry-type"}, types[0]}
	}
	// Use "any" of equality expressions: the legacy filter form
	// ["in", "$type", ...] is ambiguous with the expression form
	// ["in", value, array] and is misread by MapLibre.
	branches := make([]any, 0, len(types)+1)
	branches = append(branches, "any")
	for _, t := range types {
		branches = append(branches, []any{"==", []any{"geometry-type"}, t})
	}
	return branches
}

// Style builds the MapLibre style document. All sources are relative URLs
// served by this same API, so the map works offline and without keys.
func Style(basePath string) map[string]any {
	if basePath == "" {
		basePath = "/ogc"
	}

	sources := map[string]any{
		"ne_land": map[string]any{
			"type": "geojson",
			"data": basePath + "/basemap/land",
		},
		"ne_boundaries": map[string]any{
			"type": "geojson",
			"data": basePath + "/basemap/boundaries",
		},
		"ne_lakes": map[string]any{
			"type": "geojson",
			"data": basePath + "/basemap/lakes",
		},
		"ne_rivers": map[string]any{
			"type": "geojson",
			"data": basePath + "/basemap/rivers",
		},
	}

	var layers []any

	// Base canvas.
	layers = append(layers, map[string]any{
		"id":   "background",
		"type": "background",
		"paint": map[string]any{
			"background-color": "#070c10",
		},
	})

	// Ocean-ish graticule feel: land fill on the dark canvas.
	layers = append(layers, map[string]any{
		"id":     "land",
		"type":   "fill",
		"source": "ne_land",
		"paint": map[string]any{
			"fill-color":   "#1b2a32",
			"fill-opacity": 1.0,
		},
	})

	layers = append(layers, map[string]any{
		"id":     "land-outline",
		"type":   "line",
		"source": "ne_land",
		"paint": map[string]any{
			"line-color": "#3f6373",
			"line-width": 0.8,
		},
	})

	// Inland water gives regional context where the coastline does not.
	layers = append(layers, map[string]any{
		"id":     "lakes",
		"type":   "fill",
		"source": "ne_lakes",
		"paint": map[string]any{
			"fill-color":   "#0b1620",
			"fill-opacity": 1.0,
		},
	})

	layers = append(layers, map[string]any{
		"id":     "rivers",
		"type":   "line",
		"source": "ne_rivers",
		"paint": map[string]any{
			"line-color": "#22485c",
			"line-width": 1.1,
		},
	})

	layers = append(layers, map[string]any{
		"id":     "boundaries",
		"type":   "line",
		"source": "ne_boundaries",
		"paint": map[string]any{
			"line-color":     "#3a5561",
			"line-width":     0.5,
			"line-dasharray": []float64{2, 2},
		},
	})

	// Samplier collections, drawn above the basemap.
	//
	// Collections that can carry polygons (projects, tasks) get a fill +
	// outline filtered to area geometry, plus a circle filtered to points, so
	// a site boundary renders as a boundary while a point project still shows
	// as a marker. Point-only collections get circles.
	for _, l := range Layers {
		id := "samplier-" + l.Collection
		geometryFilterPoint := geometryFilter("Point", "MultiPoint")

		sources[id] = map[string]any{
			"type": "geojson",
			"data": basePath + "/collections/" + l.Collection + "/items?limit=2000",
		}

		if l.Area {
			areaFilter := geometryFilter("Polygon", "MultiPolygon")

			// A soft outer glow separates the area from the basemap without
			// drawing attention away from the data inside it.
			layers = append(layers, map[string]any{
				"id":      id + "-glow",
				"type":    "line",
				"source":  id,
				"filter":  areaFilter,
				"minzoom": l.MinZoom,
				"paint": map[string]any{
					"line-color":   l.Color,
					"line-width":   9,
					"line-opacity": 0.16,
					"line-blur":    6,
				},
			})

			// Solid fill so the site reads as an area, not an outline.
			layers = append(layers, map[string]any{
				"id":      id + "-fill",
				"type":    "fill",
				"source":  id,
				"filter":  areaFilter,
				"minzoom": l.MinZoom,
				"paint": map[string]any{
					"fill-color":   l.Color,
					"fill-opacity": 0.26,
				},
			})

			// Continuous outline: dashed lines read as a point set at small
			// sizes, which is exactly the wrong impression for a site boundary.
			layers = append(layers, map[string]any{
				"id":      id + "-outline",
				"type":    "line",
				"source":  id,
				"filter":  areaFilter,
				"minzoom": l.MinZoom,
				"layout": map[string]any{
					"line-join": "round",
					"line-cap":  "round",
				},
				"paint": map[string]any{
					"line-color":   l.Color,
					"line-width":   2,
					"line-opacity": 1.0,
				},
			})
		}

		// A halo improves contrast for filled markers. Hollow rings must not
		// get one: it would paint over the marker they encircle (samples over
		// their location).
		if !l.Hollow {
			layers = append(layers, map[string]any{
				"id":      id + "-halo",
				"type":    "circle",
				"source":  id,
				"filter":  geometryFilterPoint,
				"minzoom": l.MinZoom,
				"paint": map[string]any{
					"circle-radius":  float64(l.Radius) + 2.5,
					"circle-color":   "#070c10",
					"circle-opacity": 0.9,
				},
			})
		}

		if l.Hollow {
			layers = append(layers, map[string]any{
				"id":      id,
				"type":    "circle",
				"source":  id,
				"filter":  geometryFilterPoint,
				"minzoom": l.MinZoom,
				"paint": map[string]any{
					"circle-radius":       float64(l.Radius),
					"circle-color":        "rgba(0,0,0,0)",
					"circle-stroke-width": 2,
					"circle-stroke-color": l.Color,
				},
			})
			continue
		}
		layers = append(layers, map[string]any{
			"id":      id,
			"type":    "circle",
			"source":  id,
			"filter":  geometryFilterPoint,
			"minzoom": l.MinZoom,
			"paint": map[string]any{
				"circle-radius":       float64(l.Radius),
				"circle-color":        l.Color,
				"circle-stroke-width": 1,
				"circle-stroke-color": "#070c10",
			},
		})
	}

	return map[string]any{
		"version": 8,
		"name":    "Samplier (self-hosted)",
		"metadata": map[string]any{
			"samplier:basemap": "natural-earth-110m",
		},
		"sources": sources,
		"layers":  layers,
	}
}

// Handler serves the basemap style and the embedded GeoJSON.
func Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/style.json", func(w http.ResponseWriter, r *http.Request) {
		// Derive the mount path so the style works behind any prefix.
		writeGeoJSON(w, Style(basePrefix(r)))
	})

	mux.HandleFunc("/land", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, landGeoJSON)
	})

	mux.HandleFunc("/boundaries", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, boundariesGeoJSON)
	})

	mux.HandleFunc("/lakes", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, lakesGeoJSON)
	})

	mux.HandleFunc("/rivers", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, riversGeoJSON)
	})

	return mux
}

// basePrefix returns the API prefix that serves OGC routes, e.g. "/ogc".
// Requests arrive as "/ogc/basemap/style.json", so strip the suffix.
func basePrefix(r *http.Request) string {
	const suffix = "/basemap/style.json"
	p := r.URL.Path
	if len(p) > len(suffix) && p[len(p)-len(suffix):] == suffix {
		return p[:len(p)-len(suffix)]
	}
	return "/ogc"
}

func writeGeoJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)

	switch t := v.(type) {
	case []byte:
		_, _ = w.Write(t)
	default:
		_ = json.NewEncoder(w).Encode(v)
	}
}
