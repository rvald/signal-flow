package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User represents a registered identity in Signal-Flow.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Credential holds an encrypted OAuth2 refresh token for a specific provider.
type Credential struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	Provider       string     `json:"provider"` // e.g. "bluesky", "youtube", "github"
	EncryptedToken []byte     `json:"-"`        // AES-256-GCM ciphertext (never serialized)
	ProviderUserID string     `json:"provider_user_id"`
	LastSeenID     string     `json:"last_seen_id"`          // Cursor for incremental polling
	NeedsReauth    bool       `json:"needs_reauth"`          // Flagged on 401 responses
	SessionID      string     `json:"session_id,omitempty"`  // Bluesky OAuth session ID
	AccountDID     string     `json:"account_did,omitempty"` // Bluesky AT Protocol DID
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IdentityRepository defines the contract for user identity and credential data access.
type IdentityRepository interface {
	// LinkProvider encrypts rawToken and stores/updates the credential for the user+provider pair.
	LinkProvider(ctx context.Context, userID uuid.UUID, provider string, rawToken []byte) error

	// GetActiveToken decrypts and returns the stored token for the given user and provider.
	GetActiveToken(ctx context.Context, userID uuid.UUID, provider string) ([]byte, error)

	// ListUsersByProvider returns all users that have a credential for the given provider.
	ListUsersByProvider(ctx context.Context, provider string) ([]*User, error)

	// ListActiveCredentials returns all credentials for a provider that do not need reauthentication.
	ListActiveCredentials(ctx context.Context, provider string) ([]*Credential, error)

	// UpdateLastSeenID sets the polling cursor for a credential.
	UpdateLastSeenID(ctx context.Context, credentialID uuid.UUID, lastSeenID string) error

	// MarkNeedsReauth flags a credential as requiring reauthentication (e.g. after a 401).
	MarkNeedsReauth(ctx context.Context, credentialID uuid.UUID) error

	// SaveToken updates the encrypted token for a credential (used after token rotation).
	SaveToken(ctx context.Context, credentialID uuid.UUID, encryptedToken []byte) error
}
