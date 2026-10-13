package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var jiraCommentCmd = &cobra.Command{
	Use:          "comment <ticket-id> <comment>",
	Short:        "Post a comment to a Jira ticket",
	Args:         cobra.MinimumNArgs(2),
	SilenceUsage: true,
	Example:      "ssk jira comment PROJ-123 'hello world'",
	RunE:         runJiraComment,
}

func init() {
	jiraCmd.AddCommand(jiraCommentCmd)
}

func runJiraComment(cmd *cobra.Command, args []string) error {
	verboseEnabled := verbose || viper.GetBool("verbose")

	ticketKey := strings.TrimSpace(args[0])
	if ticketKey == "" {
		return fmt.Errorf("ticket-id cannot be empty")
	}

	commentText := strings.TrimSpace(strings.Join(args[1:], " "))
	if commentText == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	jiraClient, cleanup, err := initJiraClient(verboseEnabled)
	if err != nil {
		return err
	}
	defer cleanup()

	comment, err := jiraClient.AddComment(ticketKey, commentText)
	if err != nil {
		return err
	}

	if comment.ID != "" {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Comment posted on %s: %s\n", ticketKey, comment.ID)
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Comment posted on %s\n", ticketKey)
	return err
}
