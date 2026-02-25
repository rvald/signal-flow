package intelligence

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence/prompts"
)

// ClaudeSummarizer implements domain.Summarizer using the Anthropic Claude API.
type ClaudeSummarizer struct {
	client *anthropic.Client
	model  anthropic.Model
}

// NewClaudeSummarizer creates a ClaudeSummarizer using the given API key and model.
// The model string should be a valid Anthropic model (e.g. "claude-sonnet-4-5-20250514").
func NewClaudeSummarizer(apiKey string, model anthropic.Model) *ClaudeSummarizer {
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &ClaudeSummarizer{client: &client, model: model}
}

// Summarize generates a full technical brief using the Claude model.
func (c *ClaudeSummarizer) Summarize(ctx context.Context, content string, params domain.ContextParams) (*domain.Summary, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatDistillationUserPrompt(content, "")
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: prompts.DistillationSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: claude summarize: %w", err)
	}

	latency := time.Since(start)

	// Extract text from response content blocks.
	text := extractClaudeText(resp)
	summary, err := parseSummaryJSON(text)
	if err != nil {
		return nil, nil, fmt.Errorf("intelligence: parse claude response: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            string(c.model),
		PromptTokens:     int(resp.Usage.InputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		Latency:          latency,
	}

	return summary, usage, nil
}

// ExtractMetadata identifies tech stack tags and high-signal markers using the Claude model.
func (c *ClaudeSummarizer) ExtractMetadata(ctx context.Context, content string) ([]string, bool, *domain.LLMUsage, error) {
	start := time.Now()

	userPrompt := prompts.FormatAnalysisUserPrompt(content)
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: prompts.AnalysisSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: claude extract: %w", err)
	}

	latency := time.Since(start)

	text := extractClaudeText(resp)
	tags, highSig, err := parseAnalysisJSON(text)
	if err != nil {
		return nil, false, nil, fmt.Errorf("intelligence: parse claude analysis: %w", err)
	}

	usage := &domain.LLMUsage{
		Model:            string(c.model),
		PromptTokens:     int(resp.Usage.InputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		Latency:          latency,
	}

	return tags, highSig, usage, nil
}

// extractClaudeText pulls the text content from Claude's response content blocks.
func extractClaudeText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}
