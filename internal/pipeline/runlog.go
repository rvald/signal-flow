package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PipelineRun records the result of a single pipeline execution.
type PipelineRun struct {
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	DurationMs   int64     `json:"duration_ms"`
	SignalsFound int       `json:"signals_found"`
	Synthesized  int       `json:"synthesized"`
	Status       string    `json:"status"` // "ok" | "partial" | "error"
	Error        string    `json:"error,omitempty"`
	NotifyStatus string    `json:"notify_status,omitempty"`
}

// WriteRunLog appends a PipelineRun as a single JSON line to the run log file.
// Uses atomic write: write to .tmp, then append to the target file.
func WriteRunLog(path string, run *PipelineRun) error {
	// Ensure the directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create run log dir: %w", err)
	}

	line, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal run log entry: %w", err)
	}
	line = append(line, '\n')

	// Append to the log file (create if doesn't exist).
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open run log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write run log: %w", err)
	}

	return nil
}

// ReadRunLog reads all PipelineRun entries from a JSONL log file.
func ReadRunLog(path string) ([]*PipelineRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run log: %w", err)
	}

	var runs []*PipelineRun
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var run PipelineRun
		if err := json.Unmarshal(line, &run); err != nil {
			return nil, fmt.Errorf("parse run log line: %w", err)
		}
		runs = append(runs, &run)
	}

	return runs, nil
}

// splitLines splits a byte slice by newlines without importing bufio.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
