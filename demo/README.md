# Samplier demo

Two contrasting environmental sites so you can explore every feature — and every
outcome — without preparing data.

## Run it

```bash
./demo/run.sh              # stack + demo data + API + UI
./demo/run.sh --api-only   # skip the UI dev server
./demo/run.sh --reset      # rebuild the demo project from scratch
```

Or via make:

```bash
make demo
make demo-api      # API only
make demo-data     # reload the dataset into a running database
make demo-reset    # rebuild data and restart the API
```

Then open:

| What | Where |
|---|---|
| UI | <http://localhost:5173> |
| API | <http://localhost:8080> |
| OGC API — Features | <http://localhost:8080/ogc/collections> |
| Keycloak admin | <http://localhost:8081> (admin / admin) |

Auth is disabled in the demo, so the REST API is callable without a token.

## The datasets

Two contrasting sites, so every UI state has real data behind it.

### DEMO-1 — Riverside Industrial Site

A former plating facility under quarterly groundwater monitoring.

**3 monitoring wells** (Portland, OR) with construction details:

| Well | Role | Depth | Screen |
|---|---|---|---|
| MW-01 | Upgradient background | 32 ft | 20–32 ft |
| MW-02 | Source area | 38 ft | 26–38 ft |
| MW-03 | Downgradient boundary | 35 ft | 23–35 ft |

**2 events** (`Q1-2024`, `Q2-2024`) · **7 samples** (including a field duplicate
linked to its parent) · **12 tests** (EPA 6020B metals, EPA 8260D VOCs) ·
**36 results**. MW-02 is contaminated across all six analytes; MW-03 shows
migrating Lead.

### DEMO-2 — Northgate Landfill

A closed municipal landfill under semi-annual groundwater and surface water
monitoring, ~5 km from the Riverside site.

| Location | Type | Role |
|---|---|---|
| NG-MW-01 | Well, 28 ft | Upgradient — clean |
| NG-MW-02 | Well, 34 ft | Downgradient — leachate-affected |
| NG-SW-01 | Surface water | Downgradient receiving water |

**2 events** (`NG-2024S` spring, `NG-2024F` autumn) · **6 samples** ·
**12 tests** · **48 results** across four metals (Pb, As, Cd, Cr) and four VOCs
(vinyl chloride, benzene, TCE, PCE).

### Every outcome is represented

The point of two projects is that the numbers *differ*:

| Sample | Detects | Above EPA MCL | Reads as |
|---|---|---|---|
| `NG-2024S-MW-01`, `NG-2024F-MW-01` | 0 of 8 | 0 | **nothing detected** |
| `Q2-2024-MW-01` | 0 of 3 | 0 | **nothing detected** |
| `Q1-2024-MW-01` | 1 of 3 | 0 | compliant (one low detect) |
| `NG-2024S-SW-01` | 4 of 8 | 0 | compliant (all below limits) |
| `NG-2024F-SW-01` | 2 of 8 | 0 | compliant |
| `NG-2024S-MW-02` | 8 of 8 | 5 | **exceedances** |
| `NG-2024F-MW-02` | 5 of 8 | 1 | mostly recovering, one breach |
| `Q1/Q2-2024-MW-02` (+FD) | 6 of 6 | 6 | **exceedances** |
| `Q1/Q2-2024-MW-03` | 2–4 | 1 | one Lead breach |

Two action-level sets apply to both projects — `DEMO-EPA-MCL` (drinking-water
MCLs) and `DEMO-STATE-SL` (stricter state screening levels), so switching the
type in the UI changes the exceedance count (24 vs 20 for Riverside).

## Things to try

**Company-wide metrics and per-project rollup:**

```bash
curl -s localhost:8080/api/v1/stats \
  | jq '{projects, samples, results, project_detail: [.project_detail[] | {code, samples, results}]}'
```

**Exceedances per project (EPA vs. the stricter state levels)** — note how the
two projects differ:

```bash
for CODE in DEMO-1 DEMO-2; do
  PROJ=$(curl -s localhost:8080/api/v1/projects | jq -r --arg c "$CODE" '.items[] | select(.code==$c) | .id')
  printf "%s  EPA=%s  STATE=%s\n" "$CODE" \
    "$(curl -s "localhost:8080/api/v1/exceedances?project_id=$PROJ&type=EPA"   | jq '.count')" \
    "$(curl -s "localhost:8080/api/v1/exceedances?project_id=$PROJ&type=STATE" | jq '.count')"
done
```

**Recent activity (imports and record changes):**

```bash
curl -s "localhost:8080/api/v1/activity?limit=5" | jq '.items[] | {kind, title, at}'
```

**Basemap style (no external dependencies):**

```bash
curl -s localhost:8080/ogc/basemap/style.json | jq '{name, sources: (.sources|keys)}'
curl -s localhost:8080/ogc/basemap/style.json | grep -c 'https\?://' || echo "0 external URLs"
```

**Wells on the map (OGC API — Features):**

```bash
curl -s "localhost:8080/ogc/collections/locations/items" | jq '.numberMatched'
curl -s "localhost:8080/ogc/collections/locations/items" \
  | jq '.features[] | {name: .properties.name, coords: .geometry.coordinates}'
```

**Samples filtered by time (bbox + datetime):**

```bash
curl -s "localhost:8080/ogc/collections/samples/items?datetime=2024-04-01/2024-04-30" \
  | jq '.numberMatched'
```

**Reports and regulatory exports:**

```bash
curl -o crosstab.xlsx "localhost:8080/api/v1/exports/CROSSTAB?project_id=$PROJ&type=EPA"
curl -o results.csv   "localhost:8080/api/v1/exports/CSV?project_id=$PROJ&type=EPA"
curl -o wqx.xml       "localhost:8080/api/v1/exports/WQX?project_id=$PROJ&type=EPA"
curl -o aqs.txt       "localhost:8080/api/v1/exports/AQS?project_id=$PROJ&type=EPA"
curl -o eim.csv       "localhost:8080/api/v1/exports/EIM?project_id=$PROJ&type=EPA"

# Detects only (common for regulatory submissions)
curl -o detects.csv "localhost:8080/api/v1/exports/CSV?project_id=$PROJ&detects_only=true"
```

The crosstab has two sheets: **Results** (analytes as rows, samples as column
groups, exceedances highlighted red, non-detects italic grey) and **Exceedances**.

**EDD import round trip.** Build an EFWEDD ZIP and load it:

```bash
mkdir -p /tmp/edd && cd /tmp/edd
cat > EFW2FSample.csv <<'CSV'
SYS_LOC_CODE,SAMPLE_ID,SAMPLE_TYPE_CODE,MATRIX_CODE,SAMPLE_DATE
1,DEMO-NEW-1,N,WATER,2024-07-15
CSV
cat > EFW2LabTST.csv <<'CSV'
SAMPLE_ID,TEST_ID,ANALYTIC_METHOD,FRACTION,ANALYSIS_DATE
DEMO-NEW-1,DEMO-NEW-1-MET,EPA 6020B,TOTAL,2024-07-19
CSV
cat > EFW2LabRES.csv <<'CSV'
SAMPLE_ID,TEST_ID,CAS_RN,CHEMICAL_NAME,RESULT_VALUE,RESULT_UNIT,DETECT_FLAG
DEMO-NEW-1,DEMO-NEW-1-MET,7439-92-1,Lead,0.021,mg/L,Y
CSV
zip -q edd.zip *.csv

curl -s -X POST "localhost:8080/api/v1/edd/validate?format=EFWEDD" \
  -F "file=@edd.zip" | jq '{valid, sections}'
```

> Note: `SYS_LOC_CODE` must be an existing location's code. Look one up with
> `curl -s localhost:8080/api/v1/locations?project_id=$PROJ | jq '.items[].sys_loc_code'`.

**MCP server** (agent access over stdio):

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_exceedances","arguments":{"project_id":"'$PROJ'","type":"EPA"}}}' \
  | make -s mcp
```

**Unit conversion** — results are stored in mg/L; the conversion table covers
mg/L ↔ µg/L, mg/kg ↔ µg/kg, ft ↔ m, plus °F ↔ °C via a formula.

## What to look at in the UI

- **Dashboard** — company-wide metrics (projects, tasks, locations, samples,
  results, imports), a project card you can click through, quick actions, and a
  **recent activity** feed of imports and record changes
- **Map** — both sites' **project boundaries as filled irregular polygons** (site
  footprints, not bounding boxes or point clouds), with wells and the surface-water station
  inside them (green dots) and sample rings in blue. Toggle each collection with
  the chips at the top; a legend explains the colours. Self-hosted basemap
  (embedded Natural Earth — no API key, no third-party requests)
- **Projects** — pick a project from the list on the left; the right side shows
  only that project: its **boundary polygon** rendered on a mini map (with vertex
  count), plus **Overview**, **Tasks**, **Locations** and **Samples** tabs, each
  with its own counts. You can add a project, task or location inline
- **Results** — exceedance table with red highlighting; switch EPA ↔ State to see more
- **Action Levels** — both limit sets with per-analyte values, plus full CRUD
  for limit sets and their parameters
- **Reference Data** — add/edit/delete analytes and analyte grouping sets, and
  manage which analytes belong to each set
- **Custom Reports** — build a multi-tab crosstab, preview it, export XLSX, and
  save views for reuse
- **EDD Import** — validate-only or commit, with inline error/warning rows
- **Reports & Exports** — download each format; "Detects only" toggle

## Reset / clean up

```bash
make demo-reset   # rebuild demo data
make down         # stop the containers
```

The demo project is removed and recreated by `demo/demo.sql`, so it is always
safe to re-run.

## Logs

- API: `/tmp/samplier-demo-api.log`
- UI: `/tmp/samplier-demo-ui.log`
