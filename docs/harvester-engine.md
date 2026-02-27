---
title: "Phase 4: Harvester Engine & Bluesky Auth Layer"
project: signal-flow
phase: 4
status: complete
date_completed: 2026-02-26
authors:
  - rvald
tags:
  - harvester
  - bluesky
  - at-protocol
  - app-password
  - cli
  - tdd
  - resilience
dependencies:
  - github.com/bluesky-social/indigo
  - github.com/google/uuid
  - crypto/aes (stdlib)
---

# Phase 4: Harvester Engine & Bluesky Auth Layer

The data collection backbone of Signal-Flow. Content harvesting from Bluesky with app-password auth, local session storage, automatic JWT refresh, and retry resilience. CLI-first — `signal-flow login`, `signal-flow feed`, `signal-flow harvest`.

## Project Structure

```
signal-flow/
├── cmd/signal-flow/cli/
│   ├── bluesky.go                                     # login command (ServerCreateSession → config file)
│   ├── bluesky_resolve.go                             # Session resolver, expired-token helpers
│   ├── feed.go                                        # feed command (timeline links + --follows filter)
│   ├── following.go                                   # following command (list followed accounts)
│   ├── harvest.go                                     # harvest command (timeline → DB signals)
│   ├── logout.go                                      # logout command (clear session)
│   └── constants.go                                   # devTenantID
├── internal/
│   ├── config/
│   │   ├── config.go                                  # BlueskySession, Load/Save/Clear
│   │   └── config_test.go                             # 4 tests (round-trip, no-file, clear, permissions)
│   ├── domain/
│   │   ├── harvester.go                               # RawSignal, Harvester interface, provider constants
│   │   └── identity.go                                # + SessionID, AccountDID, SaveToken
│   ├── auth/
│   │   ├── refresher.go                               # TokenRefresher, TokenData, TokenSaver, TokenEndpoint
│   │   ├── oauth2_refresher.go                        # OAuth2Refresher (YouTube/GitHub token rotation)
│   │   ├── bluesky_refresher.go                       # JWT refresh via ServerRefreshSession
│   │   ├── bluesky_timeline.go                        # Feed types + ExtractLinksFromFeed
│   │   ├── refresher_test.go                          # 5 OAuth2Refresher tests
│   │   └── bluesky_test.go                            # 1 ExtractLinks test
│   ├── harvester/
│   │   ├── service.go                                 # Coordinator, AuthError, retry logic
│   │   ├── service_test.go                            # 5 Coordinator tests
│   │   └── providers/
│   │       ├── bluesky.go                             # BlueskyHarvester (stub for Coordinator mode)
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

- **BlueskyRefresher** — [internal/auth/bluesky_refresher.go](file:///signal-flow/internal/auth/bluesky_refresher.go)
  - `RefreshBlueskySession(ctx, session) (*BlueskySession, error)`
  - Uses `comatproto.ServerRefreshSession` with stored `RefreshJwt`
  - Returns updated session with new JWTs

- **Timeline types + ExtractLinksFromFeed** — [internal/auth/bluesky_timeline.go](file:///signal-flow/internal/auth/bluesky_timeline.go)
  - `TimelineResponse`, `TimelineFeedItem`, `TimelinePost`, `PostEmbed`, `EmbedExternal`
  - `PostAuthor` struct — `DID`, `Handle`, `DisplayName` (populated from SDK)
  - `FilterByFollows(items, followDIDs)` — filters feed items to followed accounts only
  - Filters for `app.bsky.embed.external` type → `[]RawSignal`

### Config Layer

- **BlueskySession** — [internal/config/config.go](file:///signal-flow/internal/config/config.go)
  - `AccessJwt`, `RefreshJwt`, `Handle`, `DID`, `Host`, `CreatedAt`
  - `SaveSession` — writes to `~/.config/signal-flow/session.json` (dir: `0700`, file: `0600`)
  - `LoadSession` — returns `ErrNoSession` if missing
  - `ClearSession` — removes session file

### CLI Commands

- **login** — [cmd/signal-flow/cli/bluesky.go](file:///signal-flow/cmd/signal-flow/cli/bluesky.go)
  - `ServerCreateSession` → `config.SaveSession` (app-password flow)
  - Flags: `--host` (default `https://bsky.social`), `--identifier`, `--password`

- **feed** — [cmd/signal-flow/cli/feed.go](file:///signal-flow/cmd/signal-flow/cli/feed.go)
  - Read-only timeline link extraction, no DB required
  - `--follows` flag filters to posts from followed accounts (all posts, not just links)
  - Flags: `--limit`, `--json`, `--follows`

- **following** — [cmd/signal-flow/cli/following.go](file:///signal-flow/cmd/signal-flow/cli/following.go)
  - Lists all accounts the user follows, paginating through `GraphGetFollows`
  - `FollowInfo` struct — `DID`, `Handle`, `DisplayName`
  - Flags: `--limit`, `--json`

- **harvest** — [cmd/signal-flow/cli/harvest.go](file:///signal-flow/cmd/signal-flow/cli/harvest.go)
  - Fetches timeline, stores signals to Postgres
  - Reads `DATABASE_URL` and `ENCRYPTION_KEY` from env vars
  - Flags: `--limit`, `--dry-run`, `--follows`

- **logout** — [cmd/signal-flow/cli/logout.go](file:///signal-flow/cmd/signal-flow/cli/logout.go)
  - Clears stored session file

- **resolveBlueskyClient** — [cmd/signal-flow/cli/bluesky_resolve.go](file:///signal-flow/cmd/signal-flow/cli/bluesky_resolve.go)
  - Loads session → refreshes JWTs → returns `*xrpc.Client`
  - Detects `ExpiredToken` on refresh failure → returns friendly re-login message
  - Falls back to stored access token only for non-auth errors (network, etc.)

- **Expired-token helpers** — [cmd/signal-flow/cli/bluesky_resolve.go](file:///signal-flow/cmd/signal-flow/cli/bluesky_resolve.go)
  - `isExpiredTokenErr(err)` — detects `xrpc.XRPCError` with `ErrStr == "ExpiredToken"`
  - `wrapExpiredTokenErr(err)` — wraps with friendly `signal-flow login` instructions

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
  - `Test_BlueskyHarvester_ExtractLinks` — external links only → `RawSignal`
  - `Test_FilterByFollows` — filters feed items to followed DIDs
  - `Test_PostAuthor_Populated` — verifies author fields populated from SDK data

- **Config tests** — [internal/config/config_test.go](file:///signal-flow/internal/config/config_test.go)
  - `TestSaveAndLoad_RoundTrip` — save session → load → fields match
  - `TestLoad_NoFile` — returns `ErrNoSession`
  - `TestClear` — save → clear → load fails
  - `TestFilePermissions` — file `0600`, dir `0700`

## Design Decisions

- **App-password over OAuth** — CLI doesn't need browser-redirect flow. App passwords give the same JWTs via `ServerCreateSession`. Simpler, no callback server needed.
- **Local config file over DB** — `~/.config/signal-flow/session.json` means `signal-flow login` works without Postgres. DB only needed for `harvest` (storing signals).
- **Auto-refresh** — `resolveBlueskyClient` transparently refreshes JWTs via `ServerRefreshSession` before every command. Falls back to stored access token if refresh fails.
- **Graceful expired-token handling** — When the refresh token itself is expired, `resolveBlueskyClient` returns a user-friendly message instructing re-login via `signal-flow login` instead of an opaque XRPC error. All downstream API call sites (`feed`, `following`, `harvest`) also wrap errors with `wrapExpiredTokenErr`.
- **`TokenRefresher` interface** — Per-provider abstraction. YouTube/GitHub share `OAuth2Refresher`; Bluesky uses `RefreshBlueskySession`.
- **`ExtractLinksFromFeed` as pure function** — Testable without SDK or network. Filters by embed `$type` containing "external".
- **30-second expiry buffer** — `OAuth2Refresher` refreshes tokens 30s before expiry to avoid clock skew edge cases.
- **SHA-256 cursor** — `LastSeenID` stores URL hash, not platform-specific cursor. Simple and provider-agnostic.
- **`AuthError` sentinel** — Distinguishes fatal auth failures (mark `NeedsReauth`) from transient errors (retry with backoff).
- **Exponential backoff** — `100ms → 200ms → 400ms` for transient errors. Bounded at 3 attempts to avoid blocking the harvest loop.
- **Coordinator kept dormant** — Background polling via `Coordinator.Start` is preserved for a future `signal-flow serve` mode. CLI commands bypass it and call the Bluesky API directly.

## Running

```bash
# Auth + config tests (fast, no Docker)
go test ./internal/auth/... ./internal/config/... -v -count=1

# Harvester tests (fast, no Docker)
go test ./internal/harvester/... -v -count=1

# CLI usage
go run ./cmd/signal-flow login --identifier "you.bsky.social" --password "your-app-password"
go run ./cmd/signal-flow feed
go run ./cmd/signal-flow feed --json --limit 10
go run ./cmd/signal-flow feed --follows
go run ./cmd/signal-flow following
go run ./cmd/signal-flow following --json --limit 50
go run ./cmd/signal-flow harvest --dry-run
go run ./cmd/signal-flow harvest --dry-run --follows
DATABASE_URL=... ENCRYPTION_KEY=... go run ./cmd/signal-flow harvest
DATABASE_URL=... ENCRYPTION_KEY=... go run ./cmd/signal-flow harvest --follows
go run ./cmd/signal-flow logout

# Full suite (Phase 1+2 need Docker for testcontainers)
go test ./... -v -count=1
```
