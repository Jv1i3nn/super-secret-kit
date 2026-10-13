package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"ssk/internal/version"
)

const (
	updateRepo        = "Jv1i3nn/super-secret-kit"
	packagedBinary    = "/usr/local/bin/ssk"
	updateHTTPTimeout = 5 * time.Minute
	updateCheckEvery  = 24 * time.Hour
)

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update SSK to the latest GitHub release",
	Long: `Download the latest release package (.deb on Linux, .pkg on macOS)
and install it with sudo. Only works for package installs in /usr/local/bin.`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "only check whether a new version is available")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	client := &http.Client{Timeout: updateHTTPTimeout}
	latest, err := latestReleaseVersion(client)
	if err != nil {
		return err
	}
	current := version.GetVersion()
	if !isNewerVersion(latest, current) {
		fmt.Printf("✅ SSK est à jour (v%s, dernière release v%s)\n", current, latest)
		return nil
	}
	if updateCheckOnly {
		fmt.Printf("🆕 Nouvelle version disponible : v%s → v%s (lancer 'ssk update')\n", current, latest)
		return nil
	}

	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return fmt.Errorf("failed to locate ssk binary: %w", err)
	}
	if exe != packagedBinary {
		return fmt.Errorf("ssk is not installed from a package (%s); update it with git pull && make install", exe)
	}

	asset, err := releaseAssetName(runtime.GOOS, runtime.GOARCH, latest)
	if err != nil {
		return err
	}
	fmt.Printf("⬇️  Mise à jour v%s → v%s (%s)\n", current, latest, asset)

	dir, err := os.MkdirTemp("", "ssk-update-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	pkg := filepath.Join(dir, asset)
	url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", updateRepo, latest, asset)
	if err := download(client, url, pkg); err != nil {
		return err
	}

	install := exec.Command("sudo", "dpkg", "-i", pkg)
	if runtime.GOOS == "darwin" {
		install = exec.Command("sudo", "installer", "-pkg", pkg, "-target", "/")
	}
	install.Stdin, install.Stdout, install.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("package installation failed: %w", err)
	}
	fmt.Printf("✅ SSK mis à jour en v%s\n", latest)
	return nil
}

// latestReleaseVersion returns the latest release tag without its "v" prefix
// and records it in the update-check cache.
func latestReleaseVersion(client *http.Client) (string, error) {
	latest, err := fetchLatestReleaseVersion(client)
	if err == nil {
		_ = os.WriteFile(updateCachePath(), []byte(latest), 0600)
	}
	return latest, err
}

func fetchLatestReleaseVersion(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/repos/" + updateRepo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to check latest release: %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to read latest release: %w", err)
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

// releaseAssetName mirrors the package names produced by the Makefile.
func releaseAssetName(goos, goarch, ver string) (string, error) {
	if goos == "darwin" {
		return fmt.Sprintf("ssk-%s-universal.pkg", ver), nil
	}
	debArch := map[string]string{"amd64": "amd64", "arm64": "arm64", "arm": "armhf"}[goarch]
	if goos != "linux" || debArch == "" {
		return "", fmt.Errorf("no release package for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("ssk_%s_%s.deb", ver, debArch), nil
}

func download(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	return f.Close()
}

// isNewerVersion reports whether latest > current. Only plain X.Y.Z release
// versions compare: dev builds ("dev", "1.2.0-3-gabc-dirty") never update.
func isNewerVersion(latest, current string) bool {
	l, okL := parseVersion(latest)
	c, okC := parseVersion(current)
	if !okL || !okC {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// updateCachePath holds the latest known release; its mtime is the last check.
func updateCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssk", "latest-version")
}

// updateNotice returns a "new version available" message, or "" if none.
// It hits GitHub at most once per updateCheckEvery, and only for a terminal.
func updateNotice() string {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return ""
	}
	path := updateCachePath()
	cached, _ := os.ReadFile(path)
	latest := strings.TrimSpace(string(cached))
	if info, err := os.Stat(path); err != nil || time.Since(info.ModTime()) > updateCheckEvery {
		if fetched, err := fetchLatestReleaseVersion(&http.Client{Timeout: 2 * time.Second}); err == nil {
			latest = fetched
		}
		// Rewrite even on failure so an offline machine waits a full period.
		_ = os.WriteFile(path, []byte(latest), 0600)
	}
	if !isNewerVersion(latest, version.GetVersion()) {
		return ""
	}
	return fmt.Sprintf("\n🆕 SSK v%s est disponible (installée : v%s). Lancer 'ssk update'.", latest, version.GetVersion())
}
