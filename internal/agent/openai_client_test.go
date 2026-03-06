package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// mockRoundTripper intercepts HTTP requests and returns a pre-configured response.
type mockRoundTripper struct {
	Response *http.Response
	Err      error
	ReqChan  chan *http.Request
	BodyChan chan []byte
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.ReqChan != nil {
		m.ReqChan <- req
	}
	if m.BodyChan != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(b))
		m.BodyChan <- b
	}
	return m.Response, m.Err
}

func TestOpenAILLMClient_Chat(t *testing.T) {
	mockRespBody := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello, world!",
				"tool_calls": [{
					"id": "call_abc123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\":\"San Francisco\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {
			"prompt_tokens": 9,
			"completion_tokens": 12,
			"total_tokens": 21
		}
	}`

	header := make(http.Header)
	header.Set("Content-Type", "application/json")

	rt := &mockRoundTripper{
		ReqChan:  make(chan *http.Request, 1),
		BodyChan: make(chan []byte, 1),
		Response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(mockRespBody)),
			Header:     header,
		},
	}

	httpClient := &http.Client{Transport: rt}
	sdkClient := openai.NewClient(option.WithHTTPClient(httpClient), option.WithAPIKey("test-key"))

	client := &OpenAILLMClient{
		client: &sdkClient,
		model:  "gpt-4o",
	}

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "What is the weather?"},
	}

	toolDefs := []map[string]any{
		{
			"name":        "get_weather",
			"description": "Get the current weather",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	resp, err := client.Chat(context.Background(), messages, toolDefs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response parsing
	if resp.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got '%s'", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Args["location"] != "San Francisco" {
		t.Errorf("expected location 'San Francisco', got '%v'", resp.ToolCalls[0].Args["location"])
	}

	// Verify request payload payload
	reqBodyBytes := <-rt.BodyChan
	var reqPayload map[string]any
	if err := json.Unmarshal(reqBodyBytes, &reqPayload); err != nil {
		t.Fatalf("failed to parse request payload: %v", err)
	}

	// Check messages format
	msgs, ok := reqPayload["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %v", msgs)
	}

	// Check tools formatting
	tools, ok := reqPayload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in payload, got %v", tools)
	}
}
