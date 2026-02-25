package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/rvald/signal-flow/internal/domain"
)

// SignalHandler serves signal-related endpoints.
type SignalHandler struct {
	Signals domain.SignalRepository
}

// Register mounts the signal routes on the given mux.
func (h *SignalHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/signals", h.List)
	mux.HandleFunc("POST /api/signals/search", h.Search)
	mux.HandleFunc("POST /api/signals/{id}/promote", h.Promote)
}

// List returns the most recent signals for the tenant.
func (h *SignalHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	signals, err := h.Signals.FindRecentByTenant(r.Context(), tenantID, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if signals == nil {
		signals = []*domain.Signal{}
	}

	JSON(w, http.StatusOK, signals)
}

// searchRequest is the request body for semantic search.
type searchRequest struct {
	Vector []float32 `json:"vector"`
	Limit  int       `json:"limit"`
}

// Search performs a semantic similarity search.
func (h *SignalHandler) Search(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Vector) == 0 {
		Error(w, http.StatusBadRequest, "vector is required")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 10
	}

	queryVec := pgvector.NewVector(req.Vector)
	signals, err := h.Signals.SearchSemantic(r.Context(), tenantID, queryVec, req.Limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if signals == nil {
		signals = []*domain.Signal{}
	}

	JSON(w, http.StatusOK, signals)
}

// Promote changes a signal's scope from private to team.
func (h *SignalHandler) Promote(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	signalID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid signal ID")
		return
	}

	if err := h.Signals.PromoteToTeam(r.Context(), signalID, tenantID); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "promoted"})
}
