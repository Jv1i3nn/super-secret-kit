package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

func serviceEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssk", ".env"), nil
}

func readServiceEnv(path string) (*viper.Viper, error) {
	env := viper.New()
	env.SetConfigFile(path)
	env.SetConfigType("env")
	if operationErr := env.ReadInConfig(); operationErr != nil {
		if errors.Is(operationErr, os.ErrNotExist) {
			return env, nil
		}
		// Do not include potentially sensitive contents from a malformed .env.
		return nil, fmt.Errorf("could not read service URLs from %s", path)
	}
	return env, nil
}

// NormalizeBaseURL accepts HTTP(S) service base URLs, including context paths.
func NormalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(value, "\r\n\t ") {
		return "", fmt.Errorf("enter a valid base URL starting with https:// or http://, without credentials, query or fragment")
	}
	return strings.TrimRight(value, "/"), nil
}

// SaveServiceURL stores a URL without replacing other .env settings.
func SaveServiceURL(key, value string) error {
	if key != "JIRA_API_URL" && key != "HOROQUARTZ_URL" {
		return fmt.Errorf("unsupported service URL key")
	}
	path, err := serviceEnvPath()
	if err != nil {
		return err
	}
	return writeServiceURL(path, key, value)
}

func writeServiceURL(path, keyName, apiURL string) error {
	apiURL, err := NormalizeBaseURL(apiURL)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	entry := keyName + "=" + strconv.Quote(apiURL)
	found := false
	for i, line := range lines {
		key, _, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if ok && strings.TrimSpace(key) == keyName {
			lines[i] = entry
			found = true
		}
	}
	content := strings.Join(lines, "\n")
	if !found {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += entry + "\n"
	}
	if operationErr := os.MkdirAll(filepath.Dir(path), 0700); operationErr != nil {
		return operationErr
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, operationErr := file.WriteString(content); operationErr != nil {
		_ = file.Close()
		return operationErr
	}
	if operationErr := file.Close(); operationErr != nil {
		return operationErr
	}
	return os.Rename(file.Name(), path)
}
