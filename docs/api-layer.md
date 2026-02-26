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
├── cmd/signal-flow/
│   ├── main.go                                          # CLI entrypoint (Cobra root command)
│   └── cli/
│       ├── root.go                                      # Root command + subcommand registration
│       ├── bluesky.go                                   # login command
│       ├── bluesky_resolve.go                           # Session resolver, expired-token helpers
│       ├── feed.go                                      # feed command (timeline links + --follows)
│       ├── following.go                                 # following command (list followed accounts)
│       ├── harvest.go                                   # harvest command (timeline → DB)
│       ├── logout.go                                    # logout command
│       └── constants.go                                 # devTenantID
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

> **Note:** The project has shifted to CLI-first. The HTTP API handlers still exist and are tested, but the primary interface is now the CLI (`signal-flow login`, `feed`, `following`, `harvest`). A future `signal-flow serve` command can re-expose the HTTP API.

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

### CLI Entrypoint

- **main.go** — [cmd/signal-flow/main.go](file:///signal-flow/cmd/signal-flow/main.go)
  - Cobra CLI with context cancellation on `SIGINT` / `SIGTERM`
  - Dispatches to subcommands: `login`, `feed`, `following`, `harvest`, `logout`, `version`
  - `SilenceUsage` + `SilenceErrors` on root command — errors print once via `main.go`, no usage dump on non-usage errors
  - HTTP API handlers are still available but not wired to a `serve` command yet

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
- **`SilenceUsage` + `SilenceErrors`** — Prevents Cobra from printing usage on runtime errors (e.g. expired tokens) and avoids duplicate error output. Errors are printed once by `main.go`.

## Running

```bash
# API unit tests (fast, no Docker)
go test ./internal/api/... -v -count=1

# CLI usage (see docs/harvester-engine.md for full auth flow)
go run ./cmd/signal-flow login --identifier "you.bsky.social" --password "your-app-password"
go run ./cmd/signal-flow feed
go run ./cmd/signal-flow following

# Full suite (Phase 1+2 need Docker for testcontainers)
go test ./... -v -count=1
```
