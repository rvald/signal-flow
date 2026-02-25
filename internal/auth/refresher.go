// Package auth provides OAuth token management for Signal-Flow providers.
// It handles token refresh, encryption/decryption, and provider-specific
// session management (e.g., AT Protocol OAuth for Bluesky).
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
)

// TokenRefresher abstracts per-provider token refresh logic.
// Each provider implements this to handle its specific auth flow.
type TokenRefresher interface {
	// Refresh decrypts the credential's stored token, refreshes it if expired,
	// re-encrypts and persists the updated token, then returns a ready-to-use
	// access token string.
	Refresh(ctx context.Context, cred *domain.Credential) (accessToken string, err error)
}

// TokenData is the JSON structure stored inside Credential.EncryptedToken
// for standard OAuth2 providers (YouTube, GitHub).
// Bluesky uses indigo's ClientSessionData instead.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"` // Empty for GitHub PATs
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenSaver persists re-encrypted tokens after rotation.
// Satisfied by IdentityRepository.SaveToken.
type TokenSaver interface {
	SaveToken(ctx context.Context, credentialID uuid.UUID, encryptedToken []byte) error
}
