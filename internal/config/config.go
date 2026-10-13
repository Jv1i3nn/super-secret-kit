package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// DefaultJiraWorkspace is the workspace used when none is specified explicitly
// (e.g. via the --workspace flag).
const DefaultJiraWorkspace = "default"

type Config struct {
	AppEnv              string            `mapstructure:"app_env"`
	AppVersion          string            `mapstructure:"app_version"`
	ExternalProjectPath string            `mapstructure:"external_project_path"`
	DatabaseURL         string            `mapstructure:"database_url"`
	JiraAPIURL          string            `mapstructure:"jira_api_url"`
	JiraWorkspaces      map[string]string `mapstructure:"jira_workspaces"`
	TempoAPIURL         string            `mapstructure:"tempo_api_url"`
	HoroquartzURL       string            `mapstructure:"horoquartz_url"`
}

func Load() (*Config, error) {
	path, err := serviceEnvPath()
	if err != nil {
		return nil, err
	}
	return loadConfig(path)
}

func loadConfig(envPath string) (*Config, error) {
	setDefaults()

	var cfg Config

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.JiraAPIURL == "" || cfg.HoroquartzURL == "" {
		env, err := readServiceEnv(envPath)
		if err != nil {
			return nil, err
		}
		if cfg.JiraAPIURL == "" {
			cfg.JiraAPIURL = env.GetString("JIRA_API_URL")
		}
		if cfg.HoroquartzURL == "" {
			cfg.HoroquartzURL = env.GetString("HOROQUARTZ_URL")
		}
	}

	cfg.ExternalProjectPath = expandPath(cfg.ExternalProjectPath)
	cfg.DatabaseURL = expandPath(cfg.DatabaseURL)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("app_env", "dev")
	viper.SetDefault("app_version", "2.0.0")
	viper.SetDefault("external_project_path", "~/.ssk")
	viper.SetDefault("database_url", "~/.ssk/data.db")
	viper.SetDefault("jira_api_url", "")
	_ = viper.BindEnv("jira_api_url", "JIRA_API_URL")
	viper.SetDefault("jira_workspaces", map[string]string{})
	viper.SetDefault("tempo_api_url", "https://api.tempo.io")
	viper.SetDefault("horoquartz_url", "")
	_ = viper.BindEnv("horoquartz_url", "HOROQUARTZ_URL")
}

func (c *Config) Validate() error {
	if err := os.MkdirAll(c.ExternalProjectPath, 0755); err != nil {
		return err
	}

	return nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}

func (c *Config) GetPassphrasePath() string {
	return filepath.Join(c.ExternalProjectPath, ".passphrase")
}

func (c *Config) GetDatabasePath() string {
	return c.DatabaseURL
}

// GetJiraAPIURL resolves the base URL for a named Jira workspace. An empty
// workspace falls back to DefaultJiraWorkspace, preserving backward
// compatibility with the historical single-instance jira_api_url setting.
func (c *Config) GetJiraAPIURL(workspace string) (string, error) {
	if workspace == "" {
		workspace = DefaultJiraWorkspace
	}

	// jira_api_url remains the historical single-instance override (e.g. via
	// env var) and always wins for the default workspace.
	if workspace == DefaultJiraWorkspace && c.JiraAPIURL != "" {
		return c.JiraAPIURL, nil
	}

	if url, ok := c.JiraWorkspaces[workspace]; ok && url != "" {
		return url, nil
	}
	if workspace == DefaultJiraWorkspace {
		return "", fmt.Errorf("jira URL is not configured (set JIRA_API_URL in ~/.ssk/.env)")
	}

	return "", fmt.Errorf("unknown Jira workspace %q (configure it under jira_workspaces)", workspace)
}
