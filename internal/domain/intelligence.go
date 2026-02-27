package domain

import (
	"context"
	"time"
)

// Priority controls pipeline routing for synthesis jobs.
type Priority int

const (
	// PriorityLow forces the Flash tier for all passes, ignoring high-signal markers.
	PriorityLow Priority = iota
	// PriorityStandard routes to the Flash tier for all passes unless high-signal markers are found.
	PriorityStandard
	// PriorityHigh forces the Reasoning tier for the distillation pass.
	PriorityHigh
)

// Citation references a specific location in source content.
type Citation struct {
	Label   string `json:"label"`   // e.g. "12:34" or "Section 3.2"
	Context string `json:"context"` // What was said/written at that location
}

// Summary is the structured output of the intelligence pipeline.
type Summary struct {
	WhyItMatters string     `json:"why_it_matters"` // 1-sentence impact statement
	Teaser       string     `json:"teaser"`         // Hook for mobile app
	Citations    []Citation `json:"citations"`
	TechStack    []string   `json:"tech_stack"`   // e.g. ["Golang", "Raft"]
	HighSignal   bool       `json:"high_signal"`  // Contains repo/paper/benchmark?
	Distillation string     `json:"distillation"` // Full technical brief (markdown)
}

// ContextParams controls pipeline routing and content hints.
type ContextParams struct {
	Priority    Priority `json:"priority"`
	ContentType string   `json:"content_type"` // "video_transcript", "article", "thread"
}

// LLMUsage tracks token consumption for a single LLM call.
type LLMUsage struct {
	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Latency          time.Duration `json:"latency"`
}

// SynthesisResult is the combined output of a full pipeline run.
type SynthesisResult struct {
	Summary *Summary   `json:"summary,omitempty"`
	Usage   []LLMUsage `json:"usage"`
	Cached  bool       `json:"cached"` // True if result came from idempotency check
}

// Summarizer is the provider-agnostic LLM interface.
// Implementations wrap specific LLM SDKs (Gemini, Claude, etc.).
type Summarizer interface {
	// Summarize generates a full technical brief from raw content.
	Summarize(ctx context.Context, content string, params ContextParams) (*Summary, *LLMUsage, error)

	// ExtractMetadata identifies tech stack tags and high-signal markers.
	// Returns: (techTags, isHighSignal, usage, error).
	ExtractMetadata(ctx context.Context, content string) ([]string, bool, *LLMUsage, error)
}
