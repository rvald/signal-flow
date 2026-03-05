package app_test

import (
	"context"
	"testing"

	"github.com/rvald/signal-flow/internal/app"
	"github.com/rvald/signal-flow/internal/config"
)

// =============================================================================
// Test_Validate_MissingDatabaseURL
// New() must fail when DatabaseURL is empty.
// =============================================================================

func Test_Validate_MissingDatabaseURL(t *testing.T) {
	_, err := app.New(context.Background(), app.Config{
		EncryptionKey: "0123456789abcdef0123456789abcdef",
	})
	if err == nil {
		t.Fatal("expected error for missing DatabaseURL, got nil")
	}
	if got := err.Error(); got != "required config fields not set: DatabaseURL" {
		t.Errorf("error = %q, want 'required config fields not set: DatabaseURL'", got)
	}
}

// =============================================================================
// Test_Validate_MissingEncryptionKey
// New() must fail when EncryptionKey is empty.
// =============================================================================

func Test_Validate_MissingEncryptionKey(t *testing.T) {
	_, err := app.New(context.Background(), app.Config{
		DatabaseURL: "postgres://localhost/test",
	})
	if err == nil {
		t.Fatal("expected error for missing EncryptionKey, got nil")
	}
	if got := err.Error(); got != "required config fields not set: EncryptionKey" {
		t.Errorf("error = %q, want 'required config fields not set: EncryptionKey'", got)
	}
}

// =============================================================================
// Test_Validate_MissingBoth
// New() must fail and list both missing fields.
// =============================================================================

func Test_Validate_MissingBoth(t *testing.T) {
	_, err := app.New(context.Background(), app.Config{})
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
	if got := err.Error(); got != "required config fields not set: DatabaseURL, EncryptionKey" {
		t.Errorf("error = %q, want 'required config fields not set: DatabaseURL, EncryptionKey'", got)
	}
}

// =============================================================================
// Test_Validate_InvalidEncryptionKey
// New() must fail for non-hex encryption key.
// =============================================================================

func Test_Validate_InvalidEncryptionKey(t *testing.T) {
	_, err := app.New(context.Background(), app.Config{
		DatabaseURL:   "postgres://localhost/test",
		EncryptionKey: "not-valid-hex!",
	})
	if err == nil {
		t.Fatal("expected error for invalid EncryptionKey, got nil")
	}
}

// =============================================================================
// Test_BuildSummarizers_InvalidProvider
// BuildSummarizers must reject unknown providers.
// =============================================================================

func Test_BuildSummarizers_InvalidProvider(t *testing.T) {
	_, _, err := app.BuildSummarizers(context.Background(), "invalid-provider", "low")
	if err == nil {
		t.Fatal("expected error for invalid provider, got nil")
	}
	want := "invalid provider 'invalid-provider': must be gemini, claude, or openai"
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// =============================================================================
// Test_BuildSummarizers_MissingGeminiKey
// BuildSummarizers must fail when GEMINI_API_KEY is unset.
// =============================================================================

func Test_BuildSummarizers_MissingGeminiKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	_, _, err := app.BuildSummarizers(context.Background(), "gemini", "low")
	if err == nil {
		t.Fatal("expected error for missing GEMINI_API_KEY, got nil")
	}
}

// =============================================================================
// Test_BuildSummarizers_MissingClaudeKey
// BuildSummarizers must fail when ANTHROPIC_API_KEY is unset.
// =============================================================================

func Test_BuildSummarizers_MissingClaudeKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, _, err := app.BuildSummarizers(context.Background(), "claude", "low")
	if err == nil {
		t.Fatal("expected error for missing ANTHROPIC_API_KEY, got nil")
	}
}

// =============================================================================
// Test_BuildSummarizers_MissingOpenAIKey
// BuildSummarizers must fail when OPENAI_API_KEY is unset.
// =============================================================================

func Test_BuildSummarizers_MissingOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, _, err := app.BuildSummarizers(context.Background(), "openai", "low")
	if err == nil {
		t.Fatal("expected error for missing OPENAI_API_KEY, got nil")
	}
}

// =============================================================================
// Test_FromPipelineConfig
// FromPipelineConfig must correctly map PipelineConfig to app.Config.
// =============================================================================

func Test_FromPipelineConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ENCRYPTION_KEY", "abcdef0123456789abcdef0123456789")

	cfg := app.FromPipelineConfig(&config.PipelineConfig{
		Synthesizer: config.SynthesizerConfig{
			Provider: "gemini",
			Effort:   "high",
		},
		Notify: config.NotifyConfig{
			WebhookURL: "https://hooks.slack.com/test",
			Target:     "#signals",
		},
	})

	if cfg.DatabaseURL != "postgres://test:test@localhost:5432/test" {
		t.Errorf("DatabaseURL = %q, want test URL", cfg.DatabaseURL)
	}
	if cfg.SynthesizerProvider != "gemini" {
		t.Errorf("SynthesizerProvider = %q, want gemini", cfg.SynthesizerProvider)
	}
	if cfg.SynthesizerEffort != "high" {
		t.Errorf("SynthesizerEffort = %q, want high", cfg.SynthesizerEffort)
	}
	if cfg.SlackWebhookURL != "https://hooks.slack.com/test" {
		t.Errorf("SlackWebhookURL = %q, want test URL", cfg.SlackWebhookURL)
	}
	if cfg.SlackTarget != "#signals" {
		t.Errorf("SlackTarget = %q, want #signals", cfg.SlackTarget)
	}
}

// =============================================================================
// Test_DevTenantID
// DevTenantID must be the expected deterministic UUID.
// =============================================================================

func Test_DevTenantID(t *testing.T) {
	want := "00000000-0000-0000-0000-000000000001"
	if got := app.DevTenantID.String(); got != want {
		t.Errorf("DevTenantID = %q, want %q", got, want)
	}
}
