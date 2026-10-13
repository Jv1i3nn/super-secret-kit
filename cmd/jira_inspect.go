package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/services"
)

var (
	jiraInspectDepth       int
	jiraInspectMaxRelated  int
	jiraInspectDownloadDir string
)

var jiraInspectCmd = &cobra.Command{
	Use:          "inspect <jira_ticket_id>",
	Short:        "Fetch full Jira ticket context for a code agent",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Long: `Fetch full Jira context for one ticket and its related tickets.

The command prints deterministic Markdown context intended to be consumed by a
code agent, not a polished user-facing briefing.`,
	RunE: runJiraInspect,
}

func init() {
	jiraCmd.AddCommand(jiraInspectCmd)
	jiraInspectCmd.Flags().IntVar(&jiraInspectDepth, "depth", 2, "relationship traversal depth")
	jiraInspectCmd.Flags().IntVar(&jiraInspectMaxRelated, "max-related", 10, "maximum number of related Jira tickets to fetch")
	jiraInspectCmd.Flags().StringVar(&jiraInspectDownloadDir, "download-attachments", "", "download Jira attachments and embedded media to this directory")
}

func runJiraInspect(cmd *cobra.Command, args []string) error {
	verboseEnabled := verbose || viper.GetBool("verbose")
	ticketKey := strings.TrimSpace(args[0])
	if ticketKey == "" {
		return fmt.Errorf("jira_ticket_id cannot be empty")
	}
	if jiraInspectDepth < 0 {
		return fmt.Errorf("--depth cannot be negative")
	}
	if jiraInspectMaxRelated < 0 {
		return fmt.Errorf("--max-related cannot be negative")
	}

	jiraClient, cleanup, err := initJiraClient(verboseEnabled)
	if err != nil {
		return err
	}
	defer cleanup()

	briefingService := services.NewBriefingService(jiraClient)
	briefingService.SetVerbose(verboseEnabled, os.Stderr)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	opts := services.TicketInspectionOptions{
		Depth:      jiraInspectDepth,
		MaxRelated: jiraInspectMaxRelated,
	}
	var context string
	if jiraInspectDownloadDir != "" {
		context, err = briefingService.GenerateTicketInspectionWithDownloads(ctx, ticketKey, opts, jiraInspectDownloadDir)
	} else {
		context, err = briefingService.GenerateTicketInspectionWithOptions(ctx, ticketKey, opts)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(cmd.OutOrStdout(), context)
	return err
}
