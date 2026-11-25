# OpenMapTiles basemap data

This directory holds the self-hosted OpenMapTiles basemap. It is populated by
[`bin/fetch-openmaptiles.sh`](../bin/fetch-openmaptiles.sh) and is not committed
(the MBTiles extract and glyph fonts can be large).

Layout after fetching:

```
tiles/
  config.json            TileServer GL config (committed)
  openmaptiles.mbtiles   OpenMapTiles vector-tile extract (downloaded)
  fonts/                 PBF glyph fonts for the Dark Matter style (downloaded)
```

To switch the app to the self-hosted OpenMapTiles basemap:

```bash
bin/fetch-openmaptiles.sh
docker compose --profile basemap up -d tileserver
BASEMAP_PROVIDER=openmaptiles make run
```

Point the API at a different tile server with `BASEMAP_TILE_URL` (TileJSON URL
or `{z}/{x}/{y}` template), `BASEMAP_GLYPHS_URL`, and `BASEMAP_SPRITE_URL`.
