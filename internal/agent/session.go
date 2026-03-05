package agent

import "sync"

// Message represents a single message in a conversation.
type Message struct {
	Role       string // "user", "assistant", "tool"
	Content    string
	ToolCallID string // set when Role == "tool"
	ToolName   string // set when Role == "tool"
}

// estimateTokens gives a rough token count for a message.
// Rule of thumb: ~4 characters per token.
func estimateTokens(msg Message) int {
	return (len(msg.Content) + len(msg.Role)) / 4
}

// Session holds per-user conversation context with a sliding window.
type Session struct {
	UserID    string
	messages  []Message
	maxTokens int
	mu        sync.Mutex
}

// NewSession creates a session with the given token budget.
func NewSession(userID string, maxTokens int) *Session {
	return &Session{
		UserID:    userID,
		maxTokens: maxTokens,
	}
}

// AddMessage appends a message to the conversation history.
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

// Window returns the most recent messages that fit within the token budget.
// Messages are trimmed from the front (oldest first) to stay under budget.
func (s *Session) Window() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.messages) == 0 {
		return nil
	}

	// Walk backwards, summing tokens until we'd exceed the budget.
	total := 0
	start := len(s.messages)
	for i := len(s.messages) - 1; i >= 0; i-- {
		tokens := estimateTokens(s.messages[i])
		if total+tokens > s.maxTokens {
			break
		}
		total += tokens
		start = i
	}

	out := make([]Message, len(s.messages)-start)
	copy(out, s.messages[start:])
	return out
}

// Reset clears the conversation history.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

// SessionStore is a thread-safe store of per-user Sessions.
type SessionStore struct {
	sessions  map[string]*Session
	mu        sync.RWMutex
	maxTokens int
}

// NewSessionStore creates an empty session store.
func NewSessionStore(maxTokens int) *SessionStore {
	return &SessionStore{
		sessions:  make(map[string]*Session),
		maxTokens: maxTokens,
	}
}

// GetOrCreate returns the existing session for the user, or creates a new one.
func (ss *SessionStore) GetOrCreate(userID string) *Session {
	ss.mu.RLock()
	s, ok := ss.sessions[userID]
	ss.mu.RUnlock()

	if ok {
		return s
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	// Double-check after write lock.
	if s, ok := ss.sessions[userID]; ok {
		return s
	}
	s = NewSession(userID, ss.maxTokens)
	ss.sessions[userID] = s
	return s
}

// Remove deletes a user's session.
func (ss *SessionStore) Remove(userID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.sessions, userID)
}
