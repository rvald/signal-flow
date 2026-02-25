---
title: "Phase 1: Core Signal Engine"
project: signal-flow
phase: 1
status: complete
date_completed: 2026-02-24
authors:
  - rvald
tags:
  - domain-model
  - postgres
  - pgvector
  - rls
  - tdd
dependencies:
  - github.com/jackc/pgx/v5
  - github.com/pgvector/pgvector-go
  - github.com/google/uuid
  - github.com/testcontainers/testcontainers-go
---

# Phase 1: Core Signal Engine

The foundational storage domain for Signal-Flow. Defines how a **Signal** (the atomic unit of knowledge) is stored, identified, and tenant-isolated in PostgreSQL with pgvector.

## Project Structure

```
signal-flow/
├── cmd/signal-flow/main.go                          # Entrypoint (placeholder)
├── docker-compose.yml                               # Postgres 16 + pgvector dev env
├── migrations/
│   ├── 000001_create_signals_table.up.sql            # Schema + RLS policies
│   └── 000001_create_signals_table.down.sql          # Rollback
└── internal/
    ├── domain/
    │   └── signal.go                                 # Signal struct + repository interface
    └── repository/
        ├── postgres_signal.go                        # PostgresSignalRepository impl
        └── postgres_signal_test.go                   # Integration tests (testcontainers)
```

## Source Index

### Domain Layer

- **Signal struct** — [internal/domain/signal.go](file:///signal-flow/internal/domain/signal.go)
  - Fields: `ID`, `TenantID`, `SourceURL`, `Title`, `Content`, `Distillation`, `Metadata` (JSONB), `Scope`, `Vector` (1536-dim), `CreatedAt`, `UpdatedAt`
  - Scope constants: `ScopePrivate`, `ScopeTeam`

- **SignalRepository interface** — [internal/domain/signal.go](file:///signal-flow/internal/domain/signal.go)
  - `Save(ctx, *Signal) error` — upsert on `(source_url, tenant_id)`
  - `FindRecentByTenant(ctx, tenantID, limit) ([]*Signal, error)`
  - `SearchSemantic(ctx, tenantID, vector, limit) ([]*Signal, error)`
  - `PromoteToTeam(ctx, signalID, tenantID) error`

### Repository Layer

- **PostgresSignalRepository** — [internal/repository/postgres_signal.go](file:///signal-flow/internal/repository/postgres_signal.go)
  - `NewPostgresSignalRepository(pool)` — constructor
  - `withTenantTx(ctx, tenantID, fn)` — wraps every query in a tx with `SET LOCAL app.current_tenant_id`
  - `scanSignal(row)` — handles nullable `*pgvector.Vector` scanning
  - `Save` — `INSERT ... ON CONFLICT (source_url, tenant_id) DO UPDATE SET ...`
  - `FindRecentByTenant` — `ORDER BY created_at DESC LIMIT $1`
  - `SearchSemantic` — `ORDER BY vector <=> $1 LIMIT $2` (cosine distance)
  - `PromoteToTeam` — `UPDATE signals SET scope = 'team' WHERE id = $1`

### Database Schema

- **Migration** — [migrations/000001_create_signals_table.up.sql](file:///signal-flow/migrations/000001_create_signals_table.up.sql)
  - `vector` extension enabled
  - `signals` table with `vector(1536)` column
  - `UNIQUE (source_url, tenant_id)` for upsert dedup
  - Row-Level Security with `ENABLE` + `FORCE`
  - `tenant_isolation` policy: `USING (tenant_id = current_setting('app.current_tenant_id')::uuid)`
  - `tenant_insert` policy: `WITH CHECK (...)` for INSERT

### Infrastructure

- **Docker Compose** — [docker-compose.yml](file:///signal-flow/docker-compose.yml)
  - Image: `pgvector/pgvector:pg16`
  - Port: `5433:5432`
  - Credentials: `signalflow/signalflow`
  - Database: `signal_flow_dev`

### Tests

- **Integration tests** — [internal/repository/postgres_signal_test.go](file:///signal-flow/internal/repository/postgres_signal_test.go)
  - `setupTestDB(t)` — testcontainers-based Postgres lifecycle, creates non-superuser `app_user` role
  - `Test_Signal_Isolation` — proves RLS: TenantA data invisible to TenantB
  - `Test_Upsert_Deduplication` — same URL + tenant → 1 row, updated title
  - `Test_Semantic_Search_Ordering` — cosine distance returns closest vector first
  - `Test_PromoteToTeam` — scope transitions from `private` → `team`

## Design Decisions

- **RLS requires non-superuser role** — Postgres superusers bypass all RLS even with `FORCE ROW LEVEL SECURITY`. The app connects as `app_user` (non-superuser) with table grants.
- **`SET LOCAL` in transactions** — `SET LOCAL` is tx-scoped, so all RLS-dependent queries run inside `withTenantTx`. The tenant UUID is interpolated via `fmt.Sprintf` (safe — `uuid.UUID` is a validated type).
- **Nullable vectors** — Signals exist before LLM processing adds embeddings. `scanSignal` uses `*pgvector.Vector` to handle NULL columns.
- **No IVFFlat index yet** — Cosine search works via `<=>` operator without an index. IVFFlat is deferred until data volume justifies it (requires existing rows to build lists).

## Running

```bash
# Tests (self-contained via testcontainers, requires Docker daemon)
go test ./internal/repository/... -v -count=1

# Dev database
docker compose up -d
psql -h localhost -p 5433 -U signalflow -d signal_flow_dev \
  -f migrations/000001_create_signals_table.up.sql
```
