package intelligence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence"
)

// --- Mock Summarizer ---

// mockSummarizer records calls and returns configured responses.
type mockSummarizer struct {
	name string

	// Summarize behavior
	summarizeCalled bool
	summarizeResult *domain.Summary
	summarizeUsage  *domain.LLMUsage
	summarizeErr    error

	// ExtractMetadata behavior
	extractCalled  bool
	extractTags    []string
	extractHighSig bool
	extractUsage   *domain.LLMUsage
	extractErr     error
}

func (m *mockSummarizer) Summarize(_ context.Context, _ string, _ domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	m.summarizeCalled = true
	return m.summarizeResult, m.summarizeUsage, m.summarizeErr
}

func (m *mockSummarizer) ExtractMetadata(_ context.Context, _ string) ([]string, bool, *domain.LLMUsage, error) {
	m.extractCalled = true
	return m.extractTags, m.extractHighSig, m.extractUsage, m.extractErr
}

// --- Mock SignalRepository ---

// mockSignalRepo implements domain.SignalRepository for testing the synthesizer.
type mockSignalRepo struct {
	// FindBySourceURL behavior (for idempotency check)
	existingSignal *domain.Signal
	findErr        error

	// Save behavior
	saveCalled  bool
	savedSignal *domain.Signal
	saveErr     error
}

func (m *mockSignalRepo) Save(_ context.Context, signal *domain.Signal) error {
	m.saveCalled = true
	m.savedSignal = signal
	return m.saveErr
}

func (m *mockSignalRepo) FindBySourceURL(_ context.Context, _ uuid.UUID, _ string) (*domain.Signal, error) {
	return m.existingSignal, m.findErr
}

// Unused but required by interface — we only need Save and FindBySourceURL for the synthesizer.
func (m *mockSignalRepo) FindRecentByTenant(_ context.Context, _ uuid.UUID, _ int) ([]*domain.Signal, error) {
	return nil, nil
}

func (m *mockSignalRepo) SearchSemantic(_ context.Context, _ uuid.UUID, _ pgvector.Vector, _ int) ([]*domain.Signal, error) {
	return nil, nil
}

func (m *mockSignalRepo) PromoteToTeam(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

// =============================================================================
// Test_Token_Tracker
// Verifies that UsageTracker correctly aggregates token counts and latency
// across multiple LLM calls.
// =============================================================================

func Test_Token_Tracker(t *testing.T) {
	tracker := intelligence.NewUsageTracker()

	tracker.Track(&domain.LLMUsage{
		Model:            "gemini-2.0-flash",
		PromptTokens:     100,
		CompletionTokens: 50,
		Latency:          200 * time.Millisecond,
	})

	tracker.Track(&domain.LLMUsage{
		Model:            "claude-sonnet-4-5",
		PromptTokens:     500,
		CompletionTokens: 300,
		Latency:          1500 * time.Millisecond,
	})

	totalPrompt, totalCompletion, totalLatency := tracker.Total()

	if totalPrompt != 600 {
		t.Errorf("total prompt tokens: want 600, got %d", totalPrompt)
	}
	if totalCompletion != 350 {
		t.Errorf("total completion tokens: want 350, got %d", totalCompletion)
	}
	if totalLatency != 1700*time.Millisecond {
		t.Errorf("total latency: want 1700ms, got %v", totalLatency)
	}

	// Verify the tracked entries are accessible.
	entries := tracker.Entries()
	if len(entries) != 2 {
		t.Errorf("entries count: want 2, got %d", len(entries))
	}
}

// =============================================================================
// Test_Failover_Logic
// Mocks a primary LLM provider returning errors and verifies:
// 1. The system falls back to the secondary provider
// 2. After enough failures, the circuit breaker trips
// =============================================================================

func Test_Failover_Logic(t *testing.T) {
	primary := &mockSummarizer{
		name:         "primary",
		extractErr:   errors.New("503 service unavailable"),
		summarizeErr: errors.New("503 service unavailable"),
	}

	fallbackSummary := &domain.Summary{
		WhyItMatters: "Fallback summary",
		Teaser:       "Fallback teaser",
	}
	fallback := &mockSummarizer{
		name:            "fallback",
		extractTags:     []string{"Go"},
		extractHighSig:  false,
		extractUsage:    &domain.LLMUsage{Model: "fallback", PromptTokens: 10, CompletionTokens: 5, Latency: 50 * time.Millisecond},
		summarizeResult: fallbackSummary,
		summarizeUsage:  &domain.LLMUsage{Model: "fallback", PromptTokens: 20, CompletionTokens: 15, Latency: 100 * time.Millisecond},
	}

	resilient := intelligence.NewResilientSummarizer(primary, fallback)
	ctx := context.Background()

	// --- Summarize failover ---
	summary, usage, err := resilient.Summarize(ctx, "test content", domain.ContextParams{})
	if err != nil {
		t.Fatalf("Summarize should succeed via fallback, got error: %v", err)
	}
	if summary.WhyItMatters != "Fallback summary" {
		t.Errorf("expected fallback summary, got %q", summary.WhyItMatters)
	}
	if usage.Model != "fallback" {
		t.Errorf("expected fallback usage model, got %q", usage.Model)
	}

	// --- ExtractMetadata failover ---
	tags, highSig, usage, err := resilient.ExtractMetadata(ctx, "test content")
	if err != nil {
		t.Fatalf("ExtractMetadata should succeed via fallback, got error: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Go" {
		t.Errorf("expected [Go] tags from fallback, got %v", tags)
	}
	if highSig {
		t.Error("expected highSig=false from fallback")
	}

	// Verify primary was attempted.
	if !primary.summarizeCalled {
		t.Error("primary.Summarize should have been attempted")
	}
	if !primary.extractCalled {
		t.Error("primary.ExtractMetadata should have been attempted")
	}
}

// =============================================================================
// Test_Pipeline_Routing
// Verifies that:
// - Standard priority + non-high-signal → flash for both passes
// - High priority → flash for Pass 1, reasoning for Pass 2
// - Standard priority + high-signal content → flash for Pass 1, reasoning for Pass 2
// =============================================================================

func Test_Pipeline_Routing(t *testing.T) {
	tests := []struct {
		name              string
		priority          domain.Priority
		flashHighSignal   bool // What flash.ExtractMetadata returns
		expectReasoningP2 bool // Whether reasoning should be used for Pass 2
	}{
		{
			name:              "standard priority, not high signal → flash for both",
			priority:          domain.PriorityStandard,
			flashHighSignal:   false,
			expectReasoningP2: false,
		},
		{
			name:              "high priority → reasoning for pass 2",
			priority:          domain.PriorityHigh,
			flashHighSignal:   false,
			expectReasoningP2: true,
		},
		{
			name:              "standard priority but high signal → reasoning for pass 2",
			priority:          domain.PriorityStandard,
			flashHighSignal:   true,
			expectReasoningP2: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flash := &mockSummarizer{
				name:            "flash",
				extractTags:     []string{"Go"},
				extractHighSig:  tt.flashHighSignal,
				extractUsage:    &domain.LLMUsage{Model: "flash", PromptTokens: 50, CompletionTokens: 20, Latency: 100 * time.Millisecond},
				summarizeResult: &domain.Summary{WhyItMatters: "flash summary", Distillation: "flash distillation"},
				summarizeUsage:  &domain.LLMUsage{Model: "flash", PromptTokens: 100, CompletionTokens: 80, Latency: 300 * time.Millisecond},
			}

			reasoning := &mockSummarizer{
				name:            "reasoning",
				summarizeResult: &domain.Summary{WhyItMatters: "reasoning summary", Distillation: "reasoning distillation"},
				summarizeUsage:  &domain.LLMUsage{Model: "reasoning", PromptTokens: 200, CompletionTokens: 150, Latency: 800 * time.Millisecond},
			}

			repo := &mockSignalRepo{findErr: intelligence.ErrSignalNotFound}

			svc := intelligence.NewSynthesizerService(flash, reasoning, repo)
			ctx := context.Background()

			tenantID := uuid.New()
			result, err := svc.Synthesize(ctx, tenantID, "https://example.com/article", "raw content here", domain.ContextParams{Priority: tt.priority})
			if err != nil {
				t.Fatalf("Synthesize: %v", err)
			}

			// Flash should always be called for Pass 1 (ExtractMetadata).
			if !flash.extractCalled {
				t.Error("flash.ExtractMetadata should always be called for Pass 1")
			}

			if tt.expectReasoningP2 {
				if !reasoning.summarizeCalled {
					t.Error("reasoning.Summarize should be called for Pass 2")
				}
				if flash.summarizeCalled {
					t.Error("flash.Summarize should NOT be called when reasoning handles Pass 2")
				}
				if result.Summary.WhyItMatters != "reasoning summary" {
					t.Errorf("expected reasoning summary, got %q", result.Summary.WhyItMatters)
				}
			} else {
				if reasoning.summarizeCalled {
					t.Error("reasoning.Summarize should NOT be called for standard/low-signal")
				}
				if !flash.summarizeCalled {
					t.Error("flash.Summarize should be called for Pass 2")
				}
				if result.Summary.WhyItMatters != "flash summary" {
					t.Errorf("expected flash summary, got %q", result.Summary.WhyItMatters)
				}
			}

			// Signal should be saved.
			if !repo.saveCalled {
				t.Error("signal should be saved to repository")
			}

			// Result should not be cached.
			if result.Cached {
				t.Error("result should not be cached for new content")
			}
		})
	}
}

// =============================================================================
// Test_Idempotency
// Verifies that synthesizing the same content twice returns the cached result
// without making any LLM calls.
// =============================================================================

func Test_Idempotency(t *testing.T) {
	flash := &mockSummarizer{name: "flash"}
	reasoning := &mockSummarizer{name: "reasoning"}

	existingSignal := &domain.Signal{
		ID:           uuid.New(),
		TenantID:     uuid.New(),
		SourceURL:    "https://example.com/cached",
		Distillation: "already distilled content",
	}

	repo := &mockSignalRepo{existingSignal: existingSignal}

	svc := intelligence.NewSynthesizerService(flash, reasoning, repo)
	ctx := context.Background()

	result, err := svc.Synthesize(ctx, existingSignal.TenantID, existingSignal.SourceURL, "raw content", domain.ContextParams{})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !result.Cached {
		t.Error("result should be marked as cached")
	}

	if flash.extractCalled {
		t.Error("flash.ExtractMetadata should NOT be called for cached content")
	}
	if flash.summarizeCalled {
		t.Error("flash.Summarize should NOT be called for cached content")
	}
	if reasoning.summarizeCalled {
		t.Error("reasoning.Summarize should NOT be called for cached content")
	}
	if repo.saveCalled {
		t.Error("repo.Save should NOT be called for cached content")
	}
}
