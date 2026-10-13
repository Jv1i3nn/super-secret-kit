package totp

import (
	"testing"
	"time"
)

func TestGenerateCode(t *testing.T) {
	// RFC 6238 standard test vectors using secret "12345678901234567890" in base32
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	testCases := []struct {
		timestamp int64
		expected  string
	}{
		{59, "287082"},
	}

	for _, tc := range testCases {
		tm := time.Unix(tc.timestamp, 0)
		code, err := GenerateCode(secret, tm)
		if err != nil {
			t.Fatalf("unexpected error at timestamp %d: %v", tc.timestamp, err)
		}
		if code != tc.expected {
			t.Errorf("at timestamp %d, expected %s, got %s", tc.timestamp, tc.expected, code)
		}
	}
}
