package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/notify"
	"github.com/rvald/signal-flow/internal/pipeline"
)

// =============================================================================
// Mock Notifier
// =============================================================================

type mockNotifier struct {
	called  bool
	summary *notify.DigestSummary
	err     error
}

func (m *mockNotifier) Notify(_ context.Context, summary *notify.DigestSummary) error {
	m.called = true
	m.summary = summary
	return m.err
}

// =============================================================================
// Test_Pipeline_FullRun
// Verifies the full pipeline: harvest → synthesize → notify → run log.
// =============================================================================

func Test_Pipeline_FullRun(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runs.jsonl")

	harvestCalled := map[string]bool{}
	synthesizeCalled := false
	notif := &mockNotifier{}

	p := &pipeline.Pipeline{
		Sources: []string{"bluesky", "youtube"},
		Harvest: func(_ context.Context, source string) ([]domain.RawSignal, error) {
			harvestCalled[source] = true
			return []domain.RawSignal{
				{SourceURL: "https://example.com/" + source, Title: "Signal from " + source, Provider: source},
			}, nil
		},
		Synthesize: func(_ context.Context) (int, error) {
			synthesizeCalled = true
			return 2, nil
		},
		Notifier:   notif,
		RunLogPath: logPath,
	}

	run, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify harvest was called for each source.
	if !harvestCalled["bluesky"] {
		t.Error("harvest not called for bluesky")
	}
	if !harvestCalled["youtube"] {
		t.Error("harvest not called for youtube")
	}

	// Verify synthesize was called.
	if !synthesizeCalled {
		t.Error("synthesize not called")
	}

	// Verify notify was called.
	if !notif.called {
		t.Error("notifier not called")
	}

	// Verify run result.
	if run.Status != "ok" {
		t.Errorf("Status = %q, want ok", run.Status)
	}
	if run.SignalsFound != 2 {
		t.Errorf("SignalsFound = %d, want 2", run.SignalsFound)
	}
	if run.Synthesized != 2 {
		t.Errorf("Synthesized = %d, want 2", run.Synthesized)
	}
	if run.NotifyStatus != "sent" {
		t.Errorf("NotifyStatus = %q, want sent", run.NotifyStatus)
	}

	// Verify run log was written.
	runs, err := pipeline.ReadRunLog(logPath)
	if err != nil {
		t.Fatalf("ReadRunLog: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run log entry, got %d", len(runs))
	}
	if runs[0].Status != "ok" {
		t.Errorf("logged status = %q, want ok", runs[0].Status)
	}
}

// =============================================================================
// Test_Pipeline_HarvestError_ContinuesOtherSources
// Verifies that if one source fails, others are still processed.
// =============================================================================

func Test_Pipeline_HarvestError_ContinuesOtherSources(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runs.jsonl")
	notif := &mockNotifier{}

	p := &pipeline.Pipeline{
		Sources: []string{"bluesky", "youtube"},
		Harvest: func(_ context.Context, source string) ([]domain.RawSignal, error) {
			if source == "bluesky" {
				return nil, errors.New("bluesky down")
			}
			return []domain.RawSignal{
				{SourceURL: "https://youtube.com/v1", Title: "YT Video", Provider: "youtube"},
			}, nil
		},
		Synthesize: func(_ context.Context) (int, error) {
			return 1, nil
		},
		Notifier:   notif,
		RunLogPath: logPath,
	}

	run, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if run.Status != "partial" {
		t.Errorf("Status = %q, want partial", run.Status)
	}
	if run.SignalsFound != 1 {
		t.Errorf("SignalsFound = %d, want 1 (from youtube)", run.SignalsFound)
	}
	if !notif.called {
		t.Error("notifier should still be called for youtube signals")
	}
}

// =============================================================================
// Test_Pipeline_NoSignals_SkipsSynthesizeAndNotify
// Verifies that an empty harvest skips synthesize and notify phases.
// =============================================================================

func Test_Pipeline_NoSignals_SkipsSynthesizeAndNotify(t *testing.T) {
	synthesizeCalled := false
	notif := &mockNotifier{}

	p := &pipeline.Pipeline{
		Sources: []string{"bluesky"},
		Harvest: func(_ context.Context, _ string) ([]domain.RawSignal, error) {
			return []domain.RawSignal{}, nil
		},
		Synthesize: func(_ context.Context) (int, error) {
			synthesizeCalled = true
			return 0, nil
		},
		Notifier: notif,
	}

	run, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if run.Status != "ok" {
		t.Errorf("Status = %q, want ok", run.Status)
	}
	if synthesizeCalled {
		t.Error("synthesize should not be called with 0 signals")
	}
	if notif.called {
		t.Error("notifier should not be called with 0 signals")
	}
}

// =============================================================================
// Test_Pipeline_NotifyError_StillLogsRun
// Verifies that a notify failure still results in a run log entry.
// =============================================================================

func Test_Pipeline_NotifyError_StillLogsRun(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runs.jsonl")

	notif := &mockNotifier{err: errors.New("slack down")}

	p := &pipeline.Pipeline{
		Sources: []string{"bluesky"},
		Harvest: func(_ context.Context, _ string) ([]domain.RawSignal, error) {
			return []domain.RawSignal{
				{SourceURL: "https://example.com/1", Provider: "bluesky"},
			}, nil
		},
		Synthesize: func(_ context.Context) (int, error) {
			return 1, nil
		},
		Notifier:   notif,
		RunLogPath: logPath,
	}

	run, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if run.Status != "partial" {
		t.Errorf("Status = %q, want partial (notify failed)", run.Status)
	}

	// Run log should still be written.
	runs, err := pipeline.ReadRunLog(logPath)
	if err != nil {
		t.Fatalf("ReadRunLog: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("expected 1 run log entry, got %d", len(runs))
	}
}

// =============================================================================
// Test_Pipeline_RunLog_JSONL
// Verifies the JSONL run log format: valid JSON per line, correct fields.
// =============================================================================

func Test_Pipeline_RunLog_JSONL(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runs.jsonl")

	// Write two runs.
	run1 := &pipeline.PipelineRun{
		StartedAt:    time.Date(2026, 3, 3, 7, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2026, 3, 3, 7, 0, 5, 0, time.UTC),
		DurationMs:   5000,
		SignalsFound: 10,
		Synthesized:  8,
		Status:       "ok",
		NotifyStatus: "sent",
	}
	run2 := &pipeline.PipelineRun{
		StartedAt:    time.Date(2026, 3, 3, 11, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2026, 3, 3, 11, 0, 3, 0, time.UTC),
		DurationMs:   3000,
		SignalsFound: 5,
		Synthesized:  5,
		Status:       "partial",
		Error:        "bluesky: connection timeout",
		NotifyStatus: "sent",
	}

	if err := pipeline.WriteRunLog(logPath, run1); err != nil {
		t.Fatalf("WriteRunLog run1: %v", err)
	}
	if err := pipeline.WriteRunLog(logPath, run2); err != nil {
		t.Fatalf("WriteRunLog run2: %v", err)
	}

	// Read and verify.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// Each line should be valid JSON.
	lines := splitNonEmpty(data)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}

		// Verify expected fields exist.
		for _, key := range []string{"started_at", "finished_at", "duration_ms", "signals_found", "status"} {
			if _, ok := entry[key]; !ok {
				t.Errorf("line %d: missing key %q", i, key)
			}
		}
	}

	// Read via ReadRunLog.
	runs, err := pipeline.ReadRunLog(logPath)
	if err != nil {
		t.Fatalf("ReadRunLog: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("ReadRunLog: expected 2 runs, got %d", len(runs))
	}
	if runs[0].SignalsFound != 10 {
		t.Errorf("run[0].SignalsFound = %d, want 10", runs[0].SignalsFound)
	}
	if runs[1].Status != "partial" {
		t.Errorf("run[1].Status = %q, want partial", runs[1].Status)
	}
}

// splitNonEmpty splits bytes on newlines and drops empty lines.
func splitNonEmpty(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
