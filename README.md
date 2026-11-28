<div align="center">

<img src="assets/brand/logo-mark.svg" width="104" alt="Samplier" />

# Samplier

An open-source environmental data management system for laboratory and
field-collected data — samples, regulatory limits, geospatial API, and
regulator-ready exports.

[![CI](https://github.com/samplier/samplier/actions/workflows/ci.yml/badge.svg)](https://github.com/samplier/samplier/actions/workflows/ci.yml)
[![Image](https://github.com/samplier/samplier/actions/workflows/image.yml/badge.svg)](https://github.com/samplier/samplier/actions/workflows/image.yml)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)](https://react.dev)
[![PostGIS](https://img.shields.io/badge/PostgreSQL%20%2B%20PostGIS-16-336791?logo=postgresql&logoColor=white)](https://postgis.net)
[![Coverage](https://img.shields.io/badge/coverage-84%25-35b37e)](https://github.com/samplier/samplier/actions/workflows/ci.yml)
[![OGC](https://img.shields.io/badge/OGC%20API-Features-6fa8dc)](https://ogcapi.ogc.org/features/)
[![License](https://img.shields.io/badge/license-AGPL--3.0-ff5c8a)](LICENSE)

[Demo](#try-the-demo) · [Features](#features) · [Quickstart](#quickstart) ·
[API](#api-surface) · [Deployment](#deployment) · [Brand](assets/brand/README.md)

</div>

<img src="assets/brand/social-preview.png" alt="Samplier — laboratory and field data, OGC API Features, regulatory exports" width="100%" />


## Stack

| Layer | Technology |
|---|---|
| Database | PostgreSQL 16 + PostGIS |
| Backend | Go (chi, pgx, OpenTelemetry) |
| Frontend | React + TypeScript + MapLibre GL |
| Auth | Keycloak (OIDC/SSO) |
| Infrastructure | Terraform (AWS Fargate/RDS · GCP Cloud Run/Cloud SQL) |

## Features

- **Core hierarchy** — project → task → location → sample → test → result
- **Lab data model** — CAS RN analytes, units, matrix, fraction, detection limits
  (RDL/MDL/QL), detect flags, lab/validator/interpreted qualifiers
- **Well construction** — header plus screen/casing segments with depths
- **Action levels** — EPA/State/Local/Custom groups matched by matrix, fraction,
  project, dates, land use, and depth; exceedance detection with unit conversion
- **Unit conversion** — factor/delta rules plus formula expressions (e.g. °F→°C)
- **EDD import** — EFWEDD (4-file laboratory EDD) with a pluggable format framework,
  per-row validation reports, and three commit modes
- **Exports** — formatted XLSX crosstab, CSV, WQX/ESAR (XML), AQS (pipe + XML),
  Washington State EIM (CSV + XML)
- **Custom reports** — pick analytes or analyte grouping sets, evaluate against
  one or more regulatory limit sets, preview the crosstab in the UI, export a
  multi-tab Excel workbook (one tab per grouping set), and save views for reuse
- **OGC API — Features** — `/ogc` collections for projects, tasks, locations, samples
- **Self-hosted basemap** — two providers, no API keys, no third-party requests:
  an embedded Natural Earth (land/lakes/rivers/boundaries) MapLibre style by
  default, or the OpenMapTiles **Dark Matter** vector-tile theme served from a
  self-hosted tile server
- **MCP server** — 9 tools for agent-driven queries, reports, and exports
- **Observability** — structured JSON logs, OTel traces (10% sampling), metrics

## Try the demo

Two realistic demo sites load with one command — a former plating works
(groundwater) and a closed landfill (groundwater + surface water) — covering
exceedances, compliant samples, and samples where nothing is detected at all:

```bash
./demo/run.sh
```

Then open <http://localhost:5173> for the UI and
<http://localhost:8080/ogc/collections> for the OGC API. Auth is disabled in the
demo. See [`demo/README.md`](demo/README.md) for the dataset and things to try.

## Screenshots

| Dashboard — company metrics, projects, activity | Projects — pick a project, see only its data |
|---|---|
| ![Dashboard](assets/screenshots/dashboard.png) | ![Projects](assets/screenshots/projects.png) |

**Map — project boundaries, tasks, locations and samples**, with chip toggles and a
self-hosted basemap:

![Map with site boundary](assets/screenshots/map.png)

**Results — exceedances in the brand pink.** Green means sampled, blue means
served, pink means *over the limit*:

![Results with exceedances highlighted](assets/screenshots/results.png)

## Quickstart

Requirements: Docker (with Compose), Go 1.23+, Node 20+.

```bash
# 1. Database + Keycloak (applies schema.sql and seed.sql on first boot)
make up

# 2. API
make run            # or: go run ./cmd/server serve

# 3. Verify
curl localhost:8080/healthz
curl localhost:8080/ogc/conformance
curl "localhost:8080/ogc/collections/locations/items?limit=5"

# 4. UI (separate terminal)
cd ui && npm install && npm run dev   # http://localhost:5173

# 5. MCP server (stdio)
make mcp
```

### OpenMapTiles basemap (self-hosted, Dark Matter)

Swap the embedded Natural Earth basemap for the full OpenMapTiles **Dark Matter**
theme (streets, landuse, buildings, labels) served by a self-hosted tile server:

```bash
# Download an OpenMapTiles MBTiles extract + Dark Matter glyph fonts
make omt-fetch          # or: bin/fetch-openmaptiles.sh

# Start TileServer GL on :8082
make omt-up             # or: docker compose --profile basemap up -d tileserver

# Run the API with the OpenMapTiles provider
BASEMAP_PROVIDER=openmaptiles make run
```

The fetch script defaults to a small Zurich sample so you can try it immediately;
point `OMT_MBTILES_URL` at a region extract (e.g. from
<https://data.maptiler.com/downloads/>) for real data. The API generates the
Dark Matter style and overlays Samplier's OGC collections on top, so the map
stays self-hosted end to end.

## Authentication

Samplier validates OIDC bearer tokens (Keycloak by default). See
[`docs/keycloak.md`](docs/keycloak.md) for realm/client setup, token validation
details, and production guidance.

```bash
AUTH_ENABLED=true \
OIDC_ISSUER=http://localhost:8081/realms/samplier \
OIDC_CLIENT_ID=samplier-ui \
make run
```

`/ogc/*` stays public so map clients can consume features without a token.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://samplier:samplier@localhost:5432/samplier` | Postgres DSN |
| `AUTH_ENABLED` | `false` | Enforce OIDC bearer tokens |
| `OIDC_ISSUER` | — | e.g. `https://idp/realms/samplier` |
| `OIDC_CLIENT_ID` | — | Expected `aud` |
| `MAX_UPLOAD_BYTES` | `33554432` | EDD upload cap (32 MiB) |
| `BASEMAP_PROVIDER` | `natural-earth` | `natural-earth` or `openmaptiles` |
| `BASEMAP_TILE_URL` | `http://localhost:8082/data/openmaptiles.json` | TileJSON URL or `{z}/{x}/{y}` template |
| `BASEMAP_GLYPHS_URL` | `http://localhost:8082/fonts/{fontstack}/{range}.pbf` | Glyph font template |
| `BASEMAP_SPRITE_URL` | — | Sprite base URL (empty serves the bundled Dark Matter sprite) |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Enable OTLP/gRPC export |

## API surface

**OGC API — Features** (public, for map clients)

```
GET /ogc/                                      landing page
GET /ogc/conformance                           conformance classes
GET /ogc/collections                           collections
GET /ogc/collections/{id}/items                GeoJSON (bbox, datetime, limit, offset)
GET /ogc/collections/{id}/items/{featureId}    single feature

GET /ogc/basemap/style.json                    MapLibre style (self-hosted)
GET /ogc/basemap/land                          embedded Natural Earth land
GET /ogc/basemap/lakes                         embedded major lakes
GET /ogc/basemap/rivers                        embedded major rivers
GET /ogc/basemap/boundaries                    embedded country boundaries
GET /ogc/basemap/sprite.png|json               Dark Matter sprite (OpenMapTiles)
```

**REST API** (auth-guarded when `AUTH_ENABLED=true`)

```
/api/v1/projects, /tasks, /locations, /samples, /tests, /results
/api/v1/analytes, /units, /unit-conversions, /action-levels, /analyte-groups
/api/v1/reports (custom report views: preview, export, save/list)
/api/v1/locations/{id}/well-segments
/api/v1/edd/validate, /api/v1/edd/load, /api/v1/edd/formats, /api/v1/edd/batches
/api/v1/exports/formats, /api/v1/exports/{format}, /api/v1/exceedances
```

## Development

```bash
make test          # all tests
make cover         # coverage profile + total
make cover-check   # fail below 80%
make lint          # gofmt + go vet
```

Tests that need a database skip automatically when PostGIS is unreachable, so
`go test ./...` works offline. Set `TEST_DATABASE_URL` to point at another
instance.

## Deployment

See [`infra/aws/README.md`](infra/aws/README.md) and
[`infra/gcp/README.md`](infra/gcp/README.md). Both are validated with
`terraform validate` and include DNS/TLS, log retention (14 days, adjustable),
and baseline alerts.

## License

AGPL-3.0-only. See `LICENSE`.
