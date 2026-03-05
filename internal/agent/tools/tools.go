// Package tools defines typed tool definitions that the LLM agent can invoke.
// Each tool wraps a service operation as a callable function with a typed schema.
package tools

import "context"

// Tool defines a callable operation the agent can invoke.
type Tool struct {
	Name        string
	Description string
	Parameters  []Param
	Execute     func(ctx context.Context, args map[string]any) (*Result, error)
}

// Param defines a single parameter for a tool.
type Param struct {
	Name        string
	Type        string // "string", "integer", "boolean"
	Description string
	Required    bool
	Enum        []string // optional allowed values
}

// Result is the output of a tool execution.
type Result struct {
	Output string         // human-readable summary for the LLM
	Data   map[string]any // structured data
}

// Registry holds available tools.
type Registry struct {
	tools map[string]Tool
	order []string // preserves registration order
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	if _, exists := r.tools[tool.Name]; !exists {
		r.order = append(r.order, tool.Name)
	}
	r.tools[tool.Name] = tool
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool in registration order.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name])
	}
	return out
}

// Schema generates the LLM function-calling schema for all registered tools.
// Each entry matches the OpenAI/Gemini function-calling format.
func (r *Registry) Schema() []map[string]any {
	out := make([]map[string]any, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]

		properties := map[string]any{}
		var required []string

		for _, p := range tool.Parameters {
			prop := map[string]any{
				"type":        p.Type,
				"description": p.Description,
			}
			if len(p.Enum) > 0 {
				prop["enum"] = p.Enum
			}
			properties[p.Name] = prop

			if p.Required {
				required = append(required, p.Name)
			}
		}

		params := map[string]any{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			params["required"] = required
		}

		out = append(out, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  params,
		})
	}
	return out
}
