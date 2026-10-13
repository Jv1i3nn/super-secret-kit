package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/config"
	"ssk/internal/database"
	"ssk/internal/repository"
	"ssk/internal/services"
	"ssk/internal/services/arkevia"
)

// defaultListRows is the number of recent documents shown when listing or
// selecting interactively.
const defaultListRows = 10

var (
	arkeviaOutputDir  string
	arkeviaLatest     bool
	arkeviaYear       int
	arkeviaMonth      int
	arkeviaList       bool
	arkeviaRows       int
	arkeviaResetCreds bool
)

var arkeviaCmd = &cobra.Command{
	Use:     "arkevia",
	Aliases: []string{"payslip", "la_moula"},
	Short:   "Download documents from Arkevia",
	Long: `Download documents from Arkevia service.

Examples:
  # Download latest document
  ssk arkevia --latest
  ssk la_moula --latest

  # Reset stored credentials and 2FA OTP secret
  ssk arkevia --reset-credentials

  # Download latest to specific directory
  ssk arkevia --latest --output ~/Documents

  # Download every document for a specific month (a month may hold several)
  ssk arkevia --year 2026 --month 5

  # Download every document for a whole year
  ssk arkevia --year 2026

  # List available documents without downloading (non-interactive)
  ssk arkevia --list
  ssk arkevia --list --rows 24

  # List and download specific document (interactive)
  ssk arkevia`,
	RunE: runArkeviaDownload,
}

func init() {
	rootCmd.AddCommand(arkeviaCmd)

	arkeviaCmd.Flags().StringVarP(&arkeviaOutputDir, "output", "o", "", "output directory (default: ~/.ssk/payslip)")
	arkeviaCmd.Flags().BoolVarP(&arkeviaLatest, "latest", "l", false, "download latest document")
	arkeviaCmd.Flags().IntVarP(&arkeviaYear, "year", "y", 0, "download all documents for this year (e.g. 2026)")
	arkeviaCmd.Flags().IntVarP(&arkeviaMonth, "month", "m", 0, "download all documents for this month, 1-12 (requires --year)")
	arkeviaCmd.Flags().BoolVar(&arkeviaList, "list", false, "list available documents without downloading")
	arkeviaCmd.Flags().IntVarP(&arkeviaRows, "rows", "n", defaultListRows, "number of recent documents to list/select from")
	arkeviaCmd.Flags().BoolVarP(&arkeviaResetCreds, "reset-credentials", "r", false, "reset Arkevia & 2FA OTP credentials")
}

func runArkeviaDownload(cmd *cobra.Command, args []string) error {
	// Validate the year/month selection up-front so a mistake fails fast,
	// before any config, database or credential work.
	byPeriod := arkeviaYear != 0 || arkeviaMonth != 0
	if byPeriod {
		if arkeviaYear == 0 {
			return fmt.Errorf("--month requires --year: please also specify the year (e.g. --year 2026 --month %d)", arkeviaMonth)
		}
		if arkeviaMonth != 0 && (arkeviaMonth < 1 || arkeviaMonth > 12) {
			return fmt.Errorf("invalid month %d: must be between 1 and 12", arkeviaMonth)
		}
	}

	if arkeviaRows < 1 {
		return fmt.Errorf("invalid --rows %d: must be at least 1", arkeviaRows)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if arkeviaOutputDir == "" {
		arkeviaOutputDir = cfg.ExternalProjectPath + "/payslip"
	}

	err = database.Connect(cfg.GetDatabasePath(), viper.GetBool("verbose"))
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = database.Close() }()

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	repo := repository.NewCredentialsRepository(database.GetDB())
	credsService := services.NewCredentialsService(repo, encryptionService)

	if arkeviaResetCreds {
		tokenFile := cfg.ExternalProjectPath + "/arkevia_token.json"
		_ = os.Remove(tokenFile)
	}

	creds, err := getOrPromptArkeviaCredentials(credsService, arkeviaResetCreds)
	if err != nil {
		return fmt.Errorf("failed to get Arkevia credentials: %w", err)
	}

	arkeviaService := arkevia.NewService(creds, cfg.ExternalProjectPath)

	if byPeriod {
		return downloadByPeriod(arkeviaService, arkeviaYear, arkeviaMonth, arkeviaOutputDir)
	}

	if arkeviaList {
		return listDocuments(arkeviaService, arkeviaRows)
	}

	if arkeviaLatest {
		return downloadLatest(arkeviaService, arkeviaOutputDir)
	}

	return downloadInteractive(arkeviaService, arkeviaOutputDir, arkeviaRows)
}

func downloadByPeriod(arkeviaService *arkevia.Service, year, month int, outputDir string) error {
	if month != 0 {
		fmt.Printf("📥 Downloading documents for %04d-%02d...\n", year, month)
	} else {
		fmt.Printf("📥 Downloading documents for %04d...\n", year)
	}

	paths, err := arkeviaService.DownloadByPeriod(year, month, outputDir)
	if err != nil {
		return fmt.Errorf("failed to download documents: %w", err)
	}

	fmt.Printf("✅ %d document(s) downloaded successfully:\n", len(paths))
	for _, path := range paths {
		fmt.Printf("   %s\n", path)
	}

	return nil
}

func listDocuments(arkeviaService *arkevia.Service, rows int) error {
	fmt.Println("📋 Fetching document list...")

	documents, err := arkeviaService.GetDocumentList(rows)
	if err != nil {
		return fmt.Errorf("failed to get document list: %w", err)
	}

	if len(documents) == 0 {
		fmt.Println("No documents found.")
		return nil
	}

	printDocumentList(documents)

	return nil
}

func printDocumentList(documents []arkevia.Document) {
	fmt.Printf("\n📄 Available Documents (%d):\n", len(documents))
	fmt.Println("──────────────────────────────────────────────────────────")
	for i, doc := range documents {
		newTag := ""
		if doc.IsNew {
			newTag = " 🆕"
		}
		fmt.Printf("%2d. %s%s\n", i+1, doc.Filename, newTag)
	}
	fmt.Println("──────────────────────────────────────────────────────────")
}

func downloadLatest(arkeviaService *arkevia.Service, outputDir string) error {
	fmt.Println("📥 Downloading latest document...")

	outputPath, err := arkeviaService.DownloadLatest(outputDir)
	if err != nil {
		return fmt.Errorf("failed to download document: %w", err)
	}

	fmt.Printf("✅ Document downloaded successfully:\n")
	fmt.Printf("   %s\n", outputPath)

	return nil
}

func downloadInteractive(arkeviaService *arkevia.Service, outputDir string, rows int) error {
	fmt.Println("📋 Fetching document list...")

	documents, err := arkeviaService.GetDocumentList(rows)
	if err != nil {
		return fmt.Errorf("failed to get document list: %w", err)
	}

	if len(documents) == 0 {
		fmt.Println("No documents found.")
		return nil
	}

	printDocumentList(documents)

	fmt.Print("\nEnter document number to download (1-", len(documents), ") or 'q' to quit: ")
	var choice string
	_, _ = fmt.Scanln(&choice)

	if choice == "q" || choice == "" {
		fmt.Println("Cancelled.")
		return nil
	}

	var index int
	_, err = fmt.Sscanf(choice, "%d", &index)
	if err != nil || index < 1 || index > len(documents) {
		return fmt.Errorf("invalid choice: %s", choice)
	}

	selectedDoc := documents[index-1]
	fmt.Printf("\n📥 Downloading '%s'...\n", selectedDoc.Filename)

	outputPath, err := arkeviaService.DownloadDocument(selectedDoc, outputDir)
	if err != nil {
		return fmt.Errorf("failed to download document: %w", err)
	}

	fmt.Printf("✅ Document downloaded successfully:\n")
	fmt.Printf("   %s\n", outputPath)

	if openFile(outputPath) {
		fmt.Println("✅ File opened")
	}

	return nil
}

func openFile(path string) bool {
	fmt.Print("\nOpen file? (y/N): ")
	var answer string
	_, _ = fmt.Scanln(&answer)

	if answer != "y" && answer != "Y" {
		return false
	}

	var cmdName string
	switch {
	case fileExists("/usr/bin/xdg-open"):
		cmdName = "xdg-open"
	case fileExists("/usr/bin/open"):
		cmdName = "open"
	default:
		fmt.Println("Cannot automatically open file. Please open manually.")
		return false
	}

	cmd := exec.Command(cmdName, path)
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to open file: %v\n", err)
		return false
	}

	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
