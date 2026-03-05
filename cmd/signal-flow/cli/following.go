package cli

import (
	"fmt"
	"os"

	"github.com/rvald/signal-flow/internal/outfmt"
	"github.com/spf13/cobra"
)

func newFollowingCmd() *cobra.Command {
	var limit int64

	cmd := &cobra.Command{
		Use:   "following",
		Short: "List accounts you follow on Bluesky",
		Long: `Fetches the full list of accounts you follow on Bluesky.
Useful for understanding your network and seeing who feeds your timeline.`,
		Example: `  # List all followed accounts
  signal-flow following

  # Output as JSON for scripting
  signal-flow following --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, session, err := resolveBlueskyClient(ctx)
			if err != nil {
				return err
			}

			follows, err := fetchFollows(ctx, client, session.DID)
			if err != nil {
				return wrapExpiredTokenErr(fmt.Errorf("fetch follows: %w", err))
			}

			// Apply limit if set.
			display := follows
			if limit > 0 && int(limit) < len(follows) {
				display = follows[:limit]
			}

			if outfmt.IsJSON(cmd.Context()) {
				type followJSON struct {
					DID         string `json:"did"`
					Handle      string `json:"handle"`
					DisplayName string `json:"display_name"`
				}
				items := make([]followJSON, 0, len(display))
				for _, f := range display {
					items = append(items, followJSON{DID: f.DID, Handle: f.Handle, DisplayName: f.DisplayName})
				}
				return outfmt.WriteJSON(cmd.Context(), os.Stdout, map[string]any{"follows": items})
			}

			for i, f := range display {
				name := f.Handle
				if f.DisplayName != "" {
					name = f.DisplayName + " (@" + f.Handle + ")"
				}
				fmt.Printf("  %d. %s\n", i+1, name)
			}

			shown := len(display)
			total := len(follows)
			if shown < total {
				fmt.Fprintf(os.Stderr, "\nShowing %d of %d follows.\n", shown, total)
			} else {
				fmt.Fprintf(os.Stderr, "\nFollowing %d accounts.\n", total)
			}

			return nil
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 0, "max number of follows to display (0 = all)")

	return cmd
}
