package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/security"
)

// =============================================================================
// Test_BlueskyStore_SaveAndGet_RoundTrip
// Verifies SaveSession → GetSession returns identical ClientSessionData.
// The encrypt/decrypt cycle happens transparently through KeyManager.
// =============================================================================

func Test_BlueskyStore_SaveAndGet_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	kms, _ := security.NewLocalKeyManager(key)

	db := &fakeSessionDB{}
	store := auth.NewPostgresOAuthStore(kms, db)

	did := syntax.DID("did:plc:testuser123")
	sessionID := "sess-abc-123"

	original := oauth.ClientSessionData{
		AccountDID:              did,
		SessionID:               sessionID,
		HostURL:                 "https://bsky.social",
		AuthServerURL:           "https://bsky.social",
		AuthServerTokenEndpoint: "https://bsky.social/oauth/token",
		AccessToken:             "access-token-xyz",
		RefreshToken:            "refresh-token-abc",
		DPoPPrivateKeyMultibase: "z-some-private-key",
		Scopes:                  []string{"atproto", "transition:generic"},
	}

	ctx := context.Background()

	// Save.
	if err := store.SaveSession(ctx, original); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Get.
	got, err := store.GetSession(ctx, did, sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	// Verify all fields round-trip.
	if got.AccountDID != original.AccountDID {
		t.Errorf("AccountDID: got %v, want %v", got.AccountDID, original.AccountDID)
	}
	if got.SessionID != original.SessionID {
		t.Errorf("SessionID: got %v, want %v", got.SessionID, original.SessionID)
	}
	if got.AccessToken != original.AccessToken {
		t.Errorf("AccessToken: got %v, want %v", got.AccessToken, original.AccessToken)
	}
	if got.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken: got %v, want %v", got.RefreshToken, original.RefreshToken)
	}
	if got.HostURL != original.HostURL {
		t.Errorf("HostURL: got %v, want %v", got.HostURL, original.HostURL)
	}
	if got.DPoPPrivateKeyMultibase != original.DPoPPrivateKeyMultibase {
		t.Errorf("DPoPPrivateKeyMultibase: got %v, want %v", got.DPoPPrivateKeyMultibase, original.DPoPPrivateKeyMultibase)
	}
}

// =============================================================================
// Test_BlueskyStore_Encryption_AtRest
// After SaveSession, the raw stored bytes must NOT be valid JSON
// (proving they are encrypted).
// =============================================================================

func Test_BlueskyStore_Encryption_AtRest(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	kms, _ := security.NewLocalKeyManager(key)

	db := &fakeSessionDB{}
	store := auth.NewPostgresOAuthStore(kms, db)

	sess := oauth.ClientSessionData{
		AccountDID:  syntax.DID("did:plc:encrypted-test"),
		SessionID:   "sess-enc-111",
		AccessToken: "secret-access-token",
	}

	if err := store.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// The raw bytes in the DB must be encrypted (not valid JSON).
	rawBytes := db.lastSavedToken
	if len(rawBytes) == 0 {
		t.Fatal("no bytes were saved to the DB")
	}

	var check map[string]any
	if err := json.Unmarshal(rawBytes, &check); err == nil {
		t.Error("stored bytes should be encrypted, but they are valid JSON")
	}

	// However, they should be decryptable.
	decrypted, err := kms.Decrypt(context.Background(), rawBytes)
	if err != nil {
		t.Fatalf("failed to decrypt stored bytes: %v", err)
	}

	var roundTrip oauth.ClientSessionData
	if err := json.Unmarshal(decrypted, &roundTrip); err != nil {
		t.Fatalf("decrypted bytes are not valid session JSON: %v", err)
	}

	if roundTrip.AccessToken != "secret-access-token" {
		t.Errorf("got %q, want %q", roundTrip.AccessToken, "secret-access-token")
	}
}

// =============================================================================
// Test_BlueskyStore_DeleteSession
// Verify DeleteSession removes the session and GetSession returns error.
// =============================================================================

func Test_BlueskyStore_DeleteSession(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	kms, _ := security.NewLocalKeyManager(key)

	db := &fakeSessionDB{}
	store := auth.NewPostgresOAuthStore(kms, db)

	did := syntax.DID("did:plc:delete-test")
	sessionID := "sess-del-222"

	sess := oauth.ClientSessionData{
		AccountDID:  did,
		SessionID:   sessionID,
		AccessToken: "to-be-deleted",
	}

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Delete.
	if err := store.DeleteSession(ctx, did, sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// Get should fail.
	_, err := store.GetSession(ctx, did, sessionID)
	if err == nil {
		t.Error("GetSession should fail after DeleteSession")
	}
}

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

// =============================================================================
// Fake in-memory DB for testing PostgresOAuthStore without a real database.
// =============================================================================

type fakeSessionDB struct {
	// Stores encrypted tokens keyed by "did:sessionID".
	sessions       map[string][]byte
	lastSavedToken []byte
}

func (f *fakeSessionDB) SaveEncryptedSession(ctx context.Context, accountDID, sessionID string, encryptedToken []byte) error {
	if f.sessions == nil {
		f.sessions = make(map[string][]byte)
	}
	key := accountDID + ":" + sessionID
	f.sessions[key] = encryptedToken
	f.lastSavedToken = encryptedToken
	return nil
}

func (f *fakeSessionDB) GetEncryptedSession(ctx context.Context, accountDID, sessionID string) ([]byte, error) {
	if f.sessions == nil {
		return nil, fmt.Errorf("session not found")
	}
	key := accountDID + ":" + sessionID
	data, ok := f.sessions[key]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", key)
	}
	return data, nil
}

func (f *fakeSessionDB) DeleteEncryptedSession(ctx context.Context, accountDID, sessionID string) error {
	if f.sessions == nil {
		return nil
	}
	delete(f.sessions, accountDID+":"+sessionID)
	return nil
}

// Need this import for fmt.Errorf
var _ = uuid.New // keep uuid import alive for other tests
