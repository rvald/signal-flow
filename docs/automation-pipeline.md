---
title: "Phase 7: Automation Pipeline"
project: signal-flow
phase: 7
status: complete
date_completed: 2026-03-03
authors:
  - rvald
tags:
  - pipeline
  - automation
  - slack
  - youtube
  - systemd
  - docker
  - kubernetes
  - tdd
dependencies:
  - gopkg.in/yaml.v3
  - google.golang.org/api/youtube/v3
---

# Phase 7: Automation Pipeline

The scheduling and delivery backbone of Signal-Flow. A `signal-flow pipeline run` command that chains harvest → synthesize → notify into a single automated flow, with YAML config, Slack Block Kit delivery, JSONL run logging, and deployment configs for systemd, Docker, and Kubernetes.

## Project Structure

```
signal-flow/
├── cmd/signal-flow/cli/
│   └── pipeline.go                                    # pipeline run / pipeline status commands
├── internal/
│   ├── config/
│   │   ├── pipeline.go                                # PipelineConfig, YAML loader, env overrides
│   │   └── pipeline_test.go                           # 5 tests (full file, env override, defaults, not found, invalid)
│   ├── harvester/providers/
│   │   ├── youtube.go                                 # YouTubeHarvester (YouTubeAPI interface, upload filter)
│   │   └── youtube_test.go                            # 5 tests (convert, empty, skip non-upload, errors, provider)
│   ├── notify/
│   │   ├── slack.go                                   # SlackNotifier (Block Kit webhook, DigestSummary)
│   │   └── slack_test.go                              # 5 tests (success, block format, HTTP error, network, empty)
│   └── pipeline/
│       ├── pipeline.go                                # Pipeline orchestrator (harvest → synthesize → notify)
│       ├── runlog.go                                  # PipelineRun type, JSONL read/write
│       └── pipeline_test.go                           # 5 tests (full run, partial, no signals, notify error, JSONL)
├── deploy/
│   ├── systemd/
│   │   ├── signal-flow-pipeline.service               # oneshot service unit
│   │   └── signal-flow-pipeline.timer                 # 4-hour timer with Persistent=true
│   ├── k8s/
│   │   └── cronjob.yaml                               # K8s CronJob manifest
│   └── pipeline.yaml.example                          # Example pipeline config
└── Dockerfile                                         # Multi-stage build (golang → alpine)
```

## Source Index

### Config Layer

- **PipelineConfig** — [internal/config/pipeline.go](file:///signal-flow/internal/config/pipeline.go)
  - `Sources`, `GoogleAccount`, `Synthesizer`, `Notify`, `Schedule`, `RunLogPath`
  - `LoadPipelineConfig(path)` — reads YAML, applies env var overrides
  - Env overrides: `SLACK_WEBHOOK_URL`, `PIPELINE_PROVIDER`, `PIPELINE_EFFORT`, `GOG_ACCOUNT`

### YouTube Harvester

- **YouTubeHarvester** — [internal/harvester/providers/youtube.go](file:///signal-flow/internal/harvester/providers/youtube.go)
  - `YouTubeAPI` interface — mockable abstraction over YouTube Data API v3
  - `YouTubeActivity` struct — platform-agnostic activity representation
  - Filters to `upload` type only, converts to `[]RawSignal`
  - `SourceURL = https://www.youtube.com/watch?v={VideoID}`
  - Metadata: `video_id`, `channel_title`, `published_at`

### Slack Notifier

- **SlackNotifier** — [internal/notify/slack.go](file:///signal-flow/internal/notify/slack.go)
  - `Notifier` interface — `Notify(ctx, *DigestSummary) error`
  - `DigestSummary` / `DigestSignal` structs
  - Block Kit layout: header, context (stats), divider, signal sections (max 10)
  - Empty digest → "No new signals" message

### Pipeline Orchestrator

- **Pipeline** — [internal/pipeline/pipeline.go](file:///signal-flow/internal/pipeline/pipeline.go)
  - `HarvestFunc` / `SynthesizeFunc` — dependency-injected phase functions
  - `Run(ctx) (*PipelineRun, error)` — harvest all sources → synthesize → notify → log
  - Status: `"ok"` | `"partial"` (some phase failed) | `"error"` (all failed)

- **PipelineRun** — [internal/pipeline/runlog.go](file:///signal-flow/internal/pipeline/runlog.go)
  - `WriteRunLog(path, *PipelineRun)` — append JSONL line
  - `ReadRunLog(path) ([]*PipelineRun, error)` — parse JSONL

### CLI Commands

- **pipeline run** — [cmd/signal-flow/cli/pipeline.go](file:///signal-flow/cmd/signal-flow/cli/pipeline.go)
  - Loads `pipeline.yaml`, validates env vars and source config upfront, connects DB, runs pipeline
  - Harvest sources: `bluesky` (timeline links), `youtube` (subscriptions → channel activities → uploads)
  - Pre-flight validation: fails fast on missing `DATABASE_URL`/`ENCRYPTION_KEY` and missing `google_account` for YouTube source
  - Flags: `--config`, `--dry-run`

- **pipeline status** — shows recent runs from the JSONL log
  - Flags: `--config`, `--limit`

### Tests

- **Config tests** — [internal/config/pipeline_test.go](file:///signal-flow/internal/config/pipeline_test.go)
  - `Test_LoadPipelineConfig_FullFile` — complete YAML → struct
  - `Test_LoadPipelineConfig_EnvOverride` — `SLACK_WEBHOOK_URL` overrides file
  - `Test_LoadPipelineConfig_Defaults` — missing fields get defaults
  - `Test_LoadPipelineConfig_FileNotFound` — clear error
  - `Test_LoadPipelineConfig_InvalidYAML` — parse error

- **YouTube tests** — [internal/harvester/providers/youtube_test.go](file:///signal-flow/internal/harvester/providers/youtube_test.go)
  - `Test_YouTubeHarvester_ConvertActivities` — activity → RawSignal
  - `Test_YouTubeHarvester_EmptyActivities` — empty → no error
  - `Test_YouTubeHarvester_SkipNonUpload` — filters likes/favorites
  - `Test_YouTubeHarvester_ErrorHandling` — transient vs auth errors
  - `Test_YouTubeHarvester_Provider` — correct identifier

- **Slack tests** — [internal/notify/slack_test.go](file:///signal-flow/internal/notify/slack_test.go)
  - `Test_SlackNotifier_Success` — webhook POST, valid JSON
  - `Test_SlackNotifier_BlockFormat` — header + context + divider + sections
  - `Test_SlackNotifier_HTTPError` — 500 → error
  - `Test_SlackNotifier_NetworkError` — unreachable → error
  - `Test_SlackNotifier_EmptyDigest` — "no new signals" message

- **Pipeline tests** — [internal/pipeline/pipeline_test.go](file:///signal-flow/internal/pipeline/pipeline_test.go)
  - `Test_Pipeline_FullRun` — all phases, run log written
  - `Test_Pipeline_HarvestError_ContinuesOtherSources` — partial failure
  - `Test_Pipeline_NoSignals_SkipsSynthesizeAndNotify` — no wasted LLM calls
  - `Test_Pipeline_NotifyError_StillLogsRun` — run log always written
  - `Test_Pipeline_RunLog_JSONL` — valid JSON per line

## Design Decisions

- **`YouTubeAPI` interface** — Decouples the harvester from the YouTube SDK for testability. Tests inject a mock; production injects a wrapper around the real SDK.
- **Upload-only filter** — YouTube `Activities.List` returns likes, favorites, etc. Only `upload` activities produce meaningful signals worth synthesizing.
- **Block Kit for Slack** — Rich formatting with headers, sections, links, and context blocks. Capped at 10 signals per message to stay within Slack's block limit.
- **Functional dependency injection in Pipeline** — `HarvestFunc` and `SynthesizeFunc` are function types, not interfaces. This keeps the Pipeline struct simple and avoids creating wrapper interfaces for the CLI's inline closures.
- **JSONL run log** — One JSON line per run, appendable, greppable, easily parseable. Inspired by OpenClaw's run logging pattern.
- **Separate scheduler from work** — `signal-flow pipeline run` is stateless and exits. The scheduler (systemd timer, K8s CronJob) is external. Same binary, different entrypoints.
- **YAML config with env overrides** — Config file for complex settings, env vars for secrets. `SLACK_WEBHOOK_URL` env var always wins over the file value.
- **Pre-flight validation** — All required env vars and source-specific config (e.g. `google_account` for YouTube) are validated before any API calls, preventing wasted quota on partial runs.
- **Headless auth via `auth export`** — Tokens stored in GNOME Keyring (desktop) can be exported to the encrypted file backend via `signal-flow auth export`, then unlocked headlessly with `GOG_KEYRING_PASSWORD` env var.
- **`Persistent=true` on systemd timer** — Catches up missed runs if the machine was off. `RandomizedDelaySec=300` adds jitter.
- **Multi-stage Docker build** — `golang:1.25-alpine` builder → `alpine:3.21` runtime. CGO disabled, statically linked. Final image ~15MB.

## Running

```bash
# YouTube harvester tests (fast, no API keys)
go test ./internal/harvester/providers/... -v -count=1 -run Test_YouTube

# Pipeline config tests (fast)
go test ./internal/config/... -v -count=1 -run Test_LoadPipelineConfig

# Slack notifier tests (fast, uses httptest)
go test ./internal/notify/... -v -count=1

# Pipeline orchestrator tests (fast, all mocked)
go test ./internal/pipeline/... -v -count=1

# Full test suite
go test ./... -v -count=1
```

## Deployment

### Local (systemd)

```bash
# Install the binary
go build -o /usr/local/bin/signal-flow ./cmd/signal-flow

# Create env file with secrets
cat > ~/.config/signal-flow/env << 'EOF'
DATABASE_URL=postgres://...
ENCRYPTION_KEY=...
GEMINI_API_KEY=...
SLACK_WEBHOOK_URL=https://hooks.slack.com/...
GOG_KEYRING_PASSWORD=...       # keyring passphrase for headless YouTube auth
GOG_ACCOUNT=you@gmail.com      # Google account for YouTube source
EOF

# Copy and enable the timer
cp deploy/systemd/signal-flow-pipeline.* ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now signal-flow-pipeline.timer

# Check status
systemctl --user status signal-flow-pipeline.timer
journalctl --user -u signal-flow-pipeline.service
signal-flow pipeline status
```

### Docker

```bash
# Build
docker build -t signal-flow .

# Run pipeline
docker run --env-file .env signal-flow pipeline run

# Verify
docker run signal-flow pipeline --help
```

### Kubernetes

```bash
# Create secrets (include GOG_KEYRING_PASSWORD and GOG_ACCOUNT for YouTube)
kubectl create secret generic signal-flow-secrets \
  --from-literal=DATABASE_URL='...' \
  --from-literal=ENCRYPTION_KEY='...' \
  --from-literal=GEMINI_API_KEY='...' \
  --from-literal=SLACK_WEBHOOK_URL='...' \
  --from-literal=GOG_KEYRING_PASSWORD='...' \
  --from-literal=GOG_ACCOUNT='you@gmail.com'

# Create keyring secret (export tokens first: signal-flow auth export)
kubectl create secret generic signal-flow-keyring \
  --from-file=keyring=~/.config/signal-flow/keyring/

kubectl create configmap signal-flow-pipeline-config --from-file=pipeline.yaml=deploy/pipeline.yaml.example

# Deploy
kubectl apply -f deploy/k8s/cronjob.yaml

# Check
kubectl get cronjobs
kubectl get jobs --selector=app=signal-flow
```
