package tempo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ssk/internal/models"
)

// Client handles Tempo API requests
type Client struct {
	httpClient  *http.Client
	baseURL     string
	credentials *models.Credentials
}

// NewClient creates a new Tempo client
func NewClient(baseURL string, credentials *models.Credentials) *Client {
	return &Client{
		httpClient:  &http.Client{},
		baseURL:     baseURL,
		credentials: credentials,
	}
}

// GetWorklogs retrieves worklogs for a user between two dates
func (c *Client) GetWorklogs(accountID string, from, to time.Time) ([]Worklog, error) {
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	reqURL := fmt.Sprintf("%s/4/worklogs/user/%s?from=%s&to=%s", c.baseURL, accountID, fromStr, toStr)

	resp, err := c.makeRequest("GET", reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get worklogs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var worklogResp WorklogResponse
	if err := json.NewDecoder(resp.Body).Decode(&worklogResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return worklogResp.Results, nil
}

// GetDaysOffForYear retrieves French holidays for a given year
func (c *Client) GetDaysOffForYear(year int) ([]string, error) {
	// Holiday scheme ID 1 is typically for French holidays
	reqURL := fmt.Sprintf("%s/4/holiday-schemes/1/holidays?year=%d", c.baseURL, year)

	resp, err := c.makeRequest("GET", reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get holidays: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var holidayResp HolidayResponse
	if err := json.NewDecoder(resp.Body).Decode(&holidayResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract just the dates
	var dates []string
	for _, holiday := range holidayResp.Results {
		dates = append(dates, holiday.Date)
	}

	return dates, nil
}

// CreateWorklog creates a new worklog entry
func (c *Client) CreateWorklog(worklog Worklog) (*Worklog, error) {
	reqURL := fmt.Sprintf("%s/4/worklogs", c.baseURL)

	// Marshal worklog to JSON
	payload, err := json.Marshal(worklog)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal worklog: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.credentials.Username))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, errRead := io.ReadAll(resp.Body)
		if errRead != nil {
			return nil, fmt.Errorf("API returned status %d (failed to read body: %w)", resp.StatusCode, errRead)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var createdWorklog Worklog
	if err := json.NewDecoder(resp.Body).Decode(&createdWorklog); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &createdWorklog, nil
}

// makeRequest makes an authenticated HTTP request to Tempo API
func (c *Client) makeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Bearer token authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.credentials.Username))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, errRead := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if errRead != nil {
			return nil, fmt.Errorf("API returned status %d (failed to read body: %w)", resp.StatusCode, errRead)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
