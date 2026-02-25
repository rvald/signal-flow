package repository_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/rvald/signal-flow/internal/security"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testKey is a deterministic 32-byte key used only in tests.
var testKey = []byte("01234567890123456789012345678901")

// setupIdentityTestDB starts a Postgres container, runs both migrations, creates
// app_user with grants on all tables, and returns a pool + cleanup function.
func setupIdentityTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "signalflow",
			"POSTGRES_PASSWORD": "signalflow",
			"POSTGRES_DB":       "signal_flow_test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://signalflow:signalflow@%s:%s/signal_flow_test?sslmode=disable", host, port.Port())

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Run both migrations in order.
	migrations := []string{
		"000001_create_signals_table.up.sql",
		"000002_create_identity_tables.up.sql",
	}
	for _, m := range migrations {
		migrationPath := filepath.Join("..", "..", "migrations", m)
		migrationSQL, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("failed to read migration %s: %v", m, err)
		}
		_, err = pool.Exec(ctx, string(migrationSQL))
		if err != nil {
			t.Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	// Create a non-superuser role and grant on all tables.
	_, err = pool.Exec(ctx, `
		DO $$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
				CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user';
			END IF;
		END $$;
		GRANT ALL PRIVILEGES ON TABLE signals TO app_user;
		GRANT ALL PRIVILEGES ON TABLE users TO app_user;
		GRANT ALL PRIVILEGES ON TABLE user_credentials TO app_user;
	`)
	if err != nil {
		t.Fatalf("failed to create app_user role: %v", err)
	}

	// Reconnect as app_user.
	pool.Close()

	appDSN := fmt.Sprintf("postgres://app_user:app_user@%s:%s/signal_flow_test?sslmode=disable", host, port.Port())
	pool, err = pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("failed to create app_user pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		_ = container.Terminate(ctx)
	}

	return pool, cleanup
}

// seedUser inserts a user directly and returns the generated UUID.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		"INSERT INTO users (email) VALUES ($1) RETURNING id", email,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedUser(%q): %v", email, err)
	}
	return id
}

func Test_No_Plaintext_In_DB(t *testing.T) {
	pool, cleanup := setupIdentityTestDB(t)
	defer cleanup()

	km, err := security.NewLocalKeyManager(testKey)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	repo := repository.NewPostgresIdentityRepository(pool, km)
	ctx := context.Background()

	userID := seedUser(t, pool, "alice@example.com")
	rawToken := []byte("bluesky-refresh-token-supersecret")

	if err := repo.LinkProvider(ctx, userID, "bluesky", rawToken); err != nil {
		t.Fatalf("LinkProvider: %v", err)
	}

	// Raw query the DB to inspect the stored value.
	var storedToken []byte
	err = pool.QueryRow(ctx,
		"SELECT encrypted_token FROM user_credentials WHERE user_id = $1 AND provider = $2",
		userID, "bluesky",
	).Scan(&storedToken)
	if err != nil {
		t.Fatalf("raw query: %v", err)
	}

	if bytes.Equal(storedToken, rawToken) {
		t.Error("encrypted_token in DB matches the raw plaintext — token is NOT encrypted")
	}
}

func Test_Token_Rotation(t *testing.T) {
	pool, cleanup := setupIdentityTestDB(t)
	defer cleanup()

	km, err := security.NewLocalKeyManager(testKey)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	repo := repository.NewPostgresIdentityRepository(pool, km)
	ctx := context.Background()

	userID := seedUser(t, pool, "bob@example.com")

	oldToken := []byte("old-refresh-token")
	newToken := []byte("new-refresh-token")

	// First link
	if err := repo.LinkProvider(ctx, userID, "google", oldToken); err != nil {
		t.Fatalf("LinkProvider (first): %v", err)
	}

	// Rotate — same user, same provider, new token.
	if err := repo.LinkProvider(ctx, userID, "google", newToken); err != nil {
		t.Fatalf("LinkProvider (rotate): %v", err)
	}

	// Verify only 1 row exists.
	var count int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM user_credentials WHERE user_id = $1 AND provider = $2",
		userID, "google",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 credential row after rotation, got %d", count)
	}

	// Verify the new token is retrievable.
	decrypted, err := repo.GetActiveToken(ctx, userID, "google")
	if err != nil {
		t.Fatalf("GetActiveToken: %v", err)
	}
	if !bytes.Equal(decrypted, newToken) {
		t.Errorf("rotated token mismatch:\n  want: %q\n  got:  %q", newToken, decrypted)
	}
}

func Test_GetActiveToken_RoundTrip(t *testing.T) {
	pool, cleanup := setupIdentityTestDB(t)
	defer cleanup()

	km, err := security.NewLocalKeyManager(testKey)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	repo := repository.NewPostgresIdentityRepository(pool, km)
	ctx := context.Background()

	userID := seedUser(t, pool, "carol@example.com")
	rawToken := []byte("youtube-refresh-token-xyz")

	if err := repo.LinkProvider(ctx, userID, "google", rawToken); err != nil {
		t.Fatalf("LinkProvider: %v", err)
	}

	decrypted, err := repo.GetActiveToken(ctx, userID, "google")
	if err != nil {
		t.Fatalf("GetActiveToken: %v", err)
	}

	if !bytes.Equal(decrypted, rawToken) {
		t.Errorf("round-trip mismatch:\n  want: %q\n  got:  %q", rawToken, decrypted)
	}
}

func Test_ListUsersByProvider(t *testing.T) {
	pool, cleanup := setupIdentityTestDB(t)
	defer cleanup()

	km, err := security.NewLocalKeyManager(testKey)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	repo := repository.NewPostgresIdentityRepository(pool, km)
	ctx := context.Background()

	// Seed 3 users: 2 with bluesky, 1 with google only.
	user1 := seedUser(t, pool, "user1@example.com")
	user2 := seedUser(t, pool, "user2@example.com")
	user3 := seedUser(t, pool, "user3@example.com")

	_ = repo.LinkProvider(ctx, user1, "bluesky", []byte("token-1"))
	_ = repo.LinkProvider(ctx, user2, "bluesky", []byte("token-2"))
	_ = repo.LinkProvider(ctx, user3, "google", []byte("token-3"))

	blueskyUsers, err := repo.ListUsersByProvider(ctx, "bluesky")
	if err != nil {
		t.Fatalf("ListUsersByProvider(bluesky): %v", err)
	}
	if len(blueskyUsers) != 2 {
		t.Errorf("expected 2 bluesky users, got %d", len(blueskyUsers))
	}

	googleUsers, err := repo.ListUsersByProvider(ctx, "google")
	if err != nil {
		t.Fatalf("ListUsersByProvider(google): %v", err)
	}
	if len(googleUsers) != 1 {
		t.Errorf("expected 1 google user, got %d", len(googleUsers))
	}
}
