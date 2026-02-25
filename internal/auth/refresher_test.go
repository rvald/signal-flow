package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/harvester"
	"github.com/rvald/signal-flow/internal/security"
)

// =============================================================================
// Mocks
// =============================================================================

// mockKMS wraps a real LocalKeyManager for deterministic encrypt/decrypt.
type mockKMS struct {
	inner security.KeyManager
}

func newMockKMS() *mockKMS {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	km, _ := security.NewLocalKeyManager(key)
	return &mockKMS{inner: km}
}

func (m *mockKMS) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	return m.inner.Encrypt(ctx, plaintext)
}

func (m *mockKMS) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	return m.inner.Decrypt(ctx, ciphertext)
}

// mockTokenEndpoint simulates an OAuth2 token endpoint.
// It records calls and returns configurable responses.
type mockTokenEndpoint struct {
	called      bool
	callCount   int
	returnToken *auth.TokenData
	returnErr   error
}

func (m *mockTokenEndpoint) ExchangeRefresh(_ context.Context, refreshToken string) (*auth.TokenData, error) {
	m.called = true
	m.callCount++
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.returnToken, nil
}

// mockTokenSaver records SaveToken calls for verification.
type mockTokenSaver struct {
	savedCredID uuid.UUID
	savedToken  []byte
	saveCalled  bool
	saveErr     error
}

func (m *mockTokenSaver) SaveToken(_ context.Context, credID uuid.UUID, encryptedToken []byte) error {
	m.saveCalled = true
	m.savedCredID = credID
	m.savedToken = encryptedToken
	return m.saveErr
}

// =============================================================================
// Helper: create an encrypted TokenData and embed it in a Credential.
// =============================================================================

func makeCredentialWithToken(t *testing.T, kms *mockKMS, td *auth.TokenData) *domain.Credential {
	t.Helper()

	raw, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal token data: %v", err)
	}

	encrypted, err := kms.Encrypt(context.Background(), raw)
	if err != nil {
		t.Fatalf("encrypt token data: %v", err)
	}

	return &domain.Credential{
		ID:             uuid.New(),
		UserID:         uuid.New(),
		Provider:       domain.ProviderYouTube,
		EncryptedToken: encrypted,
	}
}

// =============================================================================
// Test_Token_Rotation_Logic
// Mock an expired TokenData. Verify the refresher calls the token endpoint
// and returns a new access_token. Verify SaveToken is called with
// re-encrypted data.
// =============================================================================

func Test_Token_Rotation_Logic(t *testing.T) {
	kms := newMockKMS()

	// Create an expired token.
	expired := &auth.TokenData{
		AccessToken:  "old-access",
		RefreshToken: "valid-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired 1 hour ago
	}
	cred := makeCredentialWithToken(t, kms, expired)

	// Mock token endpoint returns a fresh token.
	newToken := &auth.TokenData{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	endpoint := &mockTokenEndpoint{returnToken: newToken}
	saver := &mockTokenSaver{}

	refresher := auth.NewOAuth2Refresher(kms, saver, endpoint)

	accessToken, err := refresher.Refresh(context.Background(), cred)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Should return the NEW access token, not the old one.
	if accessToken != "new-access" {
		t.Errorf("expected new-access, got %q", accessToken)
	}

	// The token endpoint should have been called.
	if !endpoint.called {
		t.Error("token endpoint ExchangeRefresh was not called")
	}

	// SaveToken should have been called to persist the re-encrypted token.
	if !saver.saveCalled {
		t.Error("SaveToken was not called after rotation")
	}
	if saver.savedCredID != cred.ID {
		t.Errorf("SaveToken called with wrong credential ID: got %v, want %v", saver.savedCredID, cred.ID)
	}

	// Verify the saved bytes are encrypted (not raw JSON).
	var check auth.TokenData
	if err := json.Unmarshal(saver.savedToken, &check); err == nil {
		t.Error("saved token should be encrypted, but it looks like valid JSON")
	}
}

// =============================================================================
// Test_Encryption_Boundary
// Verify the refresher always encrypts before calling repo save, and that
// the saved bytes can be decrypted back to valid TokenData.
// =============================================================================

func Test_Encryption_Boundary(t *testing.T) {
	kms := newMockKMS()

	expired := &auth.TokenData{
		AccessToken:  "old-access",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-30 * time.Minute),
	}
	cred := makeCredentialWithToken(t, kms, expired)

	newToken := &auth.TokenData{
		AccessToken:  "rotated-access",
		RefreshToken: "rotated-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	endpoint := &mockTokenEndpoint{returnToken: newToken}
	saver := &mockTokenSaver{}

	refresher := auth.NewOAuth2Refresher(kms, saver, endpoint)

	_, err := refresher.Refresh(context.Background(), cred)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Decrypt the saved token and verify it round-trips to valid TokenData.
	decrypted, err := kms.Decrypt(context.Background(), saver.savedToken)
	if err != nil {
		t.Fatalf("failed to decrypt saved token: %v", err)
	}

	var roundTripped auth.TokenData
	if err := json.Unmarshal(decrypted, &roundTripped); err != nil {
		t.Fatalf("saved token is not valid TokenData JSON: %v", err)
	}

	if roundTripped.AccessToken != "rotated-access" {
		t.Errorf("round-tripped access token: got %q, want %q", roundTripped.AccessToken, "rotated-access")
	}
	if roundTripped.RefreshToken != "rotated-refresh" {
		t.Errorf("round-tripped refresh token: got %q, want %q", roundTripped.RefreshToken, "rotated-refresh")
	}
}

// =============================================================================
// Test_Provider_Isolation
// Register 3 providers (Bluesky, YouTube, GitHub) with the Coordinator.
// Make YouTube fail with a transient "quota exceeded" error.
// Verify Bluesky and GitHub still process successfully.
// =============================================================================

func Test_Provider_Isolation(t *testing.T) {
	bskyCred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderBluesky,
	}
	ytCred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderYouTube,
	}
	ghCred := &domain.Credential{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Provider: domain.ProviderGitHub,
	}

	identityRepo := &multiProviderIdentityRepo{
		creds: map[string][]*domain.Credential{
			domain.ProviderBluesky: {bskyCred},
			domain.ProviderYouTube: {ytCred},
			domain.ProviderGitHub:  {ghCred},
		},
	}

	// Bluesky and GitHub succeed with 1 signal each.
	bskyH := &mockProviderHarvester{
		provider: domain.ProviderBluesky,
		signals: []domain.RawSignal{
			{SourceURL: "https://bsky.app/post/1", Content: "bluesky content", Provider: domain.ProviderBluesky, HarvestedAt: time.Now()},
		},
	}
	ghH := &mockProviderHarvester{
		provider: domain.ProviderGitHub,
		signals: []domain.RawSignal{
			{SourceURL: "https://github.com/foo/bar", Content: "github content", Provider: domain.ProviderGitHub, HarvestedAt: time.Now()},
		},
	}
	// YouTube always fails with a quota error (NOT an AuthError — transient).
	ytH := &mockProviderHarvester{
		provider: domain.ProviderYouTube,
		err:      errors.New("YouTube API quota exceeded"),
	}

	signalRepo := &mockSignalExistsChecker{existing: map[string]bool{}}
	synth := &mockSignalProcessor{}

	coord := harvester.NewCoordinator(
		map[string]domain.Harvester{
			domain.ProviderBluesky: bskyH,
			domain.ProviderYouTube: ytH,
			domain.ProviderGitHub:  ghH,
		},
		identityRepo,
		signalRepo,
		synth,
	)

	err := coord.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Bluesky + GitHub signals should be processed (2 total).
	// YouTube's failure should not prevent others.
	if synth.processCount != 2 {
		t.Errorf("expected 2 signals (bsky+gh), got %d", synth.processCount)
	}

	// Verify the correct URLs were processed.
	urls := map[string]bool{}
	for _, u := range synth.processURLs {
		urls[u] = true
	}
	if !urls["https://bsky.app/post/1"] {
		t.Error("bluesky signal was not processed")
	}
	if !urls["https://github.com/foo/bar"] {
		t.Error("github signal was not processed")
	}
}

// =============================================================================
// Test_Unexpired_Token_NoRefresh
// Set ExpiresAt 1 hour in the future. Verify no HTTP call is made to the
// token endpoint (the access token is returned directly).
// =============================================================================

func Test_Unexpired_Token_NoRefresh(t *testing.T) {
	kms := newMockKMS()

	// Token that doesn't expire for another hour.
	valid := &auth.TokenData{
		AccessToken:  "still-valid-access",
		RefreshToken: "some-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	cred := makeCredentialWithToken(t, kms, valid)

	endpoint := &mockTokenEndpoint{
		returnToken: &auth.TokenData{AccessToken: "should-not-see-this"},
	}
	saver := &mockTokenSaver{}

	refresher := auth.NewOAuth2Refresher(kms, saver, endpoint)

	accessToken, err := refresher.Refresh(context.Background(), cred)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Should return the EXISTING access token without calling the endpoint.
	if accessToken != "still-valid-access" {
		t.Errorf("expected still-valid-access, got %q", accessToken)
	}

	// Token endpoint should NOT have been called.
	if endpoint.called {
		t.Error("token endpoint should not be called for a valid, unexpired token")
	}

	// SaveToken should NOT have been called (no rotation happened).
	if saver.saveCalled {
		t.Error("SaveToken should not be called when no rotation occurs")
	}
}

// =============================================================================
// Test_Missing_RefreshToken_AuthError
// TokenData with empty RefreshToken and an expired access token.
// Verify an AuthError is returned (which triggers NeedsReauth upstream).
// =============================================================================

func Test_Missing_RefreshToken_AuthError(t *testing.T) {
	kms := newMockKMS()

	// Expired token with NO refresh token.
	noRefresh := &auth.TokenData{
		AccessToken:  "expired-access",
		RefreshToken: "", // no refresh token
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
	}
	cred := makeCredentialWithToken(t, kms, noRefresh)

	endpoint := &mockTokenEndpoint{} // should never be called
	saver := &mockTokenSaver{}

	refresher := auth.NewOAuth2Refresher(kms, saver, endpoint)

	_, err := refresher.Refresh(context.Background(), cred)
	if err == nil {
		t.Fatal("expected error for missing refresh token, got nil")
	}

	// Should be an AuthError (fatal, triggers NeedsReauth).
	var authErr *harvester.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected AuthError, got %T: %v", err, err)
	}

	// Token endpoint should NOT have been called (can't refresh without a token).
	if endpoint.called {
		t.Error("token endpoint should not be called when refresh token is missing")
	}
}

// =============================================================================
// Test Helper Mocks for Provider Isolation test
// =============================================================================

// multiProviderIdentityRepo returns different credentials per provider.
type multiProviderIdentityRepo struct {
	creds map[string][]*domain.Credential
}

func (m *multiProviderIdentityRepo) ListActiveCredentials(_ context.Context, provider string) ([]*domain.Credential, error) {
	return m.creds[provider], nil
}
func (m *multiProviderIdentityRepo) LinkProvider(_ context.Context, _ uuid.UUID, _ string, _ []byte) error {
	return nil
}
func (m *multiProviderIdentityRepo) GetActiveToken(_ context.Context, _ uuid.UUID, _ string) ([]byte, error) {
	return nil, nil
}
func (m *multiProviderIdentityRepo) ListUsersByProvider(_ context.Context, _ string) ([]*domain.User, error) {
	return nil, nil
}
func (m *multiProviderIdentityRepo) UpdateLastSeenID(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (m *multiProviderIdentityRepo) MarkNeedsReauth(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *multiProviderIdentityRepo) SaveToken(_ context.Context, _ uuid.UUID, _ []byte) error {
	return nil
}

// mockProviderHarvester is a simple mock for provider-specific behavior.
type mockProviderHarvester struct {
	provider string
	signals  []domain.RawSignal
	err      error
}

func (h *mockProviderHarvester) Harvest(_ context.Context, _ *domain.Credential) ([]domain.RawSignal, error) {
	return h.signals, h.err
}

func (h *mockProviderHarvester) Provider() string {
	return h.provider
}

// mockSignalExistsChecker for dedup in isolation test.
type mockSignalExistsChecker struct {
	existing map[string]bool
}

func (m *mockSignalExistsChecker) ExistsByURL(_ context.Context, _ uuid.UUID, sourceURL string) (bool, error) {
	return m.existing[sourceURL], nil
}

// mockSignalProcessor records calls for verification in isolation test.
type mockSignalProcessor struct {
	processCount int
	processURLs  []string
}

func (m *mockSignalProcessor) Process(_ context.Context, _ uuid.UUID, sourceURL, _ string) error {
	m.processCount++
	m.processURLs = append(m.processURLs, sourceURL)
	return nil
}
