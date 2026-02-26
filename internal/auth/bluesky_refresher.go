package auth

import (
	"context"
	"fmt"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rvald/signal-flow/internal/config"
)

// RefreshBlueskySession uses the stored RefreshJwt to obtain new JWTs
// via comatproto.ServerRefreshSession (app-password flow).
func RefreshBlueskySession(ctx context.Context, session *config.BlueskySession) (*config.BlueskySession, error) {
	client := &xrpc.Client{
		Host: session.Host,
		Auth: &xrpc.AuthInfo{
			AccessJwt:  session.AccessJwt,
			RefreshJwt: session.RefreshJwt,
			Did:        session.DID,
			Handle:     session.Handle,
		},
	}

	refreshed, err := comatproto.ServerRefreshSession(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("refresh session: %w", err)
	}

	return &config.BlueskySession{
		AccessJwt:  refreshed.AccessJwt,
		RefreshJwt: refreshed.RefreshJwt,
		Handle:     refreshed.Handle,
		DID:        refreshed.Did,
		Host:       session.Host,
		CreatedAt:  session.CreatedAt,
	}, nil
}
