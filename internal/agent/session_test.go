package agent_test

import (
	"testing"

	"github.com/rvald/signal-flow/internal/agent"
)

// =============================================================================
// Test_Session_AddMessage
// Session stores messages and returns them.
// =============================================================================

func Test_Session_AddMessage(t *testing.T) {
	s := agent.NewSession("user1", 1000)

	s.AddMessage(agent.Message{Role: "user", Content: "hello"})
	s.AddMessage(agent.Message{Role: "assistant", Content: "hi"})

	msgs := s.Window()
	if len(msgs) != 2 {
		t.Fatalf("Window() returned %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
}

// =============================================================================
// Test_Session_Window_ExceedsBudget
// Window trims oldest messages when token count exceeds budget.
// =============================================================================

func Test_Session_Window_ExceedsBudget(t *testing.T) {
	// Budget of 10 tokens. Each ~14-char message ≈ 3-4 tokens.
	// So only ~2-3 messages should fit.
	s := agent.NewSession("user1", 10)

	s.AddMessage(agent.Message{Role: "user", Content: "aaaaaaaaaa"})      // ~10 tokens
	s.AddMessage(agent.Message{Role: "assistant", Content: "bbbbbbbbbb"}) // ~10 tokens
	s.AddMessage(agent.Message{Role: "user", Content: "cccccccccc"})      // ~10 tokens
	s.AddMessage(agent.Message{Role: "assistant", Content: "dddddddddd"}) // ~10 tokens
	s.AddMessage(agent.Message{Role: "user", Content: "eeeeeeeeee"})      // ~10 tokens
	s.AddMessage(agent.Message{Role: "assistant", Content: "ffffffffff"}) // ~10 tokens

	msgs := s.Window()
	// Should drop older messages to stay within budget
	if len(msgs) >= 6 {
		t.Errorf("Window() returned %d messages, expected fewer due to budget", len(msgs))
	}
	// Most recent message must always be present
	last := msgs[len(msgs)-1]
	if last.Content != "ffffffffff" {
		t.Errorf("last message = %q, want 'ffffffffff'", last.Content)
	}
}

// =============================================================================
// Test_Session_Reset
// Reset clears all messages.
// =============================================================================

func Test_Session_Reset(t *testing.T) {
	s := agent.NewSession("user1", 1000)
	s.AddMessage(agent.Message{Role: "user", Content: "hello"})
	s.Reset()

	msgs := s.Window()
	if len(msgs) != 0 {
		t.Fatalf("Window() after Reset = %d, want 0", len(msgs))
	}
}

// =============================================================================
// Test_SessionStore_GetOrCreate
// GetOrCreate creates a new session and returns existing one.
// =============================================================================

func Test_SessionStore_GetOrCreate(t *testing.T) {
	store := agent.NewSessionStore(1000)

	s1 := store.GetOrCreate("user1")
	s2 := store.GetOrCreate("user1")
	if s1 != s2 {
		t.Fatal("GetOrCreate should return the same session for the same user")
	}

	s3 := store.GetOrCreate("user2")
	if s1 == s3 {
		t.Fatal("GetOrCreate should return different sessions for different users")
	}
}

// =============================================================================
// Test_SessionStore_Remove
// Remove deletes a session. Next GetOrCreate creates a fresh one.
// =============================================================================

func Test_SessionStore_Remove(t *testing.T) {
	store := agent.NewSessionStore(1000)

	s1 := store.GetOrCreate("user1")
	s1.AddMessage(agent.Message{Role: "user", Content: "hello"})

	store.Remove("user1")

	s2 := store.GetOrCreate("user1")
	msgs := s2.Window()
	if len(msgs) != 0 {
		t.Fatalf("new session after Remove has %d messages, want 0", len(msgs))
	}
}
