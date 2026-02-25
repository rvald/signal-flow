---
title: "Phase 5: API Layer"
project: signal-flow
phase: 5
status: complete
date_completed: 2026-02-25
authors:
  - rvald
tags:
  - http
  - rest
  - middleware
  - stdlib
  - tdd
dependencies:
  - net/http (stdlib)
  - github.com/jackc/pgx/v5
  - github.com/google/uuid
---

# Phase 5: API Layer

The HTTP interface for Signal-Flow. Exposes all domain services over REST using Go 1.25 stdlib routing (`mux.HandleFunc("GET /path", ...)`). Zero external router dependencies. Designed for local `curl` testing during development.

## Project Structure

```
signal-flow/
├── cmd/signal-flow/main.go                              # Server entrypoint + wiring
├── internal/
│   └── api/
│       ├── response.go                                  # JSON/Error response helpers
│       ├── middleware.go                                 # TenantMiddleware, Logging, Recovery
│       ├── health_handler.go                            # GET /api/health
│       ├── signal_handler.go                            # Signals CRUD + search
│       ├── identity_handler.go                          # Credentials management
│       ├── synthesize_handler.go                        # Intelligence pipeline trigger
│       ├── harvester_handler.go                         # Harvest cycle trigger
│       └── handler_test.go                              # 9 unit tests (httptest)
└── migrations/
    ├── 000005_seed_dev_user.up.sql                      # Dev user seed
    └── 000005_seed_dev_user.down.sql                    # Rollback
```

## Source Index

### Middleware

- **TenantMiddleware** — [internal/api/middleware.go](file:///signal-flow/internal/api/middleware.go)
  - Reads `X-Tenant-ID` header, parses UUID, stores in `context.Context`
  - Returns 400 if missing or invalid
  - Exempt paths: `/api/health` (checked via `tenantExemptPrefixes`)

- **LoggingMiddleware** — `slog.Info` per request: method, path, status, duration_ms

- **RecoveryMiddleware** — Catches panics, logs stack, returns 500

- **ExportedTenantKey** — Allows tests to inject tenant context without middleware

### Response Helpers

- **JSON(w, status, data)** — [internal/api/response.go](file:///signal-flow/internal/api/response.go)
- **Error(w, status, message)** — `{ "error": "..." }`

### Endpoints

| Endpoint | Method | Handler | Description |
|---|---|---|---|
| `/api/health` | `GET` | `HealthHandler` | Returns `{ "status": "ok" }` |
| `/api/signals` | `GET` | `SignalHandler` | `FindRecentByTenant` — `?limit=20` (max 100) |
| `/api/signals/search` | `POST` | `SignalHandler` | `SearchSemantic` — body: `{ "vector": [...], "limit": 10 }` |
| `/api/signals/{id}/promote` | `POST` | `SignalHandler` | `PromoteToTeam` — path param `{id}` via `r.PathValue` |
| `/api/credentials` | `POST` | `IdentityHandler` | `LinkProvider` — body: `{ "provider": "bluesky", "token": "..." }` |
| `/api/credentials/{provider}` | `GET` | `IdentityHandler` | `GetActiveToken` — returns decrypted token (dev only) |
| `/api/credentials` | `GET` | `IdentityHandler` | `ListUsersByProvider` — `?provider=bluesky` |
| `/api/synthesize` | `POST` | `SynthesizeHandler` | `Synthesize` — body: `{ "source_url", "content", "priority" }` |
| `/api/harvest` | `POST` | `HarvesterHandler` | `RunOnce` — triggers single harvest cycle |

### Server Entrypoint

- **main.go** — [cmd/signal-flow/main.go](file:///signal-flow/cmd/signal-flow/main.go)
  - Env config: `PORT` (default `8088`), `DATABASE_URL`, `ENCRYPTION_KEY` (hex)
  - Connects `pgxpool`, instantiates `LocalKeyManager`
  - Wires: `PostgresSignalRepository`, `PostgresIdentityRepository`
  - Middleware chain: `Recovery → Logging → TenantContext`
  - Graceful shutdown on `SIGINT` / `SIGTERM`
  - Synthesizer returns 503 if no LLM keys, Harvester returns 503 until provider wiring

### Database Schema

- **Migration 000005** — [000005_seed_dev_user.up.sql](file:///signal-flow/migrations/000005_seed_dev_user.up.sql)
  - Seeds dev user `00000000-0000-0000-0000-000000000001` / `dev@signal-flow.local`
  - `ON CONFLICT (id) DO NOTHING` for idempotency

### Tests

- **Handler unit tests** — [internal/api/handler_test.go](file:///signal-flow/internal/api/handler_test.go)
  - `TestHealthCheck` — 200 + `{"status":"ok"}`
  - `TestListSignals_EmptyTenant` — 200 + `[]`
  - `TestListSignals_MissingTenantContext` — 400
  - `TestTenantMiddleware_MissingHeader` — 400
  - `TestTenantMiddleware_InvalidUUID` — 400
  - `TestTenantMiddleware_HealthExempt` — 200 without header
  - `TestLinkCredential` — 201 + verifies `LinkProvider` called
  - `TestSynthesize_NotConfigured` — 503
  - `TestHarvest_NotConfigured` — 503

## Design Decisions

- **Stdlib `net/http` only** — Go 1.25's built-in method+path routing eliminates the need for chi/gorilla/echo. Zero new dependencies added.
- **`X-Tenant-ID` header** — Simple tenant context for dev/curl testing. Not auth — must be replaced before deployment.
- **Exempt paths for middleware** — `tenantExemptPrefixes` allows health checks to work without tenant context. Extensible for future public endpoints.
- **Nil-safe handlers** — `SynthesizeHandler` and `HarvesterHandler` return 503 when their service is nil, allowing partial server startup.
- **Deterministic dev UUID** — `00000000-...-000000000001` makes curl commands copy-pasteable across machines.
- **`ExportedTenantKey`** — Tests inject tenant context directly into request context, bypassing middleware. Avoids coupling test structure to middleware internals.

## Running

```bash
# API unit tests (fast, no Docker)
go test ./internal/api/... -v -count=1

# Start dev server
docker compose up -d
psql -h localhost -p 5433 -U signalflow -d signal_flow_dev \
  -f migrations/000001_create_signals_table.up.sql \
  -f migrations/000002_create_identity_tables.up.sql \
  -f migrations/000003_add_harvester_columns.up.sql \
  -f migrations/000004_add_bluesky_session_columns.up.sql \
  -f migrations/000005_seed_dev_user.up.sql

ENCRYPTION_KEY=$(openssl rand -hex 16) go run ./cmd/signal-flow

# Test
curl localhost:8088/api/health
curl -H "X-Tenant-ID: 00000000-0000-0000-0000-000000000001" \
  localhost:8088/api/signals
```
