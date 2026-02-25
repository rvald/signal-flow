package api

import "net/http"

// HealthHandler returns a simple health check endpoint.
type HealthHandler struct{}

// Register mounts the health routes on the given mux.
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.Health)
}

// Health returns 200 OK with a status payload.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
