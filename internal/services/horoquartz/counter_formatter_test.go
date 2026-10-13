package horoquartz

import (
	"testing"
)

func TestFormatCounterName(t *testing.T) {
	testCases := []struct {
		code     string
		label    string
		expected string
	}{
		{"CNT_CP", "", "Congés Payés (CP)"},
		{"CNT_CP", "Congés Payés Acquis", "Congés Payés Acquis"},
		{"CNT_RTT", "", "RTT"},
		{"UNKNOWN_CODE", "", "UNKNOWN_CODE"},
	}

	for _, tc := range testCases {
		res := FormatCounterName(tc.code, tc.label)
		if res != tc.expected {
			t.Errorf("FormatCounterName(%q, %q) = %q; expected %q", tc.code, tc.label, res, tc.expected)
		}
	}
}

func TestFormatUnit(t *testing.T) {
	testCases := []struct {
		unit     string
		expected string
	}{
		{"D", "jours"},
		{"H", "heures"},
		{"M", "minutes"},
		{"X", "X"},
	}

	for _, tc := range testCases {
		res := FormatUnit(tc.unit)
		if res != tc.expected {
			t.Errorf("FormatUnit(%q) = %q; expected %q", tc.unit, res, tc.expected)
		}
	}
}

func TestFormatCounterValue(t *testing.T) {
	res := FormatCounterValue("15.500", "D")
	expected := "15.5 jours"
	if res != expected {
		t.Errorf("FormatCounterValue = %q; expected %q", res, expected)
	}
}
