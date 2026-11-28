# Custom reporting — demo artifacts

Generated from the running Samplier demo dataset (DEMO-1 Riverside Industrial
Site) with the custom report builder.

## Contents

| File | What it is |
|---|---|
| `custom-report.xlsx` | Multi-tab crosstab workbook produced by `POST /api/v1/reports/export`. Sheets: **Summary**, **Metals**, **VOCs**, **Exceedances**. |
| `saved-report.xlsx` | The same workbook produced from a saved view via `GET /api/v1/reports/{id}/export`. |
| `custom-report-preview.json` | The UI preview payload from `POST /api/v1/reports/preview` (tabs, sample columns, cell values, flags, exceedance marks). |

## The report that produced these

- Project: `DEMO-1` (Riverside Industrial Site)
- Analyte grouping sets (one tab each): `DEMO-METALS` (Metals), `DEMO-VOCS` (VOCs)
- Regulatory limit sets (evaluated together): `DEMO-EPA-MCL` (EPA), `DEMO-STATE-SL` (State)
- Non-detects included

Because two limit sets are selected, each exceeding result is listed once per
limit set in the **Exceedances** sheet, and the crosstab cell flags show
`EXCEED:DEMO-EPA-MCL+DEMO-STATE-SL` when a result breaches both.

## Reproduce

```bash
# 1. Reload the demo dataset (seeds analyte groups + limit sets)
make demo-data

# 2. Rebuild + restart the API
make build && ./demo/run.sh --api-only

# 3. Preview
curl -s -X POST localhost:8080/api/v1/reports/preview \
  -H 'Content-Type: application/json' \
  -d @/tmp/report-spec.json | jq

# 4. Export the workbook
curl -s -X POST localhost:8080/api/v1/reports/export \
  -H 'Content-Type: application/json' \
  -d @/tmp/report-spec.json -o custom-report.xlsx
```

The UI lives at **Custom Reports** (`http://localhost:5173/reports`); screenshots
are in `assets/screenshots/`.
