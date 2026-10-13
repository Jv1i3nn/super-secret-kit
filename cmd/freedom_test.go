package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFreedomTarget(t *testing.T) {
	target := FreedomTarget()
	if target.Year() != 2027 || target.Month() != time.June || target.Day() != 30 || target.Hour() != 17 || target.Minute() != 0 || target.Second() != 0 {
		t.Fatalf("expected June 30, 2027 17:00:00, got %v", target)
	}
}

func TestCountBusinessDays(t *testing.T) {
	start := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC) // Wed
	end := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)   // Next Wed (7 full days = 5 workdays)

	workdays := CountBusinessDays(start, end)
	if workdays != 5 {
		t.Fatalf("expected 5 workdays, got %d", workdays)
	}
}

func TestFormatFreedom_PastDate(t *testing.T) {
	target := time.Date(2027, time.June, 30, 17, 0, 0, 0, time.UTC)
	now := target.Add(1 * time.Hour)

	outFormatted := FormatFreedom(now, target, "full", false, false)
	if !strings.Contains(outFormatted, "déjà là") {
		t.Errorf("expected output to mention liberty reached, got: %q", outFormatted)
	}

	outRaw := FormatFreedom(now, target, "full", false, true)
	if outRaw != "0\n" {
		t.Errorf("expected raw output 0\\n, got: %q", outRaw)
	}
}

func TestFormatFreedom_FutureDateCombinations(t *testing.T) {
	// target: 2027-06-30 17:00:00 UTC
	target := time.Date(2027, time.June, 30, 17, 0, 0, 0, time.UTC)
	// now: 10 days (Wed to Next Sat = 7 workdays in 10 full 24h days), 2 hours, 3 minutes, 4 seconds before target
	now := target.Add(-10*24*time.Hour - 2*time.Hour - 3*time.Minute - 4*time.Second)

	// Default full format
	fullOut := FormatFreedom(now, target, "full", false, false)
	if !strings.Contains(fullOut, "Heures de travail") {
		t.Errorf("unexpected full breakdown (missing Heures de travail):\n%s", fullOut)
	}

	// Workdays + Heure: 7 workdays * 8h + 2h = 58 work hours
	workHoursOut := FormatFreedom(now, target, "heure", true, false)
	if !strings.Contains(workHoursOut, "58 heures de travail") {
		t.Errorf("expected 58 heures de travail, got %q", workHoursOut)
	}
	workHoursRaw := FormatFreedom(now, target, "heure", true, true)
	if workHoursRaw != "58\n" {
		t.Errorf("expected raw 58\\n, got %q", workHoursRaw)
	}

	// Workdays + Minutes: 58 * 60 + 3 = 3483 work minutes
	workMinutesRaw := FormatFreedom(now, target, "minutes", true, true)
	if workMinutesRaw != "3483\n" {
		t.Errorf("expected raw 3483\\n, got %q", workMinutesRaw)
	}

	// Workdays + Seconds: 3483 * 60 + 4 = 208984 work seconds
	workSecondsRaw := FormatFreedom(now, target, "secondes", true, true)
	if workSecondsRaw != "208984\n" {
		t.Errorf("expected raw 208984\\n, got %q", workSecondsRaw)
	}
}

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		in       int64
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1 000"},
		{1234567, "1 234 567"},
	}

	for _, c := range cases {
		got := formatNumber(c.in)
		if got != c.expected {
			t.Errorf("formatNumber(%d) = %q, expected %q", c.in, got, c.expected)
		}
	}
}

func TestFreedomCmd_ExecutionCombinedWorkdaysAndHours(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"freedom", "-w", "-H", "--raw"})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	out := buf.String()
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected non-empty output from freedom -w -H --raw")
	}
}
