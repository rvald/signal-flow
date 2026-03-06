// Package app provides the shared service bootstrap layer for Signal-Flow.
// Both the CLI and the Slack bot use this package to assemble a fully-wired
// service bundle without duplicating initialization logic.
package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvald/signal-flow/internal/config"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence"
	"github.com/rvald/signal-flow/internal/notify"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/rvald/signal-flow/internal/security"

	"github.com/anthropics/anthropic-sdk-go"
)

// DevTenantID is the deterministic dev user UUID seeded by migration 000005.
// Used for single-user CLI mode.
var DevTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// App holds the fully-wired service bundle for Signal-Flow.
// Create with New() and call Close() when done.
type App struct {
	SignalRepo  domain.SignalRepository
	Synthesizer *intelligence.SynthesizerService
	Notifier    notify.Notifier
	TenantID    uuid.UUID
	Logger      *slog.Logger

	pool *pgxpool.Pool
}

// Config controls which services are initialized.
type Config struct {
	// Required
	DatabaseURL   string
	EncryptionKey string // hex-encoded

	// Synthesizer (optional — nil if not configured)
	SynthesizerProvider string // "gemini", "claude", "openai"
	SynthesizerEffort   string // "low", "high"

	// Notify (optional)
	SlackWebhookURL string
	SlackTarget     string

	// Logger (optional — defaults to slog.Default())
	Logger *slog.Logger
}

// FromPipelineConfig creates an app.Config from a PipelineConfig,
// reading secrets from environment variables.
func FromPipelineConfig(cfg *config.PipelineConfig) Config {
	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
		SynthesizerProvider: cfg.Synthesizer.Provider,
		SynthesizerEffort:   cfg.Synthesizer.Effort,
		SlackWebhookURL:     cfg.Notify.WebhookURL,
		SlackTarget:         cfg.Notify.Target,
	}
}

// New creates a fully-wired App from the given config.
func New(ctx context.Context, cfg Config) (*App, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Decode encryption key.
	encryptionKey, err := hex.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}

	if _, err = security.NewLocalKeyManager(encryptionKey); err != nil {
		return nil, fmt.Errorf("create key manager: %w", err)
	}

	// Connect to DB.
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(dbCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	signalRepo := repository.NewPostgresSignalRepository(pool)

	// Build Synthesizer (optional).
	var synthesizer *intelligence.SynthesizerService
	if cfg.SynthesizerProvider != "" {
		flash, reasoning, err := BuildSummarizers(ctx, cfg.SynthesizerProvider, cfg.SynthesizerEffort)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("init synthesizer: %w", err)
		}
		synthesizer = intelligence.NewSynthesizerService(flash, reasoning, signalRepo)
	}

	// Build Notifier (optional).
	var notifier notify.Notifier
	if cfg.SlackWebhookURL != "" {
		notifier = notify.NewSlackNotifier(cfg.SlackWebhookURL, cfg.SlackTarget)
	}

	return &App{
		SignalRepo:  signalRepo,
		Synthesizer: synthesizer,
		Notifier:    notifier,
		TenantID:    DevTenantID,
		Logger:      logger,
		pool:        pool,
	}, nil
}

// Close releases all resources held by the App.
func (a *App) Close() error {
	if a.pool != nil {
		a.pool.Close()
	}
	return nil
}

// validate checks that required configuration fields are present.
func validate(cfg Config) error {
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DatabaseURL")
	}
	if cfg.EncryptionKey == "" {
		missing = append(missing, "EncryptionKey")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required config fields not set: %s", strings.Join(missing, ", "))
	}
	return nil
}

// BuildSummarizers creates flash and reasoning Summarizer implementations
// for the given provider and effort level.
func BuildSummarizers(ctx context.Context, providerName, effort string) (domain.Summarizer, domain.Summarizer, error) {
	switch strings.ToLower(providerName) {
	case "gemini":
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return nil, nil, fmt.Errorf("GEMINI_API_KEY env var is required")
		}
		flash, err := intelligence.NewGeminiSummarizer(ctx, apiKey, "gemini-2.5-flash")
		if err != nil {
			return nil, nil, err
		}
		if effort == "high" {
			reasoning, err := intelligence.NewGeminiSummarizer(ctx, apiKey, "gemini-3-flash")
			if err != nil {
				return nil, nil, err
			}
			return flash, reasoning, nil
		}
		return flash, flash, nil
	case "claude":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, nil, fmt.Errorf("ANTHROPIC_API_KEY env var is required")
		}
		flash := intelligence.NewClaudeSummarizer(apiKey, anthropic.Model("claude-haiku-3-5"))
		if effort == "high" {
			reasoning := intelligence.NewClaudeSummarizer(apiKey, anthropic.Model("claude-sonnet-4-5"))
			return flash, reasoning, nil
		}
		return flash, flash, nil
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, nil, fmt.Errorf("OPENAI_API_KEY env var is required")
		}
		flash := intelligence.NewOpenAISummarizer(apiKey, "gpt-4o-mini")
		if effort == "high" {
			reasoning := intelligence.NewOpenAISummarizer(apiKey, "o3-mini")
			return flash, reasoning, nil
		}
		return flash, flash, nil
	case "ollama":
		baseURL := os.Getenv("OLLAMA_API_URL")
		flash := intelligence.NewOllamaSummarizer(baseURL, "gemma3:4b")
		if effort == "high" {
			reasoning := intelligence.NewOllamaSummarizer(baseURL, "deepseek-r1:8b")
			return flash, reasoning, nil
		}
		return flash, flash, nil
	default:
		return nil, nil, fmt.Errorf("invalid provider '%s': must be gemini, claude, openai, or ollama", providerName)
	}
}
