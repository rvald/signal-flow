package slackbot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/rvald/signal-flow/internal/agent"
)

// Handler processes incoming Slack messages by routing them to the Agent.
// This is separated from Bot so the core logic is testable without Slack SDK.
type Handler struct {
	agent    *agent.Agent
	sessions *agent.SessionStore
	logger   *slog.Logger
}

// NewHandler creates a Handler backed by the given agent and session store.
func NewHandler(a *agent.Agent, sessions *agent.SessionStore) *Handler {
	return &Handler{
		agent:    a,
		sessions: sessions,
		logger:   slog.Default(),
	}
}

// HandleMessage processes a user message and returns the bot's reply.
// Returns empty string for empty/whitespace-only messages.
// On agent error, returns a user-friendly fallback message instead of an error.
func (h *Handler) HandleMessage(ctx context.Context, userID, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}

	session := h.sessions.GetOrCreate(userID)

	reply, err := h.agent.Handle(ctx, session, text)
	if err != nil {
		h.logger.Error("agent error", "user_id", userID, "error", err)
		return "I'm having trouble processing your request. Try running `signal-flow pipeline run` directly.", nil
	}

	return reply, nil
}
