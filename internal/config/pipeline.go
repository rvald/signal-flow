package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PipelineConfig defines the settings for the automated pipeline.
type PipelineConfig struct {
	Sources       []string          `yaml:"sources"`
	GoogleAccount string            `yaml:"google_account"`
	Synthesizer   SynthesizerConfig `yaml:"synthesizer"`
	Notify        NotifyConfig      `yaml:"notify"`
	Schedule      ScheduleConfig    `yaml:"schedule"`
	RunLogPath    string            `yaml:"run_log_path"`
}

// SynthesizerConfig controls LLM provider selection and effort level.
type SynthesizerConfig struct {
	Provider string `yaml:"provider"`
	Effort   string `yaml:"effort"`
	Limit    int    `yaml:"limit"`
}

// NotifyConfig controls where pipeline results are delivered.
type NotifyConfig struct {
	Channel    string `yaml:"channel"`
	WebhookURL string `yaml:"webhook_url"`
	Target     string `yaml:"target"`
}

// ScheduleConfig documents the intended schedule (actual scheduling is external).
type ScheduleConfig struct {
	Interval string `yaml:"interval"`
}

// DefaultPipelineConfig returns a PipelineConfig with sensible defaults.
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		Sources: []string{"bluesky"},
		Synthesizer: SynthesizerConfig{
			Provider: "gemini",
			Effort:   "low",
			Limit:    10,
		},
		Notify: NotifyConfig{
			Channel: "slack",
		},
		Schedule: ScheduleConfig{
			Interval: "4h",
		},
		RunLogPath: "~/.config/signal-flow/runs/pipeline.jsonl",
	}
}

// LoadPipelineConfig reads and parses a pipeline YAML config file.
// Environment variable overrides:
//   - SLACK_WEBHOOK_URL overrides notify.webhook_url
//   - PIPELINE_PROVIDER overrides synthesizer.provider
//   - PIPELINE_EFFORT overrides synthesizer.effort
func LoadPipelineConfig(path string) (*PipelineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline config %s: %w", path, err)
	}

	cfg := DefaultPipelineConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse pipeline config %s: %w", path, err)
	}

	// Apply environment variable overrides.
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.Notify.WebhookURL = v
	}
	if v := os.Getenv("PIPELINE_PROVIDER"); v != "" {
		cfg.Synthesizer.Provider = v
	}
	if v := os.Getenv("PIPELINE_EFFORT"); v != "" {
		cfg.Synthesizer.Effort = v
	}
	if v := os.Getenv("GOG_ACCOUNT"); v != "" {
		cfg.GoogleAccount = v
	}

	return cfg, nil
}
