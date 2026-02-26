package cli

import (
	"fmt"

	"github.com/rvald/signal-flow/internal/config"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored Bluesky session",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.ClearSession(); err != nil {
				return fmt.Errorf("logout: %w", err)
			}
			fmt.Println("Logged out — session cleared.")
			return nil
		},
	}
}
