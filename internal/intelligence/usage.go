// Package intelligence implements the LLM orchestration pipeline for Signal-Flow.
package intelligence

import (
	"log/slog"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
)

// UsageTracker aggregates LLM token usage across a pipeline run.
type UsageTracker struct {
	entries []domain.LLMUsage
}

// NewUsageTracker creates an empty UsageTracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{}
}

// Track records an LLMUsage entry and emits a structured log line for cost auditing.
func (u *UsageTracker) Track(usage *domain.LLMUsage) {
	u.entries = append(u.entries, *usage)

	slog.Info("llm_call",
		"model", usage.Model,
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
		"latency_ms", usage.Latency.Milliseconds(),
	)
}

// Total returns the aggregated totals across all tracked LLM calls.
func (u *UsageTracker) Total() (totalPrompt, totalCompletion int, totalLatency time.Duration) {
	for _, e := range u.entries {
		totalPrompt += e.PromptTokens
		totalCompletion += e.CompletionTokens
		totalLatency += e.Latency
	}
	return
}

// Entries returns a copy of all tracked usage entries.
func (u *UsageTracker) Entries() []domain.LLMUsage {
	out := make([]domain.LLMUsage, len(u.entries))
	copy(out, u.entries)
	return out
}
