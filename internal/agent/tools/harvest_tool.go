package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
)

// NewHarvestTool creates a tool that harvests signals from a platform.
// harvestFn is a function that performs the actual harvest.
func NewHarvestTool(harvestFn func(ctx context.Context, source string) ([]domain.RawSignal, error)) Tool {
	return Tool{
		Name:        "harvest",
		Description: "Fetch new signals from a content source (e.g. Bluesky timeline, YouTube subscriptions).",
		Parameters: []Param{
			{Name: "source", Type: "string", Description: "Platform to harvest from", Required: true, Enum: []string{"bluesky", "youtube"}},
		},
		Execute: func(ctx context.Context, args map[string]any) (*Result, error) {
			source, _ := args["source"].(string)
			if source == "" {
				return nil, fmt.Errorf("'source' is required")
			}

			signals, err := harvestFn(ctx, source)
			if err != nil {
				return nil, fmt.Errorf("harvest %s: %w", source, err)
			}

			return &Result{
				Output: fmt.Sprintf("Harvested %d signals from %s.", len(signals), source),
				Data:   map[string]any{"count": len(signals), "source": source},
			}, nil
		},
	}
}

// NewQueryTool creates a tool that queries stored signals.
func NewQueryTool(repo domain.SignalRepository, tenantID uuid.UUID) Tool {
	return Tool{
		Name:        "query_signals",
		Description: "Query recent signals stored in the database.",
		Parameters: []Param{
			{Name: "limit", Type: "integer", Description: "Maximum number of signals to return (default 10)", Required: false},
		},
		Execute: func(ctx context.Context, args map[string]any) (*Result, error) {
			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}
			if limit <= 0 || limit > 50 {
				limit = 10
			}

			signals, err := repo.FindRecentByTenant(ctx, tenantID, limit)
			if err != nil {
				return nil, fmt.Errorf("query signals: %w", err)
			}

			summaries := make([]map[string]any, 0, len(signals))
			for _, s := range signals {
				entry := map[string]any{
					"title":  s.Title,
					"source": s.SourceURL,
				}
				if s.Distillation != "" {
					entry["distillation"] = s.Distillation
				}
				summaries = append(summaries, entry)
			}

			return &Result{
				Output: fmt.Sprintf("Found %d signals.", len(signals)),
				Data:   map[string]any{"count": len(signals), "signals": summaries},
			}, nil
		},
	}
}
