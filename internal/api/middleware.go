package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
)

// tenantExemptPrefixes are path prefixes that do not require X-Tenant-ID.
var tenantExemptPrefixes = []string{"/api/health"}

// contextKey is an unexported type for context keys in this package.
type contextKey string

const tenantKey contextKey = "tenant_id"

// ExportedTenantKey allows tests to inject a tenant ID into the request context.
// This is used by handler tests that bypass the TenantMiddleware.
var ExportedTenantKey = tenantKey

// TenantIDFromContext extracts the tenant UUID from the request context.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantKey).(uuid.UUID)
	return id, ok
}

// TenantMiddleware reads X-Tenant-ID from the request header
// and stores the parsed UUID in the request context.
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip tenant check for exempt paths (e.g. health check).
		for _, prefix := range tenantExemptPrefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
		}

		raw := r.Header.Get("X-Tenant-ID")
		if raw == "" {
			Error(w, http.StatusBadRequest, "missing X-Tenant-ID header")
			return
		}

		tenantID, err := uuid.Parse(raw)
		if err != nil {
			Error(w, http.StatusBadRequest, fmt.Sprintf("invalid X-Tenant-ID: %v", err))
			return
		}

		ctx := context.WithValue(r.Context(), tenantKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs each request with method, path, status, and duration.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusWriter wraps ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// RecoveryMiddleware catches panics and returns a 500 error.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"error", rec,
					"stack", string(debug.Stack()),
				)
				Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
