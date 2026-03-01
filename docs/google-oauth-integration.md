---
title: "Google OAuth & CLI Auth"
project: signal-flow
phase: 6
status: complete
date_completed: 2026-02-28
authors:
  - rvald
tags:
  - oauth2
  - html-templates
  - youtube-api
  - cli
  - cobra
dependencies:
  - github.com/spf13/cobra
  - golang.org/x/oauth2
  - html/template (stdlib)
---

# Google OAuth & CLI Auth

This phase introduces a robust, native Google OAuth 2.0 desktop authentication flow and a fully-featured Cobra command tree for managing Google API credentials, tokens, aliases, and keyrings.

## Project Structure

```
signal-flow/
├── cmd/signal-flow/cli/
│   ├── auth.go                                          # Root auth Cobra command (`auth add`, `auth list`, etc.)
│   ├── auth_alias.go                                    # `auth alias` subcommands
│   ├── auth_keyring.go                                  # `auth keyring` subcommands
│   └── auth_service_account.go                          # `auth service-account` subcommands
└── internal/
    └── googleauth/
        ├── accounts_server.go                           # Local HTTP server for the OAuth callback
        ├── oauth_flow.go                                # OAuth 2.0 PKCE browser flow logic
        ├── service.go                                   # Google API Service definitions (Gmail, YouTube, Calendar, etc.)
        └── templates/
            ├── accounts.html                            # UI: Account selection / management
            ├── success.html                             # UI: OAuth Success landing page
            ├── error.html                               # UI: OAuth Error landing page
            ├── cancelled.html                           # UI: OAuth Cancelled landing page
            └── templates_embed.go                       # go:embed directive for HTML templates
```

## Source Index

### CLI Auth Commands (Cobra Migration)

The `auth` command tree has been fully migrated to use the `github.com/spf13/cobra` framework, standardizing argument parsing and generated help documentation.

- **`auth add <email>`**: Initiates a local browser-based OAuth 2.0 flow to authorize a Google Workspace account. It spawns a temporary HTTP server on a random port to receive the secure authorization code callback.
- **`auth credentials`**: Utilities for manually setting (`set`) and checking (`list`) credentials files.
- **`auth tokens`**: Utilities for managing raw tokens (`list`, `delete`, `export`, `import`).
- **`auth alias`**: Manage account aliases (`list`, `set`, `unset`) to simplify multi-account usage.
- **`auth service-account`**: Configure headless service account JSON keys.
- **`auth keyring`**: Interrogates the OS-level secure keyring for stored tokens.
- **`youtube`**: Commands for interacting with the YouTube API.
  - **`youtube subscription-list`**: Fetches a user's subscriptions. Supports `--account`, `--maxResults` (default 5, max 50), `--mine`, and `--part` (default 'snippet') flags.

### Google API Services

- **`internal/googleauth/service.go`**: Defines the supported Google APIs. By default, `auth add` requests a subset of the most common scopes. Users can narrow this by passing the `--services` flag.
- **Supported Services include**: `gmail`, `calendar`, `chat`, `classroom`, `drive`, `docs`, `slides`, `contacts`, `tasks`, `people`, `sheets`, `forms`, `appscript`, `groups`, `keep`, and **`youtube`** (YouTube Data API v3).

### Local OAuth Web UI

When the user runs `auth add` or visits the local server, they are presented with beautifully styled HTML interfaces embedded directly within the Go binary via `go:embed`.

- **Style System**: Dynamic ambient gradient orbs, "JetBrains Mono" fonts for terminal stylings, custom CSS. No external CSS libraries (Tailwind, Bootstrap) are required.
- **`accounts.html`**: A dashboard to review which Google accounts are connected to the CLI, check which service tags they have mapped to them, set the default account, and remove accounts.
- **`success.html`**: Shown when the OAuth callback is successful. It renders a mock "terminal" UI confirming the connection and automatically closes after a short countdown.
- **`error.html` / `cancelled.html`**: User-friendly landing pages for failed or aborted authorization attempts.

## Running / Testing

```bash
# Add a new account (default services)
./bin/signal-flow auth add email

# Add a new account restricted to YouTube and Gmail
./bin/signal-flow auth add email --services youtube,gmail

# List authorized accounts and aliases
./bin/signal-flow auth list
./bin/signal-flow auth alias list

# Verify the CLI command tree
./bin/signal-flow auth --help

# Query YouTube subscriptions for an account (returns JSON)
./bin/signal-flow youtube subscription-list --account you.bsky.social --maxResults 10
```
