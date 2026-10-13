package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	freedomFormat   string
	freedomDays     bool
	freedomWorkdays bool
	freedomHours    bool
	freedomMinutes  bool
	freedomSeconds  bool
	freedomRaw      bool
)

// FreedomTarget returns the target date of freedom: June 30, 2027 at 17:00:00.
func FreedomTarget() time.Time {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		loc = time.Local
	}
	return time.Date(2027, time.June, 30, 17, 0, 0, 0, loc)
}

// freedomCmd represents the freedom command
var freedomCmd = &cobra.Command{
	Use:     "freedom",
	Aliases: []string{"liberte", "liberté", "liberty", "countdown"},
	Short:   "Display remaining time until freedom (30 June 2027 at 17:00)",
	Long: `Display the remaining time until the date of freedom: June 30, 2027 at 17:00.

By default, displays a complete overview with breakdown in calendar time (24h/day)
and business work time (8h/day for jours ouvrés).
The --workdays (-w) option can be combined with any unit flag (hours, minutes, seconds).

Examples:
  # Full default overview
  ssk freedom

  # Business days overview
  ssk freedom -w

  # Combine --workdays (-w) with hours, minutes, or seconds
  ssk freedom -w -H       # Work hours (base 8h/day)
  ssk freedom -w -m       # Work minutes
  ssk freedom -w -s       # Work seconds

  # Output in specific calendar units
  ssk freedom --format jour
  ssk freedom --format heure
  ssk freedom -f minutes
  ssk freedom -f secondes

  # Output raw numerical value for scripts
  ssk freedom -w -H --raw`,
	RunE: runFreedom,
}

func init() {
	rootCmd.AddCommand(freedomCmd)

	freedomCmd.Flags().StringVarP(&freedomFormat, "format", "f", "", "Type de sortie (jour, ouvrés, heure, minutes, secondes, full)")
	freedomCmd.Flags().BoolVarP(&freedomDays, "days", "d", false, "Shortcut pour --format jour")
	freedomCmd.Flags().BoolVarP(&freedomWorkdays, "workdays", "w", false, "Filtrer uniquement sur le temps de travail ouvré (base 8h/j)")
	freedomCmd.Flags().BoolVarP(&freedomHours, "hours", "H", false, "Shortcut pour --format heure")
	freedomCmd.Flags().BoolVarP(&freedomMinutes, "minutes", "m", false, "Shortcut pour --format minutes")
	freedomCmd.Flags().BoolVarP(&freedomSeconds, "seconds", "s", false, "Shortcut pour --format secondes")
	freedomCmd.Flags().BoolVarP(&freedomRaw, "raw", "r", false, "Afficher uniquement la valeur numérique brute")
}

func isWorkdaysFormat(f string) bool {
	f = strings.ToLower(strings.TrimSpace(f))
	switch f {
	case "w", "b", "workday", "workdays", "business", "ouvre", "ouvres", "ouvrés", "jour_ouvre", "jours_ouvres":
		return true
	default:
		return false
	}
}

func getSelectedFreedomFormat() string {
	if freedomDays {
		return "jour"
	}
	if freedomHours {
		return "heure"
	}
	if freedomMinutes {
		return "minutes"
	}
	if freedomSeconds {
		return "secondes"
	}

	if freedomFormat != "" {
		f := strings.ToLower(strings.TrimSpace(freedomFormat))
		switch f {
		case "j", "jour", "jours", "d", "day", "days":
			return "jour"
		case "h", "hr", "hrs", "heure", "heures", "hour", "hours":
			return "heure"
		case "m", "min", "mins", "minute", "minutes":
			return "minutes"
		case "s", "sec", "secs", "seconde", "secondes", "second", "seconds":
			return "secondes"
		case "w", "b", "workday", "workdays", "business", "ouvre", "ouvres", "ouvrés", "jour_ouvre", "jours_ouvres":
			return "workdays"
		case "full", "all", "tout", "detail":
			return "full"
		}
	}

	if freedomWorkdays {
		return "workdays"
	}

	return "full"
}

func runFreedom(cmd *cobra.Command, args []string) error {
	now := time.Now()
	target := FreedomTarget()
	format := getSelectedFreedomFormat()
	workdaysOnly := freedomWorkdays || isWorkdaysFormat(freedomFormat)
	output := FormatFreedom(now, target, format, workdaysOnly, freedomRaw)
	_, err := fmt.Fprint(cmd.OutOrStdout(), output)
	return err
}

// CountBusinessDays counts the number of Monday-to-Friday 24h periods in the remaining full days.
func CountBusinessDays(start, end time.Time) int64 {
	if !start.Before(end) {
		return 0
	}
	diff := end.Sub(start)
	fullDays := int64(diff.Hours() / 24)

	var workDays int64
	curr := start
	for i := int64(0); i < fullDays; i++ {
		if curr.Weekday() != time.Saturday && curr.Weekday() != time.Sunday {
			workDays++
		}
		curr = curr.AddDate(0, 0, 1)
	}
	return workDays
}

// FormatFreedom computes and formats the time remaining until freedom target.
func FormatFreedom(now, target time.Time, format string, workdaysOnly bool, raw bool) string {
	if !now.Before(target) {
		if raw {
			return "0\n"
		}
		return "🎉 La liberté est déjà là ! (30 juin 2027 à 17:00 est dépassé)\n"
	}

	diff := target.Sub(now)
	totalSeconds := int64(diff.Seconds())
	totalMinutes := int64(diff.Minutes())
	totalHours := int64(diff.Hours())
	totalDays := int64(diff.Hours() / 24)

	workDays := CountBusinessDays(now, target)

	remHours := totalHours % 24
	remMinutes := totalMinutes % 60
	remSeconds := totalSeconds % 60

	// Business work time calculations (8h/day standard)
	workHours := workDays*8 + remHours
	workMinutes := workHours*60 + remMinutes
	workSeconds := workMinutes*60 + remSeconds

	switch format {
	case "workdays":
		if raw {
			return fmt.Sprintf("%d\n", workDays)
		}
		return fmt.Sprintf("⏳ %s jours ouvrés, %d heures, %d minutes, %d secondes avant la liberté !\n",
			formatNumber(workDays), remHours, remMinutes, remSeconds)

	case "heure":
		if workdaysOnly {
			if raw {
				return fmt.Sprintf("%d\n", workHours)
			}
			return fmt.Sprintf("⏳ %s heures de travail (base 8h/j) avant la liberté !\n", formatNumber(workHours))
		}
		if raw {
			return fmt.Sprintf("%d\n", totalHours)
		}
		return fmt.Sprintf("⏳ %s heures calendaires avant la liberté !\n", formatNumber(totalHours))

	case "minutes":
		if workdaysOnly {
			if raw {
				return fmt.Sprintf("%d\n", workMinutes)
			}
			return fmt.Sprintf("⏳ %s minutes de travail avant la liberté !\n", formatNumber(workMinutes))
		}
		if raw {
			return fmt.Sprintf("%d\n", totalMinutes)
		}
		return fmt.Sprintf("⏳ %s minutes calendaires avant la liberté !\n", formatNumber(totalMinutes))

	case "secondes":
		if workdaysOnly {
			if raw {
				return fmt.Sprintf("%d\n", workSeconds)
			}
			return fmt.Sprintf("⏳ %s secondes de travail avant la liberté !\n", formatNumber(workSeconds))
		}
		if raw {
			return fmt.Sprintf("%d\n", totalSeconds)
		}
		return fmt.Sprintf("⏳ %s secondes calendaires avant la liberté !\n", formatNumber(totalSeconds))

	case "jour":
		if workdaysOnly {
			if raw {
				return fmt.Sprintf("%d\n", workDays)
			}
			return fmt.Sprintf("⏳ %s jours ouvrés, %d heures, %d minutes, %d secondes avant la liberté !\n",
				formatNumber(workDays), remHours, remMinutes, remSeconds)
		}
		if raw {
			return fmt.Sprintf("%d\n", totalDays)
		}
		return fmt.Sprintf("⏳ %s jours, %d heures, %d minutes, %d secondes avant la liberté !\n",
			formatNumber(totalDays), remHours, remMinutes, remSeconds)

	case "full":
		fallthrough
	default:
		if raw {
			if workdaysOnly {
				return fmt.Sprintf("%d\n", workDays)
			}
			return fmt.Sprintf("%d\n", totalDays)
		}
		var sb strings.Builder
		sb.WriteString("\n🕊️  Décompte avant la Liberté (30 juin 2027 à 17:00)\n")
		sb.WriteString("═════════════════════════════════════════════════════════\n\n")
		fmt.Fprintf(&sb, "  Temps restant (calendrier)  : %d jours, %d heures, %d minutes, %d secondes\n",
			totalDays, remHours, remMinutes, remSeconds)
		fmt.Fprintf(&sb, "  Temps restant (ouvrés 8h/j) : %d jours ouvrés, %d heures, %d minutes, %d secondes\n\n",
			workDays, remHours, remMinutes, remSeconds)
		sb.WriteString("  En chiffres :\n")
		fmt.Fprintf(&sb, "    • Jours calendaires   : %s jours\n", formatNumber(totalDays))
		fmt.Fprintf(&sb, "    • Jours ouvrés        : %s jours ouvrés\n", formatNumber(workDays))
		fmt.Fprintf(&sb, "    • Heures calendaires   : %s heures\n", formatNumber(totalHours))
		fmt.Fprintf(&sb, "    • Heures de travail    : %s heures (base 8h/j)\n", formatNumber(workHours))
		fmt.Fprintf(&sb, "    • Minutes calendaires  : %s minutes\n", formatNumber(totalMinutes))
		fmt.Fprintf(&sb, "    • Minutes de travail   : %s minutes\n", formatNumber(workMinutes))
		fmt.Fprintf(&sb, "    • Secondes calendaires : %s secondes\n", formatNumber(totalSeconds))
		fmt.Fprintf(&sb, "    • Secondes de travail  : %s secondes\n\n", formatNumber(workSeconds))
		sb.WriteString("═════════════════════════════════════════════════════════\n\n")
		return sb.String()
	}
}

func formatNumber(n int64) string {
	in := strconv.FormatInt(n, 10)
	out := make([]byte, 0, len(in)+(len(in)-1)/3)
	for i, c := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
