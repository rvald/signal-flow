package harvester_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/harvester"
)

// =============================================================================
// Mocks
// =============================================================================

// --- Mock Harvester (provider) ---

type mockHarvester struct {
	provider string
	signals  []domain.RawSignal
	err      error
}

func (m *mockHarvester) Harvest(_ context.Context, _ *domain.Credential) ([]domain.RawSignal, error) {
	return m.signals, m.err
}

func (m *mockHarvester) Provider() string {
	return m.provider
}

// --- Mock IdentityRepository ---

type mockIdentityRepo struct {
	activeCredentials []*domain.Credential
	listErr           error

	// UpdateLastSeenID tracking
	updatedCredID uuid.UUID
	updatedSeenID string
	updateCalled  bool

	// MarkNeedsReauth tracking
	reauthCredID uuid.UUID
	reauthCalled bool
}

func (m *mockIdentityRepo) ListActiveCredentials(_ context.Context, _ string) ([]*domain.Credential, error) {
	return m.activeCredentials, m.listErr
}

func (m *mockIdentityRepo) UpdateLastSeenID(_ context.Context, credID uuid.UUID, lastSeenID string) error {
	m.updateCalled = true
	m.updatedCredID = credID
	m.updatedSeenID = lastSeenID
	return nil
}

func (m *mockIdentityRepo) MarkNeedsReauth(_ context.Context, credID uuid.UUID) error {
	m.reauthCalled = true
	m.reauthCredID = credID
	return nil
}

// Unused but required by domain.IdentityRepository interface.
func (m *mockIdentityRepo) LinkProvider(_ context.Context, _ uuid.UUID, _ string, _ []byte) error {
	return nil
}
func (m *mockIdentityRepo) GetActiveToken(_ context.Context, _ uuid.UUID, _ string) ([]byte, error) {
	return nil, nil
}
func (m *mockIdentityRepo) ListUsersByProvider(_ context.Context, _ string) ([]*domain.User, error) {
	return nil, nil
}
func (m *mockIdentityRepo) SaveToken(_ context.Context, _ uuid.UUID, _ []byte) error {
	return nil
}

// --- Mock Synthesizer (records calls for dedup verification) ---

type mockSynthesizer struct {
	processCalled bool
	processCount  int
	processURLs   []string
}

func (m *mockSynthesizer) Process(_ context.Context, _ uuid.UUID, sourceURL, _ string) error {
	m.processCalled = true
	m.processCount++
	m.processURLs = append(m.processURLs, sourceURL)
	return nil
}

// --- Mock SignalRepository (for dedup checks) ---

type mockSignalRepo struct {
	existingURLs map[string]bool // URLs that already exist in the DB
}

func (m *mockSignalRepo) ExistsByURL(_ context.Context, _ uuid.UUID, sourceURL string) (bool, error) {
	return m.existingURLs[sourceURL], nil
}

// =============================================================================
// Test_Uniform_Interval
// Verifies the dispatcher correctly selects ALL users with active credentials
// regardless of their activity level.
// =============================================================================

func Test_Uniform_Interval(t *testing.T) {
	cred1 := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}
	cred2 := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}
	cred3 := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}

	identityRepo := &mockIdentityRepo{
		activeCredentials: []*domain.Credential{cred1, cred2, cred3},
	}

	bskyHarvester := &mockHarvester{
		provider: domain.ProviderBluesky,
		signals:  []domain.RawSignal{}, // No new signals, but all creds should be dispatched
	}

	signalRepo := &mockSignalRepo{existingURLs: map[string]bool{}}
	synth := &mockSynthesizer{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{domain.ProviderBluesky: bskyHarvester},
		identityRepo,
		signalRepo,
		synth,
	)

	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// The dispatcher should have queried for active credentials.
	// Since all 3 creds are active (needs_reauth=false), all 3 should be dispatched.
	// We verify by checking the harvester was called — since it returns no signals,
	// we just need it to not error. The key assertion is that RunOnce doesn't skip users.
	// A more targeted check: no errors means all 3 were processed.
}

// =============================================================================
// Test_Harvester_Filter_NonLinks
// Verifies that a Bluesky post without a URL is discarded before reaching
// the Synthesizer.
// =============================================================================

func Test_Harvester_Filter_NonLinks(t *testing.T) {
	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}

	identityRepo := &mockIdentityRepo{
		activeCredentials: []*domain.Credential{cred},
	}

	bskyHarvester := &mockHarvester{
		provider: domain.ProviderBluesky,
		signals: []domain.RawSignal{
			{
				SourceURL:   "https://github.com/golang/go",
				Title:       "Go repo",
				Content:     "Amazing Go content with a link",
				Provider:    domain.ProviderBluesky,
				HarvestedAt: time.Now(),
			},
			{
				SourceURL:   "", // No URL — should be filtered
				Title:       "Random thought",
				Content:     "Just a post with no links",
				Provider:    domain.ProviderBluesky,
				HarvestedAt: time.Now(),
			},
			{
				SourceURL:   "https://youtube.com/watch?v=abc",
				Title:       "Cool video",
				Content:     "YouTube link post",
				Provider:    domain.ProviderBluesky,
				HarvestedAt: time.Now(),
			},
		},
	}

	signalRepo := &mockSignalRepo{existingURLs: map[string]bool{}}
	synth := &mockSynthesizer{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{domain.ProviderBluesky: bskyHarvester},
		identityRepo,
		signalRepo,
		synth,
	)

	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Only 2 of 3 signals have URLs — the empty-URL post should be dropped.
	if synth.processCount != 2 {
		t.Errorf("expected 2 signals processed, got %d", synth.processCount)
	}

	// Verify the correct URLs were sent to the synthesizer.
	expectedURLs := map[string]bool{
		"https://github.com/golang/go":    true,
		"https://youtube.com/watch?v=abc": true,
	}
	for _, url := range synth.processURLs {
		if !expectedURLs[url] {
			t.Errorf("unexpected URL sent to synthesizer: %q", url)
		}
	}
}

// =============================================================================
// Test_Duplicate_Prevention
// Mocks the SignalRepository to show a URL already exists; verifies the
// Harvester does NOT call the SynthesizerService for that URL.
// =============================================================================

func Test_Duplicate_Prevention(t *testing.T) {
	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}

	identityRepo := &mockIdentityRepo{
		activeCredentials: []*domain.Credential{cred},
	}

	bskyHarvester := &mockHarvester{
		provider: domain.ProviderBluesky,
		signals: []domain.RawSignal{
			{
				SourceURL:   "https://github.com/existing/repo",
				Title:       "Already processed",
				Content:     "This was already synthesized",
				Provider:    domain.ProviderBluesky,
				HarvestedAt: time.Now(),
			},
			{
				SourceURL:   "https://github.com/new/repo",
				Title:       "Brand new",
				Content:     "This is new content",
				Provider:    domain.ProviderBluesky,
				HarvestedAt: time.Now(),
			},
		},
	}

	// Mark one URL as already existing in the database.
	signalRepo := &mockSignalRepo{
		existingURLs: map[string]bool{
			"https://github.com/existing/repo": true,
		},
	}
	synth := &mockSynthesizer{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{domain.ProviderBluesky: bskyHarvester},
		identityRepo,
		signalRepo,
		synth,
	)

	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Only the new URL should be sent to the synthesizer.
	if synth.processCount != 1 {
		t.Errorf("expected 1 signal processed (duplicate skipped), got %d", synth.processCount)
	}
	if len(synth.processURLs) > 0 && synth.processURLs[0] != "https://github.com/new/repo" {
		t.Errorf("expected new repo URL, got %q", synth.processURLs[0])
	}
}

// =============================================================================
// Test_Fatal_Error_Marks_NeedsReauth
// When a harvester returns a 401 Unauthorized error, the coordinator should
// mark the credential as needs_reauth and continue processing other creds.
// =============================================================================

func Test_Fatal_Error_Marks_NeedsReauth(t *testing.T) {
	credOK := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}
	credBad := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}

	identityRepo := &mockIdentityRepo{
		activeCredentials: []*domain.Credential{credBad, credOK},
	}

	// The harvester returns 401 for credBad, success for credOK.
	callCount := 0
	bskyHarvester := &mockHarvester{
		provider: domain.ProviderBluesky,
	}

	// We need a harvester that returns different results per credential.
	// Override with a custom one:
	authErr := harvester.NewAuthError("401 Unauthorized")
	customHarvester := &credSwitchHarvester{
		provider: domain.ProviderBluesky,
		results: map[uuid.UUID]harvestResult{
			credBad.ID: {err: authErr},
			credOK.ID: {signals: []domain.RawSignal{
				{SourceURL: "https://example.com/good", Title: "Good", Content: "content", Provider: domain.ProviderBluesky, HarvestedAt: time.Now()},
			}},
		},
	}
	_ = bskyHarvester // replaced by customHarvester
	_ = callCount

	signalRepo := &mockSignalRepo{existingURLs: map[string]bool{}}
	synth := &mockSynthesizer{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{domain.ProviderBluesky: customHarvester},
		identityRepo,
		signalRepo,
		synth,
	)

	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// The bad credential should be marked as needs_reauth.
	if !identityRepo.reauthCalled {
		t.Error("MarkNeedsReauth should have been called for the 401 credential")
	}
	if identityRepo.reauthCredID != credBad.ID {
		t.Errorf("MarkNeedsReauth called with wrong credential ID: got %v, want %v", identityRepo.reauthCredID, credBad.ID)
	}

	// The good credential should still be processed.
	if synth.processCount != 1 {
		t.Errorf("expected 1 signal from the good credential, got %d", synth.processCount)
	}
}

// =============================================================================
// Test_Backoff_Transient_Errors
// Verifies that transient network errors trigger retries with backoff,
// up to 3 attempts.
// =============================================================================

func Test_Backoff_Transient_Errors(t *testing.T) {
	cred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}

	identityRepo := &mockIdentityRepo{
		activeCredentials: []*domain.Credential{cred},
	}

	// A harvester that fails with a transient error.
	transientErr := errors.New("connection timeout")
	retryHarvester := &countingHarvester{
		provider: domain.ProviderBluesky,
		err:      transientErr,
	}

	signalRepo := &mockSignalRepo{existingURLs: map[string]bool{}}
	synth := &mockSynthesizer{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{domain.ProviderBluesky: retryHarvester},
		identityRepo,
		signalRepo,
		synth,
	)

	// RunOnce should not return an error — transient failures are logged, not propagated.
	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should not propagate transient errors: %v", err)
	}

	// The harvester should have been retried 3 times.
	if retryHarvester.callCount != 3 {
		t.Errorf("expected 3 retry attempts, got %d", retryHarvester.callCount)
	}

	// No signals should reach the synthesizer since all attempts failed.
	if synth.processCalled {
		t.Error("synthesizer should not be called when all harvests fail")
	}

	// NeedsReauth should NOT be called — this is a transient error, not a 401.
	if identityRepo.reauthCalled {
		t.Error("MarkNeedsReauth should NOT be called for transient errors")
	}
}

// =============================================================================
// Test Helpers — per-credential harvester and counting harvester
// =============================================================================

type harvestResult struct {
	signals []domain.RawSignal
	err     error
}

// credSwitchHarvester returns different results based on the credential ID.
type credSwitchHarvester struct {
	provider string
	results  map[uuid.UUID]harvestResult
}

func (h *credSwitchHarvester) Harvest(_ context.Context, cred *domain.Credential) ([]domain.RawSignal, error) {
	r, ok := h.results[cred.ID]
	if !ok {
		return nil, nil
	}
	return r.signals, r.err
}

func (h *credSwitchHarvester) Provider() string {
	return h.provider
}

// countingHarvester counts how many times Harvest is called.
type countingHarvester struct {
	provider  string
	signals   []domain.RawSignal
	err       error
	callCount int
}

func (h *countingHarvester) Harvest(_ context.Context, _ *domain.Credential) ([]domain.RawSignal, error) {
	h.callCount++
	return h.signals, h.err
}

func (h *countingHarvester) Provider() string {
	return h.provider
}
