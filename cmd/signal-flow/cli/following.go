package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFollowingCmd() *cobra.Command {
	var asJSON bool
	var limit int64

	cmd := &cobra.Command{
		Use:   "following",
		Short: "List accounts you follow on Bluesky",
		Long: `Fetches the full list of accounts you follow on Bluesky.
Useful for understanding your network and seeing who feeds your timeline.`,
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

			if asJSON {
				for _, f := range display {
					fmt.Printf("{\"did\": %q, \"handle\": %q, \"display_name\": %q}\n",
						f.DID, f.Handle, f.DisplayName)
				}
			} else {
				for i, f := range display {
					name := f.Handle
					if f.DisplayName != "" {
						name = f.DisplayName + " (@" + f.Handle + ")"
					}
					fmt.Printf("  %d. %s\n", i+1, name)
				}
			}

			shown := len(display)
			total := len(follows)
			if shown < total {
				fmt.Printf("\nShowing %d of %d follows.\n", shown, total)
			} else {
				fmt.Printf("\nFollowing %d accounts.\n", total)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON lines")
	cmd.Flags().Int64Var(&limit, "limit", 0, "max number of follows to display (0 = all)")

	return cmd
}
