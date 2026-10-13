package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ssk/internal/config"
)

func ensureJiraAPIURL(cfg *config.Config, workspace string) (string, error) {
	return configureJiraAPIURL(cfg, workspace, os.Stdin, os.Stderr, func(value string) error { return config.SaveServiceURL("JIRA_API_URL", value) })
}

func configureJiraAPIURL(cfg *config.Config, workspace string, input io.Reader, output io.Writer, save func(string) error) (string, error) {
	apiURL, err := cfg.GetJiraAPIURL(workspace)
	if err == nil {
		return config.NormalizeBaseURL(apiURL)
	}
	if workspace != "" && workspace != config.DefaultJiraWorkspace {
		return "", err
	}
	apiURL, err = promptServiceURL("Jira", "JIRA_API_URL", "https://your-team.atlassian.net", input, output, save)
	if err == nil {
		cfg.JiraAPIURL = apiURL
	}
	return apiURL, err
}

func promptServiceURL(service, key, example string, input io.Reader, output io.Writer, save func(string) error) (string, error) {
	if _, err := fmt.Fprintf(output, "%s URL (e.g. %s): ", service, example); err != nil {
		return "", err
	}
	var value string
	if _, err := fmt.Fscanln(input, &value); err != nil {
		return "", fmt.Errorf("%s URL is required; set %s in ~/.ssk/.env or enter it on first use", service, key)
	}
	apiURL, err := config.NormalizeBaseURL(value)
	if err != nil {
		return "", err
	}
	if saveErr := save(apiURL); saveErr != nil {
		return "", fmt.Errorf("failed to save %s URL: %w", service, saveErr)
	}
	_, err = fmt.Fprintf(output, "%s URL saved in ~/.ssk/.env\n", service)
	return apiURL, err
}

func ensureHoroquartzURL(cfg *config.Config) (string, error) {
	if cfg.HoroquartzURL != "" {
		return config.NormalizeBaseURL(cfg.HoroquartzURL)
	}
	apiURL, err := promptServiceURL("Horoquartz", "HOROQUARTZ_URL", "https://your-team.cloud-horoquartz.fr", os.Stdin, os.Stderr, func(value string) error {
		return config.SaveServiceURL("HOROQUARTZ_URL", value)
	})
	if err == nil {
		cfg.HoroquartzURL = apiURL
	}
	return apiURL, err
}

func tempoAPIKeyPrompt(apiURL string) string {
	return fmt.Sprintf("Please set your Jira tempo API key (%s/plugins/servlet/ac/io.tempo.jira/tempo-app#!/configuration/api-integration).", strings.TrimRight(apiURL, "/"))
}
