package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// GeminiLLMClient implements LLMClient using the Google Gemini API
// with function calling support.
type GeminiLLMClient struct {
	client *genai.Client
	model  string
}

// NewGeminiLLMClient creates a Gemini-backed LLMClient.
// Reads GEMINI_API_KEY from the environment.
func NewGeminiLLMClient(ctx context.Context) (*GeminiLLMClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY env var is required")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}

	return &GeminiLLMClient{
		client: client,
		model:  "gemini-2.5-flash",
	}, nil
}

// Chat sends messages to Gemini with tool definitions and returns the response.
func (g *GeminiLLMClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*Response, error) {
	// Convert messages to Gemini content format.
	var contents []*genai.Content
	var systemInstruction *genai.Content

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemInstruction = genai.NewContentFromText(msg.Content, "user")
		case "user":
			contents = append(contents, genai.NewContentFromText(msg.Content, "user"))
		case "assistant":
			contents = append(contents, genai.NewContentFromText(msg.Content, "model"))
		case "tool":
			// Tool results go as user messages with function response parts.
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{FunctionResponse: &genai.FunctionResponse{
						Name: msg.ToolName,
						Response: map[string]any{
							"result": msg.Content,
						},
					}},
				},
			})
		}
	}

	// Convert tool definitions to Gemini function declarations.
	var geminiTools []*genai.Tool
	if len(toolDefs) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, td := range toolDefs {
			decl := &genai.FunctionDeclaration{
				Name:        td["name"].(string),
				Description: td["description"].(string),
			}
			if params, ok := td["parameters"].(map[string]any); ok {
				decl.Parameters = convertToGeminiSchema(params)
			}
			decls = append(decls, decl)
		}
		geminiTools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	// Build config.
	config := &genai.GenerateContentConfig{
		Tools: geminiTools,
	}
	if systemInstruction != nil {
		config.SystemInstruction = systemInstruction
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini generate: %w", err)
	}

	return parseGeminiResponse(resp)
}

// parseGeminiResponse converts a Gemini response to our Response type.
func parseGeminiResponse(resp *genai.GenerateContentResponse) (*Response, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return &Response{Content: ""}, nil
	}

	var toolCalls []ToolCall
	var textParts []string

	for _, part := range resp.Candidates[0].Content.Parts {
		if part.FunctionCall != nil {
			toolCalls = append(toolCalls, ToolCall{
				ID:   part.FunctionCall.Name, // Gemini doesn't use separate IDs
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			})
		}
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}

	result := &Response{}
	if len(toolCalls) > 0 {
		result.ToolCalls = toolCalls
	}
	if len(textParts) > 0 {
		result.Content = textParts[0]
		for _, t := range textParts[1:] {
			result.Content += t
		}
	}

	return result, nil
}

// convertToGeminiSchema converts our tool parameter schema to Gemini's Schema type.
func convertToGeminiSchema(params map[string]any) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	if props, ok := params["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for name, propRaw := range props {
			prop, ok := propRaw.(map[string]any)
			if !ok {
				continue
			}
			propSchema := &genai.Schema{}

			if t, ok := prop["type"].(string); ok {
				switch t {
				case "string":
					propSchema.Type = genai.TypeString
				case "integer":
					propSchema.Type = genai.TypeInteger
				case "boolean":
					propSchema.Type = genai.TypeBoolean
				}
			}
			if d, ok := prop["description"].(string); ok {
				propSchema.Description = d
			}
			if enumVals, ok := prop["enum"].([]string); ok {
				propSchema.Enum = enumVals
			}

			schema.Properties[name] = propSchema
		}
	}

	if required, ok := params["required"].([]string); ok {
		schema.Required = required
	}

	return schema
}

// marshalArgs converts function call args to a JSON string for logging.
func marshalArgs(args map[string]any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}
