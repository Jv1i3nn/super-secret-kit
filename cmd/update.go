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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ssk/internal/version"
)

const (
	updateRepo        = "Jv1i3nn/super-secret-kit"
	packagedBinary    = "/usr/local/bin/ssk"
	updateHTTPTimeout = 5 * time.Minute
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update SSK to the latest GitHub release",
	Long: `Download the latest release package (.deb on Linux, .pkg on macOS)
and install it with sudo. Only works for package installs in /usr/local/bin.`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
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

	client := &http.Client{Timeout: updateHTTPTimeout}
	latest, err := latestReleaseVersion(client)
	if err != nil {
		return err
	}
	current := version.GetVersion()
	if latest == current {
		fmt.Printf("✅ SSK est à jour (v%s)\n", current)
		return nil
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

// latestReleaseVersion returns the latest release tag without its "v" prefix.
func latestReleaseVersion(client *http.Client) (string, error) {
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
