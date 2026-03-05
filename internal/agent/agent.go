package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rvald/signal-flow/internal/agent/tools"
)

// LLMClient is the interface for LLM providers that support function calling.
type LLMClient interface {
	Chat(ctx context.Context, messages []Message, tools []map[string]any) (*Response, error)
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// Response is the LLM's response to a Chat call.
type Response struct {
	Content   string     // text response (empty if tool calls are present)
	ToolCalls []ToolCall // tool invocations requested by the LLM
}

// Agent interprets user messages and dispatches tool calls via the LLM.
type Agent struct {
	llm          LLMClient
	tools        *tools.Registry
	systemPrompt string
	tokenBudget  int
	logger       *slog.Logger
}

// New creates an Agent with the given LLM client, tool registry, and config.
func New(llm LLMClient, toolReg *tools.Registry, systemPrompt string, tokenBudget int) *Agent {
	return &Agent{
		llm:          llm,
		tools:        toolReg,
		systemPrompt: systemPrompt,
		tokenBudget:  tokenBudget,
		logger:       slog.Default(),
	}
}

// maxToolRounds prevents infinite tool-call loops.
const maxToolRounds = 10

// Handle processes a user message through the agent loop:
// 1. Add user message to session
// 2. Build context (system prompt + windowed history + tool schemas)
// 3. Send to LLM
// 4. If LLM returns tool calls → execute → feed results back → loop
// 5. If LLM returns text → return as reply
func (a *Agent) Handle(ctx context.Context, session *Session, userMsg string) (string, error) {
	session.AddMessage(Message{Role: "user", Content: userMsg})

	schema := a.tools.Schema()

	for round := 0; round < maxToolRounds; round++ {
		// Build messages: system prompt + windowed conversation history.
		msgs := make([]Message, 0, len(session.Window())+1)
		msgs = append(msgs, Message{Role: "system", Content: a.systemPrompt})
		msgs = append(msgs, session.Window()...)

		resp, err := a.llm.Chat(ctx, msgs, schema)
		if err != nil {
			return "", fmt.Errorf("llm chat: %w", err)
		}

		// If LLM returned text (no tool calls), we're done.
		if len(resp.ToolCalls) == 0 {
			session.AddMessage(Message{Role: "assistant", Content: resp.Content})
			return resp.Content, nil
		}

		// Execute each tool call and feed results back.
		for _, tc := range resp.ToolCalls {
			tool, ok := a.tools.Get(tc.Name)
			if !ok {
				// Unknown tool — report error to the LLM.
				session.AddMessage(Message{
					Role:       "tool",
					Content:    fmt.Sprintf("error: unknown tool '%s'", tc.Name),
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
				continue
			}

			result, err := tool.Execute(ctx, tc.Args)
			if err != nil {
				// Tool error — report to LLM for graceful degradation.
				session.AddMessage(Message{
					Role:       "tool",
					Content:    fmt.Sprintf("error: %s", err),
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
				continue
			}

			session.AddMessage(Message{
				Role:       "tool",
				Content:    result.Output,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			})
		}
	}

	return "", fmt.Errorf("agent exceeded maximum tool rounds (%d)", maxToolRounds)
}
