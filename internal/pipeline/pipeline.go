package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/notify"
)

// HarvestFunc fetches raw signals from a single source.
type HarvestFunc func(ctx context.Context, source string) ([]domain.RawSignal, error)

// SynthesizeFunc processes unsynthesized signals through the LLM pipeline.
// Returns the count of successfully synthesized signals.
type SynthesizeFunc func(ctx context.Context) (int, error)

// Pipeline orchestrates the full harvest → synthesize → notify flow.
type Pipeline struct {
	Harvest    HarvestFunc
	Synthesize SynthesizeFunc
	Notifier   notify.Notifier
	SignalRepo domain.SignalRepository
	Sources    []string
	RunLogPath string
	Logger     *slog.Logger
}

// Run executes the full pipeline: harvest all sources → synthesize → notify → log.
func (p *Pipeline) Run(ctx context.Context) (*PipelineRun, error) {
	run := &PipelineRun{
		StartedAt: time.Now(),
		Status:    "ok",
	}

	logger := p.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var totalSignals int
	var harvestErrors []string

	// Phase 1: Harvest from each source.
	for _, source := range p.Sources {
		signals, err := p.Harvest(ctx, source)
		if err != nil {
			logger.Error("harvest failed", "source", source, "error", err)
			harvestErrors = append(harvestErrors, fmt.Sprintf("%s: %v", source, err))
			continue
		}
		totalSignals += len(signals)
		logger.Info("harvested", "source", source, "count", len(signals))
	}

	run.SignalsFound = totalSignals

	// If all harvests failed, mark as error.
	if len(harvestErrors) == len(p.Sources) && len(p.Sources) > 0 {
		run.Status = "error"
		run.Error = fmt.Sprintf("all harvests failed: %v", harvestErrors)
		run.FinishedAt = time.Now()
		run.DurationMs = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
		p.writeLog(run)
		return run, nil
	}

	// If some harvests failed, mark as partial.
	if len(harvestErrors) > 0 {
		run.Status = "partial"
		run.Error = fmt.Sprintf("some harvests failed: %v", harvestErrors)
	}

	// Phase 2: Synthesize (only if we have signals).
	if totalSignals > 0 {
		synthesized, err := p.Synthesize(ctx)
		if err != nil {
			logger.Error("synthesize failed", "error", err)
			run.Status = "partial"
			if run.Error != "" {
				run.Error += "; "
			}
			run.Error += fmt.Sprintf("synthesize: %v", err)
		} else {
			run.Synthesized = synthesized
			logger.Info("synthesized", "count", synthesized)
		}
	}

	// Phase 3: Notify (only if we have synthesized signals).
	if totalSignals > 0 && p.Notifier != nil {
		digest := &notify.DigestSummary{
			GeneratedAt: time.Now(),
			SignalCount: totalSignals,
			Sources:     p.Sources,
			Signals:     []notify.DigestSignal{}, // Filled by the CLI layer with actual data
		}

		if err := p.Notifier.Notify(ctx, digest); err != nil {
			logger.Error("notify failed", "error", err)
			run.NotifyStatus = fmt.Sprintf("error: %v", err)
			if run.Status == "ok" {
				run.Status = "partial"
			}
		} else {
			run.NotifyStatus = "sent"
			logger.Info("notification sent")
		}
	} else if totalSignals == 0 {
		run.NotifyStatus = "skipped (no signals)"
	}

	run.FinishedAt = time.Now()
	run.DurationMs = run.FinishedAt.Sub(run.StartedAt).Milliseconds()

	p.writeLog(run)

	return run, nil
}

// writeLog writes the run result to the JSONL log file if a path is configured.
func (p *Pipeline) writeLog(run *PipelineRun) {
	if p.RunLogPath == "" {
		return
	}
	if err := WriteRunLog(p.RunLogPath, run); err != nil {
		if p.Logger != nil {
			p.Logger.Error("write run log", "error", err)
		}
	}
}
