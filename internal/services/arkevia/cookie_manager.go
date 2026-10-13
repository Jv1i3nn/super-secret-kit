package arkevia

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const tokenFile = "arkevia_token.json"

type CookieManager struct {
	tokenDir string
}

func NewCookieManager(tokenDir string) *CookieManager {
	return &CookieManager{
		tokenDir: tokenDir,
	}
}

func (cm *CookieManager) LoadCookies() (string, error) {
	filePath := filepath.Join(cm.tokenDir, tokenFile)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", fmt.Errorf("failed to parse cookies: %w", err)
	}

	var cookiePairs []string
	for name, value := range cookies {
		cookiePairs = append(cookiePairs, fmt.Sprintf("%s=%s", name, value))
	}

	return strings.Join(cookiePairs, "; "), nil
}

func (cm *CookieManager) SaveCookies(setCookieHeaders []string) error {
	cookies := make(map[string]string)
	cookieRegex := regexp.MustCompile(`([^=]+)=([^;]+)`)

	for _, header := range setCookieHeaders {
		matches := cookieRegex.FindStringSubmatch(header)
		if len(matches) >= 3 {
			cookies[matches[1]] = matches[2]
		}
	}

	filePath := filepath.Join(cm.tokenDir, tokenFile)
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

func (cm *CookieManager) GetToken(key string) (string, error) {
	filePath := filepath.Join(cm.tokenDir, tokenFile)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", fmt.Errorf("failed to parse cookies: %w", err)
	}

	return cookies[key], nil
}
