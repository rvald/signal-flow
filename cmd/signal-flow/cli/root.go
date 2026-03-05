package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/rvald/signal-flow/internal/outfmt"
	"github.com/rvald/signal-flow/internal/ui"
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

type RootFlags struct {
	Color          string `help:"Color output: auto|always|never" default:"${color}"`
	Account        string `help:"Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript)" aliases:"acct" short:"a"`
	Client         string `help:"OAuth client name (selects stored credentials + token bucket)" default:"${client}"`
	EnableCommands string `help:"Comma-separated list of enabled top-level commands (restricts CLI)" default:"${enabled_commands}"`
	JSON           bool   `help:"Output JSON to stdout (best for scripting)" default:"${json}" aliases:"machine" short:"j"`
	Plain          bool   `help:"Output stable, parseable text to stdout (TSV; no colors)" default:"${plain}" aliases:"tsv" short:"p"`
	ResultsOnly    bool   `name:"results-only" help:"In JSON mode, emit only the primary result (drops envelope fields like nextPageToken)"`
	Select         string `name:"select" aliases:"pick,project" help:"In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands."`
	DryRun         bool   `help:"Do not make changes; print intended actions and exit successfully" aliases:"noop,preview,dryrun" short:"n"`
	Force          bool   `help:"Skip confirmations for destructive commands" aliases:"yes,assume-yes" short:"y"`
	NoInput        bool   `help:"Never prompt; fail instead (useful for CI)" aliases:"non-interactive,noninteractive"`
	Verbose        bool   `help:"Enable verbose logging" short:"v"`
}

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "signal-flow",
		Short:         "Signal-Flow CLI",
		Long:          "A command-line interface for Signal-Flow\n" + gettingStarted + accessibilityHelp + "\nReport issues: https://github.com/rvald/signal-flow/issues\n",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Extract our global format + color flags
			jsonOutput, _ := cmd.Flags().GetBool("json")
			plainOutput, _ := cmd.Flags().GetBool("plain")
			colorMode, _ := cmd.Flags().GetString("color")

			// Initialize UI explicitly and inject it into the command context
			u, err := ui.New(ui.Options{
				Color: colorMode,
			})
			if err != nil {
				return err
			}

			// Replace context inside cobra command with the new ui-injected context
			ctx := ui.WithUI(cmd.Context(), u)
			// Add Output Format (JSON/Plain) to context

			mode, err := outfmt.FromFlags(jsonOutput, plainOutput)
			if err != nil {
				return err // Returns error if user passes both --json AND --plain
			}
			ctx = outfmt.WithMode(ctx, mode)

			cmd.SetContext(ctx)

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// 1. ADD THESE GLOBAL FLAGS!
	cmd.PersistentFlags().BoolP("json", "j", false, "Output JSON to stdout (best for scripting)")
	cmd.PersistentFlags().BoolP("plain", "p", false, "Output stable, parseable text to stdout (TSV)")
	cmd.PersistentFlags().String("color", "auto", "Color output: auto|always|never")

	// Add subcommands here
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newBlueskyLoginCmd())
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newFeedCmd())
	cmd.AddCommand(newFollowingCmd())
	cmd.AddCommand(newHarvestCmd())
	cmd.AddCommand(newSynthesizeCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newYoutubeCmd())
	cmd.AddCommand(newPipelineCmd())

	// Replace default help command with custom one that supports -t flag

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(cmd.Context(), os.Stdout, map[string]any{
					"version":    Version,
					"commit":     Commit,
					"go_version": runtime.Version(),
					"os":         runtime.GOOS,
					"arch":       runtime.GOARCH,
				})
			}
			fmt.Printf("signal-flow %s (%s)\n", Version, Commit)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
