package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/rvald/signal-flow/internal/security"
	"github.com/spf13/cobra"
)

func newSynthesizeCmd() *cobra.Command {
	var provider string
	var effort string
	var limit int
	var url string

	cmd := &cobra.Command{
		Use:   "synthesize",
		Short: "Run the intelligence pipeline on signals",
		Long: `Process raw signals through the LLM pipeline to generate structured summaries.
Requires DATABASE_URL, ENCRYPTION_KEY, and the API key for your chosen provider.

Effort tiers:
  low  - single-pass processing using a fast model (flash)
  high - two-pass processing: fast model for analysis, reasoning model for distillation.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSynthesize(cmd.Context(), provider, effort, limit, url)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "gemini", "LLM provider (gemini, claude, openai)")
	cmd.Flags().StringVar(&effort, "effort", "low", "synthesizer effort level: low (flash), high (reasoning)")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of signals to process")
	cmd.Flags().StringVar(&url, "url", "", "synthesize a specific signal by URL")

	return cmd
}

func runSynthesize(ctx context.Context, providerName, effort string, limit int, targetURL string) error {
	// --- Defensive checks ---
	if effort != "low" && effort != "high" {
		return fmt.Errorf("invalid effort '%s': must be 'low' or 'high'", effort)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL env var is required")
	}

	encryptionKeyHex := os.Getenv("ENCRYPTION_KEY")
	if encryptionKeyHex == "" {
		return fmt.Errorf("ENCRYPTION_KEY env var is required")
	}

	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		return fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}

	_, err = security.NewLocalKeyManager(encryptionKey)
	if err != nil {
		return fmt.Errorf("create key manager: %w", err)
	}

	// --- Initialize Provider ---
	var flash, reasoning domain.Summarizer
	var apiKey string
	var flashModel, reasoningModel string

	switch strings.ToLower(providerName) {
	case "gemini":
		apiKey = os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("GEMINI_API_KEY env var is required for the gemini provider")
		}
		flashModel = "gemini-2.5-flash"
		reasoningModel = "gemini-3-flash"
		f, err := intelligence.NewGeminiSummarizer(ctx, apiKey, flashModel)
		if err != nil {
			return fmt.Errorf("init gemini flash: %w", err)
		}
		flash = f
		if effort == "high" {
			r, err := intelligence.NewGeminiSummarizer(ctx, apiKey, reasoningModel)
			if err != nil {
				return fmt.Errorf("init gemini reasoning: %w", err)
			}
			reasoning = r
		} else {
			reasoning = f
		}
	case "claude":
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY env var is required for the claude provider")
		}
		flashModel = "claude-haiku-3-5"
		reasoningModel = "claude-sonnet-4-5"
		flash = intelligence.NewClaudeSummarizer(apiKey, anthropic.Model(flashModel))
		if effort == "high" {
			reasoning = intelligence.NewClaudeSummarizer(apiKey, anthropic.Model(reasoningModel))
		} else {
			reasoning = flash
		}
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENAI_API_KEY env var is required for the openai provider")
		}
		flashModel = "gpt-4o-mini"
		reasoningModel = "o3-mini"
		flash = intelligence.NewOpenAISummarizer(apiKey, shared.ResponsesModel(flashModel))
		if effort == "high" {
			reasoning = intelligence.NewOpenAISummarizer(apiKey, shared.ResponsesModel(reasoningModel))
		} else {
			reasoning = flash
		}
	default:
		return fmt.Errorf("invalid provider '%s': must be gemini, claude, or openai", providerName)
	}

	// Print configuration
	fmt.Printf("Provider: %s\n", providerName)
	fmt.Printf("  Analysis:     %s (always)\n", flashModel)
	if effort == "high" {
		fmt.Printf("  Distillation: %s (high effort)\n\n", reasoningModel)
	} else {
		fmt.Printf("  Distillation: %s (low effort)\n\n", flashModel)
	}

	// --- Connect to DB ---
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(dbCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(dbCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	signalRepo := repository.NewPostgresSignalRepository(pool)
	tenantID := devTenantID

	// --- Fetch Signals ---
	var signals []*domain.Signal
	if targetURL != "" {
		sig, err := signalRepo.FindBySourceURL(ctx, tenantID, targetURL)
		if err != nil {
			return fmt.Errorf("find signal by URL: %w", err)
		}
		if sig == nil {
			// This happens if the signal isn't in DB at all.
			return fmt.Errorf("signal not found in database for URL: %s (run harvest first)", targetURL)
		}
		signals = []*domain.Signal{sig}
		fmt.Printf("Processing 1 explicit signal...\n")
	} else {
		signals, err = signalRepo.FindUnsynthesized(ctx, tenantID, limit)
		if err != nil {
			return fmt.Errorf("find unsynthesized signals: %w", err)
		}
		if len(signals) == 0 {
			fmt.Println("No unsynthesized signals found.")
			return nil
		}
		fmt.Printf("Processing %d unsynthesized signals...\n", len(signals))
	}

	// --- Synthesize ---
	synthesizer := intelligence.NewSynthesizerService(flash, reasoning, signalRepo)
	priority := domain.PriorityStandard
	if effort == "high" {
		priority = domain.PriorityHigh
	}

	totalTokens := 0
	synthesizedCount := 0
	cachedCount := 0

	for i, sig := range signals {
		fmt.Printf("  %d. %s  → ", i+1, sig.SourceURL)

		result, err := synthesizer.Synthesize(ctx, tenantID, sig.SourceURL, sig.Content, domain.ContextParams{
			Priority: priority,
		})

		if err != nil {
			fmt.Printf("✗ failed (%v)\n", err)
			continue
		}

		if result.Cached {
			fmt.Println("✓ cached (already synthesized)")
			cachedCount++
			continue
		}

		var sigTokens int
		var sigLatency time.Duration
		for _, u := range result.Usage {
			sigTokens += u.PromptTokens + u.CompletionTokens
			sigLatency += u.Latency
			totalTokens += u.PromptTokens + u.CompletionTokens
		}

		fmt.Printf("✓ synthesized (%d tokens, %.1fs)\n", sigTokens, sigLatency.Seconds())
		synthesizedCount++
	}

	fmt.Printf("\nDone. %d synthesized, %d cached. Total: %d tokens.\n", synthesizedCount, cachedCount, totalTokens)
	return nil
}
