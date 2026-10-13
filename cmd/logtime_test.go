package cmd

import (
	"testing"
)

func TestCalculateTimeToLog(t *testing.T) {
	tests := []struct {
		name           string
		requestedHours int
		alreadySpent   int // in seconds
		wantToLog      int // in seconds
		wantShouldLog  bool
	}{
		{
			name:           "empty day, log 8h",
			requestedHours: 8,
			alreadySpent:   0,
			wantToLog:      28800,
			wantShouldLog:  true,
		},
		{
			name:           "empty day, log 2h",
			requestedHours: 2,
			alreadySpent:   0,
			wantToLog:      7200,
			wantShouldLog:  true,
		},
		{
			name:           "2h already logged, log 6h to complete day to 8h",
			requestedHours: 6,
			alreadySpent:   7200,
			wantToLog:      21600,
			wantShouldLog:  true,
		},
		{
			name:           "5h already logged, log 3h to complete day to 8h",
			requestedHours: 3,
			alreadySpent:   18000,
			wantToLog:      10800,
			wantShouldLog:  true,
		},
		{
			name:           "5h already logged, request 5h (should cap to 3h remaining)",
			requestedHours: 5,
			alreadySpent:   18000,
			wantToLog:      10800,
			wantShouldLog:  true,
		},
		{
			name:           "5h already logged, default --hours 8 (should cap to 3h remaining)",
			requestedHours: 8,
			alreadySpent:   18000,
			wantToLog:      10800,
			wantShouldLog:  true,
		},
		{
			name:           "8h already logged, request 2h (already full, skip)",
			requestedHours: 2,
			alreadySpent:   28800,
			wantToLog:      0,
			wantShouldLog:  false,
		},
		{
			name:           "8h already logged, request 8h (already full, skip)",
			requestedHours: 8,
			alreadySpent:   28800,
			wantToLog:      0,
			wantShouldLog:  false,
		},
		{
			name:           "9h already logged (overlogged), request 2h (already full, skip)",
			requestedHours: 2,
			alreadySpent:   32400,
			wantToLog:      0,
			wantShouldLog:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToLog, gotShouldLog := calculateTimeToLog(tt.requestedHours, tt.alreadySpent)
			if gotShouldLog != tt.wantShouldLog {
				t.Errorf("calculateTimeToLog() gotShouldLog = %v, want %v", gotShouldLog, tt.wantShouldLog)
			}
			if gotToLog != tt.wantToLog {
				t.Errorf("calculateTimeToLog() gotToLog = %v, want %v", gotToLog, tt.wantToLog)
			}
		})
	}
}
