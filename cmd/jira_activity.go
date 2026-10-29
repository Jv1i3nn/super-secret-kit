package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/services"
)

func newJiraActivityCommand() *cobra.Command {
	command := &cobra.Command{Use: "activity", Short: "Export current Jira context and dated activity for a period", Args: cobra.NoArgs, SilenceUsage: true, RunE: runJiraActivity}
	addJiraCompactFlags(command)
	command.Flags().String("since", "", "reporting period start, RFC 3339 with timezone (required; inclusive)")
	command.Flags().String("until", "", "reporting period end, RFC 3339 with timezone (exclusive; default: command start)")
	command.Flags().String("format", "markdown", "output format (markdown, json)")
	command.Flags().Int("workers", 6, "maximum concurrent issue retrievals (1-32)")
	return command
}

func init() { jiraCmd.AddCommand(newJiraActivityCommand()) }

func jiraActivityOptions(cmd *cobra.Command, started time.Time) (services.JiraActivityOptions, error) {
	var opts services.JiraActivityOptions
	since, err := cmd.Flags().GetString("since")
	if err != nil {
		return opts, err
	}
	if strings.TrimSpace(since) == "" {
		return opts, fmt.Errorf("--since is required")
	}
	until, err := cmd.Flags().GetString("until")
	if err != nil {
		return opts, err
	}
	if cmd.Flags().Changed("until") && strings.TrimSpace(until) == "" {
		return opts, fmt.Errorf("--until cannot be empty")
	}
	opts.Period, err = services.ParseJiraPeriod(since, until, started)
	if err != nil {
		return opts, fmt.Errorf("--since/--until: %w", err)
	}
	opts.Workers, err = cmd.Flags().GetInt("workers")
	if err != nil {
		return opts, err
	}
	if opts.Workers < 1 || opts.Workers > 32 {
		return opts, fmt.Errorf("--workers must be between 1 and 32")
	}
	compact, err := jiraHistoryOptions(cmd, started)
	if err != nil {
		return opts, err
	}
	opts.DescriptionLines, opts.CommentChars = compact.DescriptionLines, compact.CommentChars
	if compact.CommentsLimit != nil {
		opts.CommentsLimit = *compact.CommentsLimit
	}
	opts.IncludeDevelopment, err = cmd.Flags().GetBool("include-development")
	return opts, err
}

func runJiraActivity(cmd *cobra.Command, _ []string) error {
	started := time.Now().UTC()
	format, err := jiraOutputFormat(cmd, "markdown")
	if err != nil {
		return err
	}
	opts, err := jiraActivityOptions(cmd, started)
	if err != nil {
		return err
	}
	verboseEnabled := verbose || viper.GetBool("verbose")
	client, cleanup, err := initJiraClient(cmd, verboseEnabled)
	if err != nil {
		return err
	}
	defer cleanup()
	service := services.NewJiraActivityService(client)
	service.SetVerbose(verboseEnabled, os.Stderr)
	result, err := service.Generate(client.Context(), effectiveJiraWorkspace(), opts, started)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJiraJSON(client.Context(), cmd.OutOrStdout(), result)
	}
	markdown, err := services.RenderJiraActivityMarkdown(result)
	if err != nil {
		return err
	}
	if contextErr := client.Context().Err(); contextErr != nil {
		return contextErr
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), markdown)
	return err
}
