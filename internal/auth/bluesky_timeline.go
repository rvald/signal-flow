package auth

import (
	"strings"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
)

// TimelineFeedItem represents a single item from the Bluesky timeline response.
type TimelineFeedItem struct {
	Post TimelinePost `json:"post"`
}

// TimelinePost represents a Bluesky post in the timeline.
type TimelinePost struct {
	URI    string      `json:"uri"`    // at:// URI
	CID    string      `json:"cid"`    // Content ID
	Author *PostAuthor `json:"author"` // Post author
	Record PostRecord  `json:"record"` // Post content
	Embed  *PostEmbed  `json:"embed"`  // Embedded content (links, images, etc.)
}

// PostAuthor identifies the author of a Bluesky post.
type PostAuthor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name,omitempty"`
}

// PostRecord contains the text content of a Bluesky post.
type PostRecord struct {
	Type      string `json:"$type,omitempty"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// PostEmbed represents embedded content in a Bluesky post.
type PostEmbed struct {
	Type     string         `json:"$type"`
	External *EmbedExternal `json:"external,omitempty"`
}

// EmbedExternal represents an external link embed in a Bluesky post.
type EmbedExternal struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// TimelineResponse is the response from app.bsky.feed.getTimeline.
type TimelineResponse struct {
	Feed   []TimelineFeedItem `json:"feed"`
	Cursor string             `json:"cursor"`
}

// ExtractLinksFromFeed filters a Bluesky timeline feed for posts with
// external link embeds and converts them to RawSignals.
func ExtractLinksFromFeed(feed []TimelineFeedItem) []domain.RawSignal {
	var signals []domain.RawSignal

	for _, item := range feed {
		// Skip posts without embeds.
		if item.Post.Embed == nil {
			continue
		}

		// Only process external link embeds.
		if !strings.Contains(item.Post.Embed.Type, "external") {
			continue
		}

		// Must have an external link.
		if item.Post.Embed.External == nil || item.Post.Embed.External.URI == "" {
			continue
		}

		ext := item.Post.Embed.External
		metadata := map[string]any{
			"at_uri": item.Post.URI,
			"cid":    item.Post.CID,
		}

		if item.Post.Author != nil {
			metadata["author_did"] = item.Post.Author.DID
			metadata["author_handle"] = item.Post.Author.Handle
		}

		signals = append(signals, domain.RawSignal{
			SourceURL:   ext.URI,
			Title:       ext.Title,
			Content:     item.Post.Record.Text,
			Provider:    domain.ProviderBluesky,
			HarvestedAt: time.Now(),
			Metadata:    metadata,
		})
	}

	return signals
}

// FilterByFollows returns only feed items authored by accounts in the followDIDs set.
func FilterByFollows(feed []TimelineFeedItem, followDIDs map[string]bool) []TimelineFeedItem {
	var filtered []TimelineFeedItem
	for _, item := range feed {
		if item.Post.Author != nil && followDIDs[item.Post.Author.DID] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
