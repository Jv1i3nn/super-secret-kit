package horoquartz

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ssk/internal/services"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/spf13/viper"
)

const (
	sessionExpirationDays = 90
	authTimeout           = 5 * time.Minute
)

// Authenticator gère l'authentification via navigateur
type Authenticator struct {
	baseURL           string
	encryptionService services.EncryptionService
	sessionPath       string
	session           *Session
}

// NewAuthenticator crée un nouvel authenticator
func NewAuthenticator(baseURL string, encryptionService services.EncryptionService, sessionPath string) *Authenticator {
	return &Authenticator{
		baseURL:           baseURL,
		encryptionService: encryptionService,
		sessionPath:       sessionPath,
	}
}

// Authenticate ouvre un navigateur pour que l'utilisateur se connecte
func (a *Authenticator) Authenticate() error {
	fmt.Println("🔐 Authentification Horoquartz via navigateur...")
	fmt.Println("🌐 Un navigateur va s'ouvrir - veuillez vous connecter avec vos identifiants Azure")

	// Créer un contexte avec timeout
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()

	// Options pour chromedp (navigateur visible pour l'utilisateur)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Variables pour récupérer les données
	var jsessionid, hqToken, roleID, rollNumber, userAlias, displayName string

	// Écouter les événements réseau Chrome pour intercepter automatiquement roleId, rollNumber et x-hq-token
	chromedp.ListenTarget(browserCtx, func(ev interface{}) {
		event, ok := ev.(*network.EventRequestWillBeSent)
		if !ok {
			return
		}
		reqURL := event.Request.URL
		headers := event.Request.Headers

		for k, v := range headers {
			lk := strings.ToLower(k)
			if lk == "x-hq-token" {
				if strVal, ok := v.(string); ok && strVal != "" {
					hqToken = strVal
				}
			}
			if lk == "x-hq-roleid" {
				if strVal, ok := v.(string); ok && strVal != "" {
					roleID = strVal
				}
			}
		}

		if parsed, err := url.Parse(reqURL); err == nil {
			if rID := parsed.Query().Get("roleId"); rID != "" {
				roleID = rID
				if parts := strings.Split(rID, "/"); len(parts) > 0 && parts[0] != "" {
					rollNumber = parts[0]
				}
			}
			if rNum := parsed.Query().Get("rollNumber"); rNum != "" {
				rollNumber = rNum
			}
			if alias := parsed.Query().Get("alias"); alias != "" {
				userAlias = alias
			}
		}
	})

	// Naviguer vers le site et attendre l'authentification
	err := chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.Navigate(a.baseURL),

		// Attendre que l'utilisateur soit authentifié (cookie JSESSIONID présent)
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("⏳ En attente de votre connexion...")
			fmt.Println("   → Connectez-vous dans le navigateur avec vos identifiants Azure")
			fmt.Println("   → Le programme continuera automatiquement une fois connecté")

			// Attendre que le cookie JSESSIONID existe et qu'au moins une requête API SSO soit émise
			cookieFoundAt := time.Time{}
			for {
				select {
				case <-ctx.Done():
					return fmt.Errorf("timeout: veuillez vous reconnecter")
				default:
					if jsessionid == "" {
						var cookies []*network.Cookie
						err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
							var err error
							cookies, err = network.GetCookies().Do(ctx)
							return err
						}))
						if err == nil {
							for _, cookie := range cookies {
								if cookie.Name == "JSESSIONID" && cookie.Value != "" {
									jsessionid = cookie.Value
									cookieFoundAt = time.Now()
									fmt.Println("✅ Authentification détectée !")
									break
								}
							}
						}
					}

					// Si le cookie est trouvé et qu'on a capturé le roleID (ou attendu 3 sec supplémentaires pour les requêtes XHR)
					if jsessionid != "" {
						if roleID != "" || time.Since(cookieFoundAt) > 3*time.Second {
							return nil
						}
					}

					time.Sleep(500 * time.Millisecond)
				}
			}
		}),

		// Tenter également d'évaluer dans la page HTML/DOM si non trouvé par réseau
		chromedp.Evaluate(`
			(function() {
				let roleId = '';
				let rollNumber = '';
				let userAlias = '';
				let displayName = '';

				for (let i = 0; i < localStorage.length; i++) {
					let key = localStorage.key(i);
					let val = localStorage.getItem(key);
					if (val && val.includes('/')) {
						let match = val.match(/\b\d{4,6}\/\d{3,4}\b/);
						if (match) roleId = match[0];
					}
				}
				return JSON.stringify({ roleId: roleId, rollNumber: rollNumber, userAlias: userAlias, displayName: displayName });
			})()
		`, &displayName),
	)

	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if jsessionid == "" {
		return fmt.Errorf("JSESSIONID not found in cookies")
	}

	// Normaliser roleID et rollNumber si l'un des deux a été capturé
	if rollNumber != "" && roleID == "" {
		roleID = fmt.Sprintf("%s/0002", rollNumber)
	} else if roleID != "" && rollNumber == "" {
		parts := strings.Split(roleID, "/")
		rollNumber = parts[0]
	}

	// Si les identifiants n'ont pas pu être capturés automatiquement, demander le matricule
	if roleID == "" || rollNumber == "" {
		fmt.Print("👤 Entrez votre numéro de matricule Horoquartz (ex: XXXXXX) : ")
		reader := bufio.NewReader(os.Stdin)
		inputMatricule, _ := reader.ReadString('\n')
		inputMatricule = strings.TrimSpace(inputMatricule)

		if inputMatricule == "" {
			return fmt.Errorf("impossible de récupérer roleID et rollNumber - le matricule est obligatoire")
		}

		rollNumber = inputMatricule
		roleID = fmt.Sprintf("%s/0002", inputMatricule)
	}

	fmt.Printf("✅ Session Horoquartz initialisée pour le matricule %s (roleID: %s)\n", rollNumber, roleID)

	// Créer la session
	a.session = &Session{
		JSESSIONID:  jsessionid,
		HQToken:     hqToken,
		RoleID:      roleID,
		RollNumber:  rollNumber,
		UserAlias:   userAlias,
		DisplayName: displayName,
		ExpiresAt:   time.Now().Add(sessionExpirationDays * 24 * time.Hour),
		CreatedAt:   time.Now(),
	}

	// Sauvegarder la session
	if err := a.SaveSession(); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	fmt.Printf("✅ Authentification réussie (valide pour %d jours)\n", sessionExpirationDays)
	return nil
}

// IsSessionValid vérifie si la session est valide auprès de l’API
func (a *Authenticator) IsSessionValid() (bool, error) {
	if a.session == nil || !a.session.IsValid() {
		if viper.GetBool("verbose") {
			fmt.Printf("🔍 Session locale invalide ou expirée (expires: %s)\n", a.session.ExpiresAt.Format("2006-01-02 15:04:05"))
		}
		return false, nil
	}

	// Vérifier avec l'API en incluant les en-têtes et le roleId si disponible
	checkURL := fmt.Sprintf("%s/hq-web/api/v2/self/settings/reasons", a.baseURL)
	if a.session.RoleID != "" {
		checkURL = fmt.Sprintf("%s/hq-web/api/v2/self/settings/reasons?roleId=%s", a.baseURL, url.QueryEscape(a.session.RoleID))
	}

	if viper.GetBool("verbose") {
		fmt.Printf("🔍 Vérification de la session auprès de l'API: %s\n", checkURL)
	}

	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		if viper.GetBool("verbose") {
			fmt.Printf("❌ Erreur création requête: %v\n", err)
		}
		return false, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("JSESSIONID=%s", a.session.JSESSIONID))
	if a.session.HQToken != "" {
		req.Header.Set("x-hq-token", a.session.HQToken)
	}
	if a.session.RoleID != "" {
		req.Header.Set("x-hq-roleid", a.session.RoleID)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if viper.GetBool("verbose") {
			fmt.Printf("❌ Erreur appel API: %v\n", err)
		}
		return false, err
	}
	defer resp.Body.Close()

	if viper.GetBool("verbose") {
		fmt.Printf("🔍 Status code: %d\n", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if viper.GetBool("verbose") {
			fmt.Println("❌ Session non autorisée (401/403)")
		}
		return false, nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, "HQInvalidTokenException") || strings.Contains(bodyStr, "Invalid token") {
		if viper.GetBool("verbose") {
			fmt.Println("❌ Jeton Horoquartz invalide ou expiré (HQInvalidTokenException)")
		}
		return false, nil
	}

	var baseResp struct {
		Status   string       `json:"status"`
		Messages []APIMessage `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &baseResp); err == nil {
		if viper.GetBool("verbose") {
			fmt.Printf("🔍 API Response - Status: %s\n", baseResp.Status)
		}
		if baseResp.Status == "FAILED" {
			return false, nil
		}
		return baseResp.Status == "SUCCESS", nil
	}

	return resp.StatusCode == http.StatusOK, nil
}

// LoadSession charge la session depuis le fichier chiffré
func (a *Authenticator) LoadSession() error {
	// Vérifier que le fichier existe
	if _, err := os.Stat(a.sessionPath); os.IsNotExist(err) {
		return fmt.Errorf("session file not found")
	}

	// Lire le fichier chiffré
	encrypted, err := os.ReadFile(a.sessionPath)
	if err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	// Déchiffrer
	decrypted, err := a.encryptionService.Decrypt(string(encrypted))
	if err != nil {
		return fmt.Errorf("failed to decrypt session: %w", err)
	}

	// Désérialiser
	var session Session
	if err := json.Unmarshal([]byte(decrypted), &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	a.session = &session
	return nil
}

// SaveSession sauvegarde la session dans un fichier chiffré
func (a *Authenticator) SaveSession() error {
	if a.session == nil {
		return fmt.Errorf("no session to save")
	}

	// Créer le répertoire parent si nécessaire
	dir := filepath.Dir(a.sessionPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Sérialiser la session
	data, err := json.Marshal(a.session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Chiffrer
	encrypted, err := a.encryptionService.Encrypt(string(data))
	if err != nil {
		return fmt.Errorf("failed to encrypt session: %w", err)
	}

	// Écrire dans le fichier
	if err := os.WriteFile(a.sessionPath, []byte(encrypted), 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// ClearSession supprime la session stockée
func (a *Authenticator) ClearSession() error {
	a.session = nil

	if _, err := os.Stat(a.sessionPath); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(a.sessionPath)
}

// GetSession retourne la session actuelle
func (a *Authenticator) GetSession() *Session {
	return a.session
}
