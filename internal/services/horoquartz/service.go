package horoquartz

import (
	"fmt"
	"strings"
	"time"

	"ssk/internal/services"
)

// Service gère les interactions avec Horoquartz
type Service struct {
	baseURL           string
	authenticator     *Authenticator
	client            *Client
	encryptionService services.EncryptionService
	sessionPath       string
}

// NewService crée un nouveau service Horoquartz
func NewService(baseURL string, encryptionService services.EncryptionService, sessionPath string) *Service {
	return &Service{
		baseURL:           baseURL,
		encryptionService: encryptionService,
		sessionPath:       sessionPath,
	}
}

// EnsureAuthenticated s'assure qu'une session valide existe
func (s *Service) EnsureAuthenticated(forceReauth bool) error {
	// Si forceReauth, supprimer la session existante
	if forceReauth {
		if s.authenticator != nil {
			if err := s.authenticator.ClearSession(); err != nil {
				return fmt.Errorf("failed to clear Horoquartz session: %w", err)
			}
		}
		s.authenticator = nil
		s.client = nil
	}

	// Créer l'authenticator si nécessaire
	if s.authenticator == nil {
		s.authenticator = NewAuthenticator(s.baseURL, s.encryptionService, s.sessionPath)
	}

	// Essayer de charger une session existante
	if !forceReauth {
		if err := s.authenticator.LoadSession(); err == nil {
			// Vérifier si la session est valide
			valid, err := s.authenticator.IsSessionValid()
			if err == nil && valid {
				fmt.Println("🔐 Session Horoquartz valide trouvée")
				s.client = NewClient(s.baseURL, s.authenticator.GetSession())
				return nil
			}
			fmt.Println("⚠️  Session expirée, ré-authentification nécessaire")
		}
	}

	// Authentification nécessaire
	if err := s.authenticator.Authenticate(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Créer le client avec la nouvelle session
	s.client = NewClient(s.baseURL, s.authenticator.GetSession())

	return nil
}

// GetCounters récupère les compteurs à une date donnée
func (s *Service) GetCounters(date time.Time) ([]Counter, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	resp, err := s.client.GetCounters(date)
	if err != nil {
		if IsInvalidTokenErr(err) {
			fmt.Println("⚠️  Jeton Horoquartz expiré ou invalide, ré-authentification automatique...")
			if reauthErr := s.EnsureAuthenticated(true); reauthErr != nil {
				return nil, fmt.Errorf("re-authentication failed: %w", reauthErr)
			}
			resp, err = s.client.GetCounters(date)
			if err != nil {
				return nil, err
			}
			return resp.Content.Counters, nil
		}
		return nil, err
	}

	return resp.Content.Counters, nil
}

// AddTelework soumet une demande de télétravail (journée, demi-journée ou période)
func (s *Service) AddTelework(startDate, endDate time.Time, startUnit, endUnit string) (*CreateRequestResponse, error) {
	return s.AddRequest("telework", startDate, endDate, startUnit, endUnit)
}

// AddRequest soumet une demande d'absence ou télétravail (CP, RTT, Récup, Télétravail)
func (s *Service) AddRequest(reqType string, startDate, endDate time.Time, startUnit, endUnit string) (*CreateRequestResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	functionCode := "WD05"
	defaultReason := "01.CP"

	switch strings.ToLower(reqType) {
	case "telework", "tt":
		functionCode = "WD01"
		defaultReason = "01.TWHW"
	case "rtt":
		functionCode = "WD05"
		defaultReason = "01.RTT"
	case "cp", "conge", "conges":
		functionCode = "WD05"
		defaultReason = "01.CP"
	case "recup", "recovery":
		functionCode = "WD11"
		defaultReason = "01.REC"
	}

	reasonCode := defaultReason
	if reasonsResp, err := s.client.GetReasons(); err == nil {
		for _, fn := range reasonsResp.Content.Functions {
			if fn.Function == functionCode {
				for _, r := range fn.Reasons {
					codeLower := strings.ToLower(r.Code)
					titleLower := strings.ToLower(r.Title)

					switch strings.ToLower(reqType) {
					case "telework", "tt":
						if r.Code == "01.TWHW" || (!strings.Contains(titleLower, "dérogatoire") && !strings.Contains(codeLower, "twd")) {
							reasonCode = r.Code
						}
					case "rtt":
						if r.Code == "01.RTT" || strings.Contains(titleLower, "rtt") || strings.Contains(codeLower, "rtt") {
							reasonCode = r.Code
						}
					case "cp", "conge", "conges":
						if r.Code == "01.CP" || strings.Contains(titleLower, "congé payé") || strings.Contains(codeLower, "cp") {
							reasonCode = r.Code
						}
					case "recup", "recovery":
						if strings.Contains(titleLower, "récup") || strings.Contains(titleLower, "recup") || strings.Contains(codeLower, "rec") {
							reasonCode = r.Code
						}
					}
				}
			}
		}
	}

	session := s.GetSession()
	rollNum := ""
	if session != nil {
		rollNum = session.RollNumber
	}

	payload := CreateRequestPayload{
		Function:   functionCode,
		Reason:     reasonCode,
		Start:      startDate.Format("2006-01-02"),
		End:        endDate.Format("2006-01-02"),
		StartUnit:  startUnit,
		EndUnit:    endUnit,
		RollNumber: rollNum,
	}

	res, err := s.client.CreateRequest(payload)
	if err != nil && IsInvalidTokenErr(err) {
		fmt.Println("⚠️  Jeton Horoquartz expiré ou invalide, ré-authentification automatique...")
		if reauthErr := s.EnsureAuthenticated(true); reauthErr != nil {
			return nil, fmt.Errorf("re-authentication failed: %w", reauthErr)
		}
		return s.client.CreateRequest(payload)
	}

	return res, err
}

// GetYearEndCounters récupère les compteurs au 31/12 de l'année donnée
func (s *Service) GetYearEndCounters(year int) ([]Counter, error) {
	date := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	return s.GetCounters(date)
}

// GetCurrentYearEndCounters récupère les compteurs au 31/12 de l'année en cours
func (s *Service) GetCurrentYearEndCounters() ([]Counter, error) {
	currentYear := time.Now().Year()
	return s.GetYearEndCounters(currentYear)
}

// GetSession retourne la session actuelle (si authentifiée)
func (s *Service) GetSession() *Session {
	if s.authenticator == nil {
		return nil
	}
	return s.authenticator.GetSession()
}

// ClearSession supprime la session stockée
func (s *Service) ClearSession() error {
	if s.authenticator == nil {
		s.authenticator = NewAuthenticator(s.baseURL, s.encryptionService, s.sessionPath)
	}
	return s.authenticator.ClearSession()
}
