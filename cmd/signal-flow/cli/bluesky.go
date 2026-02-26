package cli

import (
	"context"
	"fmt"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rvald/signal-flow/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var host string
	var identifier string
	var password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Bluesky using an app password",
		Long: `Authenticate with Bluesky using your handle and an app password.
Create an app password at: Settings → App Passwords in the Bluesky app.

The session is stored locally at ~/.config/signal-flow/session.json
and refreshed automatically on subsequent commands.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd.Context(), host, identifier, password)
		},
	}

	cmd.Flags().StringVar(&host, "host", "https://bsky.social", "PDS host URL")
	cmd.Flags().StringVar(&identifier, "identifier", "", "your handle (e.g. spike.bsky.social)")
	cmd.Flags().StringVar(&password, "password", "", "your app password")

	cmd.MarkFlagRequired("identifier")
	cmd.MarkFlagRequired("password")

	return cmd
}

func runLogin(ctx context.Context, host, identifier, password string) error {
	client := &xrpc.Client{
		Host: host,
	}

	session, err := comatproto.ServerCreateSession(ctx, client, &comatproto.ServerCreateSession_Input{
		Identifier: identifier,
		Password:   password,
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Persist session to config file.
	bskySession := &config.BlueskySession{
		AccessJwt:  session.AccessJwt,
		RefreshJwt: session.RefreshJwt,
		Handle:     session.Handle,
		DID:        session.Did,
		Host:       host,
		CreatedAt:  time.Now(),
	}

	if err := config.SaveSession(bskySession); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	fmt.Printf("Logged in as: %s (%s)\n", session.Handle, session.Did)
	fmt.Println("Session saved to ~/.config/signal-flow/session.json")

	return nil
}
