package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rvald/signal-flow/internal/notify"
)

// =============================================================================
// Test_SlackNotifier_Success
// Verifies that a valid digest is POSTed to the webhook and returns no error.
// =============================================================================

func Test_SlackNotifier_Success(t *testing.T) {
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := notify.NewSlackNotifier(server.URL, "#test")

	summary := &notify.DigestSummary{
		Signals: []notify.DigestSignal{
			{
				Title:     "Go 1.25 Released",
				SourceURL: "https://go.dev/blog/go1.25",
				Provider:  "bluesky",
				Teaser:    "Major release with new features",
			},
		},
		GeneratedAt: time.Date(2026, 3, 3, 7, 0, 0, 0, time.UTC),
		SignalCount: 1,
		Sources:     []string{"bluesky"},
	}

	err := n.Notify(context.Background(), summary)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if len(receivedBody) == 0 {
		t.Fatal("expected request body, got empty")
	}

	// Verify it's valid JSON with blocks.
	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if _, ok := payload["blocks"]; !ok {
		t.Error("payload missing 'blocks' key")
	}
}

// =============================================================================
// Test_SlackNotifier_BlockFormat
// Verifies the Block Kit JSON structure contains expected elements.
// =============================================================================

func Test_SlackNotifier_BlockFormat(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := notify.NewSlackNotifier(server.URL, "#test")

	summary := &notify.DigestSummary{
		Signals: []notify.DigestSignal{
			{Title: "Signal A", SourceURL: "https://a.com", Provider: "bluesky", Teaser: "Teaser A"},
			{Title: "Signal B", SourceURL: "https://b.com", Provider: "youtube"},
		},
		GeneratedAt: time.Date(2026, 3, 3, 7, 0, 0, 0, time.UTC),
		SignalCount: 2,
		Sources:     []string{"bluesky", "youtube"},
	}

	err := n.Notify(context.Background(), summary)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	blocks, ok := receivedPayload["blocks"].([]any)
	if !ok {
		t.Fatal("blocks is not an array")
	}

	// Expect: header, context (stats), divider, signal A, signal B = 5 blocks.
	if len(blocks) != 5 {
		t.Errorf("expected 5 blocks, got %d", len(blocks))
	}

	// First block should be a header.
	first := blocks[0].(map[string]any)
	if first["type"] != "header" {
		t.Errorf("first block type = %q, want header", first["type"])
	}

	// Third block should be a divider.
	third := blocks[2].(map[string]any)
	if third["type"] != "divider" {
		t.Errorf("third block type = %q, want divider", third["type"])
	}
}

// =============================================================================
// Test_SlackNotifier_HTTPError
// Verifies that a non-200 response returns an error.
// =============================================================================

func Test_SlackNotifier_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	n := notify.NewSlackNotifier(server.URL, "#test")
	summary := &notify.DigestSummary{
		GeneratedAt: time.Now(),
		SignalCount: 1,
		Signals:     []notify.DigestSignal{{Title: "X", SourceURL: "https://x.com", Provider: "bluesky"}},
		Sources:     []string{"bluesky"},
	}

	err := n.Notify(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// =============================================================================
// Test_SlackNotifier_NetworkError
// Verifies that an unreachable URL returns an error.
// =============================================================================

func Test_SlackNotifier_NetworkError(t *testing.T) {
	n := notify.NewSlackNotifier("http://127.0.0.1:1", "#test")
	summary := &notify.DigestSummary{
		GeneratedAt: time.Now(),
		SignalCount: 1,
		Signals:     []notify.DigestSignal{{Title: "X", SourceURL: "https://x.com", Provider: "bluesky"}},
		Sources:     []string{"bluesky"},
	}

	err := n.Notify(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for unreachable URL, got nil")
	}
}

// =============================================================================
// Test_SlackNotifier_EmptyDigest
// Verifies that an empty digest posts a "no new signals" message.
// =============================================================================

func Test_SlackNotifier_EmptyDigest(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := notify.NewSlackNotifier(server.URL, "#test")
	summary := &notify.DigestSummary{
		GeneratedAt: time.Now(),
		SignalCount: 0,
		Signals:     []notify.DigestSignal{},
		Sources:     []string{},
	}

	err := n.Notify(context.Background(), summary)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	blocks, ok := receivedPayload["blocks"].([]any)
	if !ok {
		t.Fatal("blocks is not an array")
	}

	// Should have header + "no new signals" section = 2 blocks.
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks for empty digest, got %d", len(blocks))
	}

	// Second block should mention "no new signals".
	second := blocks[1].(map[string]any)
	text := second["text"].(map[string]any)
	if textStr, ok := text["text"].(string); !ok || textStr != "No new signals found in this run." {
		t.Errorf("empty message = %q, want 'No new signals found in this run.'", textStr)
	}
}
