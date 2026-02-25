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
	Provider       string     `json:"provider"` // e.g. "bluesky", "google"
	EncryptedToken []byte     `json:"-"`        // AES-256-GCM ciphertext (never serialized)
	ProviderUserID string     `json:"provider_user_id"`
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
}
