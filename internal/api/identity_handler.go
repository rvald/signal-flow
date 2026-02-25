package api

import (
	"encoding/json"
	"net/http"

	"github.com/rvald/signal-flow/internal/domain"
)

// IdentityHandler serves credential management endpoints.
type IdentityHandler struct {
	Identity domain.IdentityRepository
}

// Register mounts the identity routes on the given mux.
func (h *IdentityHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/credentials", h.LinkProvider)
	mux.HandleFunc("GET /api/credentials/{provider}", h.GetToken)
	mux.HandleFunc("GET /api/credentials", h.ListUsers)
}

// linkRequest is the request body for linking a provider credential.
type linkRequest struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

// LinkProvider encrypts and stores a provider credential for the tenant.
func (h *IdentityHandler) LinkProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Provider == "" || req.Token == "" {
		Error(w, http.StatusBadRequest, "provider and token are required")
		return
	}

	if err := h.Identity.LinkProvider(r.Context(), tenantID, req.Provider, []byte(req.Token)); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// GetToken returns the decrypted token for a provider. Dev/debug only.
func (h *IdentityHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	provider := r.PathValue("provider")
	if provider == "" {
		Error(w, http.StatusBadRequest, "provider is required")
		return
	}

	token, err := h.Identity.GetActiveToken(r.Context(), tenantID, provider)
	if err != nil {
		Error(w, http.StatusNotFound, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{"token": string(token)})
}

// ListUsers returns all users that have a credential for a given provider.
func (h *IdentityHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		Error(w, http.StatusBadRequest, "provider query parameter is required")
		return
	}

	users, err := h.Identity.ListUsersByProvider(r.Context(), provider)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if users == nil {
		users = []*domain.User{}
	}

	JSON(w, http.StatusOK, users)
}
