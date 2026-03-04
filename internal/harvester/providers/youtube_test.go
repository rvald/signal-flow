package providers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/harvester/providers"
)

// =============================================================================
// Mock YouTubeAPI
// =============================================================================

type mockYouTubeAPI struct {
	activities []providers.YouTubeActivity
	err        error
}

func (m *mockYouTubeAPI) ListSubscriptionActivities(ctx context.Context, maxResults int64) ([]providers.YouTubeActivity, error) {
	return m.activities, m.err
}

// =============================================================================
// Test_YouTubeHarvester_ConvertActivities
// Verifies that YouTube activity items are correctly converted to RawSignals
// with proper URLs, titles, and metadata.
// =============================================================================

func Test_YouTubeHarvester_ConvertActivities(t *testing.T) {
	published := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	api := &mockYouTubeAPI{
		activities: []providers.YouTubeActivity{
			{
				VideoID:      "dQw4w9WgXcQ",
				Title:        "Go 1.25 Release Notes",
				Description:  "What's new in Go 1.25",
				ChannelTitle: "Go Team",
				PublishedAt:  published,
				Type:         "upload",
			},
			{
				VideoID:      "abc123def45",
				Title:        "Building CLI Tools in Go",
				Description:  "A deep dive into Cobra",
				ChannelTitle: "TechTalks",
				PublishedAt:  published.Add(1 * time.Hour),
				Type:         "upload",
			},
		},
	}

	h := providers.NewYouTubeHarvester(api)

	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderYouTube,
	}

	signals, err := h.Harvest(context.Background(), cred)
	if err != nil {
		t.Fatalf("Harvest() error: %v", err)
	}

	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}

	// Verify first signal.
	s := signals[0]
	expectedURL := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	if s.SourceURL != expectedURL {
		t.Errorf("SourceURL = %q, want %q", s.SourceURL, expectedURL)
	}
	if s.Title != "Go 1.25 Release Notes" {
		t.Errorf("Title = %q, want %q", s.Title, "Go 1.25 Release Notes")
	}
	if s.Content != "What's new in Go 1.25" {
		t.Errorf("Content = %q, want %q", s.Content, "What's new in Go 1.25")
	}
	if s.Provider != domain.ProviderYouTube {
		t.Errorf("Provider = %q, want %q", s.Provider, domain.ProviderYouTube)
	}

	// Verify metadata.
	if vid, ok := s.Metadata["video_id"].(string); !ok || vid != "dQw4w9WgXcQ" {
		t.Errorf("Metadata[video_id] = %v, want %q", s.Metadata["video_id"], "dQw4w9WgXcQ")
	}
	if ch, ok := s.Metadata["channel_title"].(string); !ok || ch != "Go Team" {
		t.Errorf("Metadata[channel_title] = %v, want %q", s.Metadata["channel_title"], "Go Team")
	}

	// Verify second signal URL.
	expectedURL2 := "https://www.youtube.com/watch?v=abc123def45"
	if signals[1].SourceURL != expectedURL2 {
		t.Errorf("signals[1].SourceURL = %q, want %q", signals[1].SourceURL, expectedURL2)
	}
}

// =============================================================================
// Test_YouTubeHarvester_EmptyActivities
// Verifies that an empty activity list returns an empty slice and no error.
// =============================================================================

func Test_YouTubeHarvester_EmptyActivities(t *testing.T) {
	api := &mockYouTubeAPI{
		activities: []providers.YouTubeActivity{},
	}

	h := providers.NewYouTubeHarvester(api)
	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderYouTube,
	}

	signals, err := h.Harvest(context.Background(), cred)
	if err != nil {
		t.Fatalf("Harvest() error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected 0 signals, got %d", len(signals))
	}
}

// =============================================================================
// Test_YouTubeHarvester_SkipNonUpload
// Verifies that only "upload" type activities are converted to signals.
// Other activity types (like, favorite, etc.) are filtered out.
// =============================================================================

func Test_YouTubeHarvester_SkipNonUpload(t *testing.T) {
	api := &mockYouTubeAPI{
		activities: []providers.YouTubeActivity{
			{
				VideoID:      "upload1",
				Title:        "Uploaded Video",
				Description:  "A new upload",
				ChannelTitle: "Creator",
				PublishedAt:  time.Now(),
				Type:         "upload",
			},
			{
				VideoID:      "liked1",
				Title:        "Liked Video",
				Description:  "A liked video",
				ChannelTitle: "Other",
				PublishedAt:  time.Now(),
				Type:         "like",
			},
			{
				VideoID:      "fav1",
				Title:        "Favorited Video",
				Description:  "A fav",
				ChannelTitle: "Other",
				PublishedAt:  time.Now(),
				Type:         "favorite",
			},
			{
				VideoID:      "upload2",
				Title:        "Another Upload",
				Description:  "Another new upload",
				ChannelTitle: "Creator2",
				PublishedAt:  time.Now(),
				Type:         "upload",
			},
		},
	}

	h := providers.NewYouTubeHarvester(api)
	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderYouTube,
	}

	signals, err := h.Harvest(context.Background(), cred)
	if err != nil {
		t.Fatalf("Harvest() error: %v", err)
	}

	if len(signals) != 2 {
		t.Errorf("expected 2 upload signals, got %d", len(signals))
	}

	// Verify only upload videos came through.
	for _, s := range signals {
		vid := s.Metadata["video_id"].(string)
		if vid != "upload1" && vid != "upload2" {
			t.Errorf("unexpected video_id %q in results", vid)
		}
	}
}

// =============================================================================
// Test_YouTubeHarvester_ErrorHandling
// Verifies that API errors are returned to the caller.
// Transient errors are plain errors, 401s are wrapped as AuthError.
// =============================================================================

func Test_YouTubeHarvester_ErrorHandling(t *testing.T) {
	t.Run("transient error", func(t *testing.T) {
		api := &mockYouTubeAPI{
			err: errors.New("connection timeout"),
		}

		h := providers.NewYouTubeHarvester(api)
		cred := &domain.Credential{
			ID:       uuid.New(),
			UserID:   uuid.New(),
			Provider: domain.ProviderYouTube,
		}

		_, err := h.Harvest(context.Background(), cred)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Should NOT be an AuthError for transient errors.
		var authErr *providers.YouTubeAuthError
		if errors.As(err, &authErr) {
			t.Error("transient error should not be an AuthError")
		}
	})

	t.Run("auth error", func(t *testing.T) {
		api := &mockYouTubeAPI{
			err: providers.NewYouTubeAuthError("401 Unauthorized: token revoked"),
		}

		h := providers.NewYouTubeHarvester(api)
		cred := &domain.Credential{
			ID:       uuid.New(),
			UserID:   uuid.New(),
			Provider: domain.ProviderYouTube,
		}

		_, err := h.Harvest(context.Background(), cred)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Should be an AuthError for auth failures.
		var authErr *providers.YouTubeAuthError
		if !errors.As(err, &authErr) {
			t.Errorf("expected AuthError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Test_YouTubeHarvester_Provider
// Verifies that Provider() returns the correct identifier.
// =============================================================================

func Test_YouTubeHarvester_Provider(t *testing.T) {
	h := providers.NewYouTubeHarvester(nil)
	if h.Provider() != domain.ProviderYouTube {
		t.Errorf("Provider() = %q, want %q", h.Provider(), domain.ProviderYouTube)
	}
}
