package slackbot

import (
	"context"
	"log/slog"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/rvald/signal-flow/internal/agent"
)

// Bot runs the Slack Socket Mode event loop, routing messages to the Handler.
type Bot struct {
	handler *Handler
	api     *slack.Client
	socket  *socketmode.Client
	botID   string
	logger  *slog.Logger
}

// BotConfig holds the required Slack tokens.
type BotConfig struct {
	AppToken string // xapp-... for Socket Mode
	BotToken string // xoxb-... for API calls
}

// NewBot creates a Bot with Socket Mode wiring.
func NewBot(cfg BotConfig, a *agent.Agent, sessions *agent.SessionStore) *Bot {
	api := slack.New(cfg.BotToken,
		slack.OptionAppLevelToken(cfg.AppToken),
	)
	socket := socketmode.New(api)

	return &Bot{
		handler: NewHandler(a, sessions),
		api:     api,
		socket:  socket,
		logger:  slog.Default(),
	}
}

// Start runs the Socket Mode event loop. Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	// Resolve bot user ID for ignoring own messages.
	authResp, err := b.api.AuthTest()
	if err != nil {
		return err
	}
	b.botID = authResp.UserID
	b.logger.Info("bot connected", "user_id", b.botID, "team", authResp.Team)

	go b.eventLoop(ctx)

	return b.socket.RunContext(ctx)
}

// eventLoop processes Socket Mode events.
func (b *Bot) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-b.socket.Events:
			if !ok {
				return
			}
			b.handleEvent(ctx, evt)
		}
	}
}

// handleEvent dispatches a single Socket Mode event.
func (b *Bot) handleEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		b.socket.Ack(*evt.Request)
		b.handleEventsAPI(ctx, eventsAPIEvent)
	}
}

// handleEventsAPI processes Events API payloads (messages, app_mentions, etc).
func (b *Bot) handleEventsAPI(ctx context.Context, event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Ignore bot's own messages and message edits.
			if ev.User == b.botID || ev.User == "" || ev.SubType != "" {
				return
			}

			b.logger.Info("message received",
				"user", ev.User,
				"channel", ev.Channel,
				"text_len", len(ev.Text),
			)

			reply, err := b.handler.HandleMessage(ctx, ev.User, ev.Text)
			if err != nil || reply == "" {
				return
			}

			blocks := FormatBlocks(reply)
			_, _, err = b.api.PostMessage(ev.Channel,
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionText(reply, false), // fallback for notifications
			)
			if err != nil {
				b.logger.Error("post message failed", "channel", ev.Channel, "error", err)
			}

		case *slackevents.AppMentionEvent:
			b.logger.Info("mention received",
				"user", ev.User,
				"channel", ev.Channel,
			)

			reply, err := b.handler.HandleMessage(ctx, ev.User, ev.Text)
			if err != nil || reply == "" {
				return
			}

			blocks := FormatBlocks(reply)
			_, _, err = b.api.PostMessage(ev.Channel,
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionText(reply, false),
			)
			if err != nil {
				b.logger.Error("post message failed", "channel", ev.Channel, "error", err)
			}
		}
	}
}
