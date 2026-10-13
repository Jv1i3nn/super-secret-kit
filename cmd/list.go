package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// listCommandsCmd shows all commands with aliases
var listCommandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List all available commands with their aliases",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("SSK Commands:")
		fmt.Println("═════════════════════════════════════════════════════════")
		fmt.Println()

		for _, command := range rootCmd.Commands() {
			if command.Hidden {
				continue
			}

			// Skip help and completion commands
			if command.Name() == "help" || command.Name() == "completion" {
				continue
			}

			// Build command line with aliases
			cmdLine := fmt.Sprintf("  %-15s", command.Name())
			if len(command.Aliases) > 0 {
				aliases := strings.Join(command.Aliases, ", ")
				cmdLine += fmt.Sprintf(" [%s]", aliases)
			}

			fmt.Println(cmdLine)
			fmt.Printf("    %s\n\n", command.Short)
		}

		fmt.Println("═════════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Println("Use 'ssk <command> --help' for more information about a command.")
		fmt.Println("Aliases can be used interchangeably with the main command name.")
	},
}

func init() {
	rootCmd.AddCommand(listCommandsCmd)
}
