package arkevia

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"ssk/internal/models"
	"ssk/internal/utils/totp"
)

const (
	baseURL             = "https://www.myarkevia.com"
	keycloakBaseURL     = "https://srh-iamb2c.cegedim-srh.net/auth/realms/Arkevia"
	keycloakClientID    = "arkevia-refonte-front"
	keycloakRedirectURI = "https://www.myarkevia.com/"
)

type Authenticator struct {
	client        *http.Client
	cookieManager *CookieManager
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func NewAuthenticator(client *http.Client, cookieManager *CookieManager) *Authenticator {
	return &Authenticator{
		client:        client,
		cookieManager: cookieManager,
	}
}

func (a *Authenticator) Authenticate(credentials *models.Credentials) error {
	// 1. Try Keycloak OIDC authentication flow with TOTP support
	err := a.authenticateKeycloak(credentials)
	if err == nil {
		return nil
	}

	// 2. Fallback to legacy Bonita portal authentication if Keycloak flow fails
	return a.authenticateLegacy(credentials)
}

func (a *Authenticator) RefreshToken(refreshToken string) (*TokenResponse, error) {
	tokenURL := keycloakBaseURL + "/protocol/openid-connect/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", keycloakClientID)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if operationErr := json.Unmarshal(body, &tokenResp); operationErr != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", operationErr)
	}

	return &tokenResp, nil
}

func (a *Authenticator) authenticateKeycloak(credentials *models.Credentials) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Client with custom redirect handler to capture authorization code
	var authCode string
	kcClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.HasPrefix(req.URL.String(), keycloakRedirectURI) {
				code := req.URL.Query().Get("code")
				if code != "" {
					authCode = code
					return http.ErrUseLastResponse
				}
			}
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Step 1: GET initial Keycloak auth page
	authInitURL := fmt.Sprintf("%s/protocol/openid-connect/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid",
		keycloakBaseURL, keycloakClientID, url.QueryEscape(keycloakRedirectURI))

	resp, err := kcClient.Get(authInitURL)
	if err != nil {
		return fmt.Errorf("failed to initialize Keycloak auth: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Keycloak auth page: %w", err)
	}
	htmlContent := string(bodyBytes)

	// Extract login form action
	formAction := extractFormAction(htmlContent)
	if formAction == "" {
		return fmt.Errorf("could not find login form action in Keycloak page")
	}

	// Step 2: POST Username and Password
	loginData := url.Values{}
	loginData.Set("username", credentials.Username)
	loginData.Set("password", credentials.Password)

	req, err := http.NewRequest("POST", formAction, strings.NewReader(loginData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = kcClient.Do(req)
	if err != nil {
		return fmt.Errorf("login POST request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check if redirected directly with auth code
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		loc := resp.Header.Get("Location")
		if locURL, operationErr := url.Parse(loc); operationErr == nil {
			if code := locURL.Query().Get("code"); code != "" {
				authCode = code
			}
		}
	}

	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %w", err)
	}
	htmlContent = string(bodyBytes)

	// Step 3: Handle 2FA / OTP challenge if code not yet obtained
	if authCode == "" {
		otpFormAction := extractFormAction(htmlContent)
		if otpFormAction == "" {
			return fmt.Errorf("login failed or invalid credentials (no code or 2FA form returned)")
		}

		var otpCode string
		if credentials.OTPSecret != "" {
			// Generate TOTP code automatically using stored secret
			code, otpErr := totp.GenerateCode(credentials.OTPSecret, time.Now())
			if otpErr != nil {
				return fmt.Errorf("failed to generate TOTP code: %w", otpErr)
			}
			otpCode = code
			fmt.Printf("🔑 Automatically generated 2FA code (TOTP): %s\n", otpCode)
		} else {
			// Prompt interactively if no TOTP secret stored
			fmt.Print("🔑 Enter 2FA code (SMS / Email / App): ")
			reader := bufio.NewReader(os.Stdin)
			code, _ := reader.ReadString('\n')
			otpCode = strings.TrimSpace(code)
			if otpCode == "" {
				return fmt.Errorf("2FA code cannot be empty")
			}
		}

		otpData := url.Values{}
		otpData.Set("otp", otpCode)
		otpData.Set("otpCode", otpCode)

		req, err = http.NewRequest("POST", otpFormAction, strings.NewReader(otpData.Encode()))
		if err != nil {
			return fmt.Errorf("failed to create 2FA request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err = kcClient.Do(req)
		if err != nil {
			return fmt.Errorf("2FA POST request failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
			loc := resp.Header.Get("Location")
			if locURL, operationErr := url.Parse(loc); operationErr == nil {
				if code := locURL.Query().Get("code"); code != "" {
					authCode = code
				}
			}
		}

		if authCode == "" {
			return fmt.Errorf("2FA verification failed or invalid OTP code")
		}
	}

	// Step 4: Exchange Authorization Code for Tokens
	tokenURL := keycloakBaseURL + "/protocol/openid-connect/token"
	tokenData := url.Values{}
	tokenData.Set("grant_type", "authorization_code")
	tokenData.Set("client_id", keycloakClientID)
	tokenData.Set("code", authCode)
	tokenData.Set("redirect_uri", keycloakRedirectURI)

	req, err = http.NewRequest("POST", tokenURL, strings.NewReader(tokenData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = a.client.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if operationErr := json.Unmarshal(bodyBytes, &tokenResp); operationErr != nil {
		return fmt.Errorf("failed to parse token exchange response: %w", operationErr)
	}

	var setCookieHeaders []string

	// Step 5: Call /portal/notifyLogin.htm to exchange Keycloak token for Arkevia portal session
	notifyURL := baseURL + "/portal/notifyLogin.htm"
	notifyReq, notifyErr := http.NewRequest("POST", notifyURL, bytes.NewBufferString("{}"))
	if notifyErr == nil {
		notifyReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		notifyReq.Header.Set("Content-Type", "application/json")
		notifyResp, notifyErr := a.client.Do(notifyReq)
		if notifyErr == nil {
			defer func() { _ = notifyResp.Body.Close() }()
			setCookieHeaders = append(setCookieHeaders, notifyResp.Header.Values("Set-Cookie")...)
		}
	}

	// Step 6: Save cookies and tokens
	u, _ := url.Parse(keycloakBaseURL)
	for _, cookie := range jar.Cookies(u) {
		setCookieHeaders = append(setCookieHeaders, cookie.String())
	}
	if tokenResp.AccessToken != "" {
		setCookieHeaders = append(setCookieHeaders, fmt.Sprintf("access_token=%s", tokenResp.AccessToken))
	}
	if tokenResp.RefreshToken != "" {
		setCookieHeaders = append(setCookieHeaders, fmt.Sprintf("refresh_token=%s", tokenResp.RefreshToken))
	}

	if len(setCookieHeaders) > 0 {
		if operationErr := a.cookieManager.SaveCookies(setCookieHeaders); operationErr != nil {
			return fmt.Errorf("failed to save tokens/cookies: %w", operationErr)
		}
	}

	return nil
}

func (a *Authenticator) authenticateLegacy(credentials *models.Credentials) error {
	authData := url.Values{}
	authData.Set("j_username", credentials.Username)
	authData.Set("j_password", credentials.Password)
	authData.Set("domain", "arkevia")

	authURL := baseURL + "/j_bonitaportal_check"
	req, err := http.NewRequest("POST", authURL, strings.NewReader(authData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cookieString, err := a.cookieManager.LoadCookies()
	if err != nil {
		return fmt.Errorf("failed to load cookies: %w", err)
	}
	if cookieString != "" {
		req.Header.Set("Cookie", cookieString)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("authentication request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	setCookieHeaders := resp.Header.Values("Set-Cookie")
	if len(setCookieHeaders) > 0 {
		if operationErr := a.cookieManager.SaveCookies(setCookieHeaders); operationErr != nil {
			return fmt.Errorf("failed to save cookies: %w", operationErr)
		}
	}

	return nil
}

func extractFormAction(htmlContent string) string {
	re := regexp.MustCompile(`(?i)<form[^>]+action=["']([^"']+)["']`)
	matches := re.FindStringSubmatch(htmlContent)
	if len(matches) >= 2 {
		return html.UnescapeString(matches[1])
	}
	return ""
}
