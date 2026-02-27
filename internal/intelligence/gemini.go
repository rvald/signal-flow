package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence/prompts"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// GeminiSummarizer implements domain.Summarizer using the Google Gemini API.
type GeminiSummarizer struct {
	client  *genai.Client
	model   string
	limiter *rate.Limiter
}

// NewGeminiSummarizer creates a GeminiSummarizer using the given API key and model.
// The model string should be a valid Gemini model name (e.g. "gemini-2.0-flash").
func NewGeminiSummarizer(ctx context.Context, apiKey, model string) (*GeminiSummarizer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("intelligence: create gemini client: %w", err)
	}

	// 5 requests per minute = 1 request every 12 seconds
	limit := rate.Every(12 * time.Second)
	limiter := rate.NewLimiter(limit, 1)

	return &GeminiSummarizer{
		client:  client,
		model:   model,
		limiter: limiter,
	}, nil
}

// Summarize generates a full technical brief using the Gemini model.
func (g *GeminiSummarizer) Summarize(ctx context.Context, content string, params domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, nil, fmt.Errorf("intelligence: gemini rate limiter: %w", err)
	}

	start := time.Now()

	userPrompt := prompts.FormatDistillationUserPrompt(content, "")
	resp, err := g.client.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{genai.NewContentFromText(userPrompt, "user")},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(prompts.DistillationSystemPrompt, "system"),
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: gemini summarize: %w", err)
	}

	latency := time.Since(start)

	text := resp.Text()
	summary, err := parseSummaryJSON(text)
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: parse gemini response: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            g.model,
		PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
		CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
		Latency:          latency,
	}

	return summary, usage, nil
}

// ExtractMetadata identifies tech stack tags and high-signal markers using the Gemini model.
func (g *GeminiSummarizer) ExtractMetadata(ctx context.Context, content string) ([]string, bool, *domain.LLMUsage, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: gemini rate limiter: %w", err)
	}

	start := time.Now()

	userPrompt := prompts.FormatAnalysisUserPrompt(content)
	resp, err := g.client.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{genai.NewContentFromText(userPrompt, "user")},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(prompts.AnalysisSystemPrompt, "system"),
		},
	)
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: gemini extract: %w", err)
	}

	latency := time.Since(start)

	text := resp.Text()
	tags, highSig, err := parseAnalysisJSON(text)
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: parse gemini analysis: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            g.model,
		PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
		CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
		Latency:          latency,
	}

	return tags, highSig, usage, nil
}

// --- JSON Parsing Helpers ---

// analysisResponse matches the JSON output from the analysis prompt.
type analysisResponse struct {
	TechStack  []string `json:"tech_stack"`
	HighSignal bool     `json:"high_signal"`
}

// summaryResponse matches the JSON output from the distillation prompt.
type summaryResponse struct {
	WhyItMatters string `json:"why_it_matters"`
	Teaser       string `json:"teaser"`
	Citations    []struct {
		Label   string `json:"label"`
		Context string `json:"context"`
	} `json:"citations"`
	Distillation string `json:"distillation"`
}

func parseAnalysisJSON(text string) ([]string, bool, error) {
	text = cleanJSON(text)
	var resp analysisResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, false, fmt.Errorf("unmarshal analysis: %w (raw: %s)", err, text)
	}
	return resp.TechStack, resp.HighSignal, nil
}

func parseSummaryJSON(text string) (*domain.Summary, error) {
	text = cleanJSON(text)
	var resp summaryResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal summary: %w (raw: %s)", err, text)
	}

	citations := make([]domain.Citation, len(resp.Citations))
	for i, c := range resp.Citations {
		citations[i] = domain.Citation{Label: c.Label, Context: c.Context}
	}

	return &domain.Summary{
		WhyItMatters: resp.WhyItMatters,
		Teaser:       resp.Teaser,
		Citations:    citations,
		Distillation: resp.Distillation,
	}, nil
}

// cleanJSON strips markdown code fences that LLMs sometimes wrap around JSON responses.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
