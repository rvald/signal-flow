package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/rvald/signal-flow/internal/security"
	"github.com/spf13/cobra"
)

func newHarvestCmd() *cobra.Command {
	var limit int64
	var dryRun bool
	var followsOnly bool

	cmd := &cobra.Command{
		Use:   "harvest",
		Short: "Fetch timeline links and store as signals",
		Long: `Fetches your Bluesky timeline, extracts links, and stores them as
Signals in PostgreSQL. Requires DATABASE_URL and ENCRYPTION_KEY env vars.

By default, harvests all timeline links. Use --follows to limit to posts
from accounts you follow.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarvest(cmd.Context(), limit, dryRun, followsOnly)
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 50, "number of timeline posts to fetch")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview links without saving to database")
	cmd.Flags().BoolVar(&followsOnly, "follows", false, "only harvest links from accounts you follow")

	return cmd
}

func runHarvest(ctx context.Context, limit int64, dryRun, followsOnly bool) error {
	// --- Auth (from config file) ---
	client, session, err := resolveBlueskyClient(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Harvesting timeline for %s...\n", session.Handle)

	// --- Fetch timeline ---
	resp, err := bsky.FeedGetTimeline(ctx, client, "", "", limit)
	if err != nil {
		return wrapExpiredTokenErr(fmt.Errorf("fetch timeline: %w", err))
	}

	// --- Extract links (reuse shared helper from feed.go) ---
	feedItems := sdkFeedToTimeline(resp.Feed)

	// --- Filter by follows if requested ---
	if followsOnly {
		followDIDs, err := fetchFollowDIDs(ctx, client, session.DID)
		if err != nil {
			return wrapExpiredTokenErr(fmt.Errorf("fetch follows: %w", err))
		}
		feedItems = auth.FilterByFollows(feedItems, followDIDs)
	}

	signals := auth.ExtractLinksFromFeed(feedItems)

	if len(signals) == 0 {
		fmt.Println("No links found in timeline.")
		return nil
	}

	fmt.Printf("Found %d links from %d posts.\n", len(signals), len(resp.Feed))

	if dryRun {
		fmt.Println("\n[dry-run] Would store:")
		for i, s := range signals {
			fmt.Printf("  %d. %s\n     %s\n", i+1, s.Title, s.SourceURL)
		}
		return nil
	}

	// --- Connect to DB ---
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL env var is required for harvest (set it or use --dry-run)")
	}

	encryptionKeyHex := os.Getenv("ENCRYPTION_KEY")
	if encryptionKeyHex == "" {
		return fmt.Errorf("ENCRYPTION_KEY env var is required for harvest (set it or use --dry-run)")
	}

	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		return fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}

	_, err = security.NewLocalKeyManager(encryptionKey)
	if err != nil {
		return fmt.Errorf("create key manager: %w", err)
	}

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

	// --- Store signals ---
	// Use the DID-based UUID for the tenant. For single-user CLI, we use the
	// deterministic dev user UUID.
	tenantID := devTenantID

	stored := 0
	skipped := 0
	for _, raw := range signals {
		sig := &domain.Signal{
			TenantID:  tenantID,
			SourceURL: raw.SourceURL,
			Title:     raw.Title,
			Content:   raw.Content,
			Scope:     domain.ScopePrivate,
			Metadata:  raw.Metadata,
		}

		if err := signalRepo.Save(ctx, sig); err != nil {
			fmt.Printf("  ⚠ failed to save: %s (%v)\n", raw.SourceURL, err)
			continue
		}
		stored++
	}
	skipped = len(signals) - stored

	fmt.Printf("\nHarvested %d new signals", stored)
	if skipped > 0 {
		fmt.Printf(", %d skipped", skipped)
	}
	fmt.Println(".")

	return nil
}
