package slackbot

import (
	"github.com/slack-go/slack"
)

// FormatBlocks converts agent reply text into Slack Block Kit blocks.
// Returns nil for empty text.
func FormatBlocks(text string) []slack.Block {
	if text == "" {
		return nil
	}

	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, text, false, false),
			nil, nil,
		),
	}
}
