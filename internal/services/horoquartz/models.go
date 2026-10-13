package horoquartz

import (
	"fmt"
	"strings"
	"time"
)

// ErrInvalidToken est renvoyé lorsque le jeton Horoquartz est invalide ou expiré
var ErrInvalidToken = fmt.Errorf("invalid token: session Horoquartz expirée ou invalide")

// IsInvalidTokenErr vérifie si une erreur correspond à un jeton invalide
func IsInvalidTokenErr(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrInvalidToken || strings.Contains(err.Error(), "Invalid token") || strings.Contains(err.Error(), "HQInvalidTokenException")
}

// HQRawCounter représente la structure brute d'un compteur renvoyé par Horoquartz
type HQRawCounter struct {
	CounterName   string `json:"counterName"`
	CounterLabel  string `json:"counterLabel"`
	CounterDomain string `json:"counterDomain"`
	CounterValue  string `json:"counterValue"`
}

// ToCounter convertit un HQRawCounter en Counter
func (rc HQRawCounter) ToCounter() Counter {
	unit := "D"
	if rc.CounterDomain == "HEUR" || rc.CounterDomain == "H" {
		unit = "H"
	}
	return Counter{
		Code:  rc.CounterName,
		Label: rc.CounterLabel,
		Value: rc.CounterValue,
		Unit:  unit,
	}
}

// Counter représente un compteur Horoquartz
type Counter struct {
	Code  string `json:"code"`  // CNT_CP, IJRSGCONGE, etc.
	Label string `json:"label"` // Nom du compteur
	Value string `json:"value"` // Valeur numérique
	Unit  string `json:"unit"`  // D (jours), H (heures)
	Type  string `json:"type"`  // LEAVE, WORKED, SAVINGS
	Date  string `json:"date"`  // Date de référence
}

// CountersResponse représente la réponse API Horoquartz
type CountersResponse struct {
	Status   string       `json:"status"`
	Messages []APIMessage `json:"messages"`
	Content  struct {
		EmployeeCounters      []HQRawCounter `json:"employeeCounters"`
		EmployeeDailyCounters []HQRawCounter `json:"employeeDailyCounters"`
		Counters              []Counter      `json:"counters"`
	} `json:"content"`
}

// Session représente une session authentifiée (stockée chiffrée)
type Session struct {
	JSESSIONID  string    `json:"jsessionid"`
	HQToken     string    `json:"hqToken"`
	RoleID      string    `json:"roleId"`
	RollNumber  string    `json:"rollNumber"`
	UserAlias   string    `json:"userAlias"`
	DisplayName string    `json:"displayName"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

// IsValid vérifie si la session est encore valide
func (s *Session) IsValid() bool {
	if s.JSESSIONID == "" {
		return false
	}
	return time.Now().Before(s.ExpiresAt)
}

// CounterDisplay représente un compteur formaté pour l'affichage
type CounterDisplay struct {
	Name  string  // Nom lisible
	Value float64 // Valeur numérique
	Unit  string  // Unité formatée ("jours", "heures")
	Type  string  // Type de compteur
}

// AuthRequest représente une requête de login
type AuthRequest struct {
	Ticket string `json:"ticket"`
}

// AuthResponse représente la réponse de login
type AuthResponse struct {
	Status  string `json:"status"`
	Content struct {
		User struct {
			Alias       string `json:"alias"`
			RollNumber  string `json:"rollNumber"`
			RoleID      string `json:"roleId"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
		Session struct {
			SessionID string `json:"sessionId"`
			ExpiresIn int    `json:"expiresIn"`
		} `json:"session"`
	} `json:"content"`
}

// SettingsResponse représente la réponse de vérification de session
type SettingsResponse struct {
	Status  string `json:"status"`
	Content struct {
		Authenticated bool `json:"authenticated"`
	} `json:"content"`
}

// ReasonItem représente un motif d'absence/télétravail
type ReasonItem struct {
	Code  string   `json:"code"`
	Title string   `json:"title"`
	Units []string `json:"units"`
}

// ReasonsFunction représente un groupe de motifs par fonction (ex: WD01)
type ReasonsFunction struct {
	Function string       `json:"function"`
	Reasons  []ReasonItem `json:"reasons"`
}

// ReasonsResponse représente la réponse de GET /hq-web/api/v2/self/settings/reasons
type ReasonsResponse struct {
	Status  string `json:"status"`
	Content struct {
		Functions []ReasonsFunction `json:"functions"`
	} `json:"content"`
}

// CreateRequestPayload représente le corps de la requête POST /hq-web/api/v1/self/requests
type CreateRequestPayload struct {
	Function   string `json:"function"`   // "WD01" pour télétravail
	Reason     string `json:"reason"`     // ex: "01.TWHW"
	Start      string `json:"start"`      // "YYYY-MM-DD"
	End        string `json:"end"`        // "YYYY-MM-DD"
	StartUnit  string `json:"startUnit"`  // "D", "M", "A"
	EndUnit    string `json:"endUnit"`    // "D", "M", "A"
	Value      string `json:"value"`      // ""
	StartTime  string `json:"startTime"`  // ""
	EndTime    string `json:"endTime"`    // ""
	Confirm    bool   `json:"confirm"`    // false
	RollNumber string `json:"rollNumber"` // "XXXXX"
}

// APIMessage représente un message d'information ou d'erreur renvoyé par Horoquartz
type APIMessage struct {
	Level  string `json:"level"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// CreateRequestResponse représente la réponse de POST /hq-web/api/v1/self/requests
type CreateRequestResponse struct {
	Status   string       `json:"status"`
	Messages []APIMessage `json:"messages"`
}
