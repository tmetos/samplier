# Samplier — Open-Source Environmental Data Management System — Specification

> Name: **Samplier** (sampling + sommelier — a connoisseur of environmental samples)
> Status: DRAFT v0.4 — all decisions and data model finalized
> License: AGPL-3.0-only
> Purpose: An open-source system for managing laboratory and field-collected
> environmental data (water, soil, air, sediment), with geospatial query,
> regulatory-limit comparison, regulatory exports, and reporting.

---

## 1. Overview

The system stores, validates, queries, and exports environmental analytical data,
organized around the environmental data hierarchy:

```
Project ─▶ Task ─▶ Location ─▶ Sample ─▶ Test ─▶ Result (analyte measurement)
```

The model follows the standard environmental data hierarchy used by laboratory
deliverables: project → task → location → sample → test → result.

### 1.1 Goals

- Store lab analytical results (analyte, concentration, units, matrix, fraction,
  detection limits, qualifiers, non-detect flags) linked to samples/locations/tasks/projects.
- Store sample, location, task, and project metadata including geometry and depth.
- Store regulatory/action limits (EPA, State, Local, Custom) and compare results
  against them (exceedance detection).
- Store common analytes and unit conversions between concentrations.
- Implement a conformant **OGC API — Features** interface plus a general REST API.
- Provide a web UI for querying, viewing, loading (EDD import), reporting, dashboards,
  and Excel export.
- Provide an **MCP server** for AI-agent-driven reports, queries, and exports.
- Export to regulatory formats: formatted crosstab, **WQX/ESAR**, **AQS**, and
  **Washington State EIM**.

### 1.2 Non-Goals (v1)

- No lab LIMS integrations, no instrument raw-data file parsing (beyond tabular EDDs).
- No geostatistical modeling, 3D visualization, or contouring.
- No billing/licensing subsystem.
- No mobile field data collection app (may come later).
- Not a hosted multi-tenant SaaS (see §2 Decisions).

---

## 2. Confirmed Decisions

| Decision | Choice |
|---|---|
| License | **AGPL-3.0-only** |
| Database | PostgreSQL + PostGIS |
| Backend | Go |
| Frontend | React + TypeScript, MapLibre GL JS for mapping |
| Tenancy | Single organization (one company/consultant); project-level roles |
| Auth | **OIDC / SSO** via **Keycloak** (generic OIDC discovery retained so other IdPs work); API keys for programmatic/MCP access |
| Hierarchy | project → task → location → sample → test → result |
| Well construction | Yes — `well` header + `well_segment` screen/casing intervals |
| Lab data load | **EFWEDD (4-file)** lab format as default, with a pluggable format framework |
| Exports | Formatted crosstab (XLSX), **WQX/ESAR** (XML), **AQS** (both legacy pipe-delimited + XML), **WA State EIM** (both Excel import + XML export) |
| Geospatial API | OGC API — Features (conformant) |
| OGC collections | `projects`, `tasks`, `locations`, **`samples`** (own feature collection) |
| CRS | **Store EPSG:4326; reproject to the requested CRS** (arbitrary SRID on input/output) |
| Data model | Authoritative DDL in `schema.sql`; seed values in `seed.sql` |

### 2.1 Name

**Samplier** — a blend of *sampling* and *sommelier*: a connoisseur of environmental
samples. Short, memorable, and evocative of curated lab/field data.

---

## 3. Architecture

```
┌──────────────┐   ┌──────────────────────┐   ┌─────────────────────────┐
│  React + TS  │   │  Go API server       │   │  PostgreSQL + PostGIS   │
│  MapLibre UI │──▶│  (REST + OGC-Feats)  │──▶│  (data + geometry)      │
└──────────────┘   │  + MCP server        │   └─────────────────────────┘
                   │  + EDD importer      │
                   │  + export adapters   │
                   └──────────────────────┘
```

### 3.1 Go stack (proposed)

- Router: `net/http` + `chi` (or `gin`).
- PostGIS access: `jackc/pgx` (native) with `github.com/paulmach/orb` for geometry
  types and GeoJSON encoding, plus `orb/geojson`.
- Migrations: `golang-migrate` or embedded SQL migrations.
- OGC API Features: implement directly against the spec; reference Crunchy Data
  `pg_featureserv` (Go, OGC API Features over PostGIS) for conformance patterns.
- Excel I/O: `github.com/xuri/excelize/v2` (read/write XLSX).
- XML exports: `encoding/xml` (WQX/ESAR, AQS, EIM).
- MCP: `github.com/modelcontextprotocol/go-sdk` (or hand-rolled JSON-RPC 2.0 over
  stdio per the MCP spec).

### 3.2 Frontend stack

- React + TypeScript + Vite.
- MapLibre GL JS for map; TanStack Table for grids; TanStack Query for data fetching;
  a chart library (e.g., ECharts or visx) for dashboards/trends.
- State: server state via TanStack Query; UI state local.

---

## 4. Data Model

The authoritative DDL is in **`schema.sql`** (create order: reference vocabularies →
units → analytes → auth → project/task/location/well → sample/batch/test/result →
action levels → EDD artifacts). Essential seed values are in **`seed.sql`**.

Table inventory:

| Group | Tables |
|---|---|
| Units | `unit`, `unit_conversion` |
| Reference vocab | `matrix`, `fraction`, `sample_type`, `location_type`, `method`, `lab`, `qualifier`, `material`, `well_segment_type`, `test_type` |
| Analytes | `analyte`, `analyte_group`, `analyte_group_member` |
| Auth (OIDC) | `app_user`, `project_member`, `api_key` |
| Core hierarchy | `project`, `task`, `location`, `sample`, `batch`, `test`, `result` |
| Well construction | `well`, `well_segment` |
| Action levels | `action_level_type`, `action_level`, `action_level_parameter` |
| EDD import | `edd_format`, `edd_batch`, `edd_validation_error` |

### 4.1 Design notes

- **PKs/FKs**: UUID primary keys (`gen_random_uuid()`); business keys retained for
  EDD interoperability (`code`, `sys_loc_code`, `sys_sample_code`, `test_id`).
- **Non-detect**: `detect_flag = 'N'` ⇒ `result_numeric` is NULL; the reported value
  is `< reporting_detection_limit` (e.g., `< 5 mg/L`).
- **Unit conversion**: `converted = value * conversion_factor + delta`; optional
  `formula` for non-linear conversions (e.g. temperature).
- **Well construction**: `well` (header, 1:1 with a well-type location) + `well_segment`
  (screen/casing/filter-pack intervals with start/end depth, diameter, material).
- **Samples as features**: `sample` joins `task` + `location`, so the OGC `samples`
  collection derives point geometry from its location while carrying task/date/depth context.
- **Geometry policy**: stored in EPSG:4326; `project.srid` records the native/display
  SRID for that project (arbitrary SRID supported by reprojecting on ingest/emit).
- **Auth**: OIDC/SSO via `app_user` (issuer + subject); project-scoped roles via
  `project_member`; `api_key` for programmatic/MCP access (SHA-256 hashes only).
- **Audit**: `created_at`/`updated_at`/`created_by` on all mutable tables; EDD imports
  retained in `edd_batch` with per-row `edd_validation_error` records.

---

## 5. Functional Requirements

### 5.1 Projects, Tasks, Locations, Samples, Tests, Results

As described in the data model (§4). CRUD for each, with filtering, sorting,
pagination, and project-scoped visibility. Depth and elevation handling with unit
conversion. Duplicate/batch sample relationships (field duplicate, lab duplicate).
Well construction (`well` + `well_segment`) CRUD for well-type locations, including
screen/casing intervals with diameters and materials.

### 5.2 Action Levels

Three-level model (`action_level_type → action_level → action_level_parameter`).
v1 comparison: after unit conversion to the limit's unit, flag
`result_numeric > action_level` (or `< action_level_min`). Optional: flag non-detect
when `reporting_detection_limit > action_level`. Later: hardness-based limits,
TEQ/MAG sums, depth/fraction/method matching.

### 5.3 Reference Data

Controlled vocabularies (analytes, matrices, fractions, sample/location types,
methods, labs, qualifiers, coord types) seeded with common environmental values and
editable. Unit conversions maintained in `unit_conversion`.

### 5.4 EDD Import (lab data loading)

- Accept **XLSX, CSV, TSV, TXT, ZIP**.
- **Pluggable format framework**: a format is a definition that maps file "sections"
  (worksheets / delimited files) to tables, with per-column rules. Formats are
  registered as code + config, so new formats can be added without core changes.
- **Default format: EFWEDD (4-file)** with sections:
  - `EFW2FSample` (field sample) / `EFW2LabSMP` (lab sample) → `sample`
  - `EFW2LabTST` → `test`
  - `EFW2LabRES` → `result`
  - plus optional `EFW2Batch` (SDG) and Chain-of-Custody.
- EDD header convention: optional `#`-prefixed header rows (column name = DB field
  name, optional type/length row) ignored on import.
- **Validation pipeline** (pre-commit):
  1. required fields + data types;
  2. reference-value lookups (analyte, matrix, unit, method, sample type);
  3. parent/child integrity (Result → Test → Sample → Location/Task);
  4. duplicate detection via alternate keys.
  - Produce a per-row **error/warning report**; errors block commit, warnings allow it.
- **Commit modes**: Insert Only, Insert and Update, Insert and Merge.
- Dry-run (validate only) and rollback support.

### 5.5 Export

- **Crosstab (XLSX)**: analytes as columns, locations as rows, samples by date;
  action-level exceedances color-highlighted. Configurable layout, titles, date styles.
- **Round-trip EDD export** (regenerate an EFWEDD-style deliverable).
- **Regulatory export adapters** (see §6).
- CSV export for any query result.

### 5.6 Reports & Dashboards

- Saved parameterized queries → tabular and crosstab reports.
- Exceedance reports (results vs. selected action-level types).
- Time-series/trend charts per location/analyte.
- Dashboard widgets: sample counts by project/task, exceedance summary, data
  completeness, recent imports.

### 5.7 API

REST API under `/api/v1/...` for all resources (CRUD + filtering, sorting, pagination,
unit conversion on request). OGC API — Features under `/ogc/...` (see §7).

### 5.8 MCP Server

MCP tools (names illustrative):

- `query_results(filters)`
- `get_exceedances(project, action_level_types, date_range)`
- `generate_crosstab(project, analytes, locations, action_level_types)` → XLSX
- `export_regulatory(project, format=WQX|AQS|EIM|CROSSTAB)`
- `load_edd(file, dry_run)`
- `list_analytes`, `list_locations`, `list_action_levels`, `list_tasks`

The MCP server enforces the same project-scoped auth as the REST API.

---

## 6. Regulatory Export Adapters

Each export is an adapter producing a file conforming to the authoritative external
schema. Adapters are pluggable (same pattern as EDD formats).

### 6.1 Formatted Crosstab (XLSX)

Default deliverable for reports to regulators and project tables. Multi-sheet XLSX:
summary, results (crosstab), exceedances, qualifiers/notes. Formatting: merged header
rows, units row, flags for non-detect (`< DL`) and exceedances.

### 6.2 WQX / ESAR (water quality)

- WQX is an XML implementation of the **ESAR** standard (successor to STORET).
- Mapping:
  - `project` → `Organization`/`Project`
  - `location` → `MonitoringLocation` (`MonitoringLocationIdentity`)
  - `task` → `Activity` (sampling event)
  - `sample`/`test`/`result` → `Result` with `CharacteristicName` (analyte),
    `MethodSpeciation` (fraction), `ResultMeasure`/`ResultMeasureValue`, `ResultUnit`,
    `DetectionQuantitationLimit` (type, value, unit), qualifiers, method, and
    `ActivityIdentifier` linkage.
- Supported result profiles (v1): `physical-chemistry` (full) and stubs for
  biological/habitat (later).
- Reference: WQX XML schema (exchangenetwork.net); WQX domain values
  (`cdx.epa.gov/wqx/download/DomainValues/All.zip`); WQX JSON schema
  (`github.com/datastreamapp/wqx`) as a convenient mapping reference.

### 6.3 AQS (air quality)

- EPA Air Quality System. Support **both** formats:
  1. legacy pipe-delimited **transaction format** (raw-data transaction code `RD`,
     plus site/monitor transactions as needed);
  2. modern **AQS XML** submission.
- Mapping: `location` → AQS site/monitor; `sample`/`result` → raw-data transaction
  (RD) with parameter (pollutant) code, method code, unit, sample date/time,
  detection limit, and null/qualifier codes.
- Reference: AQS Data Coding Manual / AQS Transaction Format documentation.

### 6.4 Washington State EIM

- WA Dept. of Ecology Environmental Information Management.
- Mapping: `project` → Study; `location` → Location; `sample`/`result` → Result rows
  using EIM parameter (analyte) codes, units, detection limits, and QA codes.
- Support **both** the EIM Excel import template and the EIM XML export, per the
  authoritative Ecology EIM schema.
- Reference: WA Ecology EIM data-submission documentation and EIM XML schema.

---

## 7. OGC API — Features (conformant)

Root path: `/ogc`. Conformance target: **OGC API — Features, Part 1: Core**
(OGC 17-069r3).

### 7.1 Resources

| Resource | Path | Description |
|---|---|---|
| Landing page | `/ogc/` | Links to conformance, collections, API definition |
| Conformance | `/ogc/conformance` | JSON array of conformance class URIs |
| Collections | `/ogc/collections` | List of feature collections |
| Collection | `/ogc/collections/{collectionId}` | Collection metadata |
| Features | `/ogc/collections/{collectionId}/items` | GeoJSON FeatureCollection |
| Feature | `/ogc/collections/{collectionId}/items/{featureId}` | Single GeoJSON feature |
| Basemap style | `/ogc/basemap/style.json` | Self-hosted MapLibre style (no external sources) |
| Basemap data | `/ogc/basemap/land`, `/ogc/basemap/boundaries` | Embedded Natural Earth GeoJSON (default provider) |
| Basemap sprites | `/ogc/basemap/sprite.png`, `/ogc/basemap/sprite.json` | Bundled Dark Matter sprite (OpenMapTiles provider) |
| API definition | `/ogc/api` (OpenAPI 3.0) | Service description (`service-desc` link) |

### 7.2 Conformance classes declared

- `http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core`
- `http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/oas30`
- `http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson`
- `http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/html` (landing/collection HTML)
- Extension (Part 3): CQL2-text filtering (`/req/cql2-text`).

### 7.3 Feature collections

| collectionId | Feature geometry | Notes |
|---|---|---|
| `projects` | polygon/point (project geometry) | |
| `tasks` | point/geometry (task extent) | linked to project |
| `locations` | point (primary; line/polygon supported) | the main map layer |
| `samples` | point (inherited/derived from location) | carries sample metadata |

- `results` are not standalone features (no geometry); they are exposed via REST
  (`/api/v1/results`) and as a sub-resource of samples
  (`/ogc/collections/samples/items/{featureId}/results` extension).

### 7.4 Query parameters (features)

`bbox` (CRS84 by default, `bbox-crs` for others), `datetime` (task/sample time),
`limit` (default/max page size), `offset` or next-page links, and `filter` (CQL2-text,
with `filter-lang=cql2-text`). GeoJSON responses include `numberMatched`,
`numberReturned`, and `next` links.

### 7.5 Notes

- CRS: serve GeoJSON in WGS84 (CRS84) per the spec; accept `bbox-crs` and reproject
  from PostGIS on demand.
- Use PostGIS `ST_AsGeoJSON`/`orb` for geometry; `orb/geojson` for encoding.

---

## 8. UI

Pages/screens:

1. **Dashboard** — company-wide metrics (projects, sampling events, locations,
   samples, results, imports); a navigable project grid; quick actions; and a
   recent-activity feed (EDD imports and record changes, newest first).
2. **Map** — MapLibre on a **self-hosted basemap** with two providers (no external
   tile service or API keys): embedded Natural Earth land/lakes/rivers/boundaries
   served by the API, or the OpenMapTiles **Dark Matter** vector-tile theme served
   from a self-hosted tile server (`BASEMAP_PROVIDER=openmaptiles`). Project
   **boundaries are true polygons** — real site footprints stored as
   `Polygon`/`MultiPolygon` geometry, rendered as a solid fill with a continuous
   outline and a soft outer glow (dashed outlines read as a point set at small
   sizes), with point locations filtered to markers so either geometry type works;
   chip toggles with per-collection counts and a colour legend; popups on click.
   The view auto-frames the project extent, an adaptive graticule supplies scale
   context on the Natural Earth provider, and a clear message replaces the map if
   WebGL is unavailable.
3. **Projects** — master/detail: select a project from a list and the workspace
   loads only that project's boundary (rendered as a polygon on a small map, with
   its vertex count), overview, tasks, locations and samples, each in its own tab
   with counts. Projects, tasks and locations can be created inline.
4. **Locations & Samples** — grids + detail; depth profiles (v2).
5. **Results** — query builder, result grid with non-detect/qualifier display,
   unit conversion toggle, exceedance highlighting.
6. **Action Levels** — manage types/groups/parameters; compare results.
7. **Reference Data** — analytes, units/conversions, matrices, methods, etc.
8. **EDD Import** — upload, format selection, validation error/warning report,
   commit/review workflow.
9. **Reports & Exports** — saved queries, crosstab generator, regulatory export wizard
   (WQX/AQS/EIM/Excel).

---

## 9. Non-Functional Requirements

- **Single-org**: one database per deployment.
- **Auth**: OIDC/SSO (generic OIDC discovery — works with Keycloak, Auth0, Entra ID,
  Google) is the primary human login; `api_key` (scoped, hashed) for programmatic and
  MCP access. Project-scoped roles: viewer, editor, admin.
- **Performance**: query pagination; indexed result/analyte/location lookups; spatial
  indexes (GiST) on geometry.
- **Auditability**: created/updated timestamps and user on all records; EDD import
  batches retained with their original file and validation report.
- **Testing**: OGC compliance (self-check + optional CITE test suite), importer
  golden-file tests, export schema validation tests.
- **Deployment**: Docker Compose (api, ui, postgis); single binary for the API.
- **License**: AGPL-3.0-only.

---

## 10. Roadmap (proposed)

1. **M0 — Foundation**: PostGIS schema + migrations (see `schema.sql`/`seed.sql`);
   Go API skeleton; OGC Features (core + geojson + oas30) for projects/tasks/locations/
   samples; OIDC auth + Docker Compose.
2. **M1 — Core data**: samples/tests/results CRUD; analytes/units/conversions;
   action levels + exceedance query.
3. **M2 — Import**: EDD format framework + EFWEDD importer + validation reports.
4. **M3 — Exports**: crosstab XLSX, CSV, EFWEDD round-trip.
5. **M4 — Regulatory exports**: WQX/ESAR, AQS, WA EIM adapters.
6. **M5 — UI**: map, query/result grids, EDD import wizard, action-level editor,
   reference data editor.
7. **M6 — Reports/dashboards + MCP server**.
8. **M7 — Hardening**: compliance tests, docs, packaging.

---

## 11. Decisions Complete
All open questions are resolved:

- **Name**: Samplier (AGPL-3.0-only)
- **OIDC**: Keycloak (generic OIDC discovery retained so other IdPs still work)
- **Geometry**: store EPSG:4326, reproject to whatever CRS is requested

Everything else is reflected in §2 and `schema.sql`.

---

## 11a. Testing & Continuous Integration

### 11a.1 Coverage requirement

- **Minimum 80% statement coverage** for the Go backend (`go test -coverprofile`),
  enforced in CI. The build fails below the threshold.
- Coverage is measured across `./internal/...` and `./cmd/...`; generated code and
  `main` bootstrap are excluded where appropriate.
- Frontend (once present): unit tests with **Vitest** and a coverage threshold of 80%
  for `src/lib` and `src/hooks`.

### 11a.2 Test layers

| Layer | Tooling | Scope |
|---|---|---|
| Unit | `go test` | unit conversion, action-level comparison, EDD validation, export encoding, OGC query parsing |
| Integration | `go test` + testcontainers/Docker PostGIS | store layer, REST handlers, OGC endpoints against real PostGIS |
| Golden files | `go test` | EDD import fixtures, XLSX/XML export output |
| Conformance | HTTP assertions | OGC API — Features core/geojson/oas30 responses |

Integration tests skip automatically when no database is reachable, so `go test ./...`
works offline and the CI job runs the full suite against PostGIS.

### 11a.3 GitHub Actions CI

Workflows in `.github/workflows/`:

1. **`ci.yml`** (on push + pull request)
   - `lint`: `gofmt -l`, `go vet`.
   - `test`: PostGIS service container, apply `schema.sql` + `seed.sql`, run
     `go test -race -coverprofile` with an **80% gate**.
   - `build`: compile all binaries.
2. **`image.yml`** (on push to `main` + tags)
   - Build and push multi-arch (`linux/amd64`, `linux/arm64`) images for the API to
     **GHCR** (`ghcr.io/<owner>/samplier`), tagged by semver/sha.
   - Frontend image/bundle build once the UI lands.
3. **`release.yml`** (on tag)
   - Cross-compiled Go binaries, checksums, GitHub release notes.

Secrets: `GITHUB_TOKEN` for GHCR. No AWS/GCP credentials needed for CI; Terraform
workflows live with the infrastructure code and require manual approval.

---

---

## 12. Deployment (Terraform: AWS + GCP)

Provide Infrastructure-as-Code to deploy the full stack on **AWS** and **GCP**.
Target components:

- **PostGIS**: managed PostgreSQL (AWS RDS for PostgreSQL / GCP Cloud SQL) with the
  PostGIS extension enabled, in a private subnet with restricted ingress.
- **API**: Go service as a container (AWS Fargate/ECS or GCP Cloud Run).
- **Frontend**: static React bundle (AWS S3 + CloudFront or GCP Cloud Storage + CDN).
- **Keycloak**: self-hosted container (AWS ECS/Fargate or GCP Cloud Run) with a
  realm/config imported on first boot.
- **Networking**: VPC + subnets + security groups/firewall; TLS via ACM / Google-managed
  certificates; DNS via Route 53 / Cloud DNS.
- **State**: Terraform remote state (AWS S3 + DynamoDB lock / GCP GCS bucket).

### 12.1 Confirmed deployment architecture

| Concern | AWS | GCP |
|---|---|---|
| Compute (API + Keycloak) | ECS **Fargate** (private subnets) | **Cloud Run** |
| PostGIS | **RDS for PostgreSQL** (Multi-AZ, PostGIS ext) | **Cloud SQL** (HA, PostGIS ext) |
| Frontend | **S3 + CloudFront** | **Cloud Storage + Cloud CDN** |
| Keycloak | Self-hosted Fargate container, **realm imported on boot** | Self-hosted Cloud Run container, **realm imported on boot** |
| HA | **Multi-AZ + autoscaling** | **Regional + autoscaling** |

Shared: VPC with public/private subnets + NAT, security groups / firewall rules,
secrets in AWS Secrets Manager / GCP Secret Manager, TLS via ACM / Google-managed
certificates, Terraform remote state (S3 + DynamoDB / GCS).

### 12.2 Deployment decisions — complete

- **DNS/TLS**: Terraform manages **Route 53 / Cloud DNS records + ACM / Google-managed
  certificates** (end-to-end deploy, requires a domain).
- **Scale baseline**: dev `db.t4g.small` / `db-f1-micro`; prod `db.m5.large` /
  `db-custom-4-8192`; API 1–3 Fargate tasks / min 1 Cloud Run instance.

---

## 13. Observability (Logging, Tracing, Metrics)

Samplier uses **OpenTelemetry (OTel)** as the single instrumentation standard across
the Go API, MCP server, and (optionally) the frontend.

- **Logging**: structured JSON via Go `log/slog`; every log line carries `trace_id`
  and `span_id` when inside a request; emitted to stdout and collected by the OTel
  Collector (or vendor agent).
- **Tracing**: OTel SDK, **OTLP** exporter, **W3C Trace Context** propagation.
- **Metrics**: OTel metrics + Prometheus exposition — HTTP RED (rate/errors/duration),
  DB pool saturation, runtime (GC/goroutines), and business metrics (samples loaded,
  EDD validation errors, exceedance counts).
- **Dev stack**: docker-compose adds OTel Collector + Prometheus + Grafana + Loki
  (logs) + Tempo (traces).
- **Prod**: OTel Collector → vendor observability (AWS CloudWatch/X-Ray/AMP; GCP Cloud
  Monitoring/Cloud Trace/Managed Prometheus), deployed by the Terraform modules.

### 13.1 Decisions

- **Backend**: dev = OSS Grafana stack (Prometheus + Loki + Tempo + Collector);
  prod = vendor-native via OTel (AWS CloudWatch/X-Ray/AMP; GCP Cloud Monitoring /
  Cloud Trace / Managed Prometheus).
- **Terraform**: the OTel Collector + Grafana stack are included in the Terraform
  modules (not just docker-compose).
- **Alerting**: implement basic alerts from the start — HTTP 5xx/error rate, p95/p99
  latency, DB pool saturation, EDD validation-failure spikes.
- **RUM**: server-side only for now (React UI RUM deferred to v2).
- **Retention**: default **14 days** for logs/metrics, **adjustable** via config;
  trace sampling **10%**.

### 13.2 Log availability and viewing

Logs are **structured JSON written to stdout/stderr** — the source of truth. The OTel
Collector is a *forwarder*, not the log destination, so logs remain available even if
Grafana/Loki/Tempo are down.

| Environment | Where logs land | How they are viewed |
|---|---|---|
| Dev | OTel Collector → Loki | Grafana → Explore (Loki datasource); also `docker compose logs` |
| AWS prod | Fargate → CloudWatch Logs (via awslogs driver) | CloudWatch console and/or Grafana CloudWatch datasource |
| GCP prod | Cloud Run → Cloud Logging | Cloud Logging console and/or Grafana Cloud Logging datasource |

Guarantees:
- Container runtime logs (docker/Fargate/Cloud Run) always capture stdout even without
  the collector.
- Prod vendor log sinks retain per policy (default 14 days, adjustable).
- Logs carry `trace_id`/`span_id` for correlation from a log line to its trace.

---

## 14. Implementation Status

Everything in this specification is implemented and verified.

| Area | Status | Evidence |
|---|---|---|
| Data model (`schema.sql`) | Done | 36 tables applied to PostGIS 16; seed data loads |
| Project → task → location → sample → test → result CRUD | Done | `internal/store`, `internal/api` |
| Well construction (header + segments) | Done | `well`, `well_segment`, `/locations/{id}/well-segments` |
| Action levels + exceedance detection | Done | `internal/actionlevel` (92.9% coverage), `/api/v1/exceedances` |
| Unit conversion (factor/delta + formulas) | Done | `internal/units` (90.4% coverage) |
| EFWEDD import (XLSX/CSV/TSV/ZIP) | Done | `internal/edd`; verified end-to-end with a 4-file ZIP |
| Pluggable EDD format framework | Done | `edd.Definition` registry |
| Validation reports + 3 commit modes | Done | per-row findings; INSERT_ONLY/UPDATE/MERGE |
| Crosstab XLSX export | Done | `internal/export/crosstab.go` |
| WQX/ESAR XML export | Done | `internal/export/wqx.go`, XML validated |
| AQS export (pipe + XML) | Done | `internal/export/aqs.go` |
| WA EIM export (CSV + XML) | Done | `internal/export/eim.go` |
| OGC API — Features (core/geojson/oas30) | Done | `/ogc/*`; bbox, datetime, paging verified |
| REST API | Done | `/api/v1/*` |
| MCP server (9 tools) | Done | stdio JSON-RPC verified against PostGIS |
| Keycloak OIDC/SSO | Done | live realm + token accepted; `docs/keycloak.md` |
| Observability (logs/traces/metrics) | Done | `internal/telemetry`; OTel + Grafana dev stack |
| React + MapLibre UI | Done | `ui/` — map, projects, results, limits, import, exports |
| Self-hosted basemap | Done | `internal/basemap` (100% coverage); Natural Earth + OpenMapTiles/Dark Matter, no external tile service or API keys |
| Terraform (AWS + GCP) | Done | `infra/`; `terraform validate` passes for both |
| Docker image + CI/CD | Done | `Dockerfile`, `ci.yml`, `image.yml`, `release.yml` |
| Test coverage ≥ 80% | Done | Backend 83.8%; UI 100% on `lib/` |

### Verification performed

- Schema + seed applied to PostGIS 16 (36 tables, seeded vocabularies).
- Full EDD cycle: ZIP of 4 CSVs → validate → load (2 samples, 2 tests,
  3 results, 1 batch) → exceedance detection → five export formats.
- OGC endpoints return valid GeoJSON with `numberMatched`/`numberReturned`,
  bbox and datetime filtering, and `[]` (not `null`) for empty pages.
- Keycloak: live token → 200; missing/invalid token → 401; `/ogc` remains public.
- All Go tests pass repeatedly (14 packages); `go vet` and `gofmt` clean.

---

## 15. Brand

**Name:** Samplier — *sampling* + *sommelier*.
**Theme:** night market — deep ink canvas with sampling green, serving blue, and
hot pink reserved exclusively for regulatory exceedances.

The mark is a single sample droplet with a meniscus fill line: the one detail
that makes a droplet read as a measured volume. It is deliberately literal, has
no tagline, and holds at 16 px.

| Asset | Location |
|---|---|
| Mark (transparent) | `assets/brand/logo-mark.svg` |
| Mark (tiled) | `assets/brand/logo-mark-tile.svg`, `ui/public/favicon.svg` |
| Lockup (dark / light) | `assets/brand/logo-lockup.svg`, `logo-lockup-light.svg` |
| Social preview | `assets/brand/social-preview.png` (1280×640) |
| Palette, type, voice, usage | `assets/brand/README.md` |
| Screenshots | `assets/screenshots/` |
| In-app mark component | `ui/src/components/BrandMark.tsx` |

Colour semantics are part of the contract, not decoration:

| Colour | Token | Meaning |
|---|---|---|
| Green `#35b37e` | `--green` | Sampling; primary accent, locations |
| Blue `#6fa8dc` | `--blue` | Serving; links, samples |
| Pink `#ff5c8a` | `--exceed` | **Exceedances only** |
| Amber `#e0a952` | `--warn` | Warnings, tasks |
| Red `#e05252` | `--danger` | System errors (never exceedances) |

Pink is applied consistently to the UI (exceedance badge, row wash, result value),
the map, and the Excel crosstab (`#FFE3EC` fill / `#B3245A` text).
