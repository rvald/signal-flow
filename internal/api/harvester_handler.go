package api

import (
	"net/http"

	"github.com/rvald/signal-flow/internal/harvester"
)

// HarvesterHandler exposes the harvest coordinator over HTTP.
type HarvesterHandler struct {
	Coordinator *harvester.Coordinator
}

// Register mounts the harvester routes on the given mux.
func (h *HarvesterHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/harvest", h.RunOnce)
}

// RunOnce triggers a single harvest cycle across all providers.
func (h *HarvesterHandler) RunOnce(w http.ResponseWriter, r *http.Request) {
	if h.Coordinator == nil {
		Error(w, http.StatusServiceUnavailable, "harvester not configured")
		return
	}

	if err := h.Coordinator.RunOnce(r.Context()); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "harvest complete"})
}
