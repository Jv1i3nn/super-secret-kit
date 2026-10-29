package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMarkdownSafetyTruncationPreservesUnicode(t *testing.T) {
	for budget := 40; budget < 60; budget++ {
		text := truncateString(strings.Repeat("🦊", 100), budget)
		if !utf8.ValidString(text) || len([]rune(text)) > budget || !strings.Contains(text, "truncated") {
			t.Fatalf("invalid Unicode or budget overflow at %d: %q", budget, text)
		}
	}
}
