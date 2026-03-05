package tools

import (
	"context"
	"fmt"

	"github.com/rvald/signal-flow/internal/pipeline"
)

// NewStatusTool creates a tool that reports recent pipeline run status.
func NewStatusTool(runLogPath string) Tool {
	return Tool{
		Name:        "pipeline_status",
		Description: "Check the status of recent pipeline runs.",
		Parameters: []Param{
			{Name: "limit", Type: "integer", Description: "Number of recent runs to show (default 5)", Required: false},
		},
		Execute: func(ctx context.Context, args map[string]any) (*Result, error) {
			limit := 5
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}
			if limit <= 0 || limit > 20 {
				limit = 5
			}

			runs, err := pipeline.ReadRunLog(runLogPath)
			if err != nil {
				return nil, fmt.Errorf("read run log: %w", err)
			}

			// Take last N runs.
			start := 0
			if len(runs) > limit {
				start = len(runs) - limit
			}
			recent := runs[start:]

			summaries := make([]map[string]any, 0, len(recent))
			for _, r := range recent {
				summaries = append(summaries, map[string]any{
					"started_at":  r.StartedAt.Format("2006-01-02 15:04:05"),
					"status":      r.Status,
					"signals":     r.SignalsFound,
					"synthesized": r.Synthesized,
					"duration_ms": r.DurationMs,
				})
			}

			return &Result{
				Output: fmt.Sprintf("Showing %d recent pipeline runs.", len(recent)),
				Data:   map[string]any{"count": len(recent), "runs": summaries},
			}, nil
		},
	}
}
