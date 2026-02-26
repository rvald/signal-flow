package auth_test

import (
	"testing"

	"github.com/rvald/signal-flow/internal/auth"
)

// =============================================================================
// Test_BlueskyHarvester_ExtractLinks
// Feed a mock timeline response and verify only posts with external embeds
// produce RawSignals.
// =============================================================================

func Test_BlueskyHarvester_ExtractLinks(t *testing.T) {
	// This tests the pure link extraction logic, no SDK or network needed.
	feed := []auth.TimelineFeedItem{
		{
			Post: auth.TimelinePost{
				URI: "at://did:plc:abc/app.bsky.feed.post/1",
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
				Record: auth.PostRecord{
					Text: "Just a thought, no links here.",
				},
			},
		},
		{
			Post: auth.TimelinePost{
				URI: "at://did:plc:abc/app.bsky.feed.post/3",
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

	if signals[0].SourceURL != "https://github.com/golang/go" {
		t.Errorf("signal[0] URL: got %q, want %q", signals[0].SourceURL, "https://github.com/golang/go")
	}
	if signals[0].Title != "Go Programming Language" {
		t.Errorf("signal[0] Title: got %q, want %q", signals[0].Title, "Go Programming Language")
	}

	if signals[1].SourceURL != "https://youtube.com/watch?v=123" {
		t.Errorf("signal[1] URL: got %q, want %q", signals[1].SourceURL, "https://youtube.com/watch?v=123")
	}
}
