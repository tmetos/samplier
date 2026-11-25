// Package basemap serves a self-hosted MapLibre basemap so Samplier needs no
// third-party tile service (no API keys, no external requests from the browser).
//
// Two providers are supported:
//
//   - "natural-earth" (default): embedded public-domain Natural Earth 1:110m
//     land polygons, country boundaries, lakes and rivers composed into a dark
//     MapLibre style.
//   - "openmaptiles": the OpenMapTiles "Dark Matter" vector-tile theme, served
//     from a self-hosted tile server (e.g. TileServer GL), with Samplier's OGC
//     API — Features collections overlaid on top.
package basemap

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
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

// Config selects the basemap provider and configures self-hosted OpenMapTiles
// endpoints. It is intentionally simple: either the embedded Natural Earth
// basemap or the OpenMapTiles "Dark Matter" theme.
type Config struct {
	// Provider is "natural-earth" (default) or "openmaptiles".
	Provider string
	// TileURL is a TileJSON URL or a {z}/{x}/{y} tile template for OpenMapTiles.
	TileURL string
	// GlyphsURL is a {fontstack}/{range}.pbf template for OpenMapTiles labels.
	GlyphsURL string
	// SpriteURL is the sprite base URL for OpenMapTiles; empty resolves to a
	// relative "sprite" served by this API.
	SpriteURL string
}

// withDefaults fills provider-specific defaults so callers can pass an empty
// Config and still get a working natural-earth style.
func (c Config) withDefaults() Config {
	if c.Provider == "" {
		c.Provider = "natural-earth"
	}
	if c.Provider == "openmaptiles" {
		if c.TileURL == "" {
			c.TileURL = "http://localhost:8082/data/openmaptiles.json"
		}
		if c.GlyphsURL == "" {
			c.GlyphsURL = "http://localhost:8082/fonts/{fontstack}/{range}.pbf"
		}
		if c.SpriteURL == "" {
			c.SpriteURL = "sprite"
		}
	}
	return c
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

// Style builds the default MapLibre style document (embedded Natural Earth).
func Style(basePath string) map[string]any {
	return StyleFor(basePath, Config{})
}

// StyleFor builds the MapLibre style document for the selected provider. All
// basemap sources are self-hosted URLs served by this API or the configured
// tile server; Samplier's OGC collections are overlaid on top.
func StyleFor(basePath string, cfg Config) map[string]any {
	cfg = cfg.withDefaults()
	if basePath == "" {
		basePath = "/ogc"
	}

	sources := map[string]any{}
	var layers []any
	metadata := map[string]any{}

	if cfg.Provider == "openmaptiles" {
		sources["openmaptiles"] = openMapTilesSource(cfg.TileURL)
		layers = append(layers, darkMatterLayers()...)
		metadata["samplier:basemap"] = "openmaptiles"
	} else {
		addNaturalEarthBase(sources, &layers, basePath)
		metadata["samplier:basemap"] = "natural-earth-110m"
	}

	addSamplierLayers(sources, &layers, basePath)

	st := map[string]any{
		"version":  8,
		"name":     "Samplier (self-hosted)",
		"metadata": metadata,
		"sources":  sources,
		"layers":   layers,
	}

	// Vector-tile basemaps need glyphs for labels and a sprite for pattern
	// and icon layers; the embedded GeoJSON basemap needs neither.
	if cfg.Provider == "openmaptiles" {
		st["glyphs"] = cfg.GlyphsURL
		st["sprite"] = cfg.SpriteURL
	}

	return st
}

// openMapTilesSource builds the "openmaptiles" vector source. A template URL
// ({z}/{x}/{y}) is passed through as a tile list; anything else is treated as
// a TileJSON document URL (the MapLibre "url" form).
func openMapTilesSource(tileURL string) map[string]any {
	src := map[string]any{"type": "vector"}
	if strings.Contains(tileURL, "{z}") && strings.Contains(tileURL, "{x}") && strings.Contains(tileURL, "{y}") {
		src["tiles"] = []string{tileURL}
		src["minzoom"] = 0
		src["maxzoom"] = 14
	} else {
		src["url"] = tileURL
	}
	return src
}

// addNaturalEarthBase appends the embedded Natural Earth basemap layers.
func addNaturalEarthBase(sources map[string]any, layers *[]any, basePath string) {
	sources["ne_land"] = map[string]any{
		"type": "geojson",
		"data": basePath + "/basemap/land",
	}
	sources["ne_boundaries"] = map[string]any{
		"type": "geojson",
		"data": basePath + "/basemap/boundaries",
	}
	sources["ne_lakes"] = map[string]any{
		"type": "geojson",
		"data": basePath + "/basemap/lakes",
	}
	sources["ne_rivers"] = map[string]any{
		"type": "geojson",
		"data": basePath + "/basemap/rivers",
	}

	*layers = append(*layers, map[string]any{
		"id":   "background",
		"type": "background",
		"paint": map[string]any{
			"background-color": "#070c10",
		},
	})

	// Ocean-ish graticule feel: land fill on the dark canvas.
	*layers = append(*layers, map[string]any{
		"id":     "land",
		"type":   "fill",
		"source": "ne_land",
		"paint": map[string]any{
			"fill-color":   "#1b2a32",
			"fill-opacity": 1.0,
		},
	})

	*layers = append(*layers, map[string]any{
		"id":     "land-outline",
		"type":   "line",
		"source": "ne_land",
		"paint": map[string]any{
			"line-color": "#3f6373",
			"line-width": 0.8,
		},
	})

	// Inland water gives regional context where the coastline does not.
	*layers = append(*layers, map[string]any{
		"id":     "lakes",
		"type":   "fill",
		"source": "ne_lakes",
		"paint": map[string]any{
			"fill-color":   "#0b1620",
			"fill-opacity": 1.0,
		},
	})

	*layers = append(*layers, map[string]any{
		"id":     "rivers",
		"type":   "line",
		"source": "ne_rivers",
		"paint": map[string]any{
			"line-color": "#22485c",
			"line-width": 1.1,
		},
	})

	*layers = append(*layers, map[string]any{
		"id":     "boundaries",
		"type":   "line",
		"source": "ne_boundaries",
		"paint": map[string]any{
			"line-color":     "#3a5561",
			"line-width":     0.5,
			"line-dasharray": []float64{2, 2},
		},
	})
}

// addSamplierLayers appends the Samplier OGC collection layers above the
// basemap. Collections that can carry polygons (projects, tasks) get a fill +
// outline filtered to area geometry, plus a circle filtered to points, so a
// site boundary renders as a boundary while a point project still shows as a
// marker. Point-only collections get circles.
func addSamplierLayers(sources map[string]any, layers *[]any, basePath string) {
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
			*layers = append(*layers, map[string]any{
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
			*layers = append(*layers, map[string]any{
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
			*layers = append(*layers, map[string]any{
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
			*layers = append(*layers, map[string]any{
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
			*layers = append(*layers, map[string]any{
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
		*layers = append(*layers, map[string]any{
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
}

// Handler serves the default (Natural Earth) basemap style and data.
func Handler() http.Handler {
	return HandlerFor(Config{})
}

// HandlerFor serves the basemap style, embedded GeoJSON, and Dark Matter
// sprite assets for the selected provider.
func HandlerFor(cfg Config) http.Handler {
	cfg = cfg.withDefaults()
	mux := http.NewServeMux()

	mux.HandleFunc("/style.json", func(w http.ResponseWriter, r *http.Request) {
		// Derive the mount path so the style works behind any prefix.
		writeGeoJSON(w, StyleFor(basePrefix(r), cfg))
	})

	mux.HandleFunc("/land", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, landGeoJSON)
	})

	mux.HandleFunc("/boundaries", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, boundariesGeoJSON)
	})

	mux.HandleFunc("/lakes", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, Lakes())
	})

	mux.HandleFunc("/rivers", func(w http.ResponseWriter, r *http.Request) {
		writeGeoJSON(w, Rivers())
	})

	// Dark Matter sprite assets. The style references these with a relative
	// "sprite" URL, so they live next to style.json under /ogc/basemap.
	mux.HandleFunc("/sprite.json", func(w http.ResponseWriter, r *http.Request) {
		serveSprite(w, "sprite.json")
	})
	mux.HandleFunc("/sprite.png", func(w http.ResponseWriter, r *http.Request) {
		serveSprite(w, "sprite.png")
	})
	mux.HandleFunc("/sprite@2x.json", func(w http.ResponseWriter, r *http.Request) {
		serveSprite(w, "sprite@2x.json")
	})
	mux.HandleFunc("/sprite@2x.png", func(w http.ResponseWriter, r *http.Request) {
		serveSprite(w, "sprite@2x.png")
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

func serveSprite(w http.ResponseWriter, name string) {
	data, contentType, ok := spriteRoute(name)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
