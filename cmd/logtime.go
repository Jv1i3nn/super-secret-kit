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
	ticketKey       string
	dayStr          string
	fromStr         string
	toStr           string
	hours           int
	off             bool
	noConfirmation  bool
	resetCredsJira  bool
	resetCredsTempo bool
)

// logtimeCmd represents the logtime command
var logtimeCmd = &cobra.Command{
	Use:     "logtime",
	Aliases: []string{"tas_pas_timeshIte"},
	Short:   "Log your time on Jira Tempo",
	Long: `Log work time to Jira Tempo with smart filtering.

Features:
  - Automatically skips weekends
  - Automatically skips French holidays
  - Checks existing time entries
  - Interactive ticket selection

Examples:
  # Log 8 hours for today
  ssk logtime --ticket PROJ-123

  # Log 4 hours for specific day
  ssk logtime --ticket PROJ-123 --day 2025-09-15 --hours 4

  # Log full week
  ssk logtime --ticket PROJ-123 --from 2025-09-01 --to 2025-09-05

  # Log out-of-office time
  ssk logtime --off --day 2025-09-20

  # Log without confirmation prompt
  ssk logtime --ticket PROJ-123 --no-confirmation`,
	RunE: runLogtime,
}

func init() {
	rootCmd.AddCommand(logtimeCmd)

	logtimeCmd.Flags().StringVar(&ticketKey, "ticket", "", "ticket key (e.g., PROJ-123)")
	logtimeCmd.Flags().StringVar(&dayStr, "day", "now", "single day (format: YYYY-MM-DD, default: today)")
	logtimeCmd.Flags().StringVar(&fromStr, "from", "", "start date for range (format: YYYY-MM-DD)")
	logtimeCmd.Flags().StringVar(&toStr, "to", "", "end date for range (format: YYYY-MM-DD)")
	logtimeCmd.Flags().IntVar(&hours, "hours", 8, "hours to log (1-8)")
	logtimeCmd.Flags().BoolVar(&off, "off", false, "log to TOT-1 (out of office)")
	logtimeCmd.Flags().BoolVar(&noConfirmation, "no-confirmation", false, "skip confirmation prompt and log immediately")
	logtimeCmd.Flags().BoolVar(&resetCredsJira, "reset-credentials-jira", false, "reset Jira credentials")
	logtimeCmd.Flags().BoolVar(&resetCredsTempo, "reset-credentials-tempo", false, "reset Tempo credentials")
}

func runLogtime(cmd *cobra.Command, args []string) error {
	// Validate hours
	if hours < 1 || hours > 8 {
		return fmt.Errorf("--hours must be between 1 and 8")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	apiURL, err := ensureJiraAPIURL(cfg, config.DefaultJiraWorkspace)
	if err != nil {
		return err
	}

	// Connect to database
	err = database.Connect(cfg.GetDatabasePath(), viper.GetBool("verbose"))
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = database.Close() }()

	// Initialize services
	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	repo := repository.NewCredentialsRepository(database.GetDB())
	credsService := services.NewCredentialsService(repo, encryptionService)

	// Check/update credentials if needed
	if resetCredsJira {
		_, resetErr := getOrPromptCredentials(credsService, CredentialOptions{
			Service:        models.ServiceJira,
			UsernamePrompt: "Please set your Jira username (email).",
			PasswordPrompt: "Please set your Jira api token: (https://id.atlassian.com/manage-profile/security/api-tokens).",
			ForcePrompt:    true,
		})
		if resetErr != nil {
			return fmt.Errorf("failed to reset Jira credentials: %w", resetErr)
		}
	}

	if resetCredsTempo {
		_, resetErr := getOrPromptCredentials(credsService, CredentialOptions{
			Service:        models.ServiceJiraTempo,
			UsernamePrompt: tempoAPIKeyPrompt(apiURL),
			SkipPassword:   true,
			ForcePrompt:    true,
		})
		if resetErr != nil {
			return fmt.Errorf("failed to reset Tempo credentials: %w", resetErr)
		}
	}

	// Get Jira credentials
	jiraCreds, err := getOrPromptCredentials(credsService, CredentialOptions{
		Service:        models.ServiceJira,
		UsernamePrompt: "Please set your Jira username (email).",
		PasswordPrompt: "Please set your Jira api token: (https://id.atlassian.com/manage-profile/security/api-tokens).",
	})
	if err != nil {
		return fmt.Errorf("failed to get Jira credentials: %w", err)
	}

	// Get Tempo credentials
	tempoCreds, err := getOrPromptCredentials(credsService, CredentialOptions{
		Service:        models.ServiceJiraTempo,
		UsernamePrompt: tempoAPIKeyPrompt(apiURL),
		SkipPassword:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to get Tempo credentials: %w", err)
	}

	// Create Jira/Tempo service
	jiraTempoService, err := services.NewJiraTempoService(apiURL, cfg.TempoAPIURL, jiraCreds, tempoCreds)
	if err != nil {
		return fmt.Errorf("failed to initialize Jira/Tempo service: %w", err)
	}

	// Parse date range
	dates, err := parseDateRange(dayStr, fromStr, toStr)
	if err != nil {
		return err
	}

	if len(dates) > 15 {
		return fmt.Errorf("you can only log time for 15 days at a time")
	}

	// Get ticket ID
	var selectedTicketKey string
	if off {
		selectedTicketKey = "TOT-1"
	} else if ticketKey != "" {
		selectedTicketKey = ticketKey
	} else {
		// Interactive ticket selection
		var ticket *services.Ticket
		ticket, err = selectTicketInteractive(jiraTempoService)
		if err != nil {
			return err
		}
		selectedTicketKey = ticket.Key
	}

	// Verify ticket exists
	ticket, err := jiraTempoService.GetTicketByKey(selectedTicketKey)
	if err != nil {
		return fmt.Errorf("ticket not found: %s", selectedTicketKey)
	}

	fmt.Printf("📋 Ticket: %s\n", ticket.Key)
	if summary, ok := ticket.Fields["summary"].(string); ok {
		fmt.Printf("   %s\n\n", summary)
	}

	// Get holidays and existing worklogs
	fmt.Println("📅 Fetching holidays and existing worklogs...")
	daysOff, err := jiraTempoService.GetDaysOffForCurrentYear()
	if err != nil {
		return fmt.Errorf("failed to get holidays: %w", err)
	}

	timeSpent, err := jiraTempoService.GetTimeAlreadySpentByDay(dates[0], dates[len(dates)-1])
	if err != nil {
		return fmt.Errorf("failed to get existing worklogs: %w", err)
	}

	// Process each date
	fmt.Println("\n📊 Time logging analysis:")
	fmt.Println("────────────────────────────────────────────────")

	totalDays := 0
	skippedDays := 0
	for _, date := range dates {
		dateStr := date.Format("2006-01-02")
		displayDate := date.Format("02/01/2006")

		// Check if day off
		if services.IsDayOff(date, daysOff) {
			fmt.Printf("⏭️  %s - Day off, skipping\n", displayDate)
			skippedDays++
			continue
		}

		// Check if weekend
		if services.IsWeekend(date) {
			fmt.Printf("⏭️  %s - Weekend, skipping\n", displayDate)
			skippedDays++
			continue
		}

		// Calculate time to log
		toLog, shouldLog := calculateTimeToLog(hours, timeSpent[dateStr])
		alreadySpent := timeSpent[dateStr]

		if !shouldLog {
			if alreadySpent%3600 == 0 {
				fmt.Printf("✓  %s - Already logged (%dh), skipping\n", displayDate, alreadySpent/3600)
			} else {
				fmt.Printf("✓  %s - Already logged (%.1fh), skipping\n", displayDate, float64(alreadySpent)/3600.0)
			}
			skippedDays++
			continue
		}

		totalDays++
		hoursToLog := float64(toLog) / 3600.0
		fmt.Printf("📝 %s - Would log %.1fh", displayDate, hoursToLog)
		if alreadySpent > 0 {
			if alreadySpent%3600 == 0 {
				fmt.Printf(" (%dh already logged)", alreadySpent/3600)
			} else {
				fmt.Printf(" (%.1fh already logged)", float64(alreadySpent)/3600.0)
			}
		}
		fmt.Println()
	}

	fmt.Println("────────────────────────────────────────────────")
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   Total days in range: %d\n", len(dates))
	fmt.Printf("   Days to log: %d\n", totalDays)
	fmt.Printf("   Days skipped: %d\n", skippedDays)

	if totalDays == 0 {
		fmt.Println("\n✅ No time to log!")
		return nil
	}

	// Confirmation prompt (unless --no-confirmation is set)
	if !noConfirmation {
		fmt.Print("\n⚠️  Proceed with time logging? (y/N): ")
		var confirm string
		_, _ = fmt.Scanln(&confirm)

		if confirm != "y" && confirm != "Y" {
			fmt.Println("❌ Cancelled")
			return nil
		}
	}

	// Get ticket ID as integer
	ticketID := 0

	// Try to parse ID from string field
	if ticket.ID != "" {
		if _, operationErr := fmt.Sscanf(ticket.ID, "%d", &ticketID); operationErr != nil {
			return fmt.Errorf("invalid ticket ID format: %s", ticket.ID)
		}
	}

	if ticketID == 0 {
		return fmt.Errorf("invalid ticket ID: got empty or zero value")
	}

	// Process each date and log time
	fmt.Println("\n📝 Logging time...")
	successCount := 0
	failureCount := 0

	for _, date := range dates {
		dateStr := date.Format("2006-01-02")
		displayDate := date.Format("02/01/2006")

		// Skip if day off, weekend, or already logged
		if services.IsDayOff(date, daysOff) || services.IsWeekend(date) {
			continue
		}

		// Calculate time to log
		toLog, shouldLog := calculateTimeToLog(hours, timeSpent[dateStr])
		if !shouldLog {
			continue
		}

		// Log the time
		_, err := jiraTempoService.LogWorkTime(ticketID, date, toLog)
		if err != nil {
			fmt.Printf("❌ %s - Failed: %v\n", displayDate, err)
			failureCount++
		} else {
			hoursLogged := float64(toLog) / 3600.0
			fmt.Printf("✅ %s - Logged %.1fh\n", displayDate, hoursLogged)
			successCount++
		}
	}

	// Final summary
	fmt.Println("\n📊 Final Results:")
	fmt.Printf("   ✅ Successfully logged: %d days\n", successCount)
	if failureCount > 0 {
		fmt.Printf("   ❌ Failed: %d days\n", failureCount)
	}

	return nil
}

// parseDateRange parses date flags into a slice of dates
func parseDateRange(day, from, to string) ([]time.Time, error) {
	if from != "" && to != "" {
		// Date range
		fromDate, err := parseDate(from)
		if err != nil {
			return nil, fmt.Errorf("invalid --from date: %w", err)
		}

		toDate, err := parseDate(to)
		if err != nil {
			return nil, fmt.Errorf("invalid --to date: %w", err)
		}

		var dates []time.Time
		for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
			dates = append(dates, d)
		}

		return dates, nil
	}

	// Single day
	singleDate, err := parseDate(day)
	if err != nil {
		return nil, fmt.Errorf("invalid --day date: %w", err)
	}

	return []time.Time{singleDate}, nil
}

// parseDate parses a date string or "now"
func parseDate(dateStr string) (time.Time, error) {
	if dateStr == "now" || dateStr == "" {
		return time.Now(), nil
	}

	return time.Parse("2006-01-02", dateStr)
}

// selectTicketInteractive presents an interactive ticket selector
func selectTicketInteractive(service *services.JiraTempoService) (*services.Ticket, error) {
	fmt.Println("📋 Fetching assigned tickets...")

	tickets, err := service.GetAssignedTickets()
	if err != nil {
		return nil, fmt.Errorf("failed to get assigned tickets: %w", err)
	}

	if len(tickets) == 0 {
		return nil, fmt.Errorf("no assigned tickets found")
	}

	// Display tickets
	fmt.Printf("\n📄 Assigned Tickets (%d):\n", len(tickets))
	fmt.Println("──────────────────────────────────────────────────────────")
	for i, ticket := range tickets {
		summary := "No summary"
		if s, ok := ticket.Fields["summary"].(string); ok {
			summary = s
		}
		fmt.Printf("%2d. %-15s %s\n", i+1, ticket.Key, summary)
	}
	fmt.Println("──────────────────────────────────────────────────────────")

	// Ask user to select
	fmt.Print("\nEnter ticket number (1-", len(tickets), ") or 'q' to quit: ")
	var choice string
	_, _ = fmt.Scanln(&choice)

	if choice == "q" || choice == "" {
		return nil, fmt.Errorf("cancelled")
	}

	// Parse choice
	var index int
	if _, operationErr := fmt.Sscanf(choice, "%d", &index); operationErr != nil || index < 1 || index > len(tickets) {
		return nil, fmt.Errorf("invalid choice: %s", choice)
	}

	// Return as service.Ticket type
	selected := tickets[index-1]
	return &services.Ticket{
		ID:     selected.ID,
		Key:    selected.Key,
		Fields: selected.Fields,
	}, nil
}

// calculateTimeToLog computes the number of seconds to log given the requested hours for a ticket
// and the number of seconds already logged today.
// It caps the total time logged for the day at 8 hours (28800 seconds).
// Returns toLog (in seconds) and shouldLog (boolean indicating if any time should be logged).
func calculateTimeToLog(requestedHours int, alreadySpent int) (int, bool) {
	const maxDailySeconds = 8 * 3600
	remainingSeconds := maxDailySeconds - alreadySpent
	if remainingSeconds <= 0 {
		return 0, false
	}

	requestedSeconds := requestedHours * 3600
	toLog := requestedSeconds
	if toLog > remainingSeconds {
		toLog = remainingSeconds
	}

	return toLog, true
}
