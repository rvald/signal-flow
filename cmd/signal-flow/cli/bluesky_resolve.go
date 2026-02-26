package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rvald/signal-flow/internal/auth"
	"github.com/rvald/signal-flow/internal/config"
)

// isExpiredTokenErr returns true if the error is an XRPC "ExpiredToken" error.
func isExpiredTokenErr(err error) bool {
	var xrpcErr *xrpc.XRPCError
	if errors.As(err, &xrpcErr) {
		return xrpcErr.ErrStr == "ExpiredToken"
	}
	// Also check the error string as a fallback for wrapped errors.
	return strings.Contains(err.Error(), "ExpiredToken")
}

// wrapExpiredTokenErr checks if err is an expired-token error and, if so, wraps
// it with a user-friendly message instructing them to re-authenticate.
func wrapExpiredTokenErr(err error) error {
	if isExpiredTokenErr(err) {
		return fmt.Errorf("session expired — please log in again:\n\n  signal-flow login --identifier <your-handle> --password <your-app-password>\n\nOriginal error: %w", err)
	}
	return err
}

// FollowInfo holds metadata about a followed account.
type FollowInfo struct {
	DID         string
	Handle      string
	DisplayName string
}

// fetchFollows returns the full list of accounts the actor follows,
// paginating through all results.
func fetchFollows(ctx context.Context, client *xrpc.Client, actorDID string) ([]FollowInfo, error) {
	var all []FollowInfo
	cursor := ""
	for {
		resp, err := bsky.GraphGetFollows(ctx, client, actorDID, cursor, 100)
		if err != nil {
			return nil, fmt.Errorf("GraphGetFollows: %w", err)
		}
		for _, f := range resp.Follows {
			displayName := ""
			if f.DisplayName != nil {
				displayName = *f.DisplayName
			}
			all = append(all, FollowInfo{
				DID:         f.Did,
				Handle:      f.Handle,
				DisplayName: displayName,
			})
		}
		if resp.Cursor == nil || *resp.Cursor == "" {
			break
		}
		cursor = *resp.Cursor
	}
	return all, nil
}

// fetchFollowDIDs returns a set of DIDs for all accounts the actor follows.
func fetchFollowDIDs(ctx context.Context, client *xrpc.Client, actorDID string) (map[string]bool, error) {
	follows, err := fetchFollows(ctx, client, actorDID)
	if err != nil {
		return nil, err
	}
	dids := make(map[string]bool, len(follows))
	for _, f := range follows {
		dids[f.DID] = true
	}
	return dids, nil
}

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
		// If the refresh token itself is expired, there's no point
		// falling back to the (also-expired) access token. Tell the
		// user to re-authenticate.
		if isExpiredTokenErr(err) {
			return nil, nil, wrapExpiredTokenErr(err)
		}

		// Refresh failed for another reason (network, etc.) — the
		// access JWT might still be valid. Let the caller try.
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
