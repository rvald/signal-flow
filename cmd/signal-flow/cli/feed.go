package cli

import (
	"fmt"

	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/spf13/cobra"
)

func newFeedCmd() *cobra.Command {
	var limit int64
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Fetch links from your Bluesky timeline",
		Long: `Fetches your Bluesky home timeline and extracts posts that contain
external links (articles, repos, videos, etc.). Read-only — no database required.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, session, err := resolveBlueskyClient(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Fetching timeline for %s...\n\n", session.Handle)

			cursor := ""
			resp, err := bsky.FeedGetTimeline(ctx, client, "", cursor, limit)
			if err != nil {
				return fmt.Errorf("fetch timeline: %w", err)
			}

			// Convert the SDK feed items to our timeline types for extraction.
			var feedItems []auth.TimelineFeedItem
			for _, item := range resp.Feed {
				if item.Post == nil || item.Post.Record == nil {
					continue
				}

				fi := auth.TimelineFeedItem{
					Post: auth.TimelinePost{
						URI: item.Post.Uri,
						CID: item.Post.Cid,
					},
				}

				// Extract the post text from the record.
				if rec, ok := item.Post.Record.Val.(*bsky.FeedPost); ok {
					fi.Post.Record = auth.PostRecord{
						Text: rec.Text,
					}
				}

				// Extract external embeds.
				if item.Post.Embed != nil {
					if ext := item.Post.Embed.EmbedExternal_View; ext != nil && ext.External != nil {
						fi.Post.Embed = &auth.PostEmbed{
							Type: "app.bsky.embed.external#view",
							External: &auth.EmbedExternal{
								URI:         ext.External.Uri,
								Title:       ext.External.Title,
								Description: ext.External.Description,
							},
						}
					}
				}

				feedItems = append(feedItems, fi)
			}

			signals := auth.ExtractLinksFromFeed(feedItems)

			if len(signals) == 0 {
				fmt.Println("No links found in your timeline.")
				return nil
			}

			if asJSON {
				for _, s := range signals {
					fmt.Printf("{\"url\": %q, \"title\": %q, \"provider\": %q}\n", s.SourceURL, s.Title, s.Provider)
				}
			} else {
				for i, s := range signals {
					fmt.Printf("  %d. %s\n", i+1, s.Title)
					fmt.Printf("     %s\n\n", s.SourceURL)
				}
			}

			fmt.Printf("Found %d links from %d posts.\n", len(signals), len(resp.Feed))

			return nil
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 50, "number of timeline posts to fetch")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON lines")

	return cmd
}
