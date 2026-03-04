package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rvald/signal-flow/internal/config"
)

// =============================================================================
// Test_LoadPipelineConfig_FullFile
// Verifies that a complete YAML file is parsed into the correct struct.
// =============================================================================

func Test_LoadPipelineConfig_FullFile(t *testing.T) {
	yaml := `
sources:
  - bluesky
  - youtube
synthesizer:
  provider: claude
  effort: high
  limit: 20
notify:
  channel: slack
  webhook_url: https://hooks.slack.com/services/T00/B00/xxx
  target: "#daily-digest"
schedule:
  interval: "6h"
run_log_path: /tmp/test-runs.jsonl
`
	path := writeTemp(t, "pipeline.yaml", yaml)

	cfg, err := config.LoadPipelineConfig(path)
	if err != nil {
		t.Fatalf("LoadPipelineConfig: %v", err)
	}

	if len(cfg.Sources) != 2 || cfg.Sources[0] != "bluesky" || cfg.Sources[1] != "youtube" {
		t.Errorf("Sources = %v, want [bluesky youtube]", cfg.Sources)
	}
	if cfg.Synthesizer.Provider != "claude" {
		t.Errorf("Synthesizer.Provider = %q, want %q", cfg.Synthesizer.Provider, "claude")
	}
	if cfg.Synthesizer.Effort != "high" {
		t.Errorf("Synthesizer.Effort = %q, want %q", cfg.Synthesizer.Effort, "high")
	}
	if cfg.Synthesizer.Limit != 20 {
		t.Errorf("Synthesizer.Limit = %d, want 20", cfg.Synthesizer.Limit)
	}
	if cfg.Notify.WebhookURL != "https://hooks.slack.com/services/T00/B00/xxx" {
		t.Errorf("Notify.WebhookURL = %q", cfg.Notify.WebhookURL)
	}
	if cfg.Notify.Target != "#daily-digest" {
		t.Errorf("Notify.Target = %q, want %q", cfg.Notify.Target, "#daily-digest")
	}
	if cfg.Schedule.Interval != "6h" {
		t.Errorf("Schedule.Interval = %q, want %q", cfg.Schedule.Interval, "6h")
	}
	if cfg.RunLogPath != "/tmp/test-runs.jsonl" {
		t.Errorf("RunLogPath = %q", cfg.RunLogPath)
	}
}

// =============================================================================
// Test_LoadPipelineConfig_EnvOverride
// Verifies that SLACK_WEBHOOK_URL env var overrides the YAML webhook_url.
// =============================================================================

func Test_LoadPipelineConfig_EnvOverride(t *testing.T) {
	yaml := `
notify:
  webhook_url: https://original.slack.com/webhook
`
	path := writeTemp(t, "pipeline.yaml", yaml)

	t.Setenv("SLACK_WEBHOOK_URL", "https://override.slack.com/webhook")

	cfg, err := config.LoadPipelineConfig(path)
	if err != nil {
		t.Fatalf("LoadPipelineConfig: %v", err)
	}

	if cfg.Notify.WebhookURL != "https://override.slack.com/webhook" {
		t.Errorf("WebhookURL = %q, want env override value", cfg.Notify.WebhookURL)
	}
}

// =============================================================================
// Test_LoadPipelineConfig_Defaults
// Verifies that missing fields get their default values.
// =============================================================================

func Test_LoadPipelineConfig_Defaults(t *testing.T) {
	yaml := `
sources:
  - youtube
`
	path := writeTemp(t, "pipeline.yaml", yaml)

	cfg, err := config.LoadPipelineConfig(path)
	if err != nil {
		t.Fatalf("LoadPipelineConfig: %v", err)
	}

	// Sources should be overwritten by the file.
	if len(cfg.Sources) != 1 || cfg.Sources[0] != "youtube" {
		t.Errorf("Sources = %v, want [youtube]", cfg.Sources)
	}

	// Synthesizer defaults should remain.
	if cfg.Synthesizer.Provider != "gemini" {
		t.Errorf("Synthesizer.Provider = %q, want default %q", cfg.Synthesizer.Provider, "gemini")
	}
	if cfg.Synthesizer.Effort != "low" {
		t.Errorf("Synthesizer.Effort = %q, want default %q", cfg.Synthesizer.Effort, "low")
	}
	if cfg.Synthesizer.Limit != 10 {
		t.Errorf("Synthesizer.Limit = %d, want default 10", cfg.Synthesizer.Limit)
	}
}

// =============================================================================
// Test_LoadPipelineConfig_FileNotFound
// Verifies that a missing file returns a clear error.
// =============================================================================

func Test_LoadPipelineConfig_FileNotFound(t *testing.T) {
	_, err := config.LoadPipelineConfig("/tmp/nonexistent-pipeline-config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// =============================================================================
// Test_LoadPipelineConfig_InvalidYAML
// Verifies that invalid YAML returns a parse error.
// =============================================================================

func Test_LoadPipelineConfig_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "bad.yaml", "{{{{not yaml at all")

	_, err := config.LoadPipelineConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// writeTemp creates a temporary file with the given content and returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
