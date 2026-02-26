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
	var followsOnly bool

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Fetch links from your Bluesky timeline",
		Long: `Fetches your Bluesky home timeline and extracts posts that contain
external links (articles, repos, videos, etc.). Read-only — no database required.

Use --follows to filter to only posts from accounts you follow (all posts, not just links).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, session, err := resolveBlueskyClient(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Fetching timeline for %s...\n\n", session.Handle)

			resp, err := bsky.FeedGetTimeline(ctx, client, "", "", limit)
			if err != nil {
				return wrapExpiredTokenErr(fmt.Errorf("fetch timeline: %w", err))
			}

			// Convert SDK feed items to our timeline types.
			feedItems := sdkFeedToTimeline(resp.Feed)

			// --follows mode: filter to followed accounts, show all posts.
			if followsOnly {
				followDIDs, err := fetchFollowDIDs(ctx, client, session.DID)
				if err != nil {
					return wrapExpiredTokenErr(fmt.Errorf("fetch follows: %w", err))
				}
				fmt.Printf("Loaded %d follows.\n\n", len(followDIDs))

				filtered := auth.FilterByFollows(feedItems, followDIDs)
				if len(filtered) == 0 {
					fmt.Println("No posts from followed accounts in your timeline.")
					return nil
				}

				printAllPosts(filtered, asJSON)
				fmt.Printf("\nShowing %d posts from followed accounts (out of %d total).\n", len(filtered), len(resp.Feed))
				return nil
			}

			// Default mode: link extraction only.
			signals := auth.ExtractLinksFromFeed(feedItems)

			if len(signals) == 0 {
				fmt.Println("No links found in your timeline.")
				return nil
			}

			if asJSON {
				for _, s := range signals {
					author := ""
					if h, ok := s.Metadata["author_handle"].(string); ok {
						author = h
					}
					fmt.Printf("{\"url\": %q, \"title\": %q, \"author\": %q, \"provider\": %q}\n",
						s.SourceURL, s.Title, author, s.Provider)
				}
			} else {
				for i, s := range signals {
					author := ""
					if h, ok := s.Metadata["author_handle"].(string); ok {
						author = " (@" + h + ")"
					}
					fmt.Printf("  %d. %s%s\n", i+1, s.Title, author)
					fmt.Printf("     %s\n\n", s.SourceURL)
				}
			}

			fmt.Printf("Found %d links from %d posts.\n", len(signals), len(resp.Feed))
			return nil
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 50, "number of timeline posts to fetch")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON lines")
	cmd.Flags().BoolVar(&followsOnly, "follows", false, "show only posts from accounts you follow (all posts, not just links)")

	return cmd
}

// sdkFeedToTimeline converts SDK FeedViewPost items to our internal TimelineFeedItem types.
func sdkFeedToTimeline(feed []*bsky.FeedDefs_FeedViewPost) []auth.TimelineFeedItem {
	var items []auth.TimelineFeedItem
	for _, item := range feed {
		if item.Post == nil || item.Post.Record == nil {
			continue
		}

		fi := auth.TimelineFeedItem{
			Post: auth.TimelinePost{
				URI: item.Post.Uri,
				CID: item.Post.Cid,
			},
		}

		// Extract the author.
		if item.Post.Author != nil {
			displayName := ""
			if item.Post.Author.DisplayName != nil {
				displayName = *item.Post.Author.DisplayName
			}
			fi.Post.Author = &auth.PostAuthor{
				DID:         item.Post.Author.Did,
				Handle:      item.Post.Author.Handle,
				DisplayName: displayName,
			}
		}

		// Extract the post text.
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

		items = append(items, fi)
	}
	return items
}

// printAllPosts prints all posts (not just links) with author info.
func printAllPosts(items []auth.TimelineFeedItem, asJSON bool) {
	for i, item := range items {
		author := ""
		if item.Post.Author != nil {
			author = item.Post.Author.Handle
		}

		link := ""
		linkTitle := ""
		if item.Post.Embed != nil && item.Post.Embed.External != nil {
			link = item.Post.Embed.External.URI
			linkTitle = item.Post.Embed.External.Title
		}

		if asJSON {
			fmt.Printf("{\"author\": %q, \"text\": %q", author, item.Post.Record.Text)
			if link != "" {
				fmt.Printf(", \"link\": %q, \"link_title\": %q", link, linkTitle)
			}
			fmt.Println("}")
		} else {
			fmt.Printf("  %d. @%s\n", i+1, author)
			fmt.Printf("     %s\n", item.Post.Record.Text)
			if link != "" {
				fmt.Printf("     🔗 %s — %s\n", linkTitle, link)
			}
			fmt.Println()
		}
	}
}
