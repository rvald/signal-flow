package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/rvald/signal-flow/internal/api"
	"github.com/rvald/signal-flow/internal/domain"
)

// --- Fakes ---

type fakeSignalRepo struct {
	signals []*domain.Signal
	saveErr error
}

func (f *fakeSignalRepo) Save(_ context.Context, s *domain.Signal) error { return f.saveErr }
func (f *fakeSignalRepo) FindRecentByTenant(_ context.Context, _ uuid.UUID, limit int) ([]*domain.Signal, error) {
	if limit > len(f.signals) {
		return f.signals, nil
	}
	return f.signals[:limit], nil
}
func (f *fakeSignalRepo) SearchSemantic(_ context.Context, _ uuid.UUID, _ pgvector.Vector, limit int) ([]*domain.Signal, error) {
	return f.signals, nil
}
func (f *fakeSignalRepo) PromoteToTeam(ctx context.Context, signalID uuid.UUID, tenantID uuid.UUID) error {
	return nil
}

func (f *fakeSignalRepo) FindUnsynthesized(ctx context.Context, tenantID uuid.UUID, limit int) ([]*domain.Signal, error) {
	return nil, nil
}
func (f *fakeSignalRepo) FindBySourceURL(_ context.Context, _ uuid.UUID, _ string) (*domain.Signal, error) {
	return nil, nil
}

type fakeIdentityRepo struct {
	linked bool
	token  []byte
	users  []*domain.User
}

func (f *fakeIdentityRepo) LinkProvider(_ context.Context, _ uuid.UUID, _ string, _ []byte) error {
	f.linked = true
	return nil
}
func (f *fakeIdentityRepo) GetActiveToken(_ context.Context, _ uuid.UUID, _ string) ([]byte, error) {
	if f.token == nil {
		return nil, context.DeadlineExceeded // simulate not found
	}
	return f.token, nil
}
func (f *fakeIdentityRepo) ListUsersByProvider(_ context.Context, _ string) ([]*domain.User, error) {
	return f.users, nil
}
func (f *fakeIdentityRepo) ListActiveCredentials(_ context.Context, _ string) ([]*domain.Credential, error) {
	return nil, nil
}
func (f *fakeIdentityRepo) UpdateLastSeenID(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeIdentityRepo) MarkNeedsReauth(_ context.Context, _ uuid.UUID) error { return nil }
func (f *fakeIdentityRepo) SaveToken(_ context.Context, _ uuid.UUID, _ []byte) error {
	return nil
}

// --- Helpers ---

func tenantHeader(tenantID string) http.Header {
	h := http.Header{}
	h.Set("X-Tenant-ID", tenantID)
	return h
}

const devTenantID = "00000000-0000-0000-0000-000000000001"

func newTestMux(handler interface{ Register(mux *http.ServeMux) }) *http.ServeMux {
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux
}

// --- Tests ---

func TestHealthCheck(t *testing.T) {
	mux := newTestMux(&api.HealthHandler{})

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %s", body["status"])
	}
}

func TestListSignals_EmptyTenant(t *testing.T) {
	repo := &fakeSignalRepo{signals: []*domain.Signal{}}
	mux := newTestMux(&api.SignalHandler{Signals: repo})

	req := httptest.NewRequest("GET", "/api/signals", nil)
	req.Header = tenantHeader(devTenantID)
	// Inject tenant into context since we're not going through middleware
	ctx := context.WithValue(req.Context(), api.ExportedTenantKey, uuid.MustParse(devTenantID))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var signals []domain.Signal
	json.NewDecoder(w.Body).Decode(&signals)
	if len(signals) != 0 {
		t.Fatalf("expected empty array, got %d signals", len(signals))
	}
}

func TestListSignals_MissingTenantContext(t *testing.T) {
	repo := &fakeSignalRepo{}
	mux := newTestMux(&api.SignalHandler{Signals: repo})

	req := httptest.NewRequest("GET", "/api/signals", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenantMiddleware_MissingHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := api.TenantMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/signals", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenantMiddleware_InvalidUUID(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := api.TenantMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/signals", nil)
	req.Header.Set("X-Tenant-ID", "not-a-uuid")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTenantMiddleware_HealthExempt(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := api.TenantMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/health", nil)
	// No X-Tenant-ID header — should still pass through.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Fatal("inner handler was not called")
	}
}

func TestLinkCredential(t *testing.T) {
	repo := &fakeIdentityRepo{}
	mux := newTestMux(&api.IdentityHandler{Identity: repo})

	body := `{"provider": "bluesky", "token": "test-token"}`
	req := httptest.NewRequest("POST", "/api/credentials", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), api.ExportedTenantKey, uuid.MustParse(devTenantID))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !repo.linked {
		t.Fatal("LinkProvider was not called")
	}
}

func TestSynthesize_NotConfigured(t *testing.T) {
	mux := newTestMux(&api.SynthesizeHandler{Synthesizer: nil})

	body := `{"source_url": "https://example.com", "content": "test"}`
	req := httptest.NewRequest("POST", "/api/synthesize", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.ExportedTenantKey, uuid.MustParse(devTenantID))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHarvest_NotConfigured(t *testing.T) {
	mux := newTestMux(&api.HarvesterHandler{Coordinator: nil})

	req := httptest.NewRequest("POST", "/api/harvest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
