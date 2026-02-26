package auth_test

import (
	"testing"

	"github.com/rvald/signal-flow/internal/auth"
)

// =============================================================================
// Test_BlueskyHarvester_ExtractLinks
// Feed a mock timeline response and verify only posts with external embeds
// produce RawSignals, and author metadata is included.
// =============================================================================

func Test_BlueskyHarvester_ExtractLinks(t *testing.T) {
	feed := []auth.TimelineFeedItem{
		{
			Post: auth.TimelinePost{
				URI: "at://did:plc:abc/app.bsky.feed.post/1",
				Author: &auth.PostAuthor{
					DID:         "did:plc:abc",
					Handle:      "alice.bsky.social",
					DisplayName: "Alice",
				},
				Record: auth.PostRecord{
					Text: "Check out this cool project",
				},
				Embed: &auth.PostEmbed{
					Type: "app.bsky.embed.external#view",
					External: &auth.EmbedExternal{
						URI:   "https://github.com/golang/go",
						Title: "Go Programming Language",
					},
				},
			},
		},
		{
			// Post with no embed — should be skipped.
			Post: auth.TimelinePost{
				URI: "at://did:plc:abc/app.bsky.feed.post/2",
				Author: &auth.PostAuthor{
					DID:    "did:plc:abc",
					Handle: "alice.bsky.social",
				},
				Record: auth.PostRecord{
					Text: "Just a thought, no links here.",
				},
			},
		},
		{
			Post: auth.TimelinePost{
				URI: "at://did:plc:xyz/app.bsky.feed.post/3",
				Author: &auth.PostAuthor{
					DID:         "did:plc:xyz",
					Handle:      "bob.bsky.social",
					DisplayName: "Bob",
				},
				Record: auth.PostRecord{
					Text: "Cool video I found",
				},
				Embed: &auth.PostEmbed{
					Type: "app.bsky.embed.external#view",
					External: &auth.EmbedExternal{
						URI:   "https://youtube.com/watch?v=123",
						Title: "Cool Video",
					},
				},
			},
		},
		{
			// Post with non-external embed (e.g., image) — should be skipped.
			Post: auth.TimelinePost{
				URI: "at://did:plc:abc/app.bsky.feed.post/4",
				Record: auth.PostRecord{
					Text: "Look at this photo",
				},
				Embed: &auth.PostEmbed{
					Type: "app.bsky.embed.images#view",
				},
			},
		},
	}

	signals := auth.ExtractLinksFromFeed(feed)

	if len(signals) != 2 {
		t.Fatalf("expected 2 signals from feed, got %d", len(signals))
	}

	// Signal 0: Alice's post
	if signals[0].SourceURL != "https://github.com/golang/go" {
		t.Errorf("signal[0] URL: got %q, want %q", signals[0].SourceURL, "https://github.com/golang/go")
	}
	if signals[0].Metadata["author_handle"] != "alice.bsky.social" {
		t.Errorf("signal[0] author_handle: got %q, want %q", signals[0].Metadata["author_handle"], "alice.bsky.social")
	}
	if signals[0].Metadata["author_did"] != "did:plc:abc" {
		t.Errorf("signal[0] author_did: got %q, want %q", signals[0].Metadata["author_did"], "did:plc:abc")
	}

	// Signal 1: Bob's post
	if signals[1].SourceURL != "https://youtube.com/watch?v=123" {
		t.Errorf("signal[1] URL: got %q, want %q", signals[1].SourceURL, "https://youtube.com/watch?v=123")
	}
	if signals[1].Metadata["author_handle"] != "bob.bsky.social" {
		t.Errorf("signal[1] author_handle: got %q, want %q", signals[1].Metadata["author_handle"], "bob.bsky.social")
	}
}

// =============================================================================
// Test_FilterByFollows
// Only posts from followed accounts should be returned.
// =============================================================================

func Test_FilterByFollows(t *testing.T) {
	feed := []auth.TimelineFeedItem{
		{
			Post: auth.TimelinePost{
				URI:    "at://did:plc:alice/app.bsky.feed.post/1",
				Author: &auth.PostAuthor{DID: "did:plc:alice", Handle: "alice.bsky.social"},
				Record: auth.PostRecord{Text: "Hello from Alice"},
			},
		},
		{
			Post: auth.TimelinePost{
				URI:    "at://did:plc:bob/app.bsky.feed.post/2",
				Author: &auth.PostAuthor{DID: "did:plc:bob", Handle: "bob.bsky.social"},
				Record: auth.PostRecord{Text: "Hello from Bob"},
			},
		},
		{
			Post: auth.TimelinePost{
				URI:    "at://did:plc:carol/app.bsky.feed.post/3",
				Author: &auth.PostAuthor{DID: "did:plc:carol", Handle: "carol.bsky.social"},
				Record: auth.PostRecord{Text: "Hello from Carol"},
			},
		},
		{
			// Post with nil author — should be excluded.
			Post: auth.TimelinePost{
				URI:    "at://did:plc:unknown/app.bsky.feed.post/4",
				Record: auth.PostRecord{Text: "Ghost post"},
			},
		},
	}

	followDIDs := map[string]bool{
		"did:plc:alice": true,
		"did:plc:carol": true,
	}

	filtered := auth.FilterByFollows(feed, followDIDs)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(filtered))
	}
	if filtered[0].Post.Author.Handle != "alice.bsky.social" {
		t.Errorf("filtered[0] handle: got %q, want %q", filtered[0].Post.Author.Handle, "alice.bsky.social")
	}
	if filtered[1].Post.Author.Handle != "carol.bsky.social" {
		t.Errorf("filtered[1] handle: got %q, want %q", filtered[1].Post.Author.Handle, "carol.bsky.social")
	}
}
