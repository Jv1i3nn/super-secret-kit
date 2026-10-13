package database

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ssk/internal/models"

	sqlite "gorm.io/driver/sqlite"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// DB is the global database instance
var DB *gorm.DB

// Connect initializes the database connection
func Connect(databasePath string, verbose bool) error {
	// Configure logger
	logLevel := logger.Silent
	if verbose {
		logLevel = logger.Info
	}

	// Open database/sql connection with modernc.org/sqlite
	sqlDB, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Wrap with GORM using sqlite driver
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	DB = db
	return nil
}

// AutoMigrate runs database migrations
func AutoMigrate() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	// Run auto migrations for all models
	if err := DB.AutoMigrate(&models.Credentials{}); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	return sqlDB.Close()
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}
