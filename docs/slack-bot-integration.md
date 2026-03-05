# Slack Bot Integration

> Interactive Slack bot for Signal-Flow: users issue natural language commands, an LLM agent interprets and executes CLI operations, then responds with results.

**Status:** Phase 1 complete, Phases 2–5 pending.

---

## Architecture

```
Slack (Socket Mode)
    → Bot.HandleMessage(event)
        → SessionStore.GetOrCreate(userID)
        → Agent.Handle(session, message)
            → LLM (interprets → tool calls)
            → Tool.Execute(ctx, args)          // direct function calls
            → LLM (synthesizes results → text)
        → Bot.Reply(blocks)
```

**Key decisions:**
- **Thread-safe `SessionStore`** — per-user conversation context (inspired by [goclaw](../goclaw) Registry pattern)
- **Direct tool execution** — no channel-based invoker; tools are local function calls with `context.WithTimeout`
- **Socket Mode** — no public URL needed; simpler for dev and Docker

---

## Module Map

```
internal/
  app/           ← Service bootstrap (DB, repos, synthesizer, notifier)     ✅
  agent/         ← Conversational LLM agent with tool dispatch              ✅
    tools/       ← Typed tool definitions (harvest, query, status)           ✅
  slackbot/      ← Slack Socket Mode event handler + Block Kit formatter    ⬜

cmd/signal-flow/cli/
  bot.go         ← `signal-flow bot start` command                          ⬜
```

---

## Phases

### Phase 1: Extract Service Bootstrap (`internal/app`) ✅

Extracted service assembly from `cli/pipeline.go` into a reusable package so both CLI and bot can share it.

| File | Status | Notes |
|------|--------|-------|
| `internal/app/app.go` | ✅ Done | `App`, `Config`, `New()`, `BuildSummarizers()`, `FromPipelineConfig()` |
| `internal/app/app_test.go` | ✅ Done | 10 tests (validation, provider errors, config mapping) |
| `cli/pipeline.go` | ✅ Refactored | Uses `app.BuildSummarizers()`, −45 lines |
| `main.go` | ✅ Cleaned | Deleted 125 lines of commented-out HTTP server code |

### Phase 2: Agent Tool Registry (`internal/agent/tools`) ✅

Typed tool definitions the LLM can invoke. Each tool wraps an `App` service operation.

| File | Status | Notes |
|------|--------|-------|
| `tools.go` | ✅ Done | `Tool`, `Param`, `Result`, `Registry` types with 8 tests |
| `harvest_tool.go` | ✅ Done | `harvest` tool (wraps harvest fn), `query_signals` tool (wraps repo), 4 tests |
| `status_tool.go` | ✅ Done | `pipeline_status` tool (wraps ReadRunLog) |

### Phase 3: Conversational Agent Core (`internal/agent`) ✅

LLM-powered agent with tool dispatch, session management, and context windowing.

| File | Status | Notes |
|------|--------|-------|
| `agent.go` | ✅ Done | `Agent`, `LLMClient` interface, `Handle()` loop with tool dispatch, 4 tests |
| `session.go` | ✅ Done | `Session` with sliding window, `SessionStore` (thread-safe), 5 tests |

### Phase 4: Slack Bot Event Handler (`internal/slackbot`) ⬜

- Socket Mode event loop
- Message handler → agent dispatch → Block Kit response
- Per-user rate limiting
- User → tenant resolution

### Phase 5: CLI Integration & Deployment ⬜

- `signal-flow bot start` command
- Pipeline config updates for bot settings
- Dockerfile and deployment configs
- Documentation

---

## Engineering Practices

| Practice | Where |
|---|---|
| TDD red-green-refactor | All phases |
| Structured logging with request IDs | `internal/app` |
| Rate limiting | Bot — per-user on message handling |
| Circuit breaker | Reuse `ResilientSummarizer` for agent LLM calls |
| Token budget management | Agent — cap per conversation turn |
| Context windowing | Session — sliding window within LLM context limits |
| Graceful degradation | Bot — fallback message if LLM is down |
| Idempotent operations | Existing in `SynthesizerService` |

---

## Environment Variables

New variables required for the bot:

| Variable | Description |
|---|---|
| `SLACK_APP_TOKEN` | `xapp-...` token for Socket Mode |
| `SLACK_BOT_TOKEN` | `xoxb-...` token for API calls |

Existing variables remain unchanged (`DATABASE_URL`, `ENCRYPTION_KEY`, `GEMINI_API_KEY`, etc.).
