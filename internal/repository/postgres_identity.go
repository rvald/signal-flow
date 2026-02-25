package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/security"
)

// PostgresIdentityRepository implements domain.IdentityRepository using PostgreSQL.
// Tokens are encrypted/decrypted via the provided KeyManager before storage.
type PostgresIdentityRepository struct {
	pool *pgxpool.Pool
	kms  security.KeyManager
}

// NewPostgresIdentityRepository creates a new identity repository.
func NewPostgresIdentityRepository(pool *pgxpool.Pool, kms security.KeyManager) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{pool: pool, kms: kms}
}

// LinkProvider encrypts rawToken and upserts the credential for the given user+provider.
// If a credential already exists, the token (and updated_at) are overwritten.
func (r *PostgresIdentityRepository) LinkProvider(ctx context.Context, userID uuid.UUID, provider string, rawToken []byte) error {
	encrypted, err := r.kms.Encrypt(ctx, rawToken)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	query := `
		INSERT INTO user_credentials (user_id, provider, encrypted_token)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, provider)
		DO UPDATE SET
			encrypted_token = EXCLUDED.encrypted_token,
			updated_at      = now()
	`

	_, err = r.pool.Exec(ctx, query, userID, provider, encrypted)
	if err != nil {
		return fmt.Errorf("upsert credential: %w", err)
	}

	return nil
}

// GetActiveToken retrieves and decrypts the stored token for the given user and provider.
func (r *PostgresIdentityRepository) GetActiveToken(ctx context.Context, userID uuid.UUID, provider string) ([]byte, error) {
	var encrypted []byte

	err := r.pool.QueryRow(ctx,
		"SELECT encrypted_token FROM user_credentials WHERE user_id = $1 AND provider = $2",
		userID, provider,
	).Scan(&encrypted)
	if err != nil {
		return nil, fmt.Errorf("query credential: %w", err)
	}

	plaintext, err := r.kms.Decrypt(ctx, encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	return plaintext, nil
}

// ListUsersByProvider returns all users that have an active credential for the given provider.
func (r *PostgresIdentityRepository) ListUsersByProvider(ctx context.Context, provider string) ([]*domain.User, error) {
	query := `
		SELECT u.id, u.email, u.created_at
		FROM users u
		JOIN user_credentials uc ON uc.user_id = u.id
		WHERE uc.provider = $1
	`

	rows, err := r.pool.Query(ctx, query, provider)
	if err != nil {
		return nil, fmt.Errorf("query users by provider: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	return users, rows.Err()
}

// ListActiveCredentials returns all credentials for a provider that do not need reauthentication.
func (r *PostgresIdentityRepository) ListActiveCredentials(ctx context.Context, provider string) ([]*domain.Credential, error) {
	query := `
		SELECT id, user_id, provider, encrypted_token, provider_user_id,
		       last_seen_id, needs_reauth, expires_at, created_at, updated_at
		FROM user_credentials
		WHERE provider = $1 AND needs_reauth = false
	`

	rows, err := r.pool.Query(ctx, query, provider)
	if err != nil {
		return nil, fmt.Errorf("query active credentials: %w", err)
	}
	defer rows.Close()

	var creds []*domain.Credential
	for rows.Next() {
		c := &domain.Credential{}
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Provider, &c.EncryptedToken, &c.ProviderUserID,
			&c.LastSeenID, &c.NeedsReauth, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		creds = append(creds, c)
	}

	return creds, rows.Err()
}

// UpdateLastSeenID sets the polling cursor for a credential.
func (r *PostgresIdentityRepository) UpdateLastSeenID(ctx context.Context, credentialID uuid.UUID, lastSeenID string) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE user_credentials SET last_seen_id = $1, updated_at = now() WHERE id = $2",
		lastSeenID, credentialID,
	)
	if err != nil {
		return fmt.Errorf("update last_seen_id: %w", err)
	}
	return nil
}

// MarkNeedsReauth flags a credential as requiring reauthentication.
func (r *PostgresIdentityRepository) MarkNeedsReauth(ctx context.Context, credentialID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE user_credentials SET needs_reauth = true, updated_at = now() WHERE id = $1",
		credentialID,
	)
	if err != nil {
		return fmt.Errorf("mark needs_reauth: %w", err)
	}
	return nil
}

// SaveToken updates the encrypted token for a credential (used after token rotation).
func (r *PostgresIdentityRepository) SaveToken(ctx context.Context, credentialID uuid.UUID, encryptedToken []byte) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE user_credentials SET encrypted_token = $1, updated_at = now() WHERE id = $2",
		encryptedToken, credentialID,
	)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}
