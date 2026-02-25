package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/rvald/signal-flow/internal/security"
)

// SessionDB abstracts the database operations for encrypted session storage.
// This allows testing with a fake in-memory DB.
type SessionDB interface {
	SaveEncryptedSession(ctx context.Context, accountDID, sessionID string, encryptedToken []byte) error
	GetEncryptedSession(ctx context.Context, accountDID, sessionID string) ([]byte, error)
	DeleteEncryptedSession(ctx context.Context, accountDID, sessionID string) error
}

// PostgresOAuthStore implements oauth.ClientAuthStore backed by
// user_credentials + KeyManager encryption. Session data is encrypted
// at rest using AES-256-GCM via the KeyManager.
//
// Auth request methods are stored in-memory with TTL since they are
// transient (only live during the interactive OAuth login flow).
type PostgresOAuthStore struct {
	kms security.KeyManager
	db  SessionDB

	// In-memory storage for auth requests (transient, TTL-based).
	authRequests sync.Map
}

// authRequestEntry holds an auth request with a creation timestamp for TTL.
type authRequestEntry struct {
	data      oauth.AuthRequestData
	createdAt time.Time
}

// authRequestTTL is how long auth requests are kept in memory.
const authRequestTTL = 30 * time.Minute

// Compile-time assertion.
var _ oauth.ClientAuthStore = &PostgresOAuthStore{}

// NewPostgresOAuthStore creates a new PostgresOAuthStore.
func NewPostgresOAuthStore(kms security.KeyManager, db SessionDB) *PostgresOAuthStore {
	return &PostgresOAuthStore{
		kms: kms,
		db:  db,
	}
}

// GetSession retrieves and decrypts a Bluesky OAuth session.
func (s *PostgresOAuthStore) GetSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSessionData, error) {
	encrypted, err := s.db.GetEncryptedSession(ctx, string(did), sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	plaintext, err := s.kms.Decrypt(ctx, encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt session: %w", err)
	}

	var sess oauth.ClientSessionData
	if err := json.Unmarshal(plaintext, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &sess, nil
}

// SaveSession serializes, encrypts, and persists a Bluesky OAuth session.
func (s *PostgresOAuthStore) SaveSession(ctx context.Context, sess oauth.ClientSessionData) error {
	raw, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	encrypted, err := s.kms.Encrypt(ctx, raw)
	if err != nil {
		return fmt.Errorf("encrypt session: %w", err)
	}

	if err := s.db.SaveEncryptedSession(ctx, string(sess.AccountDID), sess.SessionID, encrypted); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

// DeleteSession removes a Bluesky OAuth session.
func (s *PostgresOAuthStore) DeleteSession(ctx context.Context, did syntax.DID, sessionID string) error {
	return s.db.DeleteEncryptedSession(ctx, string(did), sessionID)
}

// GetAuthRequestInfo retrieves a pending auth request from in-memory storage.
func (s *PostgresOAuthStore) GetAuthRequestInfo(_ context.Context, state string) (*oauth.AuthRequestData, error) {
	val, ok := s.authRequests.Load(state)
	if !ok {
		return nil, fmt.Errorf("auth request not found: %s", state)
	}

	entry := val.(*authRequestEntry)

	// Check TTL.
	if time.Since(entry.createdAt) > authRequestTTL {
		s.authRequests.Delete(state)
		return nil, fmt.Errorf("auth request expired: %s", state)
	}

	return &entry.data, nil
}

// SaveAuthRequestInfo stores a pending auth request in memory.
func (s *PostgresOAuthStore) SaveAuthRequestInfo(_ context.Context, info oauth.AuthRequestData) error {
	s.authRequests.Store(info.State, &authRequestEntry{
		data:      info,
		createdAt: time.Now(),
	})
	return nil
}

// DeleteAuthRequestInfo removes a pending auth request from memory.
func (s *PostgresOAuthStore) DeleteAuthRequestInfo(_ context.Context, state string) error {
	s.authRequests.Delete(state)
	return nil
}
