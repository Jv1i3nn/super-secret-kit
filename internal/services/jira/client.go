package jira

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ssk/internal/models"
)

// Client handles Jira API requests
type Client struct {
	httpClient  *http.Client
	baseURL     string
	credentials *models.Credentials
}

// DownloadAttachment downloads a Jira-hosted attachment without exposing credentials.
func (c *Client) DownloadAttachment(sourceURL, destination string) error {
	parsedSource, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid attachment URL: %w", err)
	}
	parsedBase, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid Jira base URL: %w", err)
	}
	if parsedSource.User != nil || parsedSource.Scheme != parsedBase.Scheme || parsedSource.Host != parsedBase.Host {
		return fmt.Errorf("attachment URL is outside the configured Jira site")
	}

	resp, err := c.makeDownloadRequest(sourceURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(destination)
		}
	}()
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write local file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close local file: %w", closeErr)
	}
	success = true
	return nil
}

func (c *Client) makeDownloadRequest(sourceURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.credentials.Username, c.credentials.Password)))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", auth))
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	return resp, nil
}

// NewClient creates a new Jira client
func NewClient(baseURL string, credentials *models.Credentials) *Client {
	return &Client{
		httpClient:  &http.Client{},
		baseURL:     baseURL,
		credentials: credentials,
	}
}

// GetAssignedTickets retrieves tickets assigned to current user
func (c *Client) GetAssignedTickets() ([]Ticket, error) {
	jql := "assignee = currentUser() AND resolution = Unresolved ORDER BY updatedDate DESC"

	return c.searchTickets(jql, "id,key,summary,status,project")
}

// GetAutofillCandidateTickets retrieves recently updated tickets assigned to the current user.
func (c *Client) GetAutofillCandidateTickets() ([]Ticket, error) {
	jql := "assignee = currentUser() AND updated >= -14d ORDER BY updated DESC"

	return c.searchTickets(jql, "id,key,summary,status,created,updated,resolutiondate,assignee")
}

// GetBriefingCandidateTickets retrieves recently updated tickets assigned to the current user.
func (c *Client) GetBriefingCandidateTickets() ([]Ticket, error) {
	jql := "assignee = currentUser() AND updated >= -30d AND statusCategory != Done ORDER BY updated DESC"

	return c.searchTickets(jql, "id,key,summary,status,project,created,updated,resolutiondate,assignee,reporter,priority,labels,components,fixVersions,parent,subtasks,issuelinks")
}

// SearchTickets runs an arbitrary JQL query and returns matching tickets.
func (c *Client) SearchTickets(jql string) ([]Ticket, error) {
	return c.searchTickets(jql, "id,key,summary,description,status,resolution,project,created,updated,resolutiondate,assignee,parent,subtasks,issuelinks,components,labels,fixVersions")
}

// BaseURL is the configured Jira site used to identify exported issues.
func (c *Client) BaseURL() string {
	return strings.TrimRight(c.baseURL, "/")
}

func (c *Client) searchTickets(jql, fields string) ([]Ticket, error) {
	params := url.Values{"jql": {jql}, "fields": {fields}, "maxResults": {"100"}}
	tickets := make([]Ticket, 0)
	seenIDs := make(map[string]bool)
	seenTokens := make(map[string]bool)
	// Bound pathological responses without ever claiming a truncated success.
	const maxSearchPages = 100
	for page := 1; page <= maxSearchPages; page++ {
		reqURL := fmt.Sprintf("%s/rest/api/3/search/jql?%s", c.BaseURL(), params.Encode())
		resp, err := c.makeRequest(reqURL)
		if err != nil {
			return nil, fmt.Errorf("search page %d failed: %w", page, err)
		}
		var result TicketSearchResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode search page %d: %w", page, decodeErr)
		}
		if result.Issues == nil || result.IsLast == nil {
			return nil, fmt.Errorf("search page %d lacks issues or completion metadata", page)
		}
		if *result.IsLast && result.NextPageToken != "" {
			return nil, fmt.Errorf("search page %d has inconsistent completion metadata", page)
		}
		before := len(tickets)
		for _, ticket := range result.Issues {
			if strings.TrimSpace(ticket.ID) == "" {
				return nil, fmt.Errorf("search page %d contains an issue without an ID", page)
			}
			if !seenIDs[ticket.ID] {
				seenIDs[ticket.ID] = true
				tickets = append(tickets, ticket)
			}
		}
		if *result.IsLast {
			return tickets, nil
		}
		token := result.NextPageToken
		if token == "" || seenTokens[token] {
			return nil, fmt.Errorf("search page %d has a missing or repeated continuation token", page)
		}
		if len(tickets) == before {
			return nil, fmt.Errorf("search page %d made no progress", page)
		}
		seenTokens[token] = true
		params.Set("nextPageToken", token)
	}
	return nil, fmt.Errorf("search exceeded the %d page limit; narrow the JQL query", maxSearchPages)
}

// GetTicketByKey retrieves a ticket by its key
func (c *Client) GetTicketByKey(issueKey string, fields []string) (*Ticket, error) {
	defaultFields := []string{"id", "key", "summary", "status", "project", "description", "assignee", "reporter", "priority", "created", "updated"}
	if len(fields) == 0 {
		fields = defaultFields
	}

	params := url.Values{}
	params.Set("fields", joinStrings(fields, ","))

	reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s?%s", c.baseURL, issueKey, params.Encode())

	resp, err := c.makeRequest(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var ticket Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ticket, nil
}

// GetBriefingTicketByKey retrieves a ticket with the fields needed for briefing generation.
func (c *Client) GetBriefingTicketByKey(issueKey string) (*Ticket, error) {
	fields := []string{
		"id",
		"key",
		"summary",
		"description",
		"status",
		"project",
		"created",
		"updated",
		"resolutiondate",
		"resolution",
		"assignee",
		"reporter",
		"creator",
		"priority",
		"labels",
		"components",
		"fixVersions",
		"parent",
		"subtasks",
		"issuelinks",
		"attachment",
		"comment",
	}

	ticket, err := c.GetTicketByKey(issueKey, fields)
	if err != nil {
		return nil, err
	}

	developmentInfo := c.GetDevelopmentInfo(ticket.ID)
	if len(developmentInfo) > 0 {
		if ticket.Fields == nil {
			ticket.Fields = map[string]interface{}{}
		}
		ticket.Fields[FieldDevelopmentInfo] = developmentInfo
	}

	return ticket, nil
}

// AddComment posts a Markdown comment to a Jira issue using Atlassian Document
// Format, which Jira Cloud API v3 requires for comment bodies.
func (c *Client) AddComment(issueKey, commentText string) (*Comment, error) {
	payload := map[string]interface{}{
		"body": MarkdownADF(commentText),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal comment payload: %w", err)
	}

	reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", c.baseURL, url.PathEscape(issueKey))
	resp, err := c.makeRequestWithBody("POST", reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to add Jira comment: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var comment Comment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode comment response: %w", err)
	}

	return &comment, nil
}

// GetCurrentUser retrieves current user information
func (c *Client) GetCurrentUser() (*User, error) {
	reqURL := fmt.Sprintf("%s/rest/api/3/myself", c.baseURL)

	resp, err := c.makeRequest(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &user, nil
}

// GetDevelopmentInfo retrieves Jira development-panel links such as GitHub pull
// requests, branches, commits, and repositories. Jira exposes this through the
// dev-status endpoint, which is not available in every site/configuration, so
// callers should treat an empty result as "no readable development data".
func (c *Client) GetDevelopmentInfo(issueID string) []interface{} {
	if issueID == "" {
		return nil
	}

	dataTypes := []string{"pullrequest", "branch", "repository", "commit"}
	applicationTypes := []string{"GitHub", "github", "githube"}
	results := make([]interface{}, 0, len(dataTypes))

	for _, dataType := range dataTypes {
		for _, applicationType := range applicationTypes {
			params := url.Values{}
			params.Set("issueId", issueID)
			params.Set("applicationType", applicationType)
			params.Set("dataType", dataType)

			reqURL := fmt.Sprintf("%s/rest/dev-status/latest/issue/detail?%s", c.baseURL, params.Encode())
			resp, err := c.makeRequest(reqURL)
			if err != nil {
				continue
			}

			var payload map[string]interface{}
			decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
			_ = resp.Body.Close()
			if decodeErr != nil || !hasDevelopmentPayload(payload) {
				continue
			}

			payload["dataType"] = dataType
			payload["applicationType"] = applicationType
			results = append(results, payload)
			break
		}
	}

	return results
}

func hasDevelopmentPayload(payload map[string]interface{}) bool {
	if len(payload) == 0 {
		return false
	}

	if details, ok := payload["detail"].([]interface{}); ok && len(details) > 0 {
		return true
	}
	if repositories, ok := payload["repositories"].([]interface{}); ok && len(repositories) > 0 {
		return true
	}
	if branches, ok := payload["branches"].([]interface{}); ok && len(branches) > 0 {
		return true
	}
	if pullRequests, ok := payload["pullRequests"].([]interface{}); ok && len(pullRequests) > 0 {
		return true
	}
	if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
		return true
	}

	return false
}

// makeRequest makes an authenticated HTTP request to Jira API
func (c *Client) makeRequest(url string) (*http.Response, error) {
	return c.makeRequestWithBody(http.MethodGet, url, nil)
}

func (c *Client) makeRequestWithBody(method, url string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Basic authentication: base64(username:password)
	auth := base64.StdEncoding.EncodeToString([]byte(
		fmt.Sprintf("%s:%s", c.credentials.Username, c.credentials.Password),
	))

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", auth))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		// Upstream error bodies can contain private issue content or authentication data.
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return resp, nil
}

// joinStrings joins a slice of strings with a delimiter
func joinStrings(slice []string, delimiter string) string {
	if len(slice) == 0 {
		return ""
	}
	result := slice[0]
	for i := 1; i < len(slice); i++ {
		result += delimiter + slice[i]
	}
	return result
}
