package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/config"
	"ssk/internal/database"
	"ssk/internal/models"
	"ssk/internal/repository"
	"ssk/internal/services"
)

var (
	autoFillDryRun bool
)

var autofillTimeGapsCmd = &cobra.Command{
	Use:   "autofill-time-gaps",
	Short: "Automatically fill recent Tempo gaps to 8h",
	Long: `Automatically fill missing Tempo time for today and the previous 5 days.

Once the Jira URL is configured on first use, the command is non-interactive.
For each eligible day below 8 hours, it selects
the most recently updated assigned Jira ticket that already existed on that day
and was not done yet on that day.`,
	RunE: runAutofillTimeGaps,
}

func init() {
	rootCmd.AddCommand(autofillTimeGapsCmd)
	autofillTimeGapsCmd.Flags().BoolVar(&autoFillDryRun, "dry-run", false, "preview actions without creating Tempo worklogs")
}

func runAutofillTimeGaps(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	apiURL, err := ensureJiraAPIURL(cfg, config.DefaultJiraWorkspace)
	if err != nil {
		return err
	}

	if operationErr := database.Connect(cfg.GetDatabasePath(), viper.GetBool("verbose")); operationErr != nil {
		return fmt.Errorf("failed to connect to database: %w", operationErr)
	}
	defer func() { _ = database.Close() }()

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	repo := repository.NewCredentialsRepository(database.GetDB())
	credsService := services.NewCredentialsService(repo, encryptionService)

	jiraCreds, err := credsService.Get(models.ServiceJira)
	if err != nil {
		return fmt.Errorf("missing Jira credentials: %w", err)
	}

	tempoCreds, err := credsService.Get(models.ServiceJiraTempo)
	if err != nil {
		return fmt.Errorf("missing Tempo credentials: %w", err)
	}

	jiraTempoService, err := services.NewJiraTempoService(apiURL, cfg.TempoAPIURL, jiraCreds, tempoCreds)
	if err != nil {
		return fmt.Errorf("failed to initialize Jira/Tempo service: %w", err)
	}

	result, err := jiraTempoService.AutoFillTimeGaps(time.Now(), autoFillDryRun)
	if err != nil {
		return err
	}

	printAutoFillResult(result)

	if result.FailedDays > 0 {
		return fmt.Errorf("autofill completed with %d failed day(s)", result.FailedDays)
	}

	return nil
}

func printAutoFillResult(result *services.AutoFillResult) {
	actionLabel := "Auto-filling"
	if result.DryRun {
		actionLabel = "Dry-run for"
	}

	fmt.Printf("%s Tempo gaps for %s to %s\n\n",
		actionLabel,
		result.From.Format("2006-01-02"),
		result.To.Format("2006-01-02"),
	)

	for _, day := range result.Days {
		dateStr := day.Date.Format("2006-01-02")

		switch day.Status {
		case "filled":
			verb := "Filled"
			if result.DryRun {
				verb = "Would fill"
			}
			fmt.Printf("%s %s: +%s on %s\n", verb, dateStr, formatSecondsAsHours(day.AddedSeconds), day.TicketKey)
		case "failed":
			fmt.Printf("Failed %s: %s\n", dateStr, day.Error)
		default:
			fmt.Printf("Skipping %s: %s\n", dateStr, day.Reason)
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("- inspected: %d days\n", result.InspectedDays)
	fmt.Printf("- skipped: %d days\n", result.SkippedDays)
	fmt.Printf("- filled: %d days\n", result.FilledDays)
	fmt.Printf("- failed: %d days\n", result.FailedDays)
}

func formatSecondsAsHours(seconds int) string {
	hours := float64(seconds) / 3600.0
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	return fmt.Sprintf("%.1fh", hours)
}
