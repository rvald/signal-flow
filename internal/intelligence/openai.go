package intelligence

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence/prompts"
)

// OpenAISummarizer implements domain.Summarizer using the OpenAI Chat API.
type OpenAISummarizer struct {
	client *openai.Client
	model  string
}

// NewOpenAISummarizer creates an OpenAISummarizer using the given API key and model.
// The model string should be a valid OpenAI model (e.g. "gpt-4o", "o3-mini").
func NewOpenAISummarizer(apiKey string, model string) *OpenAISummarizer {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &OpenAISummarizer{client: &client, model: model}
}

// NewOllamaSummarizer creates an OpenAISummarizer configured to talk to a local Ollama instance.
// It uses the OpenAI API compatibility layer provided by Ollama.
func NewOllamaSummarizer(baseURL string, model string) *OpenAISummarizer {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1/"
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("ollama"), // API key is required by the SDK but ignored by Ollama
	)

	return &OpenAISummarizer{client: &client, model: model}
}

// Summarize generates a full technical brief using the OpenAI Chat logic.
func (o *OpenAISummarizer) Summarize(ctx context.Context, content string, params domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatDistillationUserPrompt(content, "")
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(prompts.DistillationSystemPrompt),
			openai.UserMessage(userPrompt),
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: openai summarize: %w", err)
	}

	latency := time.Since(start)

	var text string
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}

	summary, err := parseSummaryJSON(text)
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: parse openai response: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            o.model,
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
		Latency:          latency,
	}

	return summary, usage, nil
}

// ExtractMetadata identifies tech stack tags and high-signal markers using the OpenAI Chat Completions API.
func (o *OpenAISummarizer) ExtractMetadata(ctx context.Context, content string) ([]string, bool, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatAnalysisUserPrompt(content)
	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(prompts.AnalysisSystemPrompt),
			openai.UserMessage(userPrompt),
		},
	})
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: openai extract: %w", err)
	}

	latency := time.Since(start)

	var text string
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}

	tags, highSig, err := parseAnalysisJSON(text)
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: parse openai analysis: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            o.model,
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
		Latency:          latency,
	}

	return tags, highSig, usage, nil
}
