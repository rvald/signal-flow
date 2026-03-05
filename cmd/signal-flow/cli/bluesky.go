package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rvald/signal-flow/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newBlueskyLoginCmd() *cobra.Command {
	var host string
	var identifier string
	var password string
	var passwordFile string

	cmd := &cobra.Command{
		Use:   "bluesky-login",
		Short: "Log in to Bluesky using an app password",
		Long: `Authenticate with Bluesky using your handle and an app password.
Create an app password at: Settings → App Passwords in the Bluesky app.

The session is stored locally at ~/.config/signal-flow/session.json
and refreshed automatically on subsequent commands.

For security, prefer --password-file over --password (which is visible in
process listings). If neither is provided, you will be prompted interactively.`,
		Example: `  # Interactive prompt (most secure)
  signal-flow bluesky-login --identifier spike.bsky.social

  # Read password from a file
  signal-flow bluesky-login --identifier spike.bsky.social --password-file ~/.bsky-app-pw`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pw, err := resolvePassword(password, passwordFile)
			if err != nil {
				return err
			}
			return runLogin(cmd.Context(), host, identifier, pw)
		},
	}

	cmd.Flags().StringVar(&host, "host", "https://bsky.social", "PDS host URL")
	cmd.Flags().StringVar(&identifier, "identifier", "", "your handle (e.g. spike.bsky.social)")
	cmd.Flags().StringVar(&password, "password", "", "app password (insecure — prefer --password-file or interactive prompt)")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "path to file containing app password")

	cmd.MarkFlagRequired("identifier")

	return cmd
}

// resolvePassword determines the password from flags, file, or interactive prompt.
func resolvePassword(flagPassword, passwordFilePath string) (string, error) {
	// 1. --password-file takes priority (most secure flag-based option)
	if passwordFilePath != "" {
		b, err := os.ReadFile(passwordFilePath)
		if err != nil {
			return "", fmt.Errorf("read password file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}

	// 2. --password flag (warn about insecurity)
	if flagPassword != "" {
		fmt.Fprintln(os.Stderr, "⚠ --password is insecure (visible in ps output and shell history).")
		fmt.Fprintln(os.Stderr, "  Consider --password-file or interactive prompt instead.")
		return flagPassword, nil
	}

	// 3. Interactive prompt (only if stdin is a TTY)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "App password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(pw)), nil
	}

	return "", fmt.Errorf("password required: use --password-file, --password, or run interactively")
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

	fmt.Fprintf(os.Stderr, "Logged in as: %s (%s)\n", session.Handle, session.Did)
	fmt.Fprintln(os.Stderr, "Session saved to ~/.config/signal-flow/session.json")

	return nil
}
