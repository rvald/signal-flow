package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DigestSummary is the structured content delivered by the pipeline's notify phase.
type DigestSummary struct {
	Signals     []DigestSignal `json:"signals"`
	GeneratedAt time.Time      `json:"generated_at"`
	SignalCount int            `json:"signal_count"`
	Sources     []string       `json:"sources"`
}

// DigestSignal is a single signal in the digest, with its synthesis results.
type DigestSignal struct {
	Title        string `json:"title"`
	SourceURL    string `json:"source_url"`
	Provider     string `json:"provider"`
	Teaser       string `json:"teaser,omitempty"`
	Distillation string `json:"distillation,omitempty"`
}

// Notifier delivers a digest summary to an output channel.
type Notifier interface {
	Notify(ctx context.Context, summary *DigestSummary) error
}

// SlackNotifier sends digest summaries to a Slack channel via Incoming Webhook.
type SlackNotifier struct {
	WebhookURL string
	Channel    string
	Client     *http.Client
}

// NewSlackNotifier creates a SlackNotifier with the given webhook URL and channel.
func NewSlackNotifier(webhookURL, channel string) *SlackNotifier {
	return &SlackNotifier{
		WebhookURL: webhookURL,
		Channel:    channel,
		Client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify formats the digest as Slack Block Kit blocks and POSTs to the webhook.
func (s *SlackNotifier) Notify(ctx context.Context, summary *DigestSummary) error {
	blocks := s.buildBlocks(summary)

	payload := map[string]any{
		"blocks": blocks,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// buildBlocks constructs Slack Block Kit blocks from the digest summary.
func (s *SlackNotifier) buildBlocks(summary *DigestSummary) []map[string]any {
	var blocks []map[string]any

	// Header block.
	dateStr := summary.GeneratedAt.Format("Jan 2, 2006")
	blocks = append(blocks, map[string]any{
		"type": "header",
		"text": map[string]any{
			"type": "plain_text",
			"text": fmt.Sprintf("📡 Signal-Flow Digest — %s", dateStr),
		},
	})

	if summary.SignalCount == 0 {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": "No new signals found in this run.",
			},
		})
		return blocks
	}

	// Stats context block.
	sources := ""
	for i, src := range summary.Sources {
		if i > 0 {
			sources += ", "
		}
		sources += src
	}
	blocks = append(blocks, map[string]any{
		"type": "context",
		"elements": []map[string]any{
			{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%d signals* from %s", summary.SignalCount, sources),
			},
		},
	})

	// Divider.
	blocks = append(blocks, map[string]any{"type": "divider"})

	// Signal sections (max 10 to stay within Slack limits).
	limit := len(summary.Signals)
	if limit > 10 {
		limit = 10
	}
	for _, sig := range summary.Signals[:limit] {
		text := fmt.Sprintf("*<%s|%s>*", sig.SourceURL, sig.Title)
		if sig.Teaser != "" {
			text += "\n" + sig.Teaser
		}
		text += fmt.Sprintf("\n_%s_", sig.Provider)

		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": text,
			},
		})
	}

	if len(summary.Signals) > 10 {
		blocks = append(blocks, map[string]any{
			"type": "context",
			"elements": []map[string]any{
				{
					"type": "mrkdwn",
					"text": fmt.Sprintf("_...and %d more signals_", len(summary.Signals)-10),
				},
			},
		})
	}

	return blocks
}
