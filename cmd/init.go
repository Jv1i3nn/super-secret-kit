package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/config"
	"ssk/internal/database"
)

var initCmd = &cobra.Command{
	Use:    "init",
	Hidden: true,
	Short:  "Initialize SSK database and configuration",
	Long: `Initialize SSK by creating the database, running migrations,
and generating the encryption passphrase if needed.

This command should be run once before using any other SSK commands.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("🔧 Initializing SSK...")

	if err := os.MkdirAll(cfg.ExternalProjectPath, 0755); err != nil {
		return fmt.Errorf("failed to create .ssk directory: %w", err)
	}
	fmt.Printf("✓ Created directory: %s\n", cfg.ExternalProjectPath)

	passphrasePath := cfg.GetPassphrasePath()
	if _, err := os.Stat(passphrasePath); os.IsNotExist(err) {
		if err := generatePassphrase(passphrasePath); err != nil {
			return fmt.Errorf("failed to generate passphrase: %w", err)
		}
		fmt.Printf("✓ Generated passphrase: %s\n", passphrasePath)
	} else {
		fmt.Printf("✓ Passphrase already exists: %s\n", passphrasePath)
	}

	if err := database.Connect(cfg.GetDatabasePath(), viper.GetBool("verbose")); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = database.Close() }()

	fmt.Printf("✓ Connected to database: %s\n", cfg.GetDatabasePath())

	if err := database.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	fmt.Println("✓ Database migrations completed")

	fmt.Println("\n✅ SSK initialization complete!")
	fmt.Println("\nYou can now use SSK commands. Try:")
	fmt.Println("  ssk logtime --help")
	fmt.Println("  ssk credentials list")

	return nil
}

func generatePassphrase(path string) error {
	bytes := make([]byte, 10)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}

	passphrase := hex.EncodeToString(bytes)

	if err := os.WriteFile(path, []byte(passphrase), 0600); err != nil {
		return fmt.Errorf("failed to write passphrase file: %w", err)
	}

	return nil
}
