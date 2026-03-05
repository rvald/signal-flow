package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rvald/signal-flow/internal/agent"
	"github.com/rvald/signal-flow/internal/agent/tools"
)

// mockLLM is a test double for agent.LLMClient.
type mockLLM struct {
	responses []agent.Response // returned in order per call
	callIdx   int
}

func (m *mockLLM) Chat(_ context.Context, _ []agent.Message, _ []map[string]any) (*agent.Response, error) {
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("unexpected Chat call #%d", m.callIdx)
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return &resp, nil
}

func echoToolRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name:        "echo",
		Description: "Echoes input",
		Parameters:  []tools.Param{{Name: "text", Type: "string", Required: true}},
		Execute: func(_ context.Context, args map[string]any) (*tools.Result, error) {
			return &tools.Result{
				Output: fmt.Sprintf("echo: %s", args["text"]),
				Data:   map[string]any{"echoed": args["text"]},
			}, nil
		},
	})
	return reg
}

// =============================================================================
// Test_Agent_TextResponse
// LLM responds with text only, no tool calls.
// =============================================================================

func Test_Agent_TextResponse(t *testing.T) {
	llm := &mockLLM{responses: []agent.Response{
		{Content: "Hello! How can I help?"},
	}}

	a := agent.New(llm, echoToolRegistry(), "You are a test bot.", 8192)
	session := agent.NewSession("user1", 1000)

	reply, err := a.Handle(context.Background(), session, "hi")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "Hello! How can I help?" {
		t.Errorf("reply = %q", reply)
	}
}

// =============================================================================
// Test_Agent_SingleToolCall
// LLM requests a tool call, agent executes it, LLM responds with final text.
// =============================================================================

func Test_Agent_SingleToolCall(t *testing.T) {
	llm := &mockLLM{responses: []agent.Response{
		// First call: LLM requests tool
		{ToolCalls: []agent.ToolCall{{ID: "call_1", Name: "echo", Args: map[string]any{"text": "hello"}}}},
		// Second call: LLM responds after seeing tool result
		{Content: "The echo returned: hello"},
	}}

	a := agent.New(llm, echoToolRegistry(), "You are a test bot.", 8192)
	session := agent.NewSession("user1", 1000)

	reply, err := a.Handle(context.Background(), session, "echo hello")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "The echo returned: hello" {
		t.Errorf("reply = %q", reply)
	}
}

// =============================================================================
// Test_Agent_ToolError_GracefulDegradation
// Tool returns error, agent should tell the LLM about the error gracefully.
// =============================================================================

func Test_Agent_ToolError_GracefulDegradation(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "fail",
		Execute: func(_ context.Context, _ map[string]any) (*tools.Result, error) {
			return nil, fmt.Errorf("connection refused")
		},
	})

	llm := &mockLLM{responses: []agent.Response{
		{ToolCalls: []agent.ToolCall{{ID: "call_1", Name: "fail", Args: nil}}},
		{Content: "Sorry, the operation failed: connection refused"},
	}}

	a := agent.New(llm, reg, "You are a test bot.", 8192)
	session := agent.NewSession("user1", 1000)

	reply, err := a.Handle(context.Background(), session, "do the thing")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "Sorry, the operation failed: connection refused" {
		t.Errorf("reply = %q", reply)
	}
}

// =============================================================================
// Test_Agent_UnknownTool
// LLM requests an unregistered tool, agent responds with error message to LLM.
// =============================================================================

func Test_Agent_UnknownTool(t *testing.T) {
	llm := &mockLLM{responses: []agent.Response{
		{ToolCalls: []agent.ToolCall{{ID: "call_1", Name: "nonexistent", Args: nil}}},
		{Content: "I tried to use a tool that doesn't exist."},
	}}

	a := agent.New(llm, echoToolRegistry(), "You are a test bot.", 8192)
	session := agent.NewSession("user1", 1000)

	reply, err := a.Handle(context.Background(), session, "use nonexistent tool")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reply != "I tried to use a tool that doesn't exist." {
		t.Errorf("reply = %q", reply)
	}
}
