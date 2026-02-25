// Package harvester implements the data collection layer for Signal-Flow.
// It periodically polls external platforms for new content, deduplicates,
// and feeds it to the Synthesizer pipeline.
package harvester

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/rvald/signal-flow/internal/domain"
)

// maxRetries is the number of attempts for transient errors before giving up.
const maxRetries = 3

// AuthError indicates a fatal authentication failure (e.g. 401 Unauthorized).
// When a Harvester returns this error, the credential is marked as needs_reauth.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// NewAuthError creates a new AuthError.
func NewAuthError(msg string) *AuthError {
	return &AuthError{Message: msg}
}

// SignalExistsChecker checks whether a signal already exists for deduplication.
type SignalExistsChecker interface {
	ExistsByURL(ctx context.Context, tenantID uuid.UUID, sourceURL string) (bool, error)
}

// SignalProcessor sends a raw signal to the synthesis pipeline.
type SignalProcessor interface {
	Process(ctx context.Context, tenantID uuid.UUID, sourceURL, content string) error
}

// Coordinator orchestrates the harvest cycle.
// It dispatches to registered Harvester implementations, deduplicates results,
// and forwards new signals to the Synthesizer.
type Coordinator struct {
	harvesters  map[string]domain.Harvester
	identity    domain.IdentityRepository
	signals     SignalExistsChecker
	synthesizer SignalProcessor
	logger      *slog.Logger
}

// NewCoordinator creates a Coordinator with the given harvesters and dependencies.
func NewCoordinator(
	harvesters map[string]domain.Harvester,
	identity domain.IdentityRepository,
	signals SignalExistsChecker,
	synthesizer SignalProcessor,
) *Coordinator {
	return &Coordinator{
		harvesters:  harvesters,
		identity:    identity,
		signals:     signals,
		synthesizer: synthesizer,
		logger:      slog.Default(),
	}
}

// RunOnce executes a single harvest cycle across all providers.
// This is the testable core — Start() wraps this in a ticker loop.
func (c *Coordinator) RunOnce(ctx context.Context) error {
	for providerName, h := range c.harvesters {
		creds, err := c.identity.ListActiveCredentials(ctx, providerName)
		if err != nil {
			c.logger.Error("list active credentials", "provider", providerName, "error", err)
			continue
		}

		for _, cred := range creds {
			c.harvestCredential(ctx, h, cred)
		}
	}
	return nil
}

// harvestCredential processes a single credential with retries and error handling.
func (c *Coordinator) harvestCredential(ctx context.Context, h domain.Harvester, cred *domain.Credential) {
	var rawSignals []domain.RawSignal
	var err error

	// Retry loop with exponential backoff for transient errors.
	for attempt := 0; attempt < maxRetries; attempt++ {
		rawSignals, err = h.Harvest(ctx, cred)
		if err == nil {
			break
		}

		// Check if this is a fatal auth error — no retry.
		var authErr *AuthError
		if errors.As(err, &authErr) {
			c.logger.Error("auth failure, marking needs_reauth",
				"provider", h.Provider(),
				"credential_id", cred.ID,
				"error", err,
			)
			if markErr := c.identity.MarkNeedsReauth(ctx, cred.ID); markErr != nil {
				c.logger.Error("mark needs_reauth", "credential_id", cred.ID, "error", markErr)
			}
			return
		}

		// Transient error — backoff and retry.
		backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
		c.logger.Warn("transient harvest error, retrying",
			"provider", h.Provider(),
			"credential_id", cred.ID,
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err,
		)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
	}

	if err != nil {
		c.logger.Error("harvest failed after retries",
			"provider", h.Provider(),
			"credential_id", cred.ID,
			"error", err,
		)
		return
	}

	// Process harvested signals: filter, dedup, synthesize.
	for _, raw := range rawSignals {
		// Filter: discard signals without a source URL.
		if raw.SourceURL == "" {
			c.logger.Debug("skipping signal with no URL", "provider", h.Provider())
			continue
		}

		// Dedup: check if the signal already exists.
		exists, err := c.signals.ExistsByURL(ctx, cred.UserID, raw.SourceURL)
		if err != nil {
			c.logger.Error("dedup check", "url", raw.SourceURL, "error", err)
			continue
		}
		if exists {
			c.logger.Debug("duplicate signal, skipping", "url", raw.SourceURL)
			continue
		}

		// Forward to synthesizer.
		if err := c.synthesizer.Process(ctx, cred.UserID, raw.SourceURL, raw.Content); err != nil {
			c.logger.Error("synthesize signal", "url", raw.SourceURL, "error", err)
			continue
		}
	}

	// Update the cursor for next poll (use last signal's metadata if available).
	// For now we track the last URL hash as the seen ID.
	if len(rawSignals) > 0 {
		lastRaw := rawSignals[len(rawSignals)-1]
		seenID := fmt.Sprintf("%x", sha256.Sum256([]byte(lastRaw.SourceURL)))
		if err := c.identity.UpdateLastSeenID(ctx, cred.ID, seenID); err != nil {
			c.logger.Error("update last_seen_id", "credential_id", cred.ID, "error", err)
		}
	}
}

// Start begins the background harvest loop with the given interval.
// It blocks until the context is cancelled.
func (c *Coordinator) Start(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start.
	if err := c.RunOnce(ctx); err != nil {
		c.logger.Error("initial harvest cycle", "error", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := c.RunOnce(ctx); err != nil {
				c.logger.Error("harvest cycle", "error", err)
			}
		case <-ctx.Done():
			c.logger.Info("harvester stopped")
			return ctx.Err()
		}
	}
}
