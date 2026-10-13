package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ssk/internal/config"
	"ssk/internal/database"
	"ssk/internal/models"
	"ssk/internal/repository"
	"ssk/internal/services"
	"ssk/internal/services/jira"
)

var jiraWorkspace string

var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Inspect Jira tickets and generate agent context",
	Long: `Inspect Jira tickets and generate deterministic Markdown context
intended to be consumed by code agents.`,
}

func init() {
	rootCmd.AddCommand(jiraCmd)
	jiraCmd.PersistentFlags().StringVar(&jiraWorkspace, "workspace", config.DefaultJiraWorkspace,
		"Jira workspace to target (configured under jira_workspaces)")
}

func initJiraClient(verboseEnabled bool) (*jira.Client, func(), error) {
	return initJiraClientForWorkspace(verboseEnabled, jiraWorkspace)
}

func initJiraClientForWorkspace(verboseEnabled bool, workspace string) (*jira.Client, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	jiraVerbose(verboseEnabled, "configuration loaded")

	apiURL, err := ensureJiraAPIURL(cfg, workspace)
	if err != nil {
		return nil, nil, err
	}
	jiraVerbose(verboseEnabled, "using Jira workspace %q (%s)", workspace, apiURL)

	if operationErr := database.Connect(cfg.GetDatabasePath(), verboseEnabled); operationErr != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", operationErr)
	}
	cleanup := func() { _ = database.Close() }
	jiraVerbose(verboseEnabled, "database connected")

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to initialize encryption service: %w", err)
	}
	jiraVerbose(verboseEnabled, "encryption service initialized")

	repo := repository.NewCredentialsRepository(database.GetDB())
	credsService := services.NewCredentialsService(repo, encryptionService)

	credsKey := models.ServiceJira
	jiraCreds, err := credsService.Get(credsKey)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("missing Jira credentials (run `ssk credentials set %s <username> <password>`): %w", credsKey, err)
	}
	jiraVerbose(verboseEnabled, "Jira credentials loaded (shared across workspaces) for workspace %q", workspace)

	return jira.NewClient(apiURL, jiraCreds), cleanup, nil
}

func jiraVerbose(enabled bool, format string, args ...interface{}) {
	if !enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "jira: "+format+"\n", args...)
}
