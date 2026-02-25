package providers

import (
	"context"

	"github.com/rvald/signal-flow/internal/domain"
)

// YouTubeHarvester collects signals from YouTube Data API v3.
// Polls the user's Watch Later playlist or specified subscription list.
type YouTubeHarvester struct {
	// Client will be injected when youtube/v3 SDK is integrated.
}

// NewYouTubeHarvester creates a new YouTubeHarvester.
func NewYouTubeHarvester() *YouTubeHarvester {
	return &YouTubeHarvester{}
}

// Harvest fetches new videos from YouTube.
// Captures VideoID, Snippet.Title, Snippet.Description, and Snippet.PublishedAt.
func (h *YouTubeHarvester) Harvest(ctx context.Context, cred *domain.Credential) ([]domain.RawSignal, error) {
	// TODO: Implement using google.golang.org/api/youtube/v3
	// 1. Authenticate with cred token (OAuth2)
	// 2. List items from WL playlist or subscriptions
	// 3. Capture VideoID, Title, Description, PublishedAt
	// 4. Convert to []domain.RawSignal with SourceURL = youtube.com/watch?v={VideoID}
	return nil, nil
}

// Provider returns the YouTube provider identifier.
func (h *YouTubeHarvester) Provider() string {
	return domain.ProviderYouTube
}
