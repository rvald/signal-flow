---
title: "Phase 3: Intelligence Pipeline (The Synthesizer)"
project: signal-flow
phase: 3
status: complete
date_completed: 2026-02-24
authors:
  - rvald
tags:
  - llm
  - gemini
  - claude
  - circuit-breaker
  - tdd
  - observability
dependencies:
  - google.golang.org/genai
  - github.com/anthropics/anthropic-sdk-go
  - github.com/openai/openai-go/v3
  - github.com/sony/gobreaker/v2
---

# Phase 3: Intelligence Pipeline (The Synthesizer)

The "Brain" of Signal-Flow. A multi-pass LLM orchestration pipeline that transforms raw content into structured Oracle-1 briefings, with provider abstraction, cost tracking, and circuit-breaker resilience.

## Project Structure

```
signal-flow/
├── internal/
│   ├── domain/
│   │   ├── signal.go              # Phase 1 (+ FindBySourceURL)
│   │   ├── identity.go            # Phase 2
│   │   └── intelligence.go        # Summary, Summarizer, LLMUsage types
│   ├── intelligence/
│   │   ├── gemini.go              # GeminiSummarizer (google.golang.org/genai)
│   │   ├── claude.go              # ClaudeSummarizer (anthropic-sdk-go)
│   │   ├── openai.go              # OpenAISummarizer (openai-go/v3 Responses API)
│   │   ├── synthesizer.go         # SynthesizerService orchestrator
│   │   ├── resilience.go          # ResilientSummarizer (gobreaker circuit breaker)
│   │   ├── usage.go               # UsageTracker (slog-based cost logging)
│   │   ├── synthesizer_test.go    # Unit tests (mock-based, no API keys needed)
│   │   └── prompts/
│   │       ├── analysis.go        # Pass 1 prompt (tech stack + high-signal)
│   │       └── distillation.go    # Pass 2 prompt (Oracle-1 brief)
│   ├── repository/
│   │   └── postgres_signal.go     # + FindBySourceURL method
│   └── security/                  # Phase 2 (unchanged)
```

## Source Index

### Domain Layer

- **Summarizer interface** — [internal/domain/intelligence.go](file:///signal-flow/internal/domain/intelligence.go)
  - `Summarize(ctx, content, params) (*Summary, *LLMUsage, error)`
  - `ExtractMetadata(ctx, content) (tags, highSignal, *LLMUsage, error)`

- **Summary struct** — `WhyItMatters`, `Teaser`, `Citations`, `TechStack`, `HighSignal`, `Distillation`
- **LLMUsage struct** — `Model`, `PromptTokens`, `CompletionTokens`, `Latency`
- **SynthesisResult struct** — `Summary`, `Usage []LLMUsage`, `Cached`
- **Priority** — `PriorityStandard` (Flash for all), `PriorityHigh` (Reasoning for Pass 2)

### Intelligence Layer

- **GeminiSummarizer** — [internal/intelligence/gemini.go](file:///signal-flow/internal/intelligence/gemini.go)
  - Uses `google.golang.org/genai` SDK, BackendGeminiAPI
  - Extracts `PromptTokenCount` / `CandidatesTokenCount` from `UsageMetadata`

- **ClaudeSummarizer** — [internal/intelligence/claude.go](file:///signal-flow/internal/intelligence/claude.go)
  - Uses `github.com/anthropics/anthropic-sdk-go` with `option.WithAPIKey`
  - Extracts `InputTokens` / `OutputTokens` from response `Usage`

- **OpenAISummarizer** — [internal/intelligence/openai.go](file:///signal-flow/internal/intelligence/openai.go)
  - Uses `github.com/openai/openai-go/v3` Responses API (`client.Responses.New`)
  - Uses `Instructions` for system prompt, string `Input` for user content
  - Extracts `InputTokens` / `OutputTokens` from `ResponseUsage`

- **ResilientSummarizer** — [internal/intelligence/resilience.go](file:///signal-flow/internal/intelligence/resilience.go)
  - Wraps primary + fallback `Summarizer` with `gobreaker.CircuitBreaker`
  - 5 consecutive failures → open, 30s timeout → half-open, 1 success → closed

- **UsageTracker** — [internal/intelligence/usage.go](file:///signal-flow/internal/intelligence/usage.go)
  - `Track(*LLMUsage)` — logs via `slog.Info` with structured fields
  - `Total()` — aggregates (promptTokens, completionTokens, latency)

- **SynthesizerService** — [internal/intelligence/synthesizer.go](file:///signal-flow/internal/intelligence/synthesizer.go)
  - Two-pass orchestrator: Flash for analysis, Flash or Reasoning for distillation
  - Idempotency check via `SignalRepository.FindBySourceURL`
  - Persists results to `Signal.Distillation` and `Signal.Metadata`

### Prompt Templates

- **Analysis (Pass 1)** — [internal/intelligence/prompts/analysis.go](file:///signal-flow/internal/intelligence/prompts/analysis.go)
  - Identifies tech stack tags and high-signal markers (repos, papers, benchmarks)
  - Output: JSON `{ "tech_stack": [...], "high_signal": bool }`

- **Distillation (Pass 2)** — [internal/intelligence/prompts/distillation.go](file:///signal-flow/internal/intelligence/prompts/distillation.go)
  - Generates Oracle-1 brief with structured markdown, optimized for AI agent consumption
  - Output: JSON `{ "why_it_matters", "teaser", "citations", "distillation" }`

### Tests

- **Unit tests** — [internal/intelligence/synthesizer_test.go](file:///signal-flow/internal/intelligence/synthesizer_test.go)
  - `Test_Token_Tracker` — aggregates prompt/completion tokens and latency across 2 calls
  - `Test_Failover_Logic` — mocks 503 errors, verifies fallback + circuit breaker
  - `Test_Pipeline_Routing` — 3 subtests: standard/low → flash, high priority → reasoning, high signal → reasoning
  - `Test_Idempotency` — existing distillation → cached result, no LLM calls

## Design Decisions

- **`Summarizer` interface** — Provider-agnostic; swap Gemini/Claude/OpenAI/local without pipeline changes.
- **Three providers** — Gemini (genai SDK), Claude (anthropic-sdk-go), OpenAI (openai-go/v3 Responses API). Any can serve as primary or fallback.
- **Two-pass routing** — Flash tier for cheap analysis, Reasoning tier only when content is high-signal or priority is high. Saves cost.
- **`gobreaker` v2 generics** — Type-safe circuit breaker results without interface casts.
- **`slog` for observability** — stdlib structured logging (Go 1.21+), no external logging dep. JSON output for cost auditing.
- **JSON output prompts** — Both prompts require JSON responses for reliable parsing. `cleanJSON()` strips markdown code fences.
- **`FindBySourceURL` added to `SignalRepository`** — Needed for idempotency check. Postgres impl uses RLS-scoped transaction.
- **No RLS on intelligence** — Intelligence layer operates via the existing `SignalRepository` which handles RLS internally.

## Running

```bash
# Intelligence unit tests (fast, no Docker, no API keys)
go test ./internal/intelligence/... -v -count=1

# Full suite (Phase 1+2 need Docker for testcontainers)
go test ./... -v -count=1
```

## Environment Variables

```bash
GEMINI_API_KEY=your-gemini-key        # Required for live GeminiSummarizer
ANTHROPIC_API_KEY=your-anthropic-key  # Required for live ClaudeSummarizer
OPENAI_API_KEY=your-openai-key        # Required for live OpenAISummarizer
```
