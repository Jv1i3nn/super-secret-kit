package arkevia

import "testing"

func TestDocumentMatchesPeriod(t *testing.T) {
	const (
		may1  = "John Doe_Bulletin de paie du 2026-05-31_1.pdf"
		may2  = "John Doe_Bulletin de paie du 2026-05-31_2.pdf"
		april = "John Doe_Bulletin de paie du 2026-04-30_1.pdf"
		feb   = "John Doe_Bulletin de paie du 2026-02-28.pdf"
		noDt  = "Document sans date.pdf"
	)

	tests := []struct {
		name     string
		filename string
		year     int
		month    int
		want     bool
	}{
		{name: "month match first split", filename: may1, year: 2026, month: 5, want: true},
		{name: "month match second split", filename: may2, year: 2026, month: 5, want: true},
		{name: "month single digit padded", filename: april, year: 2026, month: 4, want: true},
		{name: "wrong month", filename: feb, year: 2026, month: 5, want: false},
		{name: "wrong year same month", filename: may1, year: 2025, month: 5, want: false},
		{name: "year only matches any month", filename: feb, year: 2026, month: 0, want: true},
		{name: "year only wrong year", filename: feb, year: 2025, month: 0, want: false},
		{name: "no date in filename", filename: noDt, year: 2026, month: 5, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := documentMatchesPeriod(tt.filename, tt.year, tt.month); got != tt.want {
				t.Fatalf("documentMatchesPeriod(%q, %d, %d) = %v, want %v", tt.filename, tt.year, tt.month, got, tt.want)
			}
		})
	}
}

func TestDocumentMatchesPeriodCountsAllSplits(t *testing.T) {
	documents := []Document{
		{ID: "1", Filename: "John Doe_Bulletin de paie du 2026-05-31_1.pdf"},
		{ID: "2", Filename: "John Doe_Bulletin de paie du 2026-05-31_2.pdf"},
		{ID: "3", Filename: "John Doe_Bulletin de paie du 2026-04-30_1.pdf"},
	}

	count := 0
	for _, doc := range documents {
		if documentMatchesPeriod(doc.Filename, 2026, 5) {
			count++
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 documents for 2026-05, got %d", count)
	}
}
