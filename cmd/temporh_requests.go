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

// Structural variables for CP, RTT, and Recup flags
var (
	reqDate           string
	reqStart          string
	reqEnd            string
	reqMorning        bool
	reqAfternoon      bool
	reqStartMorning   bool
	reqStartAfternoon bool
	reqEndMorning     bool
	reqEndAfternoon   bool
	reqResetCreds     bool
)

var cpCmd = &cobra.Command{
	Use:     "cp",
	Aliases: []string{"conge", "conges", "add-cp"},
	Short:   "Declare paid leave (CP) in Horoquartz",
	Long: `Declare paid leave (CP) in Horoquartz.

Examples:
  # Declare full day CP
  ssk temporh cp --date 2026-08-03

  # Declare morning CP
  ssk temporh cp --date 2026-08-03 --morning

  # Declare CP period
  ssk temporh cp --start 2026-08-03 --end 2026-08-14`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenericLeaveRequest("cp", "Congé Payé (CP)", "🏖️")
	},
}

var rttCmd = &cobra.Command{
	Use:     "rtt",
	Aliases: []string{"add-rtt"},
	Short:   "Declare RTT in Horoquartz",
	Long: `Declare RTT in Horoquartz.

Examples:
  # Declare full day RTT
  ssk temporh rtt --date 2026-08-03

  # Declare afternoon RTT
  ssk temporh rtt --date 2026-08-03 --afternoon

  # Declare RTT period
  ssk temporh rtt --start 2026-08-03 --end 2026-08-05`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenericLeaveRequest("rtt", "RTT", "🌴")
	},
}

var recupCmd = &cobra.Command{
	Use:     "recup",
	Aliases: []string{"recovery", "add-recup"},
	Short:   "Declare recovery day (Récupération) in Horoquartz",
	Long: `Declare recovery day (Récupération) in Horoquartz.

Examples:
  # Declare full day recovery
  ssk temporh recup --date 2026-08-03

  # Declare morning recovery
  ssk temporh recup --date 2026-08-03 --morning`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenericLeaveRequest("recup", "Récupération", "⏰")
	},
}

func init() {
	cmds := []*cobra.Command{cpCmd, rttCmd, recupCmd}

	for _, c := range cmds {
		temporhCmd.AddCommand(c)

		c.Flags().StringVarP(&reqDate, "date", "d", "", "single date (YYYY-MM-DD)")
		c.Flags().StringVarP(&reqStart, "start", "s", "", "start date for period (YYYY-MM-DD)")
		c.Flags().StringVarP(&reqEnd, "end", "e", "", "end date for period (YYYY-MM-DD)")

		c.Flags().BoolVarP(&reqMorning, "morning", "m", false, "morning only")
		c.Flags().BoolVarP(&reqAfternoon, "afternoon", "a", false, "afternoon only")

		c.Flags().BoolVar(&reqStartMorning, "start-morning", false, "start period on morning")
		c.Flags().BoolVar(&reqStartAfternoon, "start-afternoon", false, "start period on afternoon")

		c.Flags().BoolVar(&reqEndMorning, "end-morning", false, "end period on morning")
		c.Flags().BoolVar(&reqEndAfternoon, "end-afternoon", false, "end period on afternoon")

		c.Flags().BoolVarP(&reqResetCreds, "reset-credentials", "r", false, "reset authentication credentials")
	}
}

func runGenericLeaveRequest(reqType, titleName, iconStr string) error {
	var startDate, endDate time.Time
	var startUnit, endUnit string
	var err error

	hasFlags := reqDate != "" || reqStart != "" || reqEnd != "" || reqMorning || reqAfternoon || reqStartMorning || reqStartAfternoon || reqEndMorning || reqEndAfternoon

	if !hasFlags {
		startDate, endDate, startUnit, endUnit, err = runInteractiveLeaveRequest(titleName, iconStr)
		if err != nil {
			return err
		}
	} else {
		if reqDate != "" {
			startDate, err = time.Parse("2006-01-02", reqDate)
			if err != nil {
				return fmt.Errorf("format de date invalide (utiliser AAAA-MM-JJ): %w", err)
			}
			endDate = startDate
		} else if reqStart != "" {
			startDate, err = time.Parse("2006-01-02", reqStart)
			if err != nil {
				return fmt.Errorf("format de date de début invalide (utiliser AAAA-MM-JJ): %w", err)
			}

			if reqEnd != "" {
				endDate, err = time.Parse("2006-01-02", reqEnd)
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

		if reqMorning {
			startUnit = "M"
			endUnit = "M"
		} else if reqAfternoon {
			startUnit = "A"
			endUnit = "A"
		}

		if reqStartMorning {
			startUnit = "M"
		} else if reqStartAfternoon {
			startUnit = "A"
		}

		if reqEndMorning {
			endUnit = "M"
		} else if reqEndAfternoon {
			endUnit = "A"
		}
	}

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

	if operationErr := horoquartzService.EnsureAuthenticated(reqResetCreds); operationErr != nil {
		return fmt.Errorf("authentication error: %w", operationErr)
	}

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

	fmt.Printf("%s Déclaration de %s %s...\n", iconStr, titleName, detailStr)

	resp, err := horoquartzService.AddRequest(reqType, startDate, endDate, startUnit, endUnit)
	if err != nil {
		return fmt.Errorf("échec de la création de la demande de %s: %w", titleName, err)
	}

	if resp.Status == "SUCCESS" || resp.Status == "INFO" {
		fmt.Printf("✅ Demande de %s enregistrée avec succès dans Horoquartz !\n", titleName)
		for _, msg := range resp.Messages {
			if msg.Reason != "" {
				fmt.Printf("   ℹ️  %s\n", msg.Reason)
			}
		}
		return nil
	}

	fmt.Printf("⚠️  Statut de la demande: %s\n", resp.Status)
	for _, msg := range resp.Messages {
		fmt.Printf("   ❌ [%s] %s\n", msg.Level, msg.Reason)
	}

	return fmt.Errorf("demande de %s rejetée par Horoquartz", titleName)
}

func runInteractiveLeaveRequest(titleName, iconStr string) (startDate, endDate time.Time, startUnit, endUnit string, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("%s Mode interactif de déclaration de %s\n", iconStr, titleName)
	fmt.Println("─────────────────────────────────────────────────")
	fmt.Println("1) Une seule journée / demi-journée")
	fmt.Println("2) Une période (plusieurs jours)")
	fmt.Print("👉 Choix [1-2] (défaut: 1) : ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	todayStr := time.Now().Format("2006-01-02")

	if choiceStr == "2" {
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
