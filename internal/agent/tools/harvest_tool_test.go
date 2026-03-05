package tools_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rvald/signal-flow/internal/agent/tools"
	"github.com/rvald/signal-flow/internal/domain"
)

// =============================================================================
// Test_HarvestTool_Success
// Harvest tool calls the harvest function and returns signal count.
// =============================================================================

func Test_HarvestTool_Success(t *testing.T) {
	harvestFn := func(_ context.Context, source string) ([]domain.RawSignal, error) {
		return []domain.RawSignal{
			{Title: "Post 1", SourceURL: "https://bsky.app/1"},
			{Title: "Post 2", SourceURL: "https://bsky.app/2"},
		}, nil
	}

	tool := tools.NewHarvestTool(harvestFn)

	result, err := tool.Execute(context.Background(), map[string]any{"source": "bluesky"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "Harvested 2 signals from bluesky." {
		t.Errorf("Output = %q", result.Output)
	}
	if result.Data["count"] != 2 {
		t.Errorf("Data[count] = %v, want 2", result.Data["count"])
	}
}

// =============================================================================
// Test_HarvestTool_MissingSource
// Harvest tool returns error when source is empty.
// =============================================================================

func Test_HarvestTool_MissingSource(t *testing.T) {
	tool := tools.NewHarvestTool(func(_ context.Context, _ string) ([]domain.RawSignal, error) {
		return nil, nil
	})

	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

// =============================================================================
// Test_HarvestTool_HarvestError
// Harvest tool wraps errors from the harvest function.
// =============================================================================

func Test_HarvestTool_HarvestError(t *testing.T) {
	tool := tools.NewHarvestTool(func(_ context.Context, _ string) ([]domain.RawSignal, error) {
		return nil, fmt.Errorf("connection refused")
	})

	_, err := tool.Execute(context.Background(), map[string]any{"source": "bluesky"})
	if err == nil {
		t.Fatal("expected error from failed harvest")
	}
}

// =============================================================================
// Test_HarvestTool_Schema
// Harvest tool must have correct name and parameter definitions.
// =============================================================================

func Test_HarvestTool_Schema(t *testing.T) {
	tool := tools.NewHarvestTool(nil)

	if tool.Name != "harvest" {
		t.Errorf("Name = %q, want 'harvest'", tool.Name)
	}
	if len(tool.Parameters) != 1 {
		t.Fatalf("Parameters length = %d, want 1", len(tool.Parameters))
	}
	p := tool.Parameters[0]
	if p.Name != "source" || !p.Required {
		t.Errorf("param = %+v, want source/required", p)
	}
	if len(p.Enum) != 2 {
		t.Errorf("Enum = %v, want [bluesky youtube]", p.Enum)
	}
}
