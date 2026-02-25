package providers

import (
	"context"

	"github.com/rvald/signal-flow/internal/domain"
)

// BlueskyHarvester collects signals from the AT Protocol (Bluesky).
// Uses app.bsky.feed.getTimeline to fetch the user's feed,
// filtering for posts with external URIs or record links.
type BlueskyHarvester struct {
	// Client will be injected when AT Protocol SDK is integrated.
}

// NewBlueskyHarvester creates a new BlueskyHarvester.
func NewBlueskyHarvester() *BlueskyHarvester {
	return &BlueskyHarvester{}
}

// Harvest fetches new posts from Bluesky and filters for link-containing posts.
// Uses cred.LastSeenID as the cursor (CID of the most recent processed post).
func (h *BlueskyHarvester) Harvest(ctx context.Context, cred *domain.Credential) ([]domain.RawSignal, error) {
	// TODO: Implement using github.com/bluesky-social/indigo
	// 1. Authenticate with cred token
	// 2. Call app.bsky.feed.getTimeline with cursor = cred.LastSeenID
	// 3. Filter: only posts where Post.Record.Embed has external URI or record link
	// 4. Convert to []domain.RawSignal
	return nil, nil
}

// Provider returns the Bluesky provider identifier.
func (h *BlueskyHarvester) Provider() string {
	return domain.ProviderBluesky
}
