package intelligence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
)

// ErrSignalNotFound is returned when no signal exists for a given source URL and tenant.
var ErrSignalNotFound = errors.New("intelligence: signal not found")

// SynthesizerService orchestrates the two-pass intelligence pipeline.
// Pass 1 (Flash): Extract metadata and determine high-signal markers.
// Pass 2 (Flash or Reasoning): Generate the full technical brief.
type SynthesizerService struct {
	flash     domain.Summarizer
	reasoning domain.Summarizer
	signals   domain.SignalRepository
	tracker   *UsageTracker
}

// NewSynthesizerService creates a SynthesizerService with the given flash and reasoning
// summarizers and a signal repository for idempotency checks and persistence.
func NewSynthesizerService(flash, reasoning domain.Summarizer, signals domain.SignalRepository) *SynthesizerService {
	return &SynthesizerService{
		flash:     flash,
		reasoning: reasoning,
		signals:   signals,
		tracker:   NewUsageTracker(),
	}
}

// Synthesize runs the intelligence pipeline for the given content.
// It checks for existing distillations first (idempotency), then runs the two-pass flow.
func (s *SynthesizerService) Synthesize(
	ctx context.Context,
	tenantID uuid.UUID,
	sourceURL string,
	rawContent string,
	params domain.ContextParams,
) (*domain.SynthesisResult, error) {
	// --- Idempotency Check ---
	existing, err := s.signals.FindBySourceURL(ctx, tenantID, sourceURL)
	if err != nil && !errors.Is(err, ErrSignalNotFound) {
		return nil, err
	}
	if existing != nil && existing.Distillation != "" {
		return &domain.SynthesisResult{Cached: true}, nil
	}

	// --- Pass 1: Analysis (Flash Tier) ---
	techTags, isHighSignal, usage1, err := s.flash.ExtractMetadata(ctx, rawContent)
	if err != nil {
		return nil, err
	}
	s.tracker.Track(usage1)

	// --- Routing Decision ---
	var summarizer domain.Summarizer
	switch {
	case params.Priority == domain.PriorityLow:
		summarizer = s.flash
	case params.Priority == domain.PriorityHigh || isHighSignal:
		summarizer = s.reasoning
	default:
		summarizer = s.flash
	}

	// --- Pass 2: Distillation ---
	summary, usage2, err := summarizer.Summarize(ctx, rawContent, params)
	if err != nil {
		return nil, err
	}
	s.tracker.Track(usage2)

	// Merge analysis results into summary.
	summary.TechStack = techTags
	summary.HighSignal = isHighSignal

	// --- Persist ---
	signal := &domain.Signal{
		ID:           uuid.New(),
		TenantID:     tenantID,
		SourceURL:    sourceURL,
		Content:      rawContent,
		Distillation: summary.Distillation,
		Metadata: map[string]any{
			"tech_stack":     techTags,
			"high_signal":    isHighSignal,
			"why_it_matters": summary.WhyItMatters,
			"teaser":         summary.Teaser,
		},
		Scope: domain.ScopePrivate,
	}

	if err := s.signals.Save(ctx, signal); err != nil {
		return nil, err
	}

	return &domain.SynthesisResult{
		Summary: summary,
		Usage:   s.tracker.Entries(),
		Cached:  false,
	}, nil
}
