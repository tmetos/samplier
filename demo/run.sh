#!/usr/bin/env bash
#
# Samplier local demo — starts the stack, loads the demo dataset, and runs the
# API + UI with auth disabled so you can click around immediately.
#
#   ./demo/run.sh              # start everything (default)
#   ./demo/run.sh --api-only   # skip the UI dev server
#   ./demo/run.sh --reset      # recreate the demo project first
#
set -euo pipefail

cd "$(dirname "$0")/.."

API_PORT="${API_PORT:-8080}"
UI_PORT="${UI_PORT:-5173}"
DB_CONTAINER="${DB_CONTAINER:-samplier-db-1}"
DATABASE_URL="${DATABASE_URL:-postgres://samplier:samplier@localhost:5432/samplier}"

API_ONLY=0
RESET=0
for arg in "$@"; do
  case "$arg" in
    --api-only) API_ONLY=1 ;;
    --reset)    RESET=1 ;;
    -h|--help)  sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

say() { printf '\033[36m▸ %s\033[0m\n' "$*"; }
ok()  { printf '\033[32m✓ %s\033[0m\n' "$*"; }
die() { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# 1. Database + Keycloak
# ---------------------------------------------------------------------------
say "Starting PostGIS and Keycloak"
if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  die "Docker Compose not found (install the plugin or docker-compose)"
fi
$COMPOSE up -d db keycloak

say "Waiting for PostGIS to accept connections"
for i in $(seq 1 60); do
  if docker exec "$DB_CONTAINER" pg_isready -U samplier -d samplier >/dev/null 2>&1; then
    ok "database ready"
    break
  fi
  [ "$i" -eq 60 ] && die "database did not become ready in time"
  sleep 1
done

# ---------------------------------------------------------------------------
# 2. Schema (idempotent: only if the schema is missing)
# ---------------------------------------------------------------------------
if ! docker exec "$DB_CONTAINER" psql -U samplier -d samplier -tAc \
      "SELECT 1 FROM information_schema.tables WHERE table_name='project'" \
      | grep -q 1; then
  say "Applying schema.sql and seed.sql"
  docker exec -i "$DB_CONTAINER" psql -U samplier -d samplier -v ON_ERROR_STOP=1 < schema.sql >/dev/null
  docker exec -i "$DB_CONTAINER" psql -U samplier -d samplier -v ON_ERROR_STOP=1 < seed.sql >/dev/null
  ok "schema applied"
fi

# ---------------------------------------------------------------------------
# 3. Demo dataset
# ---------------------------------------------------------------------------
HAS_DEMO=$(docker exec "$DB_CONTAINER" psql -U samplier -d samplier -tAc \
  "SELECT count(*) FROM project WHERE code='DEMO-1'" 2>/dev/null || echo 0)

if [ "$RESET" -eq 1 ] || [ "$HAS_DEMO" = "0" ]; then
  say "Loading the Riverside Industrial Site demo dataset"
  docker exec -i "$DB_CONTAINER" psql -U samplier -d samplier -v ON_ERROR_STOP=1 \
    < demo/demo.sql | tail -6
  ok "demo data loaded"
else
  ok "demo data already present (use --reset to rebuild)"
fi

# ---------------------------------------------------------------------------
# 4. API
# ---------------------------------------------------------------------------
say "Building and starting the API on :$API_PORT"
mkdir -p bin
go build -trimpath -o bin/samplier ./cmd/server

pkill -f "bin/samplier serve" 2>/dev/null || true
sleep 0.3

AUTH_ENABLED=false \
DATABASE_URL="$DATABASE_URL" \
PORT="$API_PORT" \
  ./bin/samplier serve >/tmp/samplier-demo-api.log 2>&1 &
API_PID=$!

for i in $(seq 1 40); do
  if curl -fsS "localhost:$API_PORT/healthz" >/dev/null 2>&1; then
    ok "API ready (pid $API_PID)"
    break
  fi
  [ "$i" -eq 40 ] && { tail -20 /tmp/samplier-demo-api.log; die "API failed to start"; }
  sleep 0.5
done

# ---------------------------------------------------------------------------
# 5. UI
# ---------------------------------------------------------------------------
if [ "$API_ONLY" -eq 0 ]; then
  if [ ! -d ui/node_modules ]; then
    say "Installing UI dependencies (first run)"
    (cd ui && npm install --silent)
  fi
  say "Starting the UI on :$UI_PORT"
  (cd ui && VITE_API_PORT="$API_PORT" npm run dev -- --port "$UI_PORT" --strictPort \
     >/tmp/samplier-demo-ui.log 2>&1 &)
fi

# ---------------------------------------------------------------------------
# 6. Report
# ---------------------------------------------------------------------------
cat <<EOF

  Samplier demo is running
  ────────────────────────────────────────────────────────────
  UI            http://localhost:$UI_PORT
  API           http://localhost:$API_PORT
  OGC Features  http://localhost:$API_PORT/ogc/collections
  Keycloak      http://localhost:8081   (admin / admin)

  Try these:

  # Wells on the map
  curl "localhost:$API_PORT/ogc/collections/locations/items" | jq '.numberMatched'

  # EPA exceedances at the demo site
  curl "localhost:$API_PORT/api/v1/exceedances?project_id=\$(curl -s \
    "localhost:$API_PORT/api/v1/projects" | jq -r '.items[0].id')&type=EPA" | jq '.count'

  # Download a crosstab (XLSX) and the WQX XML
  PROJ=\$(curl -s "localhost:$API_PORT/api/v1/projects" | jq -r '.items[0].id')
  curl -o crosstab.xlsx  "localhost:$API_PORT/api/v1/exports/CROSSTAB?project_id=\$PROJ&type=EPA"
  curl -o wqx.xml        "localhost:$API_PORT/api/v1/exports/WQX?project_id=\$PROJ&type=EPA"

  # MCP server over stdio
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | make -s mcp

  Logs: /tmp/samplier-demo-api.log, /tmp/samplier-demo-ui.log
  Stop: make down   (or ./demo/run.sh --reset to rebuild the data)

EOF
