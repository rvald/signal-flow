package domain

import (
	"context"
	"time"
)

// Provider constants for supported harvesting platforms.
const (
	ProviderBluesky = "bluesky"
	ProviderYouTube = "youtube"
	ProviderGitHub  = "github"
)

// RawSignal is unprocessed content harvested from an external platform.
// It is the input to the Synthesizer pipeline.
type RawSignal struct {
	SourceURL   string         `json:"source_url"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Provider    string         `json:"provider"` // e.g. "bluesky", "youtube", "github"
	HarvestedAt time.Time      `json:"harvested_at"`
	Metadata    map[string]any `json:"metadata"` // Platform-specific (video_id, cid, etc.)
}

// Harvester collects raw signals from a specific platform.
type Harvester interface {
	// Harvest fetches new content from the platform using the given credential.
	// The credential's LastSeenID is used as a cursor for incremental polling.
	Harvest(ctx context.Context, cred *Credential) ([]RawSignal, error)

	// Provider returns the platform identifier (e.g. "bluesky", "youtube", "github").
	Provider() string
}
