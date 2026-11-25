package basemap

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedDataIsValidGeoJSON(t *testing.T) {
	for name, data := range map[string][]byte{
		"land":       Land(),
		"boundaries": Boundaries(),
	} {
		var fc struct {
			Type     string `json:"type"`
			Features []struct {
				Type     string          `json:"type"`
				Geometry json.RawMessage `json:"geometry"`
			} `json:"features"`
		}
		if err := json.Unmarshal(data, &fc); err != nil {
			t.Fatalf("%s: invalid JSON: %v", name, err)
		}
		if fc.Type != "FeatureCollection" {
			t.Errorf("%s: type = %q", name, fc.Type)
		}
		if len(fc.Features) == 0 {
			t.Errorf("%s: no features", name)
		}
		for i, f := range fc.Features {
			if f.Type != "Feature" || len(f.Geometry) == 0 {
				t.Errorf("%s: feature %d malformed", name, i)
			}
		}
	}
}

func TestEmbeddedDataIsCompact(t *testing.T) {
	// Keep the embedded payload small enough to ship in the binary.
	if n := len(Land()); n > 200_000 {
		t.Errorf("land geojson is %d bytes; expected < 200 KB", n)
	}
	if n := len(Boundaries()); n > 200_000 {
		t.Errorf("boundaries geojson is %d bytes; expected < 200 KB", n)
	}
}

func TestStyleHasNoExternalSources(t *testing.T) {
	raw, err := json.Marshal(Style("/ogc"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "http://") || strings.Contains(string(raw), "https://") {
		t.Error("style must not reference external URLs (no API keys, offline-safe)")
	}
}

func TestStyleStructure(t *testing.T) {
	st := Style("/ogc")

	if st["version"] != 8 {
		t.Errorf("version = %v, want 8", st["version"])
	}

	sources, ok := st["sources"].(map[string]any)
	if !ok {
		t.Fatal("sources missing")
	}
	for _, want := range []string{"ne_land", "ne_boundaries", "samplier-locations", "samplier-samples", "samplier-projects", "samplier-tasks"} {
		if _, ok := sources[want]; !ok {
			t.Errorf("missing source %q", want)
		}
	}

	// Basemap sources must point at our own endpoints.
	land := sources["ne_land"].(map[string]any)
	if land["data"] != "/ogc/basemap/land" {
		t.Errorf("land data = %v", land["data"])
	}

	layers, ok := st["layers"].([]any)
	if !ok || len(layers) < 5 {
		t.Fatalf("expected layers, got %v", st["layers"])
	}

	ids := map[string]bool{}
	for _, l := range layers {
		m := l.(map[string]any)
		ids[m["id"].(string)] = true
	}
	for _, want := range []string{"background", "land", "boundaries", "samplier-locations"} {
		if !ids[want] {
			t.Errorf("missing layer %q", want)
		}
	}

	// Every non-background layer must reference a declared source.
	for _, l := range layers {
		m := l.(map[string]any)
		src, has := m["source"].(string)
		if !has {
			continue // background layer
		}
		if _, ok := sources[src]; !ok {
			t.Errorf("layer %q references unknown source %q", m["id"], src)
		}
	}
}

func TestStyleHonoursBasePath(t *testing.T) {
	st := Style("/custom/prefix")
	sources := st["sources"].(map[string]any)
	land := sources["ne_land"].(map[string]any)
	if land["data"] != "/custom/prefix/basemap/land" {
		t.Errorf("got %v", land["data"])
	}
	locs := sources["samplier-locations"].(map[string]any)
	if !strings.HasPrefix(locs["data"].(string), "/custom/prefix/collections/locations/items") {
		t.Errorf("got %v", locs["data"])
	}
}

func TestStyleDefaultsBasePath(t *testing.T) {
	st := Style("")
	sources := st["sources"].(map[string]any)
	land := sources["ne_land"].(map[string]any)
	if land["data"] != "/ogc/basemap/land" {
		t.Errorf("got %v", land["data"])
	}
}

func TestHandlerServesStyleAndData(t *testing.T) {
	srv := httptest.NewServer(http.StripPrefix("/ogc/basemap", Handler()))
	defer srv.Close()

	cases := []struct {
		path        string
		wantType    string
		wantContent string
	}{
		{"/style.json", "application/geo+json", "Samplier"},
		{"/land", "application/geo+json", "FeatureCollection"},
		{"/boundaries", "application/geo+json", "FeatureCollection"},
		{"/lakes", "application/geo+json", "FeatureCollection"},
		{"/rivers", "application/geo+json", "FeatureCollection"},
	}

	for _, tc := range cases {
		resp, err := http.Get(srv.URL + "/ogc/basemap" + tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", tc.path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != tc.wantType {
			t.Errorf("%s: content-type %q", tc.path, ct)
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age") {
			t.Errorf("%s: expected cache header, got %q", tc.path, cc)
		}
		if !strings.Contains(string(body), tc.wantContent) {
			t.Errorf("%s: body missing %q", tc.path, tc.wantContent)
		}
	}
}

func TestBasePrefixDerivation(t *testing.T) {
	cases := map[string]string{
		"/ogc/basemap/style.json":       "/ogc",
		"/v2/ogc/basemap/style.json":    "/v2/ogc",
		"/something/else":               "/ogc",
		"/ogc/basemap/style.json/extra": "/ogc",
	}
	for path, want := range cases {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if got := basePrefix(r); got != want {
			t.Errorf("basePrefix(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLayersDeclaredConsistently(t *testing.T) {
	st := Style("/ogc")
	sources := st["sources"].(map[string]any)
	for _, l := range Layers {
		key := "samplier-" + l.Collection
		if _, ok := sources[key]; !ok {
			t.Errorf("collection %q has no style source", l.Collection)
		}
		if l.Radius <= 0 {
			t.Errorf("collection %q has non-positive radius", l.Collection)
		}
		if l.Color == "" {
			t.Errorf("collection %q has no colour", l.Collection)
		}
	}
}

func TestLayerFiltersAreExpressions(t *testing.T) {
	st := Style("/ogc")
	layers := st["layers"].([]any)

	filters := 0
	for _, l := range layers {
		m := l.(map[string]any)
		f, ok := m["filter"]
		if !ok {
			continue
		}
		filters++
		arr, ok := f.([]any)
		if !ok || len(arr) < 3 {
			t.Fatalf("layer %v: malformed filter %v", m["id"], f)
		}
		// The legacy form ["in", prop, ...] would be misread by MapLibre; we
		// must never emit "in" as the operator.
		if arr[0] == "in" {
			t.Fatalf("layer %v: legacy 'in' filter is ambiguous: %v", m["id"], f)
		}
		// geometry-type must be an expression, not a bare property string.
		if arr[0] == "any" {
			for _, br := range arr[1:] {
				b := br.([]any)
				if b[0] != "==" {
					t.Fatalf("layer %v: unexpected branch %v", m["id"], b)
				}
				if _, ok := b[1].([]any); !ok {
					t.Fatalf("layer %v: geometry-type must be an expression: %v", m["id"], b[1])
				}
			}
		} else if arr[0] == "==" {
			if _, ok := arr[1].([]any); !ok {
				t.Fatalf("layer %v: geometry-type must be an expression: %v", m["id"], arr[1])
			}
		}
	}
	if filters == 0 {
		t.Fatal("expected filters on feature layers")
	}
}

func TestAreaCollectionsGetFillAndOutline(t *testing.T) {
	st := Style("/ogc")
	ids := map[string]bool{}
	for _, l := range st["layers"].([]any) {
		ids[l.(map[string]any)["id"].(string)] = true
	}
	for _, want := range []string{
		"samplier-projects-fill", "samplier-projects-outline", "samplier-projects",
		"samplier-tasks-fill", "samplier-tasks-outline", "samplier-tasks",
		"samplier-locations", "samplier-samples",
	} {
		if !ids[want] {
			t.Errorf("missing layer %q", want)
		}
	}
	// Point-only collections must not get area fills.
	for _, unwanted := range []string{"samplier-locations-fill", "samplier-samples-fill"} {
		if ids[unwanted] {
			t.Errorf("unexpected area layer %q", unwanted)
		}
	}
}

// TestAreaLayersUseValidProperties guards against paint/layout mix-ups that
// make MapLibre reject the whole style.
func TestAreaLayersUseValidProperties(t *testing.T) {
	st := Style("/ogc")
	layoutOnly := map[string]bool{"line-join": true, "line-cap": true, "visibility": true}

	for _, l := range st["layers"].([]any) {
		m := l.(map[string]any)
		id := m["id"].(string)
		if paint, ok := m["paint"].(map[string]any); ok {
			for k := range paint {
				if layoutOnly[k] {
					t.Errorf("layer %s: %q belongs in layout, not paint", id, k)
				}
			}
		}
		if layout, ok := m["layout"].(map[string]any); ok {
			for k := range layout {
				if k == "line-color" || k == "line-width" || k == "fill-color" {
					t.Errorf("layer %s: %q belongs in paint, not layout", id, k)
				}
			}
		}
	}
}

func TestOpenMapTilesStyleStructure(t *testing.T) {
	st := StyleFor("/ogc", Config{Provider: "openmaptiles"})

	if st["version"] != 8 {
		t.Errorf("version = %v, want 8", st["version"])
	}
	if meta := st["metadata"].(map[string]any); meta["samplier:basemap"] != "openmaptiles" {
		t.Errorf("metadata samplier:basemap = %v", meta["samplier:basemap"])
	}

	sources, ok := st["sources"].(map[string]any)
	if !ok {
		t.Fatal("sources missing")
	}
	omt, ok := sources["openmaptiles"].(map[string]any)
	if !ok {
		t.Fatal("missing openmaptiles source")
	}
	if omt["type"] != "vector" {
		t.Errorf("openmaptiles type = %v, want vector", omt["type"])
	}
	if omt["url"] != "http://localhost:8082/data/openmaptiles.json" {
		t.Errorf("openmaptiles url = %v", omt["url"])
	}
	for _, want := range []string{"samplier-projects", "samplier-tasks", "samplier-locations", "samplier-samples"} {
		if _, ok := sources[want]; !ok {
			t.Errorf("missing source %q", want)
		}
	}

	if st["glyphs"] != "http://localhost:8082/fonts/{fontstack}/{range}.pbf" {
		t.Errorf("glyphs = %v", st["glyphs"])
	}
	if st["sprite"] != "sprite" {
		t.Errorf("sprite = %v", st["sprite"])
	}

	layers := st["layers"].([]any)
	ids := map[string]bool{}
	for _, l := range layers {
		m := l.(map[string]any)
		ids[m["id"].(string)] = true
	}
	for _, want := range []string{"background", "water", "building", "place_city", "samplier-locations", "samplier-projects-fill"} {
		if !ids[want] {
			t.Errorf("missing layer %q", want)
		}
	}

	// Every non-background layer must reference a declared source.
	for _, l := range layers {
		m := l.(map[string]any)
		src, has := m["source"].(string)
		if !has {
			continue // background layer
		}
		if _, ok := sources[src]; !ok {
			t.Errorf("layer %q references unknown source %q", m["id"], src)
		}
	}
}

func TestOpenMapTilesTileURLForms(t *testing.T) {
	template := StyleFor("/ogc", Config{Provider: "openmaptiles", TileURL: "http://tiles.local/{z}/{x}/{y}.pbf"})
	src := template["sources"].(map[string]any)["openmaptiles"].(map[string]any)
	if _, ok := src["tiles"]; !ok {
		t.Error("template URL should produce a tiles list, not a url")
	}
	if _, ok := src["url"]; ok {
		t.Error("template URL must not set url")
	}

	tilejson := StyleFor("/ogc", Config{Provider: "openmaptiles", TileURL: "http://tiles.local/data/openmaptiles.json"})
	src = tilejson["sources"].(map[string]any)["openmaptiles"].(map[string]any)
	if src["url"] != "http://tiles.local/data/openmaptiles.json" {
		t.Errorf("url = %v", src["url"])
	}
	if _, ok := src["tiles"]; ok {
		t.Error("TileJSON URL must not set tiles")
	}
}

func TestOpenMapTilesStyleHasNoMapTilerDependencies(t *testing.T) {
	raw, err := json.Marshal(StyleFor("/ogc", Config{Provider: "openmaptiles"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, banned := range []string{"api.maptiler.com", "{key}"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("style must not depend on MapTiler cloud (%q present)", banned)
		}
	}
}

func TestHandlerServesSprites(t *testing.T) {
	srv := httptest.NewServer(http.StripPrefix("/ogc/basemap", HandlerFor(Config{Provider: "openmaptiles"})))
	defer srv.Close()

	cases := []struct {
		path        string
		wantType    string
		wantContent string
	}{
		{"/sprite.json", "application/json", "wood-pattern"},
		{"/sprite.png", "image/png", ""},
		{"/sprite@2x.json", "application/json", "wood-pattern"},
		{"/sprite@2x.png", "image/png", ""},
	}

	for _, tc := range cases {
		resp, err := http.Get(srv.URL + "/ogc/basemap" + tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", tc.path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != tc.wantType {
			t.Errorf("%s: content-type %q, want %q", tc.path, ct, tc.wantType)
		}
		if tc.wantContent != "" && !strings.Contains(string(body), tc.wantContent) {
			t.Errorf("%s: body missing %q", tc.path, tc.wantContent)
		}
	}

	// Unknown sprite names must 404 rather than serve an empty body.
	resp, err := http.Get(srv.URL + "/ogc/basemap/sprite-missing.png")
	if err != nil {
		t.Fatalf("missing sprite: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown sprite status = %d, want 404", resp.StatusCode)
	}
}

func TestServeSpriteUnknownName(t *testing.T) {
	rr := httptest.NewRecorder()
	serveSprite(rr, "sprite-missing.png")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}

	if _, _, ok := spriteRoute("sprite-missing.png"); ok {
		t.Error("spriteRoute must reject unknown names")
	}
}

func TestGeometryFilterSingleType(t *testing.T) {
	f := geometryFilter("Point")
	if len(f) != 3 || f[0] != "==" {
		t.Fatalf("single-type filter = %v", f)
	}
	if _, ok := f[1].([]any); !ok {
		t.Fatalf("geometry-type must be an expression: %v", f[1])
	}
}

func TestDarkMatterLayersPanicsOnInvalidEmbed(t *testing.T) {
	orig := darkMatterLayersJSON
	t.Cleanup(func() { darkMatterLayersJSON = orig })

	darkMatterLayersJSON = []byte("{not valid json")
	defer func() {
		if recover() == nil {
			t.Error("expected panic for invalid embedded layers")
		}
	}()
	darkMatterLayers()
}

// TestBoundaryRendersAsArea locks in the "site boundary" look: a continuous
// outline and a visible fill, never a dashed line (which reads as a point set).
func TestBoundaryRendersAsArea(t *testing.T) {
	st := Style("/ogc")
	var fill, outline map[string]any

	for _, l := range st["layers"].([]any) {
		m := l.(map[string]any)
		switch m["id"] {
		case "samplier-projects-fill":
			fill = m
		case "samplier-projects-outline":
			outline = m
		}
	}
	if fill == nil || outline == nil {
		t.Fatal("projects fill/outline layers missing")
	}

	num := func(v any) float64 {
		switch n := v.(type) {
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case float64:
			return n
		case float32:
			return float64(n)
		}
		return 0
	}

	paint := fill["paint"].(map[string]any)
	if op := num(paint["fill-opacity"]); op < 0.2 {
		t.Errorf("fill-opacity %v is too faint to read as an area", paint["fill-opacity"])
	}

	opaint := outline["paint"].(map[string]any)
	if _, dashed := opaint["line-dasharray"]; dashed {
		t.Error("outline must be continuous; dashes read as a point set")
	}
	if w := num(opaint["line-width"]); w < 1.5 {
		t.Errorf("line-width %v too thin", opaint["line-width"])
	}
}
