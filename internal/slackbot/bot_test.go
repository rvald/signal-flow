package slackbot_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rvald/signal-flow/internal/agent"
	"github.com/rvald/signal-flow/internal/agent/tools"
	"github.com/rvald/signal-flow/internal/slackbot"
)

// mockLLM for agent construction in tests.
type mockLLM struct {
	response agent.Response
}

func (m *mockLLM) Chat(_ context.Context, _ []agent.Message, _ []map[string]any) (*agent.Response, error) {
	return &m.response, nil
}

func testAgent(response string) *agent.Agent {
	llm := &mockLLM{response: agent.Response{Content: response}}
	reg := tools.NewRegistry()
	return agent.New(llm, reg, "test bot", 8192)
}

// =============================================================================
// Test_HandleMessage_SimpleQuery
// Handler invokes agent and returns the text reply.
// =============================================================================

func Test_HandleMessage_SimpleQuery(t *testing.T) {
	h := slackbot.NewHandler(testAgent("Here are your signals."), agent.NewSessionStore(1000))

	reply, err := h.HandleMessage(context.Background(), "user1", "what are my signals?")
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if reply != "Here are your signals." {
		t.Errorf("reply = %q", reply)
	}
}

// =============================================================================
// Test_HandleMessage_EmptyMessage
// Handler ignores empty messages.
// =============================================================================

func Test_HandleMessage_EmptyMessage(t *testing.T) {
	h := slackbot.NewHandler(testAgent("should not be called"), agent.NewSessionStore(1000))

	reply, err := h.HandleMessage(context.Background(), "user1", "")
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if reply != "" {
		t.Errorf("reply = %q, want empty for empty input", reply)
	}
}

// =============================================================================
// Test_HandleMessage_AgentError
// Handler returns a user-friendly error message when the agent fails.
// =============================================================================

func Test_HandleMessage_AgentError(t *testing.T) {
	failLLM := &failingLLM{}
	reg := tools.NewRegistry()
	a := agent.New(failLLM, reg, "test", 8192)
	h := slackbot.NewHandler(a, agent.NewSessionStore(1000))

	reply, err := h.HandleMessage(context.Background(), "user1", "hello")
	if err != nil {
		t.Fatalf("HandleMessage should not return error, got: %v", err)
	}
	if reply == "" {
		t.Fatal("expected a fallback error message, got empty string")
	}
}

type failingLLM struct{}

func (f *failingLLM) Chat(_ context.Context, _ []agent.Message, _ []map[string]any) (*agent.Response, error) {
	return nil, fmt.Errorf("LLM is down")
}

// =============================================================================
// Test_FormatBlocks_TextOnly
// FormatBlocks creates a simple text block for plain text.
// =============================================================================

func Test_FormatBlocks_TextOnly(t *testing.T) {
	blocks := slackbot.FormatBlocks("Hello, world!")
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// =============================================================================
// Test_FormatBlocks_Empty
// FormatBlocks returns empty for empty text.
// =============================================================================

func Test_FormatBlocks_Empty(t *testing.T) {
	blocks := slackbot.FormatBlocks("")
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty text, got %d", len(blocks))
	}
}
