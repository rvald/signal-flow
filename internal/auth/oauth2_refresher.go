package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/harvester"
	"github.com/rvald/signal-flow/internal/security"
)

// TokenEndpoint abstracts the provider's token exchange endpoint.
// Each provider implements this to call its specific refresh URL.
type TokenEndpoint interface {
	// ExchangeRefresh sends the refresh token to the provider's token endpoint
	// and returns a fresh TokenData with a new access token (and potentially
	// a rotated refresh token).
	ExchangeRefresh(ctx context.Context, refreshToken string) (*TokenData, error)
}

// OAuth2Refresher implements TokenRefresher for standard OAuth2 providers
// (YouTube, GitHub). Bluesky uses a separate refresher backed by indigo.
type OAuth2Refresher struct {
	kms      security.KeyManager
	saver    TokenSaver
	endpoint TokenEndpoint
}

// NewOAuth2Refresher creates a refresher for standard OAuth2 providers.
func NewOAuth2Refresher(kms security.KeyManager, saver TokenSaver, endpoint TokenEndpoint) *OAuth2Refresher {
	return &OAuth2Refresher{
		kms:      kms,
		saver:    saver,
		endpoint: endpoint,
	}
}

// tokenExpiryBuffer is how early we consider a token "expired" to avoid
// edge-case failures from clock skew or request latency.
const tokenExpiryBuffer = 30 * time.Second

// Refresh decrypts the credential's stored token, refreshes it if expired,
// re-encrypts and persists the updated token, then returns a ready-to-use
// access token string.
func (r *OAuth2Refresher) Refresh(ctx context.Context, cred *domain.Credential) (string, error) {
	// 1. Decrypt stored token.
	plaintext, err := r.kms.Decrypt(ctx, cred.EncryptedToken)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}

	var td TokenData
	if err := json.Unmarshal(plaintext, &td); err != nil {
		return "", fmt.Errorf("unmarshal token data: %w", err)
	}

	// 2. If the token is still valid, return it directly.
	if time.Now().Before(td.ExpiresAt.Add(-tokenExpiryBuffer)) {
		return td.AccessToken, nil
	}

	// 3. Token is expired — we need a refresh token to rotate.
	if td.RefreshToken == "" {
		return "", harvester.NewAuthError("token expired and no refresh token available")
	}

	// 4. Exchange the refresh token for a new access token.
	newTD, err := r.endpoint.ExchangeRefresh(ctx, td.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("exchange refresh token: %w", err)
	}

	// 5. Serialize and re-encrypt the new token data.
	newJSON, err := json.Marshal(newTD)
	if err != nil {
		return "", fmt.Errorf("marshal new token data: %w", err)
	}

	encrypted, err := r.kms.Encrypt(ctx, newJSON)
	if err != nil {
		return "", fmt.Errorf("encrypt new token: %w", err)
	}

	// 6. Persist the re-encrypted token.
	if err := r.saver.SaveToken(ctx, cred.ID, encrypted); err != nil {
		return "", fmt.Errorf("save rotated token: %w", err)
	}

	return newTD.AccessToken, nil
}
