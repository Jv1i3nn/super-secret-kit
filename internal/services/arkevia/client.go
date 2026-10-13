package arkevia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"ssk/internal/models"
)

type Client struct {
	httpClient    *http.Client
	authenticator *Authenticator
	cookieManager *CookieManager
	credentials   *models.Credentials
}

func NewClient(credentials *models.Credentials, tokenDir string) *Client {
	httpClient := &http.Client{}
	cookieManager := NewCookieManager(tokenDir)
	authenticator := NewAuthenticator(httpClient, cookieManager)

	return &Client{
		httpClient:    httpClient,
		authenticator: authenticator,
		cookieManager: cookieManager,
		credentials:   credentials,
	}
}

func (c *Client) GetDocumentList(totalRequested int) ([]Document, error) {
	searchParams := map[string]interface{}{
		"Document.file_name":             "",
		"fromDate_Document.archive_date": "",
		"toDate_Document.archive_date":   "",
		"ac_Document.isafe_sendercode":   "",
		"ac_Document.isafe_sender":       "",
		"ac_Document.isafe_producer":     "",
	}

	searchParamsJSON, err := json.Marshal(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search params: %w", err)
	}

	formData := url.Values{}
	formData.Set("_search", "false")
	formData.Set("rows", fmt.Sprintf("%d", totalRequested))
	formData.Set("page", "1")
	formData.Set("sord", "desc")
	formData.Set("searchParamsStr", string(searchParamsJSON))
	formData.Set("folder", "/MySafe/MyInbox/")
	formData.Set("acSearchActive", "false")

	resp, err := c.makeAuthenticatedRequest("POST", "/portal/getGedList.htm", formData.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return nil, fmt.Errorf("failed to get document list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var docListResp DocumentListResponse
	if err := json.Unmarshal(body, &docListResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var documents []Document
	for _, result := range docListResp.Result {
		if result.Filename != "" {
			doc := Document(result)
			documents = append(documents, doc)
		}
	}

	return documents, nil
}

func (c *Client) DownloadDocument(document Document) ([]byte, error) {
	downloadURL := fmt.Sprintf(
		"%s/portal/downloadResource.htm?type=application/pdf&screenId=kiss_ged_list&gedFileIdentifier=%s",
		baseURL,
		url.QueryEscape(document.ID),
	)

	resp, err := c.makeAuthenticatedRequest("GET", downloadURL, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to download document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read document content: %w", err)
	}

	return content, nil
}

func (c *Client) makeAuthenticatedRequest(method, path, body, contentType string) (*http.Response, error) {
	reqURL := path
	if !contains(path, baseURL) {
		reqURL = baseURL + path
	}

	var reqBody io.Reader
	if body != "" {
		reqBody = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	cookieString, err := c.cookieManager.LoadCookies()
	if err != nil {
		return nil, fmt.Errorf("failed to load cookies: %w", err)
	}
	if cookieString != "" {
		req.Header.Set("Cookie", cookieString)
	}

	accessToken, _ := c.cookieManager.GetToken("access_token")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	trimmedBody := strings.TrimSpace(string(bodyBytes))
	isUnauthorized := resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusFound ||
		strings.HasPrefix(trimmedBody, "<")

	if isUnauthorized {
		// Try refresh token first if available
		refreshed := false
		if refreshToken, _ := c.cookieManager.GetToken("refresh_token"); refreshToken != "" {
			tokenResp, refErr := c.authenticator.RefreshToken(refreshToken)
			if refErr == nil && tokenResp.AccessToken != "" {
				_ = c.cookieManager.SaveCookies([]string{
					fmt.Sprintf("access_token=%s", tokenResp.AccessToken),
					fmt.Sprintf("refresh_token=%s", tokenResp.RefreshToken),
				})
				refreshed = true
			}
		}

		if !refreshed {
			err = c.authenticator.Authenticate(c.credentials)
			if err != nil {
				return nil, fmt.Errorf("authentication failed: %w", err)
			}
		}

		cookieString, err = c.cookieManager.LoadCookies()
		if err != nil {
			return nil, fmt.Errorf("failed to load cookies after auth: %w", err)
		}

		if body != "" {
			reqBody = bytes.NewBufferString(body)
		}
		req, err = http.NewRequest(method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to recreate request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Cookie", cookieString)

		accessToken, _ = c.cookieManager.GetToken("access_token")
		if accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("retry request failed: %w", err)
		}
		return resp, nil
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return resp, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || bytes.Contains([]byte(s), []byte(substr))))
}
