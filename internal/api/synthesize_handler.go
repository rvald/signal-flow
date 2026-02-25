package api

import (
	"encoding/json"
	"net/http"

	"github.com/rvald/signal-flow/internal/domain"
	"github.com/rvald/signal-flow/internal/intelligence"
)

// SynthesizeHandler exposes the intelligence pipeline over HTTP.
type SynthesizeHandler struct {
	Synthesizer *intelligence.SynthesizerService
}

// Register mounts the synthesize routes on the given mux.
func (h *SynthesizeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/synthesize", h.Synthesize)
}

// synthesizeRequest is the body for a synthesis request.
type synthesizeRequest struct {
	SourceURL string `json:"source_url"`
	Content   string `json:"content"`
	Priority  int    `json:"priority"` // 0 = standard, 1 = high
}

// Synthesize runs the two-pass intelligence pipeline on the given content.
func (h *SynthesizeHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	if h.Synthesizer == nil {
		Error(w, http.StatusServiceUnavailable, "synthesizer not configured (missing LLM API keys)")
		return
	}

	tenantID, ok := TenantIDFromContext(r.Context())
	if !ok {
		Error(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	var req synthesizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SourceURL == "" || req.Content == "" {
		Error(w, http.StatusBadRequest, "source_url and content are required")
		return
	}

	params := domain.ContextParams{
		Priority: domain.Priority(req.Priority),
	}

	result, err := h.Synthesizer.Synthesize(r.Context(), tenantID, req.SourceURL, req.Content, params)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, result)
}
