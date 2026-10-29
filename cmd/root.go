package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"ssk/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "ssk",
	Short: "Super Secret Kit - CLI tool for Jira, Tempo, and Arkevia",
	Long: `SSK (Super Secret Kit) provides secure credential management
and integration with Jira, Tempo, and Arkevia services.

Features:
  - Secure credential storage with AES-256-CBC encryption
  - Jira/Tempo time logging automation
  - Arkevia document management
  - Interactive CLI workflows`,
	Version: version.GetVersion(),
}

func Execute() error {
	notice := make(chan string, 1)
	go func() { notice <- updateNotice() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd, err := rootCmd.ExecuteContextC(ctx)
	// stderr keeps stdout (Markdown output) clean; update reports by itself.
	if msg := <-notice; msg != "" && cmd.Name() != "update" && !strings.HasPrefix(cmd.Name(), "__complete") {
		fmt.Fprintln(os.Stderr, msg)
	}
	return err
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		showCommandList()
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ssk/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorGray   = "\033[90m"
)

func showCommandList() {
	fmt.Println()
	fmt.Printf("%s%sSSK (Super Secret Kit)%s v%s\n", colorBold, colorCyan, colorReset, version.GetVersion())
	fmt.Printf("%s═════════════════════════════════════════════════════════%s\n", colorGray, colorReset)
	fmt.Println()

	for _, command := range rootCmd.Commands() {
		if command.Hidden {
			continue
		}

		if command.Name() == "help" || command.Name() == "completion" || command.Name() == "commands" {
			continue
		}

		fmt.Printf("  %s%s%-15s%s", colorBold, colorGreen, command.Name(), colorReset)

		if len(command.Aliases) > 0 {
			aliases := ""
			for i, alias := range command.Aliases {
				if i > 0 {
					aliases += ", "
				}
				aliases += alias
			}
			fmt.Printf(" %s[%s]%s", colorYellow, aliases, colorReset)
		}

		fmt.Println()
		fmt.Printf("    %s%s%s\n\n", colorGray, command.Short, colorReset)
	}

	fmt.Printf("%s═════════════════════════════════════════════════════════%s\n", colorGray, colorReset)
	fmt.Println()
	fmt.Printf("Use '%sssk <command> --help%s' for more information\n", colorCyan, colorReset)
	fmt.Printf("Use '%sssk --version%s' to show version\n", colorCyan, colorReset)
	fmt.Println()
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home + "/.ssk")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
