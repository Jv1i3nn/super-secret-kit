package models

import "gorm.io/gorm"

// Service constants for credentials
const (
	ServiceArkevia    = "arkevia"
	ServiceArkeviaOTP = "arkevia-otp"
	ServiceJira       = "jira"
	ServiceJiraTempo  = "jira_tempo"
)

// Credentials represents encrypted user credentials for external services
type Credentials struct {
	gorm.Model
	Service   string `gorm:"type:varchar(255);not null;uniqueIndex"`
	Username  string `gorm:"type:varchar(500);not null"` // Encrypted
	Password  string `gorm:"type:varchar(255)"`          // Encrypted, nullable
	OTPSecret string `gorm:"-"`                          // Transient field for runtime (stored separately in arkevia-otp)
}
