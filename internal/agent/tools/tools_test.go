package tools_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rvald/signal-flow/internal/agent/tools"
)

// =============================================================================
// Test_Registry_RegisterAndGet
// Register a tool, then retrieve it by name.
// =============================================================================

func Test_Registry_RegisterAndGet(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.Tool{
		Name:        "harvest",
		Description: "Fetch signals from a source",
		Execute: func(_ context.Context, _ map[string]any) (*tools.Result, error) {
			return &tools.Result{Output: "done"}, nil
		},
	})

	tool, ok := reg.Get("harvest")
	if !ok {
		t.Fatal("expected to find tool 'harvest'")
	}
	if tool.Name != "harvest" {
		t.Errorf("Name = %q, want 'harvest'", tool.Name)
	}
	if tool.Description != "Fetch signals from a source" {
		t.Errorf("Description = %q, want 'Fetch signals from a source'", tool.Description)
	}
}

// =============================================================================
// Test_Registry_Get_NotFound
// Get must return false for an unregistered tool.
// =============================================================================

func Test_Registry_Get_NotFound(t *testing.T) {
	reg := tools.NewRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for unregistered tool")
	}
}

// =============================================================================
// Test_Registry_All
// All must return every registered tool.
// =============================================================================

func Test_Registry_All(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.Tool{Name: "harvest", Description: "h"})
	reg.Register(tools.Tool{Name: "synthesize", Description: "s"})

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d tools, want 2", len(all))
	}

	names := map[string]bool{}
	for _, tool := range all {
		names[tool.Name] = true
	}
	if !names["harvest"] || !names["synthesize"] {
		t.Errorf("All() = %v, want harvest and synthesize", names)
	}
}

// =============================================================================
// Test_Registry_Schema
// Schema must produce a slice of maps matching the LLM function-calling format.
// =============================================================================

func Test_Registry_Schema(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.Tool{
		Name:        "harvest",
		Description: "Fetch signals",
		Parameters: []tools.Param{
			{Name: "source", Type: "string", Description: "Platform to harvest", Required: true, Enum: []string{"bluesky", "youtube"}},
		},
	})

	schema := reg.Schema()
	if len(schema) != 1 {
		t.Fatalf("Schema() returned %d entries, want 1", len(schema))
	}

	entry := schema[0]
	if entry["name"] != "harvest" {
		t.Errorf("schema name = %v, want 'harvest'", entry["name"])
	}
	if entry["description"] != "Fetch signals" {
		t.Errorf("schema description = %v, want 'Fetch signals'", entry["description"])
	}

	params, ok := entry["parameters"].(map[string]any)
	if !ok {
		t.Fatal("schema 'parameters' is not a map")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema 'properties' is not a map")
	}
	sourceProp, ok := props["source"].(map[string]any)
	if !ok {
		t.Fatal("schema missing 'source' property")
	}
	if sourceProp["type"] != "string" {
		t.Errorf("source type = %v, want 'string'", sourceProp["type"])
	}
}

// =============================================================================
// Test_Tool_Execute
// A tool's Execute function is called with args and returns a Result.
// =============================================================================

func Test_Tool_Execute(t *testing.T) {
	tool := tools.Tool{
		Name: "echo",
		Execute: func(_ context.Context, args map[string]any) (*tools.Result, error) {
			msg := args["message"].(string)
			return &tools.Result{
				Output: "echo: " + msg,
				Data:   map[string]any{"echoed": msg},
			}, nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "echo: hello" {
		t.Errorf("Output = %q, want 'echo: hello'", result.Output)
	}
	if result.Data["echoed"] != "hello" {
		t.Errorf("Data[echoed] = %v, want 'hello'", result.Data["echoed"])
	}
}

// =============================================================================
// Test_Tool_Execute_Error
// A tool's Execute function can return an error.
// =============================================================================

func Test_Tool_Execute_Error(t *testing.T) {
	tool := tools.Tool{
		Name: "fail",
		Execute: func(_ context.Context, _ map[string]any) (*tools.Result, error) {
			return nil, fmt.Errorf("something broke")
		},
	}

	_, err := tool.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "something broke" {
		t.Errorf("error = %q, want 'something broke'", err.Error())
	}
}

// =============================================================================
// Test_Registry_Schema_RequiredFields
// Schema must include required field names.
// =============================================================================

func Test_Registry_Schema_RequiredFields(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.Tool{
		Name:        "synth",
		Description: "Synthesize signals",
		Parameters: []tools.Param{
			{Name: "limit", Type: "integer", Description: "Max signals", Required: true},
			{Name: "effort", Type: "string", Description: "Effort level", Required: false},
		},
	})

	schema := reg.Schema()
	params := schema[0]["parameters"].(map[string]any)
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("schema 'required' is not a []string")
	}
	if len(required) != 1 || required[0] != "limit" {
		t.Errorf("required = %v, want [limit]", required)
	}
}

// =============================================================================
// Test_Registry_Schema_Enum
// Schema must include enum values when specified on a param.
// =============================================================================

func Test_Registry_Schema_Enum(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.Tool{
		Name: "harvest",
		Parameters: []tools.Param{
			{Name: "source", Type: "string", Required: true, Enum: []string{"bluesky", "youtube"}},
		},
	})

	schema := reg.Schema()
	params := schema[0]["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	sourceProp := props["source"].(map[string]any)

	enumVals, ok := sourceProp["enum"].([]string)
	if !ok {
		t.Fatal("source enum is not []string")
	}
	if len(enumVals) != 2 || enumVals[0] != "bluesky" || enumVals[1] != "youtube" {
		t.Errorf("enum = %v, want [bluesky youtube]", enumVals)
	}
}
