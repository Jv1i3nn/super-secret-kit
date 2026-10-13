package horoquartz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Client gère les requêtes HTTP vers l'API Horoquartz
type Client struct {
	baseURL    string
	httpClient *http.Client
	session    *Session
}

// NewClient crée un nouveau client HTTP
func NewClient(baseURL string, session *Session) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		session: session,
	}
}

// Get effectue une requête GET avec authentification
func (c *Client) Get(endpoint string, params map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Ajouter les query parameters
	if len(params) > 0 {
		q := req.URL.Query()
		for key, value := range params {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Ajouter les cookies et headers de session Horoquartz
	req.Header.Set("Cookie", fmt.Sprintf("JSESSIONID=%s", c.session.JSESSIONID))
	if c.session.HQToken != "" {
		req.Header.Set("x-hq-token", c.session.HQToken)
	}
	roleID := c.session.RoleID
	if r := params["roleId"]; r != "" {
		roleID = r
	}
	if roleID != "" {
		req.Header.Set("x-hq-roleid", roleID)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("%s/ux/home", c.baseURL))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// Post effectue une requête POST avec authentification
func (c *Client) Post(endpoint string, params map[string]string, body interface{}) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if len(params) > 0 {
		q := req.URL.Query()
		for key, value := range params {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	req.Header.Set("Cookie", fmt.Sprintf("JSESSIONID=%s", c.session.JSESSIONID))
	if c.session.HQToken != "" {
		req.Header.Set("x-hq-token", c.session.HQToken)
	}
	if c.session.RoleID != "" {
		req.Header.Set("x-hq-roleid", c.session.RoleID)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("%s/ux/request", c.baseURL))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// GetReasons récupère la liste des motifs autorisés pour les demandes
func (c *Client) GetReasons() (*ReasonsResponse, error) {
	endpoint := "/hq-web/api/v2/self/settings/reasons"
	params := map[string]string{
		"roleId": c.session.RoleID,
	}

	resp, err := c.Get(endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if strings.Contains(string(bodyBytes), "HQInvalidTokenException") || strings.Contains(string(bodyBytes), "Invalid token") {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed to get reasons (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	if strings.Contains(string(bodyBytes), "HQInvalidTokenException") || strings.Contains(string(bodyBytes), "Invalid token") {
		return nil, ErrInvalidToken
	}

	var reasonsResp ReasonsResponse
	if err := json.Unmarshal(bodyBytes, &reasonsResp); err != nil {
		return nil, fmt.Errorf("failed to decode reasons response: %w", err)
	}

	return &reasonsResp, nil
}

// CreateRequest crée une nouvelle demande (télétravail, etc.)
func (c *Client) CreateRequest(payload CreateRequestPayload) (*CreateRequestResponse, error) {
	endpoint := "/hq-web/api/v1/self/requests"
	params := map[string]string{
		"roleId": c.session.RoleID,
	}

	if payload.RollNumber == "" {
		payload.RollNumber = c.session.RollNumber
	}

	resp, err := c.Post(endpoint, params, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if viper.GetBool("verbose") {
		fmt.Printf("🔍 POST %s => Status %d: %s\n", endpoint, resp.StatusCode, string(bodyBytes))
	}

	if strings.Contains(string(bodyBytes), "HQInvalidTokenException") || strings.Contains(string(bodyBytes), "Invalid token") {
		return nil, ErrInvalidToken
	}

	var createResp CreateRequestResponse
	if err := json.Unmarshal(bodyBytes, &createResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(bodyBytes))
	}

	return &createResp, nil
}

// GetCounters récupère les compteurs à une date donnée
func (c *Client) GetCounters(date time.Time) (*CountersResponse, error) {
	dates := []time.Time{date}
	today := time.Now()
	if date.Format("2006-01-02") != today.Format("2006-01-02") {
		dates = append(dates, today)
	}

	// Un employé peut avoir plusieurs rôles : le rôle capturé au login n'a pas
	// forcément accès aux compteurs, on retente alors avec le rôle par défaut.
	roles := []string{c.session.RoleID}
	if defaultRole := c.session.RollNumber + "/0002"; c.session.RollNumber != "" && defaultRole != c.session.RoleID {
		roles = append(roles, defaultRole)
	}

	var failures []string
	hasInvalidToken := false
	for _, role := range roles {
		counters, roleFailures, invalidToken := c.fetchCounters(role, dates)
		if len(counters) > 0 {
			res := &CountersResponse{Status: "SUCCESS"}
			res.Content.Counters = counters
			return res, nil
		}
		failures = append(failures, roleFailures...)
		hasInvalidToken = hasInvalidToken || invalidToken
	}

	if hasInvalidToken {
		return nil, ErrInvalidToken
	}
	if len(failures) > 0 {
		return nil, fmt.Errorf("horoquartz a refusé l'accès aux compteurs: %s", failures[0])
	}

	return &CountersResponse{Status: "SUCCESS"}, nil
}

// fetchCounters interroge les compteurs pour un rôle et renvoie les erreurs de l'API
func (c *Client) fetchCounters(roleID string, dates []time.Time) ([]Counter, []string, bool) {
	endpoint := "/hq-web/api/v1/self/counters"

	// Seules les fonctions autorisées pour l'employé (WA12, WD05, WD01, WD11, WD34, WD36)
	functions := []string{"WA12", "WD05", "WD01", "WD11", "WD34", "WD36"}

	allCounters := []Counter{}
	seenCodes := make(map[string]bool)
	var failures []string
	hasInvalidToken := false

	for _, d := range dates {
		for _, fn := range functions {
			params := map[string]string{
				"roleId":     roleID,
				"rollNumber": c.session.RollNumber,
				"start":      d.Format("2006-01-02"),
				"function":   fn,
			}

			resp, err := c.Get(endpoint, params)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if viper.GetBool("verbose") {
				fmt.Printf("🔍 GET %s (role=%s fn=%q start=%s) => Status %d: %s\n", endpoint, roleID, fn, d.Format("2006-01-02"), resp.StatusCode, string(bodyBytes))
			}

			bodyStr := string(bodyBytes)
			if strings.Contains(bodyStr, "HQInvalidTokenException") || strings.Contains(bodyStr, "Invalid token") {
				hasInvalidToken = true
			}

			if resp.StatusCode != http.StatusOK {
				failures = append(failures, fmt.Sprintf("status %d", resp.StatusCode))
				continue
			}

			var countersResp CountersResponse
			if err := json.Unmarshal(bodyBytes, &countersResp); err != nil {
				failures = append(failures, fmt.Sprintf("réponse illisible: %v", err))
				continue
			}
			if countersResp.Status != "SUCCESS" {
				for _, m := range countersResp.Messages {
					failures = append(failures, fmt.Sprintf("%s %s", m.Code, m.Reason))
				}
				continue
			}

			// 1. employeeCounters, 2. employeeDailyCounters, 3. counters (legacy/fallback)
			var counters []Counter
			for _, rc := range append(countersResp.Content.EmployeeCounters, countersResp.Content.EmployeeDailyCounters...) {
				counters = append(counters, rc.ToCounter())
			}
			for _, counter := range append(counters, countersResp.Content.Counters...) {
				if counter.Code != "" && !seenCodes[counter.Code] {
					seenCodes[counter.Code] = true
					allCounters = append(allCounters, counter)
				}
			}
		}
		if len(allCounters) > 0 {
			break
		}
	}

	return allCounters, failures, hasInvalidToken
}
