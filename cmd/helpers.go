package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"ssk/internal/models"
	"ssk/internal/services"
)

// CredentialOptions defines options for prompting credentials
type CredentialOptions struct {
	Service        string
	UsernamePrompt string
	PasswordPrompt string
	SkipPassword   bool
	ForcePrompt    bool
	PromptOTP      bool
}

// getOrPromptArkeviaCredentials handles getting/prompting Arkevia credentials and optional 2FA secret (arkevia-otp)
func getOrPromptArkeviaCredentials(credsService services.CredentialsService, forcePrompt bool) (*models.Credentials, error) {
	if forcePrompt {
		_ = credsService.Delete(models.ServiceArkevia)
		_ = credsService.Delete(models.ServiceArkeviaOTP)
	}

	creds, err := credsService.Get(models.ServiceArkevia)
	if err != nil || forcePrompt {
		creds, err = getOrPromptCredentials(credsService, CredentialOptions{
			Service:        models.ServiceArkevia,
			UsernamePrompt: "username (votre email)",
			PasswordPrompt: "password (le mot de passe arkevia)",
			ForcePrompt:    forcePrompt,
		})
		if err != nil {
			return nil, err
		}
	}

	// Check if arkevia-otp is configured
	otpCreds, err := credsService.Get(models.ServiceArkeviaOTP)
	if err == nil && otpCreds != nil && otpCreds.Username != "" {
		creds.OTPSecret = otpCreds.Username
	} else {
		// First time user question for TOTP secret
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("🔐 Voulez-vous enregistrer une clé secrète 2FA (TOTP) pour la connexion automatique à Arkevia ? (o/N) : ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "o" || answer == "y" || answer == "oui" || answer == "yes" {
			fmt.Print("Enter 2FA Secret Key (TOTP / Google Auth): ")
			secretInput, _ := reader.ReadString('\n')
			secretInput = strings.TrimSpace(secretInput)
			if secretInput != "" {
				if err := credsService.Update(models.ServiceArkeviaOTP, secretInput, nil); err != nil {
					fmt.Printf("⚠️ Could not save 2FA OTP secret: %v\n", err)
				} else {
					fmt.Println("✅ 2FA TOTP Secret Key saved to 'arkevia-otp'!")
					creds.OTPSecret = secretInput
				}
			}
		}
	}

	return creds, nil
}

// getOrPromptCredentials retrieves credentials or prompts the user if they don't exist
func getOrPromptCredentials(credsService services.CredentialsService, opts CredentialOptions) (*models.Credentials, error) {
	if !opts.ForcePrompt {
		creds, err := credsService.Get(opts.Service)
		if err == nil {
			return creds, nil
		}
	}

	fmt.Printf("\n🔑 Credentials needed for %s\n", opts.Service)

	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	prompt := opts.UsernamePrompt
	if prompt == "" {
		prompt = "Enter username/email: "
	}
	fmt.Print(prompt + " ")

	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	var password string
	if !opts.SkipPassword {
		// Prompt for password
		prompt = opts.PasswordPrompt
		if prompt == "" {
			prompt = "Enter password/token: "
		}
		fmt.Print(prompt + " ")

		bytePassword, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // Newline after password input

		password = string(bytePassword)
		password = strings.TrimSpace(password)
	}

	// Save credentials
	if err := credsService.Update(opts.Service, username, &password); err != nil {
		return nil, fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Printf("✅ Credentials saved for %s\n\n", opts.Service)

	// Return the new credentials
	return credsService.Get(opts.Service)
}
