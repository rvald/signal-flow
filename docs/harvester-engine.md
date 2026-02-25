---
title: "Phase 4: Harvester Engine & OAuth Auth Layer"
project: signal-flow
phase: 4
status: complete
date_completed: 2026-02-25
authors:
  - rvald
tags:
  - harvester
  - oauth
  - bluesky
  - at-protocol
  - encryption
  - tdd
  - resilience
dependencies:
  - github.com/bluesky-social/indigo
  - github.com/google/uuid
  - crypto/aes (stdlib)
---

# Phase 4: Harvester Engine & OAuth Auth Layer

The data collection backbone of Signal-Flow. A background polling engine that harvests content from Bluesky, YouTube, and GitHub, with encrypted OAuth2 token management, automatic rotation, and retry resilience.

## Project Structure

```
signal-flow/
├── internal/
│   ├── domain/
│   │   ├── harvester.go                               # RawSignal, Harvester interface, provider constants
│   │   └── identity.go                                # + SessionID, AccountDID, SaveToken
│   ├── auth/
│   │   ├── refresher.go                               # TokenRefresher, TokenData, TokenSaver, TokenEndpoint
│   │   ├── oauth2_refresher.go                        # OAuth2Refresher (YouTube/GitHub token rotation)
│   │   ├── bluesky_store.go                           # PostgresOAuthStore (oauth.ClientAuthStore impl)
│   │   ├── bluesky_timeline.go                        # Feed types + ExtractLinksFromFeed
│   │   ├── refresher_test.go                          # 5 OAuth2Refresher tests
│   │   └── bluesky_test.go                            # 4 Bluesky store + extraction tests
│   ├── harvester/
│   │   ├── service.go                                 # Coordinator, AuthError, retry logic
│   │   ├── service_test.go                            # 5 Coordinator tests
│   │   └── providers/
│   │       ├── bluesky.go                             # BlueskyHarvester (stub → real when wired)
│   │       ├── youtube.go                             # YouTubeHarvester (stub)
│   │       └── github.go                              # GitHubHarvester (stub)
│   └── repository/
│       └── postgres_identity.go                       # + SaveToken implementation
└── migrations/
    ├── 000003_add_harvester_columns.up.sql            # last_seen_id, needs_reauth
    ├── 000003_add_harvester_columns.down.sql
    ├── 000004_add_bluesky_session_columns.up.sql      # session_id, account_did
    └── 000004_add_bluesky_session_columns.down.sql
```

## Source Index

### Domain Layer

- **Harvester interface** — [internal/domain/harvester.go](file:///signal-flow/internal/domain/harvester.go)
  - `Harvest(ctx, cred) ([]RawSignal, error)` — fetch new content using credential
  - `Provider() string` — returns platform identifier

- **RawSignal struct** — `SourceURL`, `Title`, `Content`, `Provider`, `HarvestedAt`, `Metadata`

- **Provider constants** — `ProviderBluesky`, `ProviderYouTube`, `ProviderGitHub`

- **Credential extensions** — [internal/domain/identity.go](file:///signal-flow/internal/domain/identity.go)
  - `SessionID` — Bluesky OAuth session identifier
  - `AccountDID` — Bluesky AT Protocol DID
  - `SaveToken(ctx, credID, encryptedToken)` — persist rotated tokens

### Auth Layer

- **TokenRefresher interface** — [internal/auth/refresher.go](file:///signal-flow/internal/auth/refresher.go)
  - `Refresh(ctx, cred) (accessToken, error)` — decrypt, refresh if expired, re-encrypt, save

- **TokenData struct** — `AccessToken`, `RefreshToken`, `TokenType`, `ExpiresAt`

- **OAuth2Refresher** — [internal/auth/oauth2_refresher.go](file:///signal-flow/internal/auth/oauth2_refresher.go)
  - Standard OAuth2 rotation for YouTube/GitHub
  - Decrypts stored token via `KeyManager`
  - Short-circuits if `ExpiresAt` > now + 30s
  - Exchanges refresh token via `TokenEndpoint` interface
  - Re-encrypts and persists via `TokenSaver`
  - Returns `AuthError` if refresh token is missing

- **PostgresOAuthStore** — [internal/auth/bluesky_store.go](file:///signal-flow/internal/auth/bluesky_store.go)
  - Implements indigo's `oauth.ClientAuthStore` (6 methods)
  - Session methods: `Get/Save/DeleteSession` — JSON marshal → `KeyManager.Encrypt` → DB
  - Auth request methods: `Get/Save/DeleteAuthRequestInfo` — in-memory `sync.Map` with 30-min TTL

- **Timeline types + ExtractLinksFromFeed** — [internal/auth/bluesky_timeline.go](file:///signal-flow/internal/auth/bluesky_timeline.go)
  - `TimelineResponse`, `TimelineFeedItem`, `TimelinePost`, `PostEmbed`, `EmbedExternal`
  - Filters for `app.bsky.embed.external` type → `[]RawSignal`

### Harvester Layer

- **Coordinator** — [internal/harvester/service.go](file:///signal-flow/internal/harvester/service.go)
  - `RunOnce(ctx)` — single harvest cycle: list credentials → harvest → dedup → synthesize
  - `Start(ctx, interval)` — background ticker loop wrapping `RunOnce`
  - Retry with exponential backoff (3 attempts, 100ms/200ms/400ms)
  - `AuthError` → `MarkNeedsReauth` (fatal, no retry)
  - SHA-256 URL hash as `LastSeenID` cursor

- **Provider stubs** — [internal/harvester/providers/](file:///signal-flow/internal/harvester/providers/)
  - `BlueskyHarvester`, `YouTubeHarvester`, `GitHubHarvester` — implement `Harvester` interface

### Database Schema

- **Migration 000003** — [000003_add_harvester_columns.up.sql](file:///signal-flow/migrations/000003_add_harvester_columns.up.sql)
  - `last_seen_id TEXT` — polling cursor per credential
  - `needs_reauth BOOLEAN` — flagged on 401 errors

- **Migration 000004** — [000004_add_bluesky_session_columns.up.sql](file:///signal-flow/migrations/000004_add_bluesky_session_columns.up.sql)
  - `session_id TEXT` — Bluesky OAuth session ID
  - `account_did TEXT` — AT Protocol DID

### Tests

- **Coordinator tests** — [internal/harvester/service_test.go](file:///signal-flow/internal/harvester/service_test.go)
  - `Test_Dispatch_To_Providers` — 3 providers, credential routing
  - `Test_Dedup_Existing_Signals` — duplicate URL → skipped
  - `Test_AuthError_Marks_Reauth` — 401 → `MarkNeedsReauth`
  - `Test_Retry_Transient_Errors` — 3 retries with backoff
  - `Test_Multiple_Credentials_Per_Provider` — fan-out per provider

- **OAuth2Refresher tests** — [internal/auth/refresher_test.go](file:///signal-flow/internal/auth/refresher_test.go)
  - `Test_Token_Rotation_Logic` — rotate, re-encrypt, save
  - `Test_Encryption_Boundary` — tokens always encrypted
  - `Test_Provider_Isolation` — YouTube 429 doesn't affect Bluesky
  - `Test_Unexpired_Token_NoRefresh` — valid token → no API call
  - `Test_Missing_RefreshToken_AuthError` — missing refresh → `AuthError`

- **Bluesky tests** — [internal/auth/bluesky_test.go](file:///signal-flow/internal/auth/bluesky_test.go)
  - `Test_BlueskyStore_SaveAndGet_RoundTrip` — encrypt/decrypt cycle
  - `Test_BlueskyStore_Encryption_AtRest` — raw DB bytes are not JSON
  - `Test_BlueskyStore_DeleteSession` — delete → get fails
  - `Test_BlueskyHarvester_ExtractLinks` — external links only → `RawSignal`

## Design Decisions

- **`TokenRefresher` interface** — Per-provider abstraction. YouTube/GitHub share `OAuth2Refresher`; Bluesky uses `indigo`'s `ResumeSession` (deferred until `ClientApp` wiring).
- **`SessionDB` interface** — Decouples `PostgresOAuthStore` from pgx, enabling `fakeSessionDB` in tests without Docker.
- **Auth requests in `sync.Map`** — Ephemeral data for the iOS login dance. 30-min TTL, no DB persistence needed.
- **`ExtractLinksFromFeed` as pure function** — Testable without SDK or network. Filters by embed `$type` containing "external".
- **30-second expiry buffer** — `OAuth2Refresher` refreshes tokens 30s before expiry to avoid clock skew edge cases.
- **SHA-256 cursor** — `LastSeenID` stores URL hash, not platform-specific cursor. Simple and provider-agnostic.
- **`AuthError` sentinel** — Distinguishes fatal auth failures (mark `NeedsReauth`) from transient errors (retry with backoff).
- **Exponential backoff** — `100ms → 200ms → 400ms` for transient errors. Bounded at 3 attempts to avoid blocking the harvest loop.
- **indigo SDK pinned** — `v0.0.0-20251010014239` from the official cookbook. Pre-v1 but it's the canonical AT Protocol Go SDK.

## Running

```bash
# Auth layer tests (fast, no Docker)
go test ./internal/auth/... -v -count=1

# Harvester tests (fast, no Docker)
go test ./internal/harvester/... -v -count=1

# Full suite (Phase 1+2 need Docker for testcontainers)
go test ./... -v -count=1
```
