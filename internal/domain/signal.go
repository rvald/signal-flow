// Package domain defines the core business types and interfaces for Signal-Flow.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// Scope constants for signal visibility.
const (
	ScopePrivate = "private"
	ScopeTeam    = "team"
)

// Signal is the atomic unit of knowledge in Signal-Flow.
type Signal struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	SourceURL    string          `json:"source_url"`
	Title        string          `json:"title"`
	Content      string          `json:"content"`
	Distillation string          `json:"distillation"`
	Metadata     map[string]any  `json:"metadata"`
	Scope        string          `json:"scope"`
	Vector       pgvector.Vector `json:"vector"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// SignalRepository defines the contract for signal data access.
// All methods are tenant-scoped via Row-Level Security.
type SignalRepository interface {
	// Save performs an upsert based on (source_url, tenant_id).
	Save(ctx context.Context, signal *Signal) error

	// FindRecentByTenant returns the most recent signals for a tenant.
	FindRecentByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]*Signal, error)

	// SearchSemantic returns signals ordered by cosine similarity to the query vector.
	SearchSemantic(ctx context.Context, tenantID uuid.UUID, queryVector pgvector.Vector, limit int) ([]*Signal, error)

	// PromoteToTeam changes a signal's scope from private to team.
	PromoteToTeam(ctx context.Context, signalID uuid.UUID, tenantID uuid.UUID) error

	// FindBySourceURL returns the signal for the given source URL and tenant, or nil if not found.
	FindBySourceURL(ctx context.Context, tenantID uuid.UUID, sourceURL string) (*Signal, error)

	// FindUnsynthesized returns signals that have not been synthesized yet, ordered by newest first.
	FindUnsynthesized(ctx context.Context, tenantID uuid.UUID, limit int) ([]*Signal, error)
}
