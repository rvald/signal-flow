# Slack Bot Integration

> Interactive Slack bot for Signal-Flow: users issue natural language commands, an LLM agent interprets and executes CLI operations, then responds with results.

**Status:** All phases complete. 42 tests across 5 packages.

---

## Request Lifecycle

```
User types in Slack: "What signals did we find recently?"
                          │
                          ▼
┌─────────────────────────────────────────────┐
│  cli/bot.go — signal-flow bot start         │
│  Validates SLACK_APP_TOKEN, SLACK_BOT_TOKEN │
│  Builds: App → Tools → GeminiLLM → Agent   │
└──────────────────┬──────────────────────────┘
                   │ starts
                   ▼
┌─────────────────────────────────────────────┐
│  slackbot/bot.go — Socket Mode Event Loop   │
│  Receives Slack events via WebSocket        │
│  Filters: ignores own msgs, edits, subtypes │
│  Routes MessageEvent / AppMentionEvent      │
└──────────────────┬──────────────────────────┘
                   │ ev.User + ev.Text
                   ▼
┌─────────────────────────────────────────────┐
│  slackbot/handler.go — HandleMessage()      │
│  Trims whitespace, skips empty messages     │
│  Gets/creates Session via SessionStore      │
│  Delegates to Agent.Handle()                │
│  On error → returns friendly fallback msg   │
└──────────────────┬──────────────────────────┘
                   │ (userID, text)
                   ▼
┌─────────────────────────────────────────────┐
│  agent/agent.go — Handle() loop             │
│                                             │
│  1. Adds user message to Session            │
│  2. Builds context:                         │
│     [system prompt] + Session.Window()      │
│  3. Calls LLMClient.Chat() with messages    │
│     + tool schemas from Registry.Schema()   │
│  4. If LLM returns text → done, return it   │
│  5. If LLM returns ToolCalls:               │
│     → execute each via Registry.Get()       │
│     → feed results back as tool messages    │
│     → loop back to step 3                   │
│  6. Max 10 rounds to prevent infinite loops │
└───────┬─────────────────────┬───────────────┘
        │                     │
   LLM call              Tool execution
        │                     │
        ▼                     ▼
┌────────────────┐  ┌──────────────────────────┐
│ agent/          │  │ agent/tools/              │
│ gemini_client   │  │                          │
│                 │  │ query_signals             │
│ Converts:       │  │  → repo.FindRecentByTenant│
│ Message → genai │  │                          │
│ Schema → genai  │  │ pipeline_status           │
│ FuncCall → Tool │  │  → pipeline.ReadRunLog    │
│ Call            │  │                          │
└────────────────┘  │ harvest (future)          │
                    │  → harvestFn(ctx, source)  │
                    └──────────────────────────┘
        │
        │ reply text
        ▼
┌─────────────────────────────────────────────┐
│  slackbot/formatter.go — FormatBlocks()     │
│  Converts text → Slack Block Kit            │
│  (Markdown section blocks)                  │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
          api.PostMessage(channel, blocks)
                   │
                   ▼
          User sees response in Slack
```

---

## Package Dependencies

```
cli/bot.go (composition root — wires everything)
    │
    ├── internal/app        ← DB pool, repos, summarizers, notifier
    │       └── uses: config, domain, repository, intelligence, security, notify
    │
    ├── internal/agent      ← LLMClient interface + Agent loop + Sessions
    │       ├── agent.go         Handle() loop, LLMClient interface
    │       ├── session.go       Session (sliding window), SessionStore (thread-safe)
    │       └── gemini_client.go GeminiLLMClient (genai adapter)
    │
    ├── internal/agent/tools ← Tool definitions + Registry
    │       ├── tools.go          Tool, Param, Result, Registry
    │       ├── harvest_tool.go   harvest + query_signals tools
    │       └── status_tool.go    pipeline_status tool
    │
    └── internal/slackbot   ← Slack integration
            ├── bot.go           Socket Mode event loop
            ├── handler.go       Message → Agent routing
            └── formatter.go     Text → Block Kit
```

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Handler separated from Bot** | `Handler` is pure Go — testable with mock LLM. `Bot` has Slack SDK deps. |
| **`LLMClient` interface** | Agent loop is provider-agnostic. Swap Gemini for Claude by implementing one method. |
| **Session sliding window** | Token budget prevents context overflow. Oldest messages drop first. |
| **Tool errors → LLM** | Instead of crashing, tool errors are fed back to the LLM so it can explain them. |
| **`app.BuildSummarizers()` shared** | Both CLI pipeline and bot reuse the same provider-switching logic. |
| **Direct function calls** | No channel-based invoker — tools are local calls with `context.WithTimeout` if needed. |
| **Socket Mode** | No public URL needed; simpler for dev and Docker. |

---

## Module Map

```
internal/
  app/           ← Service bootstrap (DB, repos, synthesizer, notifier)     ✅
  agent/         ← Conversational LLM agent with tool dispatch              ✅
    tools/       ← Typed tool definitions (harvest, query, status)           ✅
  slackbot/      ← Slack Socket Mode event handler + Block Kit formatter    ✅

cmd/signal-flow/cli/
  bot.go         ← `signal-flow bot start` command                          ✅
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
| `gemini_client.go` | ✅ Done | `GeminiLLMClient` adapter with function calling support |

### Phase 4: Slack Bot Event Handler (`internal/slackbot`) ✅

Socket Mode event handler with message routing and Block Kit formatting.

| File | Status | Notes |
|------|--------|-------|
| `bot.go` | ✅ Done | Socket Mode event loop, message + app_mention handlers |
| `handler.go` | ✅ Done | Routes messages to Agent, graceful error fallback, 3 tests |
| `formatter.go` | ✅ Done | Text → Block Kit blocks, 2 tests |

### Phase 5: CLI Integration & Deployment ✅

| File | Status | Notes |
|------|--------|-------|
| `cli/bot.go` | ✅ Done | `signal-flow bot start` command |
| `cli/root.go` | ✅ Modified | Registered `newBotCmd()` |

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

---

## Slack App Setup

### 1. Create the App

- Go to [api.slack.com/apps](https://api.slack.com/apps) → **Create New App** → **From scratch**
- Name: `Signal-Flow Bot`, select your workspace

### 2. Enable Socket Mode

- Sidebar → **Socket Mode** → Toggle **ON**
- Generate an App-Level Token (name: `signal-flow-socket`, scope: `connections:write`)
- Copy → this is your **`SLACK_APP_TOKEN`** (`xapp-...`)

### 3. Subscribe to Events

- Sidebar → **Event Subscriptions** → Toggle **ON**
- Under **Subscribe to bot events**, add:
  - `message.channels` — hears messages in public channels
  - `message.im` — hears direct messages
  - `app_mention` — responds when @mentioned

### 4. Set Bot Scopes

- Sidebar → **OAuth & Permissions** → **Bot Token Scopes**:
  - `chat:write` — post messages
  - `channels:history` — read channel messages
  - `im:history` — read DMs
  - `app_mentions:read` — see @mentions

### 5. Install to Workspace

- Sidebar → **Install App** → **Install to Workspace** → Authorize
- Copy the **Bot User OAuth Token** → this is your **`SLACK_BOT_TOKEN`** (`xoxb-...`)

### 6. Invite & Run

```bash
# Invite the bot to a channel
/invite @Signal-Flow Bot

# Set environment variables
export SLACK_APP_TOKEN=xapp-1-...
export SLACK_BOT_TOKEN=xoxb-...
export GEMINI_API_KEY=...
export DATABASE_URL=postgres://...
export ENCRYPTION_KEY=...

# Start the bot
signal-flow bot start
```

---

## Pipeline Notifications (Incoming Webhook)

Separate from the interactive bot, the pipeline can post one-way notifications after each run.

### Setup

1. In your Slack app → Sidebar → **Incoming Webhooks** → Toggle **ON**
2. Click **Add New Webhook to Workspace**
3. Pick the target channel (e.g. `#signals`) → **Allow**
4. Copy the **Webhook URL** (`https://hooks.slack.com/services/T.../B.../...`)

### Configure

In `~/.config/signal-flow/pipeline.yaml`:

```yaml
notify:
  channel: slack
  webhook_url: "https://hooks.slack.com/services/T.../B.../..."
  target: "#signals"
```

Then `signal-flow pipeline run` will post a summary to the channel after each run.
