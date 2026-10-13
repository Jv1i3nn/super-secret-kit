package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ssk/internal/config"
	"ssk/internal/services"
	"ssk/internal/services/horoquartz"
)

var (
	ttDate           string
	ttStart          string
	ttEnd            string
	ttMorning        bool
	ttAfternoon      bool
	ttStartMorning   bool
	ttStartAfternoon bool
	ttEndMorning     bool
	ttEndAfternoon   bool
	ttResetCreds     bool
)

var teleworkCmd = &cobra.Command{
	Use:     "telework",
	Aliases: []string{"tt", "add-telework"},
	Short:   "Declare a telework day, half-day, or period in Horoquartz",
	Long: `Declare telework in Horoquartz.

Examples:
  # Declare full day telework
  ssk temporh telework --date 2026-08-03

  # Declare morning telework
  ssk temporh telework --date 2026-08-03 --morning

  # Declare afternoon telework
  ssk temporh telework --date 2026-08-03 --afternoon

  # Declare a telework period (full days)
  ssk temporh telework --start 2026-08-03 --end 2026-08-07

  # Declare a period starting Monday afternoon and ending Wednesday morning
  ssk temporh telework --start 2026-08-03 --start-afternoon --end 2026-08-05 --end-morning`,
	RunE: runTelework,
}

func init() {
	temporhCmd.AddCommand(teleworkCmd)

	teleworkCmd.Flags().StringVarP(&ttDate, "date", "d", "", "single date for telework (YYYY-MM-DD)")
	teleworkCmd.Flags().StringVarP(&ttStart, "start", "s", "", "start date for telework period (YYYY-MM-DD)")
	teleworkCmd.Flags().StringVarP(&ttEnd, "end", "e", "", "end date for telework period (YYYY-MM-DD)")

	teleworkCmd.Flags().BoolVarP(&ttMorning, "morning", "m", false, "telework in the morning only")
	teleworkCmd.Flags().BoolVarP(&ttAfternoon, "afternoon", "a", false, "telework in the afternoon only")

	teleworkCmd.Flags().BoolVar(&ttStartMorning, "start-morning", false, "start period on morning")
	teleworkCmd.Flags().BoolVar(&ttStartAfternoon, "start-afternoon", false, "start period on afternoon")

	teleworkCmd.Flags().BoolVar(&ttEndMorning, "end-morning", false, "end period on morning")
	teleworkCmd.Flags().BoolVar(&ttEndAfternoon, "end-afternoon", false, "end period on afternoon")

	teleworkCmd.Flags().BoolVarP(&ttResetCreds, "reset-credentials", "r", false, "reset authentication credentials")
}

func runTelework(cmd *cobra.Command, args []string) error {
	var startDate, endDate time.Time
	var startUnit, endUnit string
	var err error

	hasFlags := ttDate != "" || ttStart != "" || ttEnd != "" || ttMorning || ttAfternoon || ttStartMorning || ttStartAfternoon || ttEndMorning || ttEndAfternoon

	if !hasFlags {
		// Mode interactif si aucun paramètre/drapeau n'est précisé
		startDate, endDate, startUnit, endUnit, err = runInteractiveTelework()
		if err != nil {
			return err
		}
	} else {
		// Mode avec drapeau CLI
		if ttDate != "" {
			startDate, err = time.Parse("2006-01-02", ttDate)
			if err != nil {
				return fmt.Errorf("format de date invalide (utiliser AAAA-MM-JJ): %w", err)
			}
			endDate = startDate
		} else if ttStart != "" {
			startDate, err = time.Parse("2006-01-02", ttStart)
			if err != nil {
				return fmt.Errorf("format de date de début invalide (utiliser AAAA-MM-JJ): %w", err)
			}

			if ttEnd != "" {
				endDate, err = time.Parse("2006-01-02", ttEnd)
				if err != nil {
					return fmt.Errorf("format de date de fin invalide (utiliser AAAA-MM-JJ): %w", err)
				}
			} else {
				endDate = startDate
			}
		} else {
			now := time.Now()
			startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			endDate = startDate
		}

		if endDate.Before(startDate) {
			return fmt.Errorf("la date de fin (%s) ne peut pas être antérieure à la date de début (%s)", endDate.Format("2006-01-02"), startDate.Format("2006-01-02"))
		}

		startUnit = "D"
		endUnit = "D"

		if ttMorning {
			startUnit = "M"
			endUnit = "M"
		} else if ttAfternoon {
			startUnit = "A"
			endUnit = "A"
		}

		if ttStartMorning {
			startUnit = "M"
		} else if ttStartAfternoon {
			startUnit = "A"
		}

		if ttEndMorning {
			endUnit = "M"
		} else if ttEndAfternoon {
			endUnit = "A"
		}
	}

	// 3. Initialiser la configuration et le service
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	apiURL, err := ensureHoroquartzURL(cfg)
	if err != nil {
		return err
	}

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	sessionPath := cfg.ExternalProjectPath + "/temporh"

	horoquartzService := horoquartz.NewService(
		apiURL,
		encryptionService,
		sessionPath,
	)

	if operationErr := horoquartzService.EnsureAuthenticated(ttResetCreds); operationErr != nil {
		return fmt.Errorf("authentication error: %w", operationErr)
	}

	// 4. Afficher le récapitulatif
	var detailStr string
	if startDate.Equal(endDate) {
		unitStr := "journée complète"
		switch startUnit {
		case "M":
			unitStr = "Matin"
		case "A":
			unitStr = "Après-midi"
		}
		detailStr = fmt.Sprintf("%s (%s)", startDate.Format("02/01/2006"), unitStr)
	} else {
		startUnitStr := "Matin"
		switch startUnit {
		case "A":
			startUnitStr = "Après-midi"
		case "D":
			startUnitStr = "Journée complète"
		}

		endUnitStr := "Fin de journée"
		switch endUnit {
		case "M":
			endUnitStr = "Midi"
		case "A":
			endUnitStr = "Après-midi"
		}

		detailStr = fmt.Sprintf("du %s (%s) au %s (%s)", startDate.Format("02/01/2006"), startUnitStr, endDate.Format("02/01/2006"), endUnitStr)
	}

	fmt.Printf("🏠 Déclaration de télétravail %s...\n", detailStr)

	// 5. Soumettre la demande
	resp, err := horoquartzService.AddTelework(startDate, endDate, startUnit, endUnit)
	if err != nil {
		return fmt.Errorf("échec de la création de la demande de télétravail: %w", err)
	}

	if resp.Status == "SUCCESS" || resp.Status == "INFO" {
		fmt.Println("✅ Demande de télétravail enregistrée avec succès dans Horoquartz !")
		for _, msg := range resp.Messages {
			if msg.Reason != "" {
				fmt.Printf("   ℹ️  %s\n", msg.Reason)
			}
		}
		return nil
	}

	// Si échec ou avertissement
	fmt.Printf("⚠️  Statut de la demande: %s\n", resp.Status)
	for _, msg := range resp.Messages {
		fmt.Printf("   ❌ [%s] %s\n", msg.Level, msg.Reason)
	}

	return fmt.Errorf("demande rejetée par Horoquartz")
}

func runInteractiveTelework() (startDate, endDate time.Time, startUnit, endUnit string, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("🏠 Mode interactif de déclaration de télétravail")
	fmt.Println("─────────────────────────────────────────────────")
	fmt.Println("1) Une seule journée / demi-journée")
	fmt.Println("2) Une période (plusieurs jours)")
	fmt.Print("👉 Choix [1-2] (défaut: 1) : ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	todayStr := time.Now().Format("2006-01-02")

	if choiceStr == "2" {
		// Période
		fmt.Printf("📅 Date de début (AAAA-MM-JJ) [défaut: %s] : ", todayStr)
		startInput, _ := reader.ReadString('\n')
		startInput = strings.TrimSpace(startInput)
		if startInput == "" {
			startInput = todayStr
		}
		startDate, err = time.Parse("2006-01-02", startInput)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", fmt.Errorf("date de début invalide: %w", err)
		}

		fmt.Printf("📅 Date de fin (AAAA-MM-JJ) [défaut: %s] : ", startInput)
		endInput, _ := reader.ReadString('\n')
		endInput = strings.TrimSpace(endInput)
		if endInput == "" {
			endInput = startInput
		}
		endDate, err = time.Parse("2006-01-02", endInput)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", fmt.Errorf("date de fin invalide: %w", err)
		}

		startUnit = "D"
		endUnit = "D"

		fmt.Print("❓ Commencer l'après-midi du premier jour ? (o/N) : ")
		saInput, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(saInput)), "o") {
			startUnit = "A"
		}

		fmt.Print("❓ Terminer le matin du dernier jour (midi) ? (o/N) : ")
		eaInput, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(eaInput)), "o") {
			endUnit = "M"
		}
	} else {
		// Journée simple
		fmt.Printf("📅 Date (AAAA-MM-JJ) [défaut: %s] : ", todayStr)
		dateInput, _ := reader.ReadString('\n')
		dateInput = strings.TrimSpace(dateInput)
		if dateInput == "" {
			dateInput = todayStr
		}
		startDate, err = time.Parse("2006-01-02", dateInput)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", fmt.Errorf("date invalide: %w", err)
		}
		endDate = startDate

		fmt.Println("⏱️  Durée :")
		fmt.Println("   1) Journée complète")
		fmt.Println("   2) Matin uniquement")
		fmt.Println("   3) Après-midi uniquement")
		fmt.Print("👉 Choix [1-3] (défaut: 1) : ")
		durInput, _ := reader.ReadString('\n')
		durInput = strings.TrimSpace(durInput)

		switch durInput {
		case "2":
			startUnit = "M"
			endUnit = "M"
		case "3":
			startUnit = "A"
			endUnit = "A"
		default:
			startUnit = "D"
			endUnit = "D"
		}
	}

	return startDate, endDate, startUnit, endUnit, nil
}
