package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rvald/signal-flow/internal/app"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence"
	"github.com/rvald/signal-flow/internal/outfmt"
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
		Example: `  # Synthesize up to 10 signals with Ollama (default)
  signal-flow synthesize

  # Use Claude with high effort
  signal-flow synthesize --provider claude --effort high

  # Re-synthesize a specific URL
  signal-flow synthesize --url https://example.com/article`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSynthesize(cmd.Context(), provider, effort, limit, url)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "ollama", "LLM provider (gemini, claude, openai, ollama)")
	cmd.Flags().StringVar(&effort, "effort", "low", "synthesizer effort level: low (flash), high (reasoning)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "maximum number of signals to process")
	cmd.Flags().StringVar(&url, "url", "", "synthesize a specific signal by URL")

	return cmd
}

func runSynthesize(ctx context.Context, providerName, effort string, limit int, targetURL string) error {
	// --- Defensive checks ---
	providerName = strings.ToLower(providerName)
	switch providerName {
	case "gemini", "claude", "openai", "ollama":
		// valid
	default:
		return fmt.Errorf("invalid provider '%s': must be gemini, claude, openai, or ollama", providerName)
	}

	if effort != "low" && effort != "high" {
		return fmt.Errorf("invalid effort '%s': must be 'low' or 'high'", effort)
	}

	// --- Initialize Provider ---
	flash, reasoning, err := app.BuildSummarizers(ctx, providerName, effort)
	if err != nil {
		return fmt.Errorf("init summarizers: %w", err)
	}

	// Print configuration
	fmt.Fprintf(os.Stderr, "Provider: %s\n", providerName)
	fmt.Fprintf(os.Stderr, "Effort:   %s\n\n", effort)

	// --- Connect to DB ---
	db, cleanup, err := connectDB(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	signalRepo := db.Repo
	tenantID := db.TenantID

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
		fmt.Fprintf(os.Stderr, "Processing 1 explicit signal...\n")
	} else {
		signals, err = signalRepo.FindUnsynthesized(ctx, tenantID, limit)
		if err != nil {
			return fmt.Errorf("find unsynthesized signals: %w", err)
		}
		if len(signals) == 0 {
			fmt.Fprintln(os.Stderr, "No unsynthesized signals found.")
			return nil
		}
		fmt.Fprintf(os.Stderr, "Processing %d unsynthesized signals...\n", len(signals))
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
		fmt.Fprintf(os.Stderr, "  %d. %s  → ", i+1, sig.SourceURL)

		result, err := synthesizer.Synthesize(ctx, tenantID, sig.SourceURL, sig.Content, domain.ContextParams{
			Priority: priority,
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ failed (%v)\n", err)
			continue
		}

		if result.Cached {
			fmt.Fprintln(os.Stderr, "✓ cached (already synthesized)")
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

		fmt.Fprintf(os.Stderr, "✓ synthesized (%d tokens, %.1fs)\n", sigTokens, sigLatency.Seconds())
		synthesizedCount++
	}

	fmt.Fprintf(os.Stderr, "\nDone. %d synthesized, %d cached. Total: %d tokens.\n", synthesizedCount, cachedCount, totalTokens)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"synthesized":  synthesizedCount,
			"cached":       cachedCount,
			"total_tokens": totalTokens,
		})
	}

	return nil
}
