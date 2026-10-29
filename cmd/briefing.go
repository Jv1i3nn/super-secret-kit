package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/services"
)

var briefingMaxTickets int

var briefingCmd = &cobra.Command{
	Use:          "briefing",
	Short:        "Fetch Jira briefing context for a code agent",
	SilenceUsage: true,
	Long: `Fetch Jira context for non-done tickets assigned to you and updated in
the last 30 days.

The command prints deterministic Markdown context intended to be consumed by a
code agent, not a polished user-facing briefing.`,
	RunE: runBriefing,
}

func init() {
	jiraCmd.AddCommand(briefingCmd)
	briefingCmd.Flags().IntVar(&briefingMaxTickets, "max-tickets", 10, "maximum number of Jira tickets to include")
}

func runBriefing(cmd *cobra.Command, args []string) error {
	verboseEnabled := verbose || viper.GetBool("verbose")

	if briefingMaxTickets <= 0 {
		return fmt.Errorf("--max-tickets must be greater than 0")
	}

	jiraClient, cleanup, err := initJiraClient(cmd, verboseEnabled)
	if err != nil {
		return err
	}
	defer cleanup()

	briefingService := services.NewBriefingService(jiraClient)
	briefingService.SetMaxTickets(briefingMaxTickets)
	briefingService.SetVerbose(verboseEnabled, os.Stderr)

	ctx := jiraClient.Context()

	context, err := briefingService.GenerateBriefing(ctx)
	if err != nil {
		return err
	}

	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), context)
	return err
}
