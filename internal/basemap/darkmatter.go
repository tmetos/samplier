package basemap

import (
	_ "embed"
	"encoding/json"
)

// OpenMapTiles "Dark Matter" basemap assets. The layer list is the verbatim
// layers array from github.com/openmaptiles/dark-matter-gl-style (v3.x), so
// the self-hosted basemap matches the published Dark Matter theme. The sprite
// holds the pattern/icon images those layers reference.

//go:embed data/darkmatter-layers.json
var darkMatterLayersJSON []byte

//go:embed data/sprite.json
var spriteJSON []byte

//go:embed data/sprite.png
var spritePNG []byte

//go:embed data/sprite@2x.json
var sprite2xJSON []byte

//go:embed data/sprite@2x.png
var sprite2xPNG []byte

// darkMatterLayers returns the Dark Matter base layers in draw order. Every
// layer references the "openmaptiles" vector source; the source URL itself is
// injected at style-build time from Config.TileURL.
func darkMatterLayers() []any {
	var layers []any
	if err := json.Unmarshal(darkMatterLayersJSON, &layers); err != nil {
		// The asset is fixed at build time; a decode failure is a bug.
		panic("basemap: invalid embedded dark-matter layers: " + err.Error())
	}
	return layers
}

// spriteRoute returns the sprite asset bytes and content type for a path name.
func spriteRoute(name string) ([]byte, string, bool) {
	switch name {
	case "sprite.json":
		return spriteJSON, "application/json", true
	case "sprite.png":
		return spritePNG, "image/png", true
	case "sprite@2x.json":
		return sprite2xJSON, "application/json", true
	case "sprite@2x.png":
		return sprite2xPNG, "image/png", true
	default:
		return nil, "", false
	}
}
