package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

const gettingStarted = `

Getting Started:
  To get started with Signal-Flow CLI, run 'signal-flow enable' to configure
  your environment. For more information, visit:
  https://github.com/rvald/signal-flow/getting-started

`

const accessibilityHelp = `
Environment Variables:
  ACCESSIBLE    Set to any value (e.g., ACCESSIBLE=1) to enable accessibility
                mode. This uses simpler text prompts instead of interactive
                TUI elements, which works better with screen readers.
`

// Version information (can be set at build time)
var (
	Version = "dev"
	Commit  = "unknown"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "signal-flow",
		Short:         "Signal-Flow CLI",
		Long:          "A command-line interface for Signal-Flow" + gettingStarted + accessibilityHelp,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Add subcommands here
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newFeedCmd())
	cmd.AddCommand(newFollowingCmd())
	cmd.AddCommand(newHarvestCmd())
	cmd.AddCommand(newLogoutCmd())

	// Replace default help command with custom one that supports -t flag

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("Entire CLI %s (%s)\n", Version, Commit)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}
