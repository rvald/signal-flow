---
title: "Phase 2: Identity & Credential Vault"
project: signal-flow
phase: 2
status: complete
date_completed: 2026-02-24
authors:
  - rvald
tags:
  - identity
  - encryption
  - aes-gcm
  - kms
  - tdd
dependencies:
  - github.com/jackc/pgx/v5
  - github.com/google/uuid
  - github.com/testcontainers/testcontainers-go
  - crypto/aes (stdlib)
  - crypto/cipher (stdlib)
---

# Phase 2: Identity & Credential Vault

The security domain for Signal-Flow. Manages user identities and securely stores encrypted OAuth2 refresh tokens that enable "Always-On" harvesting from Bluesky and YouTube.

## Project Structure

```
signal-flow/
├── internal/
│   ├── domain/
│   │   ├── signal.go                                 # Phase 1
│   │   └── identity.go                               # User, Credential, IdentityRepository
│   ├── repository/
│   │   ├── postgres_signal.go                        # Phase 1
│   │   ├── postgres_signal_test.go                   # Phase 1
│   │   ├── postgres_identity.go                      # PostgresIdentityRepository impl
│   │   └── postgres_identity_test.go                 # Integration tests (testcontainers)
│   └── security/
│       ├── kms.go                                    # KeyManager interface + LocalKeyManager
│       └── kms_test.go                               # Unit tests
└── migrations/
    ├── 000001_create_signals_table.up.sql            # Phase 1
    ├── 000001_create_signals_table.down.sql          # Phase 1
    ├── 000002_create_identity_tables.up.sql          # users + user_credentials
    └── 000002_create_identity_tables.down.sql        # Rollback
```

## Source Index

### Security Layer

- **KeyManager interface** — [internal/security/kms.go](file:///signal-flow/internal/security/kms.go)
  - `Encrypt(ctx, plaintext) (ciphertext, error)`
  - `Decrypt(ctx, ciphertext) (plaintext, error)`

- **LocalKeyManager** — [internal/security/kms.go](file:///signal-flow/internal/security/kms.go)
  - AES-256-GCM with 12-byte random nonce prepended to ciphertext
  - `NewLocalKeyManager(key []byte)` — validates 32-byte key at construction

### Domain Layer

- **User struct** — [internal/domain/identity.go](file:///signal-flow/internal/domain/identity.go)
  - Fields: `ID`, `Email`, `CreatedAt`

- **Credential struct** — [internal/domain/identity.go](file:///signal-flow/internal/domain/identity.go)
  - Fields: `ID`, `UserID`, `Provider`, `EncryptedToken` (BYTEA), `ProviderUserID`, `ExpiresAt`, `CreatedAt`, `UpdatedAt`

- **IdentityRepository interface** — [internal/domain/identity.go](file:///signal-flow/internal/domain/identity.go)
  - `LinkProvider(ctx, userID, provider, rawToken) error` — encrypt + upsert
  - `GetActiveToken(ctx, userID, provider) ([]byte, error)` — decrypt + return
  - `ListUsersByProvider(ctx, provider) ([]*User, error)` — for Harvester polling

### Repository Layer

- **PostgresIdentityRepository** — [internal/repository/postgres_identity.go](file:///signal-flow/internal/repository/postgres_identity.go)
  - Takes `pool` + `KeyManager` as dependencies
  - `LinkProvider` — `INSERT ... ON CONFLICT (user_id, provider) DO UPDATE SET encrypted_token = ...`
  - `GetActiveToken` — `SELECT encrypted_token` → `kms.Decrypt()`
  - `ListUsersByProvider` — `JOIN user_credentials ON provider = $1`

### Database Schema

- **Migration** — [migrations/000002_create_identity_tables.up.sql](file:///signal-flow/migrations/000002_create_identity_tables.up.sql)
  - `users` table with `UNIQUE (email)`
  - `user_credentials` table with `BYTEA` for encrypted tokens
  - `UNIQUE (user_id, provider)` for upsert token rotation
  - `ON DELETE CASCADE` from `users` → `user_credentials`
  - `idx_user_credentials_provider` index for Harvester queries

### Tests

- **Security unit tests** — [internal/security/kms_test.go](file:///signal-flow/internal/security/kms_test.go)
  - `TestEncryptionCycle` — round-trip correctness
  - `TestEncryptUniqueCiphertexts` — random nonce verification
  - `TestDecryptTamperedCiphertext` — GCM authentication
  - `TestNewLocalKeyManagerInvalidKey` — key length validation (6 subtests)

- **Identity integration tests** — [internal/repository/postgres_identity_test.go](file:///signal-flow/internal/repository/postgres_identity_test.go)
  - `Test_No_Plaintext_In_DB` — proves tokens are encrypted at rest
  - `Test_Token_Rotation` — upsert overwrites old token, 1 row survives
  - `Test_GetActiveToken_RoundTrip` — link → get → matches original
  - `Test_ListUsersByProvider` — correct provider filtering

## Design Decisions

- **`KeyManager` interface** — Swappable: `LocalKeyManager` for dev, future CloudKMS for prod. No business logic changes needed.
- **No RLS on identity tables** — Identity tables are accessed by internal services, not tenant-scoped. RLS stays on `signals` only.
- **`BYTEA` for tokens** — Avoids base64 encoding overhead; raw ciphertext stored directly.
- **`json:"-"` on EncryptedToken** — Prevents accidental serialization of ciphertext in API responses.
- **OAuth2 handler deferred** — No HTTP router in codebase yet; handler will be added with the HTTP server.

## Running

```bash
# Security unit tests (fast, no Docker)
go test ./internal/security/... -v -count=1

# Integration tests (requires Docker daemon)
go test ./internal/repository/... -v -count=1

# Full suite
go test ./... -v -count=1
```
