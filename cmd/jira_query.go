package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/services"
	"ssk/internal/services/jira"
)

var jiraQueryCmd = &cobra.Command{
	Use:          "query <jql>",
	Short:        "Run an arbitrary JQL query and list matching tickets",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Long: `Run an arbitrary JQL query against Jira and print matching tickets
grouped by status.

Unlike "briefing", this passes your JQL through unchanged, so it can include
Done tickets, arbitrary date ranges, or any other filter.

Example:
  ssk jira query 'assignee = currentUser() AND updated >= "2026/06/01" AND updated < "2026/07/01" ORDER BY status'`,
	RunE: runJiraQuery,
}

func init() {
	jiraCmd.AddCommand(jiraQueryCmd)
	jiraQueryCmd.Flags().String("format", "text", "output format (text, json)")
}

func runJiraQuery(cmd *cobra.Command, args []string) error {
	started := time.Now()
	format, err := jiraOutputFormat(cmd, "text")
	if err != nil {
		return err
	}
	verboseEnabled := verbose || viper.GetBool("verbose")

	jql := strings.TrimSpace(args[0])
	if jql == "" {
		return fmt.Errorf("jql cannot be empty")
	}

	jiraClient, cleanup, err := initJiraClient(verboseEnabled)
	if err != nil {
		return err
	}
	defer cleanup()

	tickets, err := jiraClient.SearchTickets(jql)
	if err != nil {
		return err
	}

	if format == "json" {
		result, exportErr := services.BuildJiraExport("jira.query", effectiveJiraWorkspace(), jiraClient.BaseURL(), &jql, started, tickets)
		if exportErr != nil {
			return exportErr
		}
		return writeJiraJSON(cmd.OutOrStdout(), result)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), formatQueryResults(tickets))
	return err
}

// formatQueryResults renders tickets grouped by status, statuses sorted, tickets by key.
func formatQueryResults(tickets []jira.Ticket) string {
	if len(tickets) == 0 {
		return "No tickets matched.\n"
	}

	byStatus := make(map[string][]jira.Ticket)
	for _, ticket := range tickets {
		status := ticket.Status()
		if status == "" {
			status = "(no status)"
		}
		byStatus[status] = append(byStatus[status], ticket)
	}

	statuses := make([]string, 0, len(byStatus))
	for status := range byStatus {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)

	var b strings.Builder
	fmt.Fprintf(&b, "%d ticket(s)\n", len(tickets))
	for _, status := range statuses {
		group := byStatus[status]
		sort.Slice(group, func(i, j int) bool { return group[i].Key < group[j].Key })
		fmt.Fprintf(&b, "\n%s (%d)\n", status, len(group))
		for _, ticket := range group {
			fmt.Fprintf(&b, "  %-12s %s\n", ticket.Key, ticket.Summary())
		}
	}
	return b.String()
}
