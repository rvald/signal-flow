package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
)

// YouTubeActivity is a platform-agnostic representation of a YouTube activity.
// It decouples the harvester from the YouTube SDK, making it testable with mocks.
type YouTubeActivity struct {
	VideoID      string
	Title        string
	Description  string
	ChannelTitle string
	PublishedAt  time.Time
	Type         string // "upload", "like", "favorite", etc.
}

// YouTubeAPI abstracts the YouTube Data API v3 calls needed by the harvester.
// Implementations wrap the real SDK; tests inject a mock.
type YouTubeAPI interface {
	ListSubscriptionActivities(ctx context.Context, maxResults int64) ([]YouTubeActivity, error)
}

// YouTubeAuthError indicates a fatal authentication failure (e.g. 401/403).
// When the Coordinator receives this, it marks the credential as needs_reauth.
type YouTubeAuthError struct {
	Message string
}

func (e *YouTubeAuthError) Error() string {
	return e.Message
}

// NewYouTubeAuthError creates a new YouTubeAuthError.
func NewYouTubeAuthError(msg string) *YouTubeAuthError {
	return &YouTubeAuthError{Message: msg}
}

// YouTubeHarvester collects signals from YouTube Data API v3.
// Fetches subscription activity (new uploads) and converts them to RawSignals.
type YouTubeHarvester struct {
	api YouTubeAPI
}

// NewYouTubeHarvester creates a new YouTubeHarvester with the given API client.
func NewYouTubeHarvester(api YouTubeAPI) *YouTubeHarvester {
	return &YouTubeHarvester{api: api}
}

// Harvest fetches new uploads from YouTube subscription activity.
// Only "upload" type activities are included; likes, favorites, etc. are filtered out.
func (h *YouTubeHarvester) Harvest(ctx context.Context, cred *domain.Credential) ([]domain.RawSignal, error) {
	activities, err := h.api.ListSubscriptionActivities(ctx, 50)
	if err != nil {
		return nil, err
	}

	var signals []domain.RawSignal
	for _, a := range activities {
		// Only include upload activities — skip likes, favorites, etc.
		if a.Type != "upload" {
			continue
		}

		signals = append(signals, domain.RawSignal{
			SourceURL:   fmt.Sprintf("https://www.youtube.com/watch?v=%s", a.VideoID),
			Title:       a.Title,
			Content:     a.Description,
			Provider:    domain.ProviderYouTube,
			HarvestedAt: time.Now(),
			Metadata: map[string]any{
				"video_id":      a.VideoID,
				"channel_title": a.ChannelTitle,
				"published_at":  a.PublishedAt.Format(time.RFC3339),
			},
		})
	}

	return signals, nil
}

// Provider returns the YouTube provider identifier.
func (h *YouTubeHarvester) Provider() string {
	return domain.ProviderYouTube
}
