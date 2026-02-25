package repository_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestDB starts a Postgres+pgvector container, runs migrations, and returns
// a connected pool plus a cleanup function.
func setupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
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

	// Run migration
	migrationPath := filepath.Join("..", "..", "migrations", "000001_create_signals_table.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	_, err = pool.Exec(ctx, string(migrationSQL))
	if err != nil {
		t.Fatalf("failed to run migration: %v", err)
	}

	// Create a non-superuser role for the application.
	// Superusers bypass RLS entirely, so we need a regular role.
	_, err = pool.Exec(ctx, `
		DO $$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
				CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user';
			END IF;
		END $$;
		GRANT ALL PRIVILEGES ON TABLE signals TO app_user;
	`)
	if err != nil {
		t.Fatalf("failed to create app_user role: %v", err)
	}

	// Close the superuser pool and reconnect as the non-superuser app_user.
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

// Test_Signal_Isolation verifies that tenant A's signals are invisible to tenant B.
func Test_Signal_Isolation(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresSignalRepository(pool)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()

	// Save two signals for tenant A
	signalA1 := &domain.Signal{
		TenantID:  tenantA,
		SourceURL: "https://example.com/article-1",
		Title:     "Article 1 for Tenant A",
		Content:   "Content A1",
	}
	signalA2 := &domain.Signal{
		TenantID:  tenantA,
		SourceURL: "https://example.com/article-2",
		Title:     "Article 2 for Tenant A",
		Content:   "Content A2",
	}

	if err := repo.Save(ctx, signalA1); err != nil {
		t.Fatalf("failed to save signal A1: %v", err)
	}
	if err := repo.Save(ctx, signalA2); err != nil {
		t.Fatalf("failed to save signal A2: %v", err)
	}

	// Tenant B should see zero signals
	resultsB, err := repo.FindRecentByTenant(ctx, tenantB, 10)
	if err != nil {
		t.Fatalf("failed to query as tenant B: %v", err)
	}
	if len(resultsB) != 0 {
		t.Errorf("tenant B should see 0 signals, got %d", len(resultsB))
	}

	// Tenant A should see exactly 2 signals
	resultsA, err := repo.FindRecentByTenant(ctx, tenantA, 10)
	if err != nil {
		t.Fatalf("failed to query as tenant A: %v", err)
	}
	if len(resultsA) != 2 {
		t.Errorf("tenant A should see 2 signals, got %d", len(resultsA))
	}
}

// Test_Upsert_Deduplication verifies that saving the same URL twice for the same
// tenant updates the existing record rather than creating a duplicate.
func Test_Upsert_Deduplication(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresSignalRepository(pool)
	ctx := context.Background()

	tenantID := uuid.New()
	url := "https://example.com/dedup-test"

	// First save
	signal := &domain.Signal{
		TenantID:  tenantID,
		SourceURL: url,
		Title:     "Original Title",
		Content:   "Original Content",
	}
	if err := repo.Save(ctx, signal); err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	// Second save — same URL, updated title
	signal2 := &domain.Signal{
		TenantID:  tenantID,
		SourceURL: url,
		Title:     "Updated Title",
		Content:   "Updated Content",
	}
	if err := repo.Save(ctx, signal2); err != nil {
		t.Fatalf("second save (upsert) failed: %v", err)
	}

	// Should be exactly 1 record
	results, err := repo.FindRecentByTenant(ctx, tenantID, 10)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 signal after upsert, got %d", len(results))
	}
	if results[0].Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got '%s'", results[0].Title)
	}
}

// Test_Semantic_Search_Ordering verifies that pgvector cosine similarity returns
// the most relevant signal first.
func Test_Semantic_Search_Ordering(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresSignalRepository(pool)
	ctx := context.Background()

	tenantID := uuid.New()

	// Create distinguishable vectors (1536 dimensions).
	// We use a simple scheme: set one dimension high to differentiate topics.
	goVector := make([]float32, 1536)
	goVector[0] = 1.0 // "Go" lives in dimension 0

	swiftVector := make([]float32, 1536)
	swiftVector[1] = 1.0 // "Swift" lives in dimension 1

	cookingVector := make([]float32, 1536)
	cookingVector[2] = 1.0 // "Cooking" lives in dimension 2

	signals := []*domain.Signal{
		{
			TenantID:  tenantID,
			SourceURL: "https://example.com/go-article",
			Title:     "Concurrency in Go",
			Content:   "Goroutines and channels...",
			Vector:    pgvector.NewVector(goVector),
		},
		{
			TenantID:  tenantID,
			SourceURL: "https://example.com/swift-article",
			Title:     "SwiftUI Layouts",
			Content:   "Building UIs with SwiftUI...",
			Vector:    pgvector.NewVector(swiftVector),
		},
		{
			TenantID:  tenantID,
			SourceURL: "https://example.com/cooking-article",
			Title:     "Italian Pasta Recipes",
			Content:   "How to make carbonara...",
			Vector:    pgvector.NewVector(cookingVector),
		},
	}

	for _, s := range signals {
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("failed to save signal '%s': %v", s.Title, err)
		}
	}

	// Query with a vector similar to "Go" (dimension 0 high)
	queryVector := make([]float32, 1536)
	queryVector[0] = 0.95 // Very close to the Go vector

	results, err := repo.SearchSemantic(ctx, tenantID, pgvector.NewVector(queryVector), 3)
	if err != nil {
		t.Fatalf("semantic search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("semantic search returned no results")
	}

	if results[0].Title != "Concurrency in Go" {
		t.Errorf("expected Go article as first result, got '%s'", results[0].Title)
	}
}

// Test_PromoteToTeam verifies that a signal's scope can be changed from private to team.
func Test_PromoteToTeam(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresSignalRepository(pool)
	ctx := context.Background()

	tenantID := uuid.New()

	signal := &domain.Signal{
		TenantID:  tenantID,
		SourceURL: "https://example.com/promote-test",
		Title:     "Promotable Signal",
		Content:   "This will be promoted to team scope",
	}
	if err := repo.Save(ctx, signal); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Retrieve to get the generated ID
	results, err := repo.FindRecentByTenant(ctx, tenantID, 1)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(results))
	}

	savedSignal := results[0]
	if savedSignal.Scope != domain.ScopePrivate {
		t.Fatalf("expected initial scope '%s', got '%s'", domain.ScopePrivate, savedSignal.Scope)
	}

	// Promote
	if err := repo.PromoteToTeam(ctx, savedSignal.ID, tenantID); err != nil {
		t.Fatalf("promote failed: %v", err)
	}

	// Verify scope changed
	results, err = repo.FindRecentByTenant(ctx, tenantID, 1)
	if err != nil {
		t.Fatalf("query after promote failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 signal after promote, got %d", len(results))
	}
	if results[0].Scope != domain.ScopeTeam {
		t.Errorf("expected scope '%s' after promote, got '%s'", domain.ScopeTeam, results[0].Scope)
	}
}
