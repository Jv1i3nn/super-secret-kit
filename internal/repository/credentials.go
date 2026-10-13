package repository

import (
	"fmt"

	"gorm.io/gorm"

	"ssk/internal/models"
)

type CredentialsRepository interface {
	FindByService(service string) (*models.Credentials, error)
	FindByServiceUnscoped(service string) (*models.Credentials, error)
	Create(creds *models.Credentials) error
	Update(creds *models.Credentials) error
	Delete(service string) error
	List() ([]models.Credentials, error)
}

type credentialsRepository struct {
	db *gorm.DB
}

func NewCredentialsRepository(db *gorm.DB) CredentialsRepository {
	return &credentialsRepository{db: db}
}

func (r *credentialsRepository) FindByService(service string) (*models.Credentials, error) {
	var creds models.Credentials
	result := r.db.Where("service = ?", service).First(&creds)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find credentials: %w", result.Error)
	}

	return &creds, nil
}

func (r *credentialsRepository) FindByServiceUnscoped(service string) (*models.Credentials, error) {
	var creds models.Credentials
	result := r.db.Unscoped().Where("service = ?", service).First(&creds)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find credentials (unscoped): %w", result.Error)
	}

	return &creds, nil
}

func (r *credentialsRepository) Create(creds *models.Credentials) error {
	result := r.db.Create(creds)
	if result.Error != nil {
		return fmt.Errorf("failed to create credentials: %w", result.Error)
	}
	return nil
}

func (r *credentialsRepository) Update(creds *models.Credentials) error {
	result := r.db.Save(creds)
	if result.Error != nil {
		return fmt.Errorf("failed to update credentials: %w", result.Error)
	}
	return nil
}

func (r *credentialsRepository) Delete(service string) error {
	result := r.db.Where("service = ?", service).Delete(&models.Credentials{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete credentials: %w", result.Error)
	}
	return nil
}

func (r *credentialsRepository) List() ([]models.Credentials, error) {
	var credentials []models.Credentials
	result := r.db.Find(&credentials)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", result.Error)
	}

	return credentials, nil
}
