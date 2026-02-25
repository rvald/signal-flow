package intelligence

import (
	"context"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/sony/gobreaker/v2"
)

// ResilientSummarizer wraps a primary and fallback Summarizer with a circuit breaker.
// If the primary fails, it automatically falls back to the secondary provider.
type ResilientSummarizer struct {
	primary     domain.Summarizer
	fallback    domain.Summarizer
	cbSummarize *gobreaker.CircuitBreaker[summarizeResult]
	cbExtract   *gobreaker.CircuitBreaker[extractResult]
}

// Internal result types for gobreaker generics.
type summarizeResult struct {
	summary *domain.Summary
	usage   *domain.LLMUsage
}

type extractResult struct {
	tags    []string
	highSig bool
	usage   *domain.LLMUsage
}

// NewResilientSummarizer creates a ResilientSummarizer with sensible circuit breaker defaults.
// Settings: 5 consecutive failures → open, 30s timeout → half-open, 1 success → closed.
func NewResilientSummarizer(primary, fallback domain.Summarizer) *ResilientSummarizer {
	settings := gobreaker.Settings{
		Name:        "llm-provider",
		MaxRequests: 1, // Allow 1 request in half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &ResilientSummarizer{
		primary:     primary,
		fallback:    fallback,
		cbSummarize: gobreaker.NewCircuitBreaker[summarizeResult](settings),
		cbExtract:   gobreaker.NewCircuitBreaker[extractResult](settings),
	}
}

// Summarize attempts the primary provider first; falls back on error.
func (r *ResilientSummarizer) Summarize(ctx context.Context, content string, params domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	result, err := r.cbSummarize.Execute(func() (summarizeResult, error) {
		s, u, err := r.primary.Summarize(ctx, content, params)
		if err != nil {
			return summarizeResult{}, err
		}
		return summarizeResult{summary: s, usage: u}, nil
	})

	if err == nil {
		return result.summary, result.usage, nil
	}

	// Primary failed — use fallback.
	return r.fallback.Summarize(ctx, content, params)
}

// ExtractMetadata attempts the primary provider first; falls back on error.
func (r *ResilientSummarizer) ExtractMetadata(ctx context.Context, content string) ([]string, bool, *domain.LLMUsage, error) {
	result, err := r.cbExtract.Execute(func() (extractResult, error) {
		tags, hs, u, err := r.primary.ExtractMetadata(ctx, content)
		if err != nil {
			return extractResult{}, err
		}
		return extractResult{tags: tags, highSig: hs, usage: u}, nil
	})

	if err == nil {
		return result.tags, result.highSig, result.usage, nil
	}

	// Primary failed — use fallback.
	return r.fallback.ExtractMetadata(ctx, content)
}
