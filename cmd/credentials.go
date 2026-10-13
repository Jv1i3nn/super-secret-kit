package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/config"
	"ssk/internal/database"
	"ssk/internal/repository"
	"ssk/internal/services"
)

var credentialsCmd = &cobra.Command{
	Use:    "credentials",
	Hidden: true,
	Short:  "Manage encrypted credentials",
	Long: `Manage encrypted credentials for external services.

Credentials are stored encrypted using AES-256-CBC.`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored credentials",
	RunE:  runList,
}

var setCmd = &cobra.Command{
	Use:   "set <service> <username> [password] [otp_secret]",
	Short: "Set credentials for a service",
	Args:  cobra.RangeArgs(2, 4),
	RunE:  runSet,
}

var deleteCmd = &cobra.Command{
	Use:   "delete <service>",
	Short: "Delete credentials for a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	rootCmd.AddCommand(credentialsCmd)
	credentialsCmd.AddCommand(listCmd)
	credentialsCmd.AddCommand(setCmd)
	credentialsCmd.AddCommand(deleteCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	credsService, err := initCredentialsService()
	if err != nil {
		return err
	}

	credsList, err := credsService.List()
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	if len(credsList) == 0 {
		fmt.Println("No credentials stored.")
		return nil
	}

	fmt.Println("\n📋 Stored Credentials:")
	fmt.Println(strings.Repeat("-", 80))
	for _, creds := range credsList {
		password := "***"
		if creds.Password == "" {
			password = "(none)"
		}
		otp := "(none)"
		if creds.OTPSecret != "" {
			otp = "Configured"
		}
		fmt.Printf("🔐 %-15s | Username: %-25s | Password: %-8s | 2FA (TOTP): %s\n",
			creds.Service, creds.Username, password, otp)
	}
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Total: %d credential(s)\n\n", len(credsList))

	return nil
}

func runSet(cmd *cobra.Command, args []string) error {
	service := args[0]
	username := args[1]
	var password *string
	if len(args) > 2 {
		password = &args[2]
	}
	var otpSecret *string
	if len(args) > 3 {
		otpSecret = &args[3]
	}

	credsService, err := initCredentialsService()
	if err != nil {
		return err
	}

	if err := credsService.UpdateWithOTP(service, username, password, otpSecret); err != nil {
		return fmt.Errorf("failed to set credentials: %w", err)
	}

	fmt.Printf("✅ Credentials for '%s' have been set successfully.\n", service)
	return nil
}

func runDelete(cmd *cobra.Command, args []string) error {
	service := args[0]

	credsService, err := initCredentialsService()
	if err != nil {
		return err
	}

	if err := credsService.Delete(service); err != nil {
		return fmt.Errorf("failed to delete credentials: %w", err)
	}

	fmt.Printf("✅ Credentials for '%s' have been deleted.\n", service)
	return nil
}

func initCredentialsService() (services.CredentialsService, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	err = database.Connect(cfg.GetDatabasePath(), viper.GetBool("verbose"))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	encryptionService, err := services.NewEncryptionService(cfg.GetPassphrasePath())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize encryption service: %w", err)
	}

	repo := repository.NewCredentialsRepository(database.GetDB())
	return services.NewCredentialsService(repo, encryptionService), nil
}
