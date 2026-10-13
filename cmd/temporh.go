package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/config"
	"ssk/internal/services"
	"ssk/internal/services/horoquartz"
)

var (
	showCounters     bool
	resetCredentials bool
	countersDate     string
	countersFormat   string
)

var temporhCmd = &cobra.Command{
	Use:   "temporh",
	Short: "Manage Horoquartz time counters",
	Long: `Retrieve and display Horoquartz time counters.

Examples:
  # Display all counters at year end
  ssk temporh --show-counters

  # Reset authentication and re-authenticate
  ssk temporh --show-counters --reset-credentials

  # Display counters at specific date
  ssk temporh --show-counters --date 2025-06-30

  # Export counters to JSON
  ssk temporh --show-counters --format json

  # Export counters to CSV
  ssk temporh --show-counters --format csv > counters.csv`,
	RunE: runTemporh,
}

func init() {
	rootCmd.AddCommand(temporhCmd)

	temporhCmd.Flags().BoolVarP(&showCounters, "show-counters", "s", false, "display all counters")
	temporhCmd.Flags().BoolVarP(&resetCredentials, "reset-credentials", "r", false, "reset authentication")
	temporhCmd.Flags().StringVarP(&countersDate, "date", "d", "", "reference date (YYYY-MM-DD, default: 31/12 of current year)")
	temporhCmd.Flags().StringVarP(&countersFormat, "format", "f", "table", "output format (table, json, csv)")
}

func runTemporh(cmd *cobra.Command, args []string) error {
	// Par défaut, afficher les compteurs
	showCounters = true

	// Charger la configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialiser le service de chiffrement
	apiURL, err := ensureHoroquartzURL(cfg)
	if err != nil {
		return err
	}

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	// Chemin de stockage de la session
	sessionPath := cfg.ExternalProjectPath + "/temporh"

	// Créer le service Horoquartz
	horoquartzService := horoquartz.NewService(
		apiURL,
		encryptionService,
		sessionPath,
	)

	// S'assurer de l'authentification
	if operationErr := horoquartzService.EnsureAuthenticated(resetCredentials); operationErr != nil {
		return fmt.Errorf("authentication error: %w", operationErr)
	}

	// Déterminer la date de référence
	var refDate time.Time
	if countersDate != "" {
		refDate, err = time.Parse("2006-01-02", countersDate)
		if err != nil {
			return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
		}
	} else {
		// Par défaut : 31/12 de l'année en cours
		currentYear := time.Now().Year()
		refDate = time.Date(currentYear, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	// Récupérer les compteurs
	if viper.GetBool("verbose") {
		fmt.Printf("📊 Récupération des compteurs au %s...\n", refDate.Format("02/01/2006"))
	}

	counters, err := horoquartzService.GetCounters(refDate)
	if err != nil {
		return fmt.Errorf("failed to get counters: %w", err)
	}

	if len(counters) == 0 {
		fmt.Println("⚠️  Aucun compteur trouvé pour cette date")
		return nil
	}

	// Afficher selon le format demandé
	switch countersFormat {
	case "json":
		return displayCountersJSON(counters)
	case "csv":
		return displayCountersCSV(counters)
	case "table":
		fallthrough
	default:
		return displayCountersTable(counters, refDate)
	}
}

func displayCountersTable(counters []horoquartz.Counter, date time.Time) error {
	// Afficher l'en-tête
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║            Compteurs Horoquartz - %-31s ║\n", date.Format("02/01/2006"))
	fmt.Println("╠═══════════════════════════════════════════════════════════════════╣")

	// Grouper par catégorie
	grouped := horoquartz.GroupCountersByCategory(counters)

	categories := []horoquartz.CounterCategory{
		horoquartz.CategoryLeave,
		horoquartz.CategoryWork,
		horoquartz.CategoryTelework,
		horoquartz.CategoryOther,
	}

	for _, category := range categories {
		categoryCounters, ok := grouped[category]
		if !ok || len(categoryCounters) == 0 {
			continue
		}

		title := horoquartz.GetCategoryTitle(category)
		titleRuneLen := utf8.RuneCountInString(title)
		padTitle := 63 - titleRuneLen
		if padTitle < 0 {
			padTitle = 0
		}
		fmt.Println("║                                                                   ║")
		fmt.Printf("║  %s%*s ║\n", title, padTitle, "")
		fmt.Println("║  ───────────────────────────────────────────────────────────────  ║")

		for _, counter := range categoryCounters {
			name := horoquartz.FormatCounterName(counter.Code, counter.Label)
			value := horoquartz.FormatCounterValue(counter.Value, counter.Unit)

			nameRuneLen := utf8.RuneCountInString(name)
			valRuneLen := utf8.RuneCountInString(value)

			dotsCount := 55 - nameRuneLen - valRuneLen
			if dotsCount < 1 {
				dotsCount = 1
			}

			line := fmt.Sprintf("  %s %s %s", name, strings.Repeat(".", dotsCount), value)
			lineRuneLen := utf8.RuneCountInString(line)
			padLine := 65 - lineRuneLen
			if padLine < 0 {
				padLine = 0
			}

			fmt.Printf("║%s%*s║\n", line, padLine, "")
		}
	}

	fmt.Println("║                                                                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("✅ Compteurs récupérés avec succès")
	return nil
}

func displayCountersJSON(counters []horoquartz.Counter) error {
	// Convertir en CounterDisplay pour un format plus lisible
	displays := make([]map[string]interface{}, len(counters))
	for i, counter := range counters {
		displays[i] = map[string]interface{}{
			"code":  counter.Code,
			"name":  horoquartz.FormatCounterName(counter.Code, counter.Label),
			"value": counter.Value,
			"unit":  horoquartz.FormatUnit(counter.Unit),
			"type":  counter.Type,
			"date":  counter.Date,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(displays)
}

func displayCountersCSV(counters []horoquartz.Counter) error {
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// En-tête
	if operationErr := writer.Write([]string{"Code", "Nom", "Valeur", "Unité", "Type", "Date"}); operationErr != nil {
		return operationErr
	}

	// Données
	for _, counter := range counters {
		name := horoquartz.FormatCounterName(counter.Code, counter.Label)
		unit := horoquartz.FormatUnit(counter.Unit)

		if operationErr := writer.Write([]string{
			counter.Code,
			name,
			counter.Value,
			unit,
			counter.Type,
			counter.Date,
		}); operationErr != nil {
			return operationErr
		}
	}
	writer.Flush()
	return writer.Error()
}
