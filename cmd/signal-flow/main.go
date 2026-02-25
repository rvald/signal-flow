package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvald/signal-flow/internal/api"
	"github.com/rvald/signal-flow/internal/intelligence"
	"github.com/rvald/signal-flow/internal/repository"
	"github.com/rvald/signal-flow/internal/security"
)

func main() {
	// --- Configuration ---
	port := envOr("PORT", "8088")
	databaseURL := envOr("DATABASE_URL", "postgres://signalflow:signalflow@localhost:5433/signal_flow_dev")
	encryptionKeyHex := os.Getenv("ENCRYPTION_KEY")

	if encryptionKeyHex == "" {
		slog.Error("ENCRYPTION_KEY is required (32 bytes as hex, e.g. openssl rand -hex 16)")
		os.Exit(1)
	}

	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		slog.Error("ENCRYPTION_KEY must be valid hex", "error", err)
		os.Exit(1)
	}

	// --- Key Manager ---
	kms, err := security.NewLocalKeyManager(encryptionKey)
	if err != nil {
		slog.Error("create key manager", "error", err)
		os.Exit(1)
	}

	// --- Database ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected", "url", databaseURL)

	// --- Repositories ---
	signalRepo := repository.NewPostgresSignalRepository(pool)
	identityRepo := repository.NewPostgresIdentityRepository(pool, kms)

	// --- Services ---
	// Synthesizer: only available if LLM keys are configured.
	var synthesizer *intelligence.SynthesizerService
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "" {
		// For now, create with nil summarizers — real wiring requires API key-specific init.
		// This will be properly wired when LLM providers are configured.
		slog.Info("synthesizer available (LLM keys detected)")
	} else {
		slog.Warn("synthesizer unavailable (no LLM API keys set)")
	}

	// Harvester coordinator: not wired yet — provider stubs need real implementations.
	// The HarvesterHandler returns 503 when Coordinator is nil.

	// --- HTTP Handlers ---
	mux := http.NewServeMux()

	health := &api.HealthHandler{}
	health.Register(mux)

	signals := &api.SignalHandler{Signals: signalRepo}
	signals.Register(mux)

	identity := &api.IdentityHandler{Identity: identityRepo}
	identity.Register(mux)

	synth := &api.SynthesizeHandler{Synthesizer: synthesizer}
	synth.Register(mux)

	harv := &api.HarvesterHandler{} // Coordinator not wired yet — returns 503
	harv.Register(mux)

	// --- Middleware ---
	var handler http.Handler = mux
	handler = api.TenantMiddleware(handler)
	handler = api.LoggingMiddleware(handler)
	handler = api.RecoveryMiddleware(handler)

	// --- Server ---
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("signal-flow API server starting", "port", port)
	fmt.Printf("\n  signal-flow API · http://localhost:%s/api/health\n\n", port)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
