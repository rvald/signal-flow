package cli

import (
	"context"
	"fmt"
	"os"

	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/outfmt"
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
		Example: `  # Preview what would be harvested
  signal-flow harvest --dry-run

  # Harvest only from followed accounts, fetch 100 posts
  signal-flow harvest --follows --limit 100`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarvest(cmd.Context(), limit, dryRun, followsOnly)
		},
	}

	cmd.Flags().Int64VarP(&limit, "limit", "l", 50, "number of timeline posts to fetch")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "preview links without saving to database")
	cmd.Flags().BoolVar(&followsOnly, "follows", false, "only harvest links from accounts you follow")

	return cmd
}

func runHarvest(ctx context.Context, limit int64, dryRun, followsOnly bool) error {
	// --- Fail-fast: check required env vars before API calls (unless dry-run) ---
	if !dryRun {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			return fmt.Errorf("DATABASE_URL env var is required for harvest (set it or use --dry-run)")
		}

		encryptionKeyHex := os.Getenv("ENCRYPTION_KEY")
		if encryptionKeyHex == "" {
			return fmt.Errorf("ENCRYPTION_KEY env var is required for harvest (set it or use --dry-run)")
		}
	}

	// --- Auth (from config file) ---
	client, session, err := resolveBlueskyClient(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Harvesting timeline for %s...\n", session.Handle)

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
		fmt.Fprintln(os.Stderr, "No links found in timeline.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d links from %d posts.\n", len(signals), len(resp.Feed))

	if dryRun {
		if outfmt.IsJSON(ctx) {
			type signalPreview struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			}
			items := make([]signalPreview, 0, len(signals))
			for _, s := range signals {
				items = append(items, signalPreview{Title: s.Title, URL: s.SourceURL})
			}
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"dry_run": true, "signals": items})
		}
		fmt.Println("\n[dry-run] Would store:")
		for i, s := range signals {
			fmt.Printf("  %d. %s\n     %s\n", i+1, s.Title, s.SourceURL)
		}
		return nil
	}

	// --- Connect to DB ---
	db, cleanup, err := connectDB(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	signalRepo := db.Repo
	tenantID := db.TenantID

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
			fmt.Fprintf(os.Stderr, "  ⚠ failed to save: %s (%v)\n", raw.SourceURL, err)
			continue
		}
		stored++
	}
	skipped = len(signals) - stored

	fmt.Fprintf(os.Stderr, "\nHarvested %d new signals", stored)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, ", %d skipped", skipped)
	}
	fmt.Fprintln(os.Stderr, ".")

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"stored":  stored,
			"skipped": skipped,
		})
	}

	return nil
}
