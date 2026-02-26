#!/usr/bin/env bash
# test-api.sh — Exercise every Signal-Flow API endpoint via curl.
# Assumes the server is running on localhost:8088.
# Usage: ./scripts/test-api.sh
set -euo pipefail

BASE_URL="${API_URL:-http://localhost:8088}"
TENANT="00000000-0000-0000-0000-000000000001"
PASS=0
FAIL=0
TOTAL=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
DIM='\033[2m'
NC='\033[0m'

# ─── Helpers ────────────────────────────────────────────────

assert() {
  local name="$1"
  local expected_status="$2"
  local actual_status="$3"
  local body="$4"

  TOTAL=$((TOTAL + 1))

  if [ "$actual_status" -eq "$expected_status" ]; then
    PASS=$((PASS + 1))
    echo -e "  ${GREEN}✓${NC} ${name} ${DIM}(${actual_status})${NC}"
  else
    FAIL=$((FAIL + 1))
    echo -e "  ${RED}✗${NC} ${name} — expected ${expected_status}, got ${actual_status}"
    echo -e "    ${DIM}${body}${NC}"
  fi
}

# curl wrapper: returns "STATUS_CODE\nBODY"
api() {
  local method="$1"
  shift
  local path="$1"
  shift
  # remaining args are passed to curl (e.g. -d '...')
  local response
  response=$(curl -s -w "\n%{http_code}" \
    -X "$method" \
    -H "Content-Type: application/json" \
    "$@" \
    "${BASE_URL}${path}")

  local status=$(echo "$response" | tail -1)
  local body=$(echo "$response" | sed '$d')
  echo "$status"
  echo "$body"
}

# Convenience: api with tenant header
tapi() {
  local method="$1"
  shift
  local path="$1"
  shift
  api "$method" "$path" -H "X-Tenant-ID: $TENANT" "$@"
}

run_test() {
  local name="$1"
  local expected="$2"
  # Read status + body from stdin
  local status
  local body
  read -r status
  body=$(cat)
  assert "$name" "$expected" "$status" "$body"
}

echo ""
echo -e "${CYAN}╔═══════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║       signal-flow API Test Suite (curl)       ║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════════╝${NC}"
echo ""

# ─── 1. Health Check ────────────────────────────────────────

echo -e "${YELLOW}Health${NC}"

result=$(api GET /api/health)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/health → 200" 200 "$status" "$body"

echo ""

# ─── 2. Middleware Guards ───────────────────────────────────

echo -e "${YELLOW}Middleware${NC}"

# Missing tenant header
result=$(api GET /api/signals)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/signals (no tenant) → 400" 400 "$status" "$body"

# Invalid tenant UUID
result=$(api GET /api/signals -H "X-Tenant-ID: not-a-uuid")
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/signals (bad UUID) → 400" 400 "$status" "$body"

echo ""

# ─── 3. Signals ─────────────────────────────────────────────

echo -e "${YELLOW}Signals${NC}"

# List signals (empty for fresh tenant)
result=$(tapi GET /api/signals)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/signals → 200" 200 "$status" "$body"

# List with custom limit
result=$(tapi GET "/api/signals?limit=5")
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/signals?limit=5 → 200" 200 "$status" "$body"

# Semantic search (minimal 3-dim vector — will work if pgvector is set up)
result=$(tapi POST /api/signals/search \
  -d '{"vector": [0.1, 0.2, 0.3], "limit": 5}')
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/signals/search → 200" 200 "$status" "$body"

# Search with missing vector
result=$(tapi POST /api/signals/search \
  -d '{"limit": 5}')
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/signals/search (no vector) → 400" 400 "$status" "$body"

# Promote with invalid UUID
result=$(tapi POST /api/signals/not-a-uuid/promote)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/signals/{bad-id}/promote → 400" 400 "$status" "$body"

echo ""

# ─── 4. Credentials ─────────────────────────────────────────

echo -e "${YELLOW}Credentials${NC}"

# Link a provider credential
result=$(tapi POST /api/credentials \
  -d '{"provider": "bluesky", "token": "test-token-12345"}')
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/credentials (link bluesky) → 201" 201 "$status" "$body"

# Get the token back (dev-only endpoint)
result=$(tapi GET /api/credentials/bluesky)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/credentials/bluesky → 200" 200 "$status" "$body"

# Verify the token value is correct
token_value=$(echo "$body" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
if [ "$token_value" = "test-token-12345" ]; then
  TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1))
  echo -e "  ${GREEN}✓${NC} Token round-trip matches ${DIM}(test-token-12345)${NC}"
else
  TOTAL=$((TOTAL + 1)); FAIL=$((FAIL + 1))
  echo -e "  ${RED}✗${NC} Token round-trip mismatch — got '${token_value}'"
fi

# List users by provider
result=$(tapi GET "/api/credentials?provider=bluesky")
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/credentials?provider=bluesky → 200" 200 "$status" "$body"

# Missing provider param
result=$(tapi GET /api/credentials)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "GET /api/credentials (no provider) → 400" 400 "$status" "$body"

# Link with missing fields
result=$(tapi POST /api/credentials \
  -d '{"provider": ""}')
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/credentials (empty provider) → 400" 400 "$status" "$body"

echo ""

# ─── 5. Synthesize ──────────────────────────────────────────

echo -e "${YELLOW}Synthesize${NC}"

# Should return 503 unless LLM keys are configured
result=$(tapi POST /api/synthesize \
  -d '{"source_url": "https://example.com/article", "content": "Test content", "priority": 0}')
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/synthesize → 503 (no LLM keys)" 503 "$status" "$body"

echo ""

# ─── 6. Harvest ─────────────────────────────────────────────

echo -e "${YELLOW}Harvest${NC}"

# Should return 503 (coordinator not wired)
result=$(tapi POST /api/harvest)
status=$(echo "$result" | head -1)
body=$(echo "$result" | tail -n +2)
assert "POST /api/harvest → 503 (not configured)" 503 "$status" "$body"

echo ""

# ─── Summary ────────────────────────────────────────────────

echo -e "${CYAN}═══════════════════════════════════════════════${NC}"
if [ "$FAIL" -eq 0 ]; then
  echo -e "  ${GREEN}All ${TOTAL} tests passed ✓${NC}"
else
  echo -e "  ${GREEN}${PASS} passed${NC}  ${RED}${FAIL} failed${NC}  (${TOTAL} total)"
fi
echo -e "${CYAN}═══════════════════════════════════════════════${NC}"
echo ""

exit "$FAIL"
