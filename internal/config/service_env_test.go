package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func readServiceURLForTest(t *testing.T, path, key string) (string, error) {
	t.Helper()
	env, err := readServiceEnv(path)
	if err != nil {
		return "", err
	}
	return env.GetString(key), nil
}

func TestLoadServiceURLsFromEnv(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("JIRA_API_URL", "")
	t.Setenv("HOROQUARTZ_URL", "")
	viper.Set("external_project_path", t.TempDir())
	path := filepath.Join(t.TempDir(), ".env")
	if operationErr := writeServiceURL(path, "JIRA_API_URL", "https://team.example/jira"); operationErr != nil {
		t.Fatal(operationErr)
	}
	if operationErr := writeServiceURL(path, "HOROQUARTZ_URL", "https://team.example/hq"); operationErr != nil {
		t.Fatal(operationErr)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JiraAPIURL != "https://team.example/jira" || cfg.HoroquartzURL != "https://team.example/hq" {
		t.Fatal("service URLs not loaded")
	}
	if operationErr := writeServiceURL(path, "JIRA_API_URL", "https://new.example"); operationErr != nil {
		t.Fatal(operationErr)
	}
	cfg, err = loadConfig(path)
	if err != nil || cfg.JiraAPIURL != "https://new.example" || cfg.HoroquartzURL != "https://team.example/hq" {
		t.Fatal("updated URL not reloaded or other service changed")
	}
	t.Setenv("JIRA_API_URL", "https://override.example")
	t.Setenv("HOROQUARTZ_URL", "https://override.example/hq")
	cfg, err = loadConfig(path)
	if err != nil || cfg.JiraAPIURL != "https://override.example" || cfg.HoroquartzURL != "https://override.example/hq" {
		t.Fatal("environment variables did not override .env")
	}
	// An existing YAML configuration takes priority over the .env fallback.
	t.Setenv("JIRA_API_URL", "")
	t.Setenv("HOROQUARTZ_URL", "")
	if operationErr := viper.MergeConfigMap(map[string]interface{}{"jira_api_url": "https://yaml.example", "horoquartz_url": "https://yaml.example/hq"}); operationErr != nil {
		t.Fatal(operationErr)
	}
	cfg, err = loadConfig(path)
	if err != nil || cfg.JiraAPIURL != "https://yaml.example" || cfg.HoroquartzURL != "https://yaml.example/hq" {
		t.Fatal("YAML settings did not override .env")
	}
}

func TestServiceURLsHaveNoDefaultHost(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("JIRA_API_URL", "")
	t.Setenv("HOROQUARTZ_URL", "")
	viper.Set("external_project_path", t.TempDir())
	cfg, err := loadConfig(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JiraAPIURL != "" || cfg.HoroquartzURL != "" || len(cfg.JiraWorkspaces) != 0 {
		t.Fatal("found predefined company URLs")
	}
}

func TestJiraEnvPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssk", ".env")
	if got, operationErr := readServiceURLForTest(t, path, "JIRA_API_URL"); operationErr != nil || got != "" {
		t.Fatalf("missing file: got %q, %v", got, operationErr)
	}
	if operationErr := writeServiceURL(path, "JIRA_API_URL", " https://team.atlassian.net/ "); operationErr != nil {
		t.Fatal(operationErr)
	}
	got, err := readServiceURLForTest(t, path, "JIRA_API_URL")
	if err != nil || got != "https://team.atlassian.net" {
		t.Fatalf("saved URL: got %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %o", info.Mode().Perm())
	}
}

func TestJiraEnvPreservesOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := "# local configuration\nOTHER_SETTING='keep me'\nexport JIRA_API_URL=https://old.example\n"
	if operationErr := os.WriteFile(path, []byte(original), 0644); operationErr != nil {
		t.Fatal(operationErr)
	}
	if operationErr := writeServiceURL(path, "JIRA_API_URL", "https://new.example/jira/"); operationErr != nil {
		t.Fatal(operationErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# local configuration\nOTHER_SETTING='keep me'\n") {
		t.Fatal("other entries changed")
	}
	if strings.Contains(string(data), "old.example") || strings.Count(string(data), "JIRA_API_URL=") != 1 {
		t.Fatal("URL was not replaced")
	}
	if got, operationErr := readServiceURLForTest(t, path, "JIRA_API_URL"); operationErr != nil || got != "https://new.example/jira" {
		t.Fatalf("updated URL: got %q, %v", got, operationErr)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	for _, value := range []string{"", "team.atlassian.net", "ftp://team.example", "https://", "https://user:token@team.example", "https://team.example?token=secret", "https://team.example/#fragment", "https://team.example\nOTHER=bad"} {
		if _, operationErr := NormalizeBaseURL(value); operationErr == nil {
			t.Errorf("accepted invalid URL %q", value)
		}
	}
	for _, value := range []string{"https://team.atlassian.net", "http://localhost:8080/jira"} {
		if got, operationErr := NormalizeBaseURL(value + "/"); operationErr != nil || got != value {
			t.Errorf("normalization: got %q, %v", got, operationErr)
		}
	}
}

func TestInvalidJiraURLDoesNotModifyEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if operationErr := writeServiceURL(path, "JIRA_API_URL", "not-a-url"); operationErr == nil {
		t.Fatal("accepted invalid URL")
	}
	if _, operationErr := os.Stat(path); !os.IsNotExist(operationErr) {
		t.Fatal("created file for invalid URL")
	}
}

func TestJiraConfigurationOverrides(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	envPath := filepath.Join(t.TempDir(), ".env")
	viper.Set("external_project_path", t.TempDir())
	viper.Set("jira_api_url", "https://configured.example")
	setDefaults()
	t.Setenv("JIRA_API_URL", "https://environment.example")
	// A config-file value has lower priority than process environment variables.
	viper.Set("jira_api_url", nil)
	if operationErr := viper.MergeConfigMap(map[string]interface{}{"jira_api_url": "https://configured.example"}); operationErr != nil {
		t.Fatal(operationErr)
	}
	cfg, err := loadConfig(envPath)
	if err != nil || cfg.JiraAPIURL != "https://environment.example" {
		t.Fatalf("environment override: got %+v, %v", cfg, err)
	}
	t.Setenv("JIRA_API_URL", "")
	cfg, err = loadConfig(envPath)
	if err != nil || cfg.JiraAPIURL != "https://configured.example" {
		t.Fatalf("config override: got %+v, %v", cfg, err)
	}
}

func TestNamedJiraWorkspaces(t *testing.T) {
	cfg := &Config{JiraAPIURL: "https://default.example", JiraWorkspaces: map[string]string{"other": "https://other.example"}}
	for workspace, want := range map[string]string{"": "https://default.example", "default": "https://default.example", "other": "https://other.example"} {
		got, err := cfg.GetJiraAPIURL(workspace)
		if err != nil || got != want {
			t.Fatalf("workspace %q: got %q, %v", workspace, got, err)
		}
	}
	if _, operationErr := cfg.GetJiraAPIURL("unknown"); operationErr == nil {
		t.Fatal("accepted unknown workspace")
	}
}
