---
title: "Deployment Reference"
project: signal-flow
status: current
date_updated: 2026-03-03
authors:
  - rvald
tags:
  - deployment
  - systemd
  - docker
  - kubernetes
  - configuration
  - environment-variables
---

# Deployment Reference

Complete reference for configuring and deploying Signal-Flow pipeline across local, Docker, and Kubernetes environments.

## Prerequisites

Before deploying, you need:

1. **A built binary** — `go build -o ./bin/signal-flow ./cmd/signal-flow`
2. **A PostgreSQL database** with pgvector — see [docker-compose.yml](file:///signal-flow/docker-compose.yml)
3. **Bluesky credentials** — run `signal-flow bluesky-login --identifier <handle> --password <app-password>`
4. **Google OAuth tokens** (for YouTube) — run `signal-flow auth add you@gmail.com`
5. **A pipeline config** — copy `deploy/pipeline.yaml.example` to `~/.config/signal-flow/pipeline.yaml`

---

## Environment Variables

All env vars required by the pipeline, organized by purpose.

### Required

| Variable | Purpose | Example |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/signal_flow?sslmode=disable` |
| `ENCRYPTION_KEY` | 32-byte hex key for signal encryption | `d41692f6...` (64 hex chars) |

### LLM Providers (one required)

| Variable | Provider | Notes |
|---|---|---|
| `GEMINI_API_KEY` | Google Gemini | Default provider |
| `ANTHROPIC_API_KEY` | Anthropic Claude | Alternative |
| `OPENAI_API_KEY` | OpenAI | Alternative |

### Notification

| Variable | Purpose | Example |
|---|---|---|
| `SLACK_WEBHOOK_URL` | Slack incoming webhook | `https://hooks.slack.com/services/T.../B.../xxx` |

### Google / YouTube Auth

| Variable | Purpose | Example |
|---|---|---|
| `GOG_ACCOUNT` | Google account email for YouTube | `you@gmail.com` |
| `GOG_KEYRING_PASSWORD` | Passphrase for the encrypted file keyring | Any strong passphrase |
| `GOG_KEYRING_BACKEND` | Force keyring backend (`auto`\|`file`) | `file` (required for headless) |

### Bluesky Auth

| Variable | Purpose | Notes |
|---|---|---|
| `BSKY_APP_PASSWORD` | Bluesky app password | Used by session refresh if set |

### Pipeline Config Overrides

These env vars override values in `pipeline.yaml`:

| Variable | Overrides | Example |
|---|---|---|
| `SLACK_WEBHOOK_URL` | `notify.webhook_url` | Always wins over file |
| `PIPELINE_PROVIDER` | `synthesizer.provider` | `gemini`, `claude`, `openai` |
| `PIPELINE_EFFORT` | `synthesizer.effort` | `low`, `high` |
| `GOG_ACCOUNT` | `google_account` | Google email |

---

## Pipeline Configuration (YAML)

**Location:** `~/.config/signal-flow/pipeline.yaml`
**Reference:** [deploy/pipeline.yaml.example](file:///signal-flow/deploy/pipeline.yaml.example)

```yaml
sources:
  - bluesky            # Bluesky timeline links
  - youtube            # YouTube subscription uploads

google_account: ""     # Google account email, or set GOG_ACCOUNT env var

synthesizer:
  provider: gemini     # gemini | claude | openai
  effort: low          # low (flash) | high (reasoning)
  limit: 10            # max signals per run

notify:
  channel: slack
  webhook_url: ""      # Set via SLACK_WEBHOOK_URL env var or here
  target: "#daily-digest"

schedule:
  interval: "4h"       # Docs only — scheduling is external

run_log_path: ~/.config/signal-flow/runs/pipeline.jsonl
```

---

## Auth Setup for Headless Deployment

On a desktop (with a browser), tokens are stored in GNOME Keyring. For headless/cloud environments, you must export them to the encrypted file backend.

### Step 1: Authenticate locally

```bash
# Bluesky
./bin/signal-flow bluesky-login --identifier you.bsky.social --password <app-password>

# Google (opens browser for OAuth)
./bin/signal-flow auth add you@gmail.com
```

### Step 2: Export Google tokens to file backend

```bash
./bin/signal-flow auth export --account you@gmail.com
# Enter a passphrase when prompted — this becomes GOG_KEYRING_PASSWORD
```

This writes encrypted token files to `~/.config/signal-flow/keyring/`.

### Step 3: Set env vars

```bash
GOG_KEYRING_BACKEND=file
GOG_KEYRING_PASSWORD=<the passphrase you chose>
GOG_ACCOUNT=you@gmail.com
```

### Step 4: Test headless

```bash
GOG_KEYRING_BACKEND=file \
GOG_KEYRING_PASSWORD="<passphrase>" \
GOG_ACCOUNT="you@gmail.com" \
./bin/signal-flow pipeline run --dry-run
```

---

## File Layout

```
~/.config/signal-flow/
├── pipeline.yaml              # Pipeline configuration
├── session.json               # Bluesky session (JWT tokens)
├── keyring/                   # Encrypted Google OAuth tokens (file backend)
│   ├── token:default:you@gmail.com
│   └── token:you@gmail.com
├── credentials/               # OAuth client credentials
│   └── default/credentials.json
├── runs/
│   └── pipeline.jsonl         # Pipeline run history (JSONL)
├── logs/
│   └── pipeline.log           # systemd output log
└── env                        # Environment file (systemd)
```

---

## Deployment Targets

### Local (systemd)

**Files:**
- [signal-flow-pipeline.service](file:///signal-flow/deploy/systemd/signal-flow-pipeline.service) — oneshot service
- [signal-flow-pipeline.timer](file:///signal-flow/deploy/systemd/signal-flow-pipeline.timer) — 4-hour timer

```bash
# 1. Install binary
go build -o /usr/local/bin/signal-flow ./cmd/signal-flow

# 2. Create env file
cat > ~/.config/signal-flow/env << 'EOF'
DATABASE_URL=postgres://...
ENCRYPTION_KEY=...
GEMINI_API_KEY=...
SLACK_WEBHOOK_URL=https://hooks.slack.com/...
GOG_KEYRING_BACKEND=file
GOG_KEYRING_PASSWORD=...
GOG_ACCOUNT=you@gmail.com
EOF

# 3. Create log directory
mkdir -p ~/.config/signal-flow/logs

# 4. Install and enable timer
cp deploy/systemd/signal-flow-pipeline.* ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now signal-flow-pipeline.timer

# 5. Verify
systemctl --user status signal-flow-pipeline.timer
systemctl --user list-timers
journalctl --user -u signal-flow-pipeline.service -f
signal-flow pipeline status
```

### Docker

**Files:**
- [Dockerfile](file:///signal-flow/Dockerfile) — multi-stage build (~15MB)
- [docker-compose.yml](file:///signal-flow/docker-compose.yml) — PostgreSQL + pgAdmin

```bash
# Build image
docker build -t signal-flow .

# Run pipeline (mount keyring + config)
docker run \
  --env-file .env \
  -v ~/.config/signal-flow/keyring:/root/.config/signal-flow/keyring:ro \
  -v ~/.config/signal-flow/pipeline.yaml:/root/.config/signal-flow/pipeline.yaml:ro \
  -v ~/.config/signal-flow/session.json:/root/.config/signal-flow/session.json:ro \
  signal-flow pipeline run

# Quick test
docker run --env-file .env signal-flow pipeline run --dry-run
```

**.env file for Docker:**

```bash
DATABASE_URL=postgres://postgres:postgres@host.docker.internal:5433/signal_flow?sslmode=disable
ENCRYPTION_KEY=<hex>
GEMINI_API_KEY=<key>
SLACK_WEBHOOK_URL=https://hooks.slack.com/...
GOG_KEYRING_BACKEND=file
GOG_KEYRING_PASSWORD=<passphrase>
GOG_ACCOUNT=you@gmail.com
```

> **Note:** Use `host.docker.internal` to reach the host's PostgreSQL from inside Docker.

### Kubernetes

**Files:**
- [cronjob.yaml](file:///signal-flow/deploy/k8s/cronjob.yaml) — CronJob running every 4 hours

```bash
# 1. Build and push image
docker build -t ghcr.io/rvald/signal-flow:latest .
docker push ghcr.io/rvald/signal-flow:latest

# 2. Create secrets
kubectl create secret generic signal-flow-secrets \
  --from-literal=DATABASE_URL='postgres://...' \
  --from-literal=ENCRYPTION_KEY='...' \
  --from-literal=GEMINI_API_KEY='...' \
  --from-literal=SLACK_WEBHOOK_URL='https://hooks.slack.com/...' \
  --from-literal=GOG_KEYRING_PASSWORD='...' \
  --from-literal=GOG_ACCOUNT='you@gmail.com'

# 3. Create keyring secret (exported tokens)
kubectl create secret generic signal-flow-keyring \
  --from-file=keyring=~/.config/signal-flow/keyring/

# 4. Create config map
kubectl create configmap signal-flow-pipeline-config \
  --from-file=pipeline.yaml=deploy/pipeline.yaml.example

# 5. Deploy
kubectl apply -f deploy/k8s/cronjob.yaml

# 6. Verify
kubectl get cronjobs
kubectl get jobs --selector=app=signal-flow
kubectl logs -l app=signal-flow --tail=50
```

**CronJob spec highlights:**
- Schedule: `0 */4 * * *` (every 4 hours)
- `concurrencyPolicy: Forbid` — no overlapping runs
- `activeDeadlineSeconds: 600` — 10-minute timeout
- `backoffLimit: 2` — retry twice on failure
- Keyring mounted read-only at `/root/.config/signal-flow/keyring`

---

## Validation & Pre-flight Checks

The pipeline validates everything before making API calls:

| Check | Fails when |
|---|---|
| `DATABASE_URL` exists | Not set |
| `ENCRYPTION_KEY` exists | Not set |
| `google_account` configured | YouTube source enabled but no account |
| Bluesky session valid | `session.json` missing or token expired |

This prevents wasted API quota from partial pipeline runs.

---

## Troubleshooting

| Problem | Cause | Fix |
|---|---|---|
| `required env vars not set: DATABASE_URL` | Missing env var | Set in `.env` or systemd env file |
| `youtube source requires google_account` | No `google_account` in YAML or `GOG_ACCOUNT` | Set `GOG_ACCOUNT` in env |
| `Enter passphrase to unlock` prompt hangs | File keyring needs password | Set `GOG_KEYRING_PASSWORD` env var |
| `The specified item could not be found` | Token not in current keyring backend | Run `auth export` or set `GOG_KEYRING_BACKEND=file` |
| `authorized as X, expected` (empty) | `auth add` called without email arg | Use `signal-flow auth add you@gmail.com` |
| `Token has expired` | Bluesky session expired | Run `signal-flow bluesky-login` again |
| `429 RESOURCE_EXHAUSTED` | Gemini API quota exceeded | Wait or upgrade to paid tier |
