// Package repository provides data access implementations for Signal-Flow.
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/rvald/signal-flow/internal/domain"
)

// PostgresSignalRepository implements domain.SignalRepository using PostgreSQL + pgvector.
type PostgresSignalRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresSignalRepository creates a new repository backed by the given connection pool.
func NewPostgresSignalRepository(pool *pgxpool.Pool) *PostgresSignalRepository {
	return &PostgresSignalRepository{pool: pool}
}

// withTenantTx starts a transaction, sets the RLS tenant context, and calls fn.
// SET LOCAL is scoped to the transaction so RLS policies see the correct tenant.
func (r *PostgresSignalRepository) withTenantTx(ctx context.Context, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_tenant_id = '%s'", tenantID.String()))
	if err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// scanSignal scans a row into a Signal, handling nullable vector columns.
func scanSignal(row pgx.Row) (*domain.Signal, error) {
	s := &domain.Signal{}
	var vec *pgvector.Vector

	err := row.Scan(
		&s.ID, &s.TenantID, &s.SourceURL, &s.Title, &s.Content,
		&s.Distillation, &s.Metadata, &s.Scope, &vec,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if vec != nil {
		s.Vector = *vec
	}

	return s, nil
}

// Save performs an UPSERT on (source_url, tenant_id). If a signal with the same
// URL already exists for this tenant, it updates the existing record.
func (r *PostgresSignalRepository) Save(ctx context.Context, signal *domain.Signal) error {
	return r.withTenantTx(ctx, signal.TenantID, func(tx pgx.Tx) error {
		query := `
			INSERT INTO signals (tenant_id, source_url, title, content, distillation, metadata, scope, vector)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (source_url, tenant_id)
			DO UPDATE SET
				title        = EXCLUDED.title,
				content      = EXCLUDED.content,
				distillation = EXCLUDED.distillation,
				metadata     = EXCLUDED.metadata,
				vector       = EXCLUDED.vector,
				updated_at   = now()
			RETURNING id, created_at, updated_at
		`

		var vectorArg any
		if len(signal.Vector.Slice()) > 0 {
			vectorArg = signal.Vector
		}

		metadata := signal.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}

		scope := signal.Scope
		if scope == "" {
			scope = domain.ScopePrivate
		}

		return tx.QueryRow(ctx, query,
			signal.TenantID,
			signal.SourceURL,
			signal.Title,
			signal.Content,
			signal.Distillation,
			metadata,
			scope,
			vectorArg,
		).Scan(&signal.ID, &signal.CreatedAt, &signal.UpdatedAt)
	})
}

const selectSignalCols = `id, tenant_id, source_url, title, content, distillation,
			       metadata, scope, vector, created_at, updated_at`

// FindRecentByTenant returns the most recent signals for a tenant, ordered by created_at DESC.
func (r *PostgresSignalRepository) FindRecentByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]*domain.Signal, error) {
	var results []*domain.Signal

	err := r.withTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := fmt.Sprintf(`
			SELECT %s
			FROM signals
			ORDER BY created_at DESC
			LIMIT $1
		`, selectSignalCols)

		rows, err := tx.Query(ctx, query, limit)
		if err != nil {
			return fmt.Errorf("query signals: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			s, err := scanSignal(rows)
			if err != nil {
				return fmt.Errorf("scan signal: %w", err)
			}
			results = append(results, s)
		}

		return rows.Err()
	})

	return results, err
}

// SearchSemantic returns signals ordered by cosine similarity to the query vector.
func (r *PostgresSignalRepository) SearchSemantic(ctx context.Context, tenantID uuid.UUID, queryVector pgvector.Vector, limit int) ([]*domain.Signal, error) {
	var results []*domain.Signal

	err := r.withTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := fmt.Sprintf(`
			SELECT %s
			FROM signals
			WHERE vector IS NOT NULL
			ORDER BY vector <=> $1
			LIMIT $2
		`, selectSignalCols)

		rows, err := tx.Query(ctx, query, queryVector, limit)
		if err != nil {
			return fmt.Errorf("semantic search: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			s, err := scanSignal(rows)
			if err != nil {
				return fmt.Errorf("scan signal: %w", err)
			}
			results = append(results, s)
		}

		return rows.Err()
	})

	return results, err
}

// PromoteToTeam changes a signal's scope from private to team.
func (r *PostgresSignalRepository) PromoteToTeam(ctx context.Context, signalID uuid.UUID, tenantID uuid.UUID) error {
	return r.withTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `UPDATE signals SET scope = 'team', updated_at = now() WHERE id = $1`

		tag, err := tx.Exec(ctx, query, signalID)
		if err != nil {
			return fmt.Errorf("promote signal: %w", err)
		}

		if tag.RowsAffected() == 0 {
			return fmt.Errorf("signal %s not found for tenant %s", signalID, tenantID)
		}

		return nil
	})
}
