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
