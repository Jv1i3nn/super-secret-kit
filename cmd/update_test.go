package cmd

import "testing"

func TestReleaseAssetName(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, want string }{
		{"linux", "amd64", "ssk_1.2.0_amd64.deb"},
		{"linux", "arm64", "ssk_1.2.0_arm64.deb"},
		{"linux", "arm", "ssk_1.2.0_armhf.deb"},
		{"darwin", "arm64", "ssk-1.2.0-universal.pkg"},
		{"darwin", "amd64", "ssk-1.2.0-universal.pkg"},
	} {
		got, err := releaseAssetName(tc.goos, tc.goarch, "1.2.0")
		if err != nil || got != tc.want {
			t.Errorf("%s/%s: got %q, %v; want %q", tc.goos, tc.goarch, got, err, tc.want)
		}
	}
	if _, err := releaseAssetName("windows", "amd64", "1.2.0"); err == nil {
		t.Error("windows: expected error")
	}
}

func TestIsNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"1.2.0", "1.1.9", true},
		{"1.10.0", "1.9.0", true},
		{"2.0.0", "1.99.99", true},
		{"1.2.0", "1.2.0", false},
		{"1.1.0", "1.2.0", false}, // no downgrade
		{"v1.3.0", "1.2.0", true},
		{"1.3.0", "dev", false},
		{"1.3.0", "1.2.0-3-gabc-dirty", false},
		{"", "1.2.0", false},
	} {
		if got := isNewerVersion(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}
