package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/rvald/signal-flow/internal/googleapi"
	"github.com/rvald/signal-flow/internal/outfmt"
	"github.com/rvald/signal-flow/internal/ui"

	"github.com/spf13/cobra"
)

var newYoutubeService = googleapi.NewYoutube

func newYoutubeCmd() *cobra.Command {
	
	cmd := &cobra.Command{
		Use:   "youtube",
		Short: "Youtube api related commands",
	}

	
	cmd.PersistentFlags().StringP("account", "a", "", "The Google account email to use")

	cmd.AddCommand(newSubscriptionListCmd())
	cmd.AddCommand(newActivitiesListCmd())

	return cmd
}

type YoutubeSubscriptionsListCmd struct {
	Mine       bool
	MaxResults int
	Part       []string
}

func newSubscriptionListCmd() *cobra.Command {
	var part []string
	var maxResults int
	var mine bool

	cmd := &cobra.Command{
		Use:   "subscription-list",
		Short: "Fetch the user's subscriptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			accountEmail, err := cmd.Flags().GetString("account")
			if err != nil {
				return err
			}
			sublistCmd := &YoutubeSubscriptionsListCmd{
				Part:       part,
				MaxResults: maxResults,
				Mine:       mine,
			}

			flags := &RootFlags{
				Account: accountEmail,
			}
			return sublistCmd.Run(cmd.Context(), flags)

		},
	}

	cmd.Flags().BoolVar(&mine, "mine", true, "Set this parameter's value to true to retrieve a feed of the authenticated user's subscriptions.")
	cmd.Flags().IntVar(&maxResults, "maxResults", 5, "The maxResults parameter specifies the maximum number of items that should be returned in the result set. Acceptable values are 0 to 50, inclusive. The default value is 5.")
	cmd.Flags().StringArrayVar(&part, "part", []string{"snippet"}, "The part parameter specifies a comma-separated list of one or more subscription resource properties that the API response will include.")

	return cmd
}

func (c *YoutubeSubscriptionsListCmd) Run(ctx context.Context, flags *RootFlags) error {

	// Extract the UI logger from the context (provided by root.go)
	u := ui.FromContext(ctx)

	if c.MaxResults > 50 {
		return usage("max search results is 50")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Subscriptions.List(c.Part)
	call = call.Mine(c.Mine)
	response, err := call.Do()
	if err != nil {
		return err
	}

	// 1. Handle JSON Output
	// If the user passed --json, this cleanly prints the struct and returns immediately
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"subscriptions": response.Items})
	}
	// 2. Handle Empty State
	// Use stderr so it doesn't pollute piped data streams
	if len(response.Items) == 0 {
		u.Err().Println("No subscriptions found")
		return nil
	}
	// 3. Handle Human Readability / TSV formatting
	// tableWriter handles pretty-printing for humans, OR pure TSV if --plain was passed
	w, flush := tableWriter(ctx)
	defer flush()
	// Print headers
	fmt.Fprintln(w, "ID\tTITLE\tCHANNEL_ID")
	
	// Print rows
	for _, item := range response.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\n", item.Id, item.Snippet.Title, item.Snippet.ResourceId.ChannelId)
	}

	return nil
}


type YoutubeActivitiesListCmd struct {
	Part []string
	ChannedId string
	MaxResults int
}

func newActivitiesListCmd() *cobra.Command {
	var part []string
	var maxResults int
	var channelId string

	cmd := &cobra.Command {
		Use: "activities-list",
		Short: "Returns a list of channel activity events that match the request criteria.",
		RunE: func(cmd *cobra.Command, args []string) error {
			accountEmail, err := cmd.Flags().GetString("account")
			if err != nil {
				return err
			}
			activitiesCmd := &YoutubeActivitiesListCmd{
				Part: part,
				MaxResults: maxResults,
				ChannedId: channelId,
			}

			flags := &RootFlags {
				Account: accountEmail,
			}

			return activitiesCmd.Run(cmd.Context(), flags)
		},

	}

	cmd.Flags().StringVar(&channelId, "channelId", "", "The channelId parameter specifies a unique YouTube channel ID. The API will then return a list of that channel's activities.")
	cmd.Flags().IntVar(&maxResults, "maxResults", 5, "The maxResults parameter specifies the maximum number of items that should be returned in the result set. Acceptable values are 0 to 50, inclusive. The default value is 5.")
	cmd.Flags().StringArrayVar(&part, "part", []string{"snippet,contentDetails"}, "The part parameter specifies a comma-separated list of one or more subscription resource properties that the API response will include.")

	return cmd

}

func (c *YoutubeActivitiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	// Extract the UI logger from the context (provided by root.go)
	u := ui.FromContext(ctx)

	if c.MaxResults > 50 {
		return usage("max search results is 50")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newYoutubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Activities.List(c.Part)
	call = call.ChannelId(c.ChannedId)
	response, err := call.Do()
	if err != nil {
		return err
	}

	// 1. Handle JSON Output
	// If the user passed --json, this cleanly prints the struct and returns immediately
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"activities": response.Items})
	}
	// 2. Handle Empty State
	// Use stderr so it doesn't pollute piped data streams
	if len(response.Items) == 0 {
		u.Err().Println("No activity found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	// Print headers
	fmt.Fprintln(w, "ID\tTITLE\tCHANNEL_ID\tDESCRIPTION\tTYPE")
	
	// Print rows
	for _, item := range response.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.Id, item.Snippet.Title, item.Snippet.ChannelId, item.Snippet.Description, item.Snippet.Type)
	}

	return  nil
}