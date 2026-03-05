package cli

import (
	"context"
	"fmt"
	"os"

	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/outfmt"
	"github.com/spf13/cobra"
)

func newFeedCmd() *cobra.Command {
	var limit int64
	var followsOnly bool

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Fetch links from your Bluesky timeline",
		Long: `Fetches your Bluesky home timeline and extracts posts that contain
external links (articles, repos, videos, etc.). Read-only — no database required.

Use --follows to filter to only posts from accounts you follow (all posts, not just links).`,
		Example: `  # Show links from your timeline
  signal-flow feed

  # Show all posts from followed accounts as JSON
  signal-flow feed --follows --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, session, err := resolveBlueskyClient(ctx)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Fetching timeline for %s...\n\n", session.Handle)

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
				fmt.Fprintf(os.Stderr, "Loaded %d follows.\n\n", len(followDIDs))

				filtered := auth.FilterByFollows(feedItems, followDIDs)
				if len(filtered) == 0 {
					fmt.Fprintln(os.Stderr, "No posts from followed accounts in your timeline.")
					return nil
				}

				printAllPosts(ctx, filtered)
				fmt.Fprintf(os.Stderr, "\nShowing %d posts from followed accounts (out of %d total).\n", len(filtered), len(resp.Feed))
				return nil
			}

			// Default mode: link extraction only.
			signals := auth.ExtractLinksFromFeed(feedItems)

			if len(signals) == 0 {
				fmt.Fprintln(os.Stderr, "No links found in your timeline.")
				return nil
			}

			if outfmt.IsJSON(ctx) {
				type signalJSON struct {
					URL      string `json:"url"`
					Title    string `json:"title"`
					Author   string `json:"author,omitempty"`
					Provider string `json:"provider"`
				}
				items := make([]signalJSON, 0, len(signals))
				for _, s := range signals {
					author := ""
					if h, ok := s.Metadata["author_handle"].(string); ok {
						author = h
					}
					items = append(items, signalJSON{
						URL:      s.SourceURL,
						Title:    s.Title,
						Author:   author,
						Provider: string(s.Provider),
					})
				}
				return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"signals": items})
			}

			for i, s := range signals {
				author := ""
				if h, ok := s.Metadata["author_handle"].(string); ok {
					author = " (@" + h + ")"
				}
				fmt.Printf("  %d. %s%s\n", i+1, s.Title, author)
				fmt.Printf("     %s\n\n", s.SourceURL)
			}

			fmt.Fprintf(os.Stderr, "Found %d links from %d posts.\n", len(signals), len(resp.Feed))
			return nil
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 50, "number of timeline posts to fetch")
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
func printAllPosts(ctx context.Context, items []auth.TimelineFeedItem) {
	if outfmt.IsJSON(ctx) {
		type postJSON struct {
			Author    string `json:"author"`
			Text      string `json:"text"`
			Link      string `json:"link,omitempty"`
			LinkTitle string `json:"link_title,omitempty"`
		}
		posts := make([]postJSON, 0, len(items))
		for _, item := range items {
			author := ""
			if item.Post.Author != nil {
				author = item.Post.Author.Handle
			}
			p := postJSON{Author: author, Text: item.Post.Record.Text}
			if item.Post.Embed != nil && item.Post.Embed.External != nil {
				p.Link = item.Post.Embed.External.URI
				p.LinkTitle = item.Post.Embed.External.Title
			}
			posts = append(posts, p)
		}
		_ = outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"posts": posts})
		return
	}

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

		fmt.Printf("  %d. @%s\n", i+1, author)
		fmt.Printf("     %s\n", item.Post.Record.Text)
		if link != "" {
			fmt.Printf("     🔗 %s — %s\n", linkTitle, link)
		}
		fmt.Println()
	}
}
