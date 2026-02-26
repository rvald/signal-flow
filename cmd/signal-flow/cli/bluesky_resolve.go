package cli

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/config"
)

// resolveBlueskyClient returns an authenticated xrpc.Client by:
// 1. Loading the session from the config file
// 2. Attempting to refresh if the token might be expired
// 3. Returning a ready-to-use client
//
// Returns a helpful error if no session exists.
func resolveBlueskyClient(ctx context.Context) (*xrpc.Client, *config.BlueskySession, error) {
	session, err := config.LoadSession()
	if err != nil {
		return nil, nil, err // ErrNoSession or file read error
	}

	// Try refreshing the session to ensure we have a valid access token.
	// ServerRefreshSession will succeed if the refresh JWT is still valid,
	// even if the access JWT has expired.
	refreshed, err := auth.RefreshBlueskySession(ctx, session)
	if err != nil {
		// Refresh failed — the session may still work if the access JWT
		// hasn't expired yet. Try using the original session.
		// If that also fails, the caller will get the API error.
		return clientFromSession(session), session, nil
	}

	// Save the refreshed session for next time.
	if saveErr := config.SaveSession(refreshed); saveErr != nil {
		// Non-fatal: we still have a valid session in memory.
		fmt.Printf("warning: could not save refreshed session: %v\n", saveErr)
	}

	return clientFromSession(refreshed), refreshed, nil
}

func clientFromSession(session *config.BlueskySession) *xrpc.Client {
	return &xrpc.Client{
		Host: session.Host,
		Auth: &xrpc.AuthInfo{
			AccessJwt:  session.AccessJwt,
			RefreshJwt: session.RefreshJwt,
			Did:        session.DID,
			Handle:     session.Handle,
		},
	}
}
