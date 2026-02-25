package intelligence

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence/prompts"
)

// OpenAISummarizer implements domain.Summarizer using the OpenAI Responses API (v3).
type OpenAISummarizer struct {
	client *openai.Client
	model  shared.ResponsesModel
}

// NewOpenAISummarizer creates an OpenAISummarizer using the given API key and model.
// The model string should be a valid OpenAI model (e.g. "gpt-4o", "o3-mini").
func NewOpenAISummarizer(apiKey string, model shared.ResponsesModel) *OpenAISummarizer {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &OpenAISummarizer{client: &client, model: model}
}

// Summarize generates a full technical brief using the OpenAI Responses API.
func (o *OpenAISummarizer) Summarize(ctx context.Context, content string, params domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatDistillationUserPrompt(content, "")
	resp, err := o.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        o.model,
		Instructions: openai.String(prompts.DistillationSystemPrompt),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(userPrompt),
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: openai summarize: %w", err)
	}

	latency := time.Since(start)

	text := resp.OutputText()
	summary, err := parseSummaryJSON(text)
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: parse openai response: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            string(o.model),
		PromptTokens:     int(resp.Usage.InputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		Latency:          latency,
	}

	return summary, usage, nil
}

// ExtractMetadata identifies tech stack tags and high-signal markers using the OpenAI Responses API.
func (o *OpenAISummarizer) ExtractMetadata(ctx context.Context, content string) ([]string, bool, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatAnalysisUserPrompt(content)
	resp, err := o.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        o.model,
		Instructions: openai.String(prompts.AnalysisSystemPrompt),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(userPrompt),
		},
	})
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: openai extract: %w", err)
	}

	latency := time.Since(start)

	text := resp.OutputText()
	tags, highSig, err := parseAnalysisJSON(text)
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: parse openai analysis: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            string(o.model),
		PromptTokens:     int(resp.Usage.InputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		Latency:          latency,
	}

	return tags, highSig, usage, nil
}
