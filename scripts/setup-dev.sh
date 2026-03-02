#!/usr/bin/env bash
# setup-dev.sh — Start Postgres, run migrations, and print the server start command.
# Usage: ./scripts/setup-dev.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}▸${NC} $*"; }
warn()  { echo -e "${YELLOW}▸${NC} $*"; }
step()  { echo -e "${CYAN}▸${NC} $*"; }

CONTAINER=signal-flow-db
DB_USER=signalflow
DB_PASS=signalflow
DB_NAME=signal_flow_dev

# --- 1. Docker Compose ---
step "Starting Postgres (pgvector:pg16)..."
docker compose -f "$PROJECT_DIR/docker-compose.yml" up -d 2>/dev/null || \
  docker-compose -f "$PROJECT_DIR/docker-compose.yml" up -d 2>/dev/null

# Wait for Postgres to accept connections via docker exec
step "Waiting for Postgres to be ready..."
for i in $(seq 1 30); do
  if docker exec "$CONTAINER" pg_isready -U "$DB_USER" -q 2>/dev/null; then
    break
  fi
  sleep 1
done

if ! docker exec "$CONTAINER" pg_isready -U "$DB_USER" -q 2>/dev/null; then
  warn "Postgres not ready after 30s — check 'docker compose logs postgres'"
  exit 1
fi
info "Postgres is ready"

# --- 2. Run Migrations ---
step "Running migrations..."
for migration in "$PROJECT_DIR"/migrations/*.up.sql; do
  base=$(basename "$migration")
  docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -q < "$migration" 2>/dev/null
  info "  ✓ $base"
done

# --- 3. Generate Encryption Key ---
if [ -z "${ENCRYPTION_KEY:-}" ]; then
  ENCRYPTION_KEY=$(openssl rand -hex 32)
fi
info "ENCRYPTION_KEY=$ENCRYPTION_KEY"

# --- 4. Summary ---
echo ""
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "  Database:  ${CYAN}localhost:5433/$DB_NAME${NC}"
echo -e "  Tenant ID: ${CYAN}00000000-0000-0000-0000-000000000001${NC}"
echo -e "  Enc Key:   ${CYAN}$ENCRYPTION_KEY${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo ""
echo -e "  Start the server:"
echo -e "  ${YELLOW}ENCRYPTION_KEY=$ENCRYPTION_KEY go run ./cmd/signal-flow${NC}"
echo ""
echo -e "  Run API tests:"
echo -e "  ${YELLOW}./scripts/test-api.sh${NC}"
echo ""
