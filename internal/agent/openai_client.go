package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAILLMClient implements LLMClient using the OpenAI API
// with function calling support. It can also be configured to talk to Ollama.
type OpenAILLMClient struct {
	client *openai.Client
	model  string
}

// NewOpenAILLMClient creates an OpenAI-backed LLMClient.
// Reads OPENAI_API_KEY from the environment.
func NewOpenAILLMClient(ctx context.Context, model string) (*OpenAILLMClient, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY env var is required")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey))

	if model == "" {
		model = "gpt-4o"
	}

	return &OpenAILLMClient{
		client: &client,
		model:  model,
	}, nil
}

// NewOllamaLLMClient creates an OpenAI-compatible LLMClient pointing to a local Ollama instance.
func NewOllamaLLMClient(ctx context.Context, model string) (*OpenAILLMClient, error) {
	baseURL := os.Getenv("OLLAMA_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1/"
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("ollama"), // Required by SDK, ignored by Ollama
	)

	if model == "" {
		model = "deepseek-r1:8b"
	}

	return &OpenAILLMClient{
		client: &client,
		model:  model,
	}, nil
}

// Chat sends messages to the OpenAI/Ollama API with tool definitions and returns the response.
func (o *OpenAILLMClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*Response, error) {
	var chatMsgs []openai.ChatCompletionMessageParamUnion

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			chatMsgs = append(chatMsgs, openai.SystemMessage(msg.Content))
		case "user":
			chatMsgs = append(chatMsgs, openai.UserMessage(msg.Content))
		case "assistant":
			chatMsgs = append(chatMsgs, openai.AssistantMessage(msg.Content))
		case "tool":
			chatMsgs = append(chatMsgs, openai.ToolMessage(msg.ToolCallID, msg.Content))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(o.model),
		Messages: chatMsgs,
	}

	// Build tools if supplied
	if len(toolDefs) > 0 {
		var tools []openai.ChatCompletionToolUnionParam
		for _, td := range toolDefs {
			name, _ := td["name"].(string)
			desc, _ := td["description"].(string)

			// Extract parameters, safely defaulting to empty object if none
			var paramsMap interface{} = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
			if p, ok := td["parameters"]; ok {
				paramsMap = p
			}

			tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters:  shared.FunctionParameters(paramsMap.(map[string]interface{})),
			}))
		}
		params.Tools = tools
	}

	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &Response{Content: ""}, nil
	}

	choice := resp.Choices[0]
	result := &Response{
		Content: choice.Message.Content,
	}

	if len(choice.Message.ToolCalls) > 0 {
		var mappedCalls []ToolCall
		for _, tc := range choice.Message.ToolCalls {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool args: %w", err)
			}
			mappedCalls = append(mappedCalls, ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			})
		}
		result.ToolCalls = mappedCalls
	}

	return result, nil
}
