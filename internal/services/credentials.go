package services

import (
	"fmt"

	"ssk/internal/models"
	"ssk/internal/repository"

	"gorm.io/gorm"
)

type CredentialsService interface {
	Get(service string) (*models.Credentials, error)
	Update(service, username string, password *string) error
	UpdateWithOTP(service, username string, password *string, otpSecret *string) error
	Delete(service string) error
	List() ([]models.Credentials, error)
}

type credentialsService struct {
	repo       repository.CredentialsRepository
	encryption EncryptionService
}

func NewCredentialsService(repo repository.CredentialsRepository, encryption EncryptionService) CredentialsService {
	return &credentialsService{
		repo:       repo,
		encryption: encryption,
	}
}

func (s *credentialsService) Get(service string) (*models.Credentials, error) {
	creds, err := s.repo.FindByService(service)
	if err != nil {
		return nil, err
	}

	if creds == nil {
		return nil, fmt.Errorf("credentials not found for service: %s", service)
	}

	username, err := s.encryption.Decrypt(creds.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt username: %w", err)
	}
	creds.Username = username

	if creds.Password != "" {
		password, err := s.encryption.Decrypt(creds.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt password: %w", err)
		}
		creds.Password = password
	}

	return creds, nil
}

func (s *credentialsService) Update(service, username string, password *string) error {
	existing, err := s.repo.FindByServiceUnscoped(service)
	if err != nil {
		return err
	}

	encryptedUsername, err := s.encryption.Encrypt(username)
	if err != nil {
		return fmt.Errorf("failed to encrypt username: %w", err)
	}

	var encryptedPassword string
	if password != nil && *password != "" {
		encryptedPassword, err = s.encryption.Encrypt(*password)
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	}

	if existing == nil {
		creds := &models.Credentials{
			Service:  service,
			Username: encryptedUsername,
			Password: encryptedPassword,
		}
		return s.repo.Create(creds)
	}

	// Update existing (even if soft deleted)
	existing.Username = encryptedUsername
	if password != nil {
		existing.Password = encryptedPassword
	}

	// Restore if deleted
	if existing.DeletedAt.Valid {
		existing.DeletedAt = gorm.DeletedAt{}
	}

	return s.repo.Update(existing)
}

func (s *credentialsService) UpdateWithOTP(service, username string, password *string, otpSecret *string) error {
	if err := s.Update(service, username, password); err != nil {
		return err
	}
	if otpSecret != nil && *otpSecret != "" {
		return s.Update(models.ServiceArkeviaOTP, *otpSecret, nil)
	}
	return nil
}

func (s *credentialsService) Delete(service string) error {
	return s.repo.Delete(service)
}

func (s *credentialsService) List() ([]models.Credentials, error) {
	credsList, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	for i := range credsList {
		username, err := s.encryption.Decrypt(credsList[i].Username)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt username for service %s: %w", credsList[i].Service, err)
		}
		credsList[i].Username = username

		if credsList[i].Password != "" {
			password, err := s.encryption.Decrypt(credsList[i].Password)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt password for service %s: %w", credsList[i].Service, err)
			}
			credsList[i].Password = password
		}
	}

	return credsList, nil
}
