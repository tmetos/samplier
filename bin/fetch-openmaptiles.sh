#!/usr/bin/env bash
set -euo pipefail

# Fetches a self-hosted OpenMapTiles basemap into ./tiles:
#   1. an OpenMapTiles MBTiles extract (default: the small Zurich sample)
#   2. the OpenMapTiles glyph fonts referenced by the Dark Matter style
#   3. a TileServer GL config.json pointing at both
#
# Usage:
#   bin/fetch-openmaptiles.sh
#
# Overrides:
#   OMT_MBTILES_URL   URL of a .mbtiles OpenMapTiles extract (e.g. a region
#                     download from https://data.maptiler.com/downloads/).
#   OMT_FONTS_BASE    Base URL for the OpenMapTiles font zips.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TILES="$ROOT/tiles"
FONTS="$TILES/fonts"

OMT_MBTILES_URL="${OMT_MBTILES_URL:-https://github.com/maptiler/tileserver-gl/releases/download/v1.3.0/zurich_switzerland.mbtiles}"
OMT_FONTS_BASE="${OMT_FONTS_BASE:-https://raw.githubusercontent.com/openmaptiles/fonts/gh-pages}"

mkdir -p "$FONTS"

# --- 1. MBTiles ---------------------------------------------------------
MBTILES="$TILES/openmaptiles.mbtiles"
if [ -f "$MBTILES" ]; then
  echo "MBTiles already present: $MBTILES"
else
  echo "Downloading OpenMapTiles MBTiles…"
  curl -L --fail --progress-bar "$OMT_MBTILES_URL" -o "$MBTILES"
fi

# --- 2. Fonts -----------------------------------------------------------
# The Dark Matter style requests these six fontstacks. OpenMapTiles' gh-pages
# names the Noto Sans stacks "Klokantech Noto Sans …", so they are renamed to
# the exact fontstack names the style requests.
fetch_font() {
  local src_name="$1" dst_name="$2"
  if [ -d "$FONTS/$dst_name" ]; then
    echo "Font already present: $dst_name"
    return
  fi
  echo "Downloading font: $dst_name"
  local encoded="${src_name// /%20}"
  local tmp
  tmp="$(mktemp -d)"
  if ! curl -L --fail --silent --show-error "$OMT_FONTS_BASE/$encoded.zip" -o "$tmp/font.zip"; then
    rm -rf "$tmp"
    echo "Failed to download $src_name from $OMT_FONTS_BASE" >&2
    exit 1
  fi
  unzip -q "$tmp/font.zip" -d "$tmp/unzipped"
  mv "$tmp/unzipped/$src_name" "$FONTS/$dst_name"
  rm -rf "$tmp"
}

fetch_font "Metropolis Light" "Metropolis Light"
fetch_font "Metropolis Light Italic" "Metropolis Light Italic"
fetch_font "Metropolis Medium Italic" "Metropolis Medium Italic"
fetch_font "Metropolis Regular" "Metropolis Regular"
fetch_font "Klokantech Noto Sans Italic" "Noto Sans Italic"
fetch_font "Klokantech Noto Sans Regular" "Noto Sans Regular"

# --- 3. TileServer GL config -------------------------------------------
cat > "$TILES/config.json" <<'EOF'
{
  "options": {
    "paths": {
      "fonts": "fonts"
    }
  },
  "data": {
    "openmaptiles": {
      "mbtiles": "openmaptiles.mbtiles"
    }
  }
}
EOF

echo
echo "Done. Start the tile server and run the API against it:"
echo "  docker compose --profile basemap up -d tileserver"
echo "  BASEMAP_PROVIDER=openmaptiles make run"
echo
echo "For a different region, re-run with OMT_MBTILES_URL set to a full"
echo "OpenMapTiles extract (https://data.maptiler.com/downloads/)."
