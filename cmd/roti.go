package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const bestNumberEver = 4

// rotiCmd represents the roti command
var rotiCmd = &cobra.Command{
	Use:    "roti",
	Hidden: true,
	Short:  "Tell the number you should vote to stay out of trouble",
	Long:   `ROTI (Return On Time Invested) - The answer is always 4.`,
	Run:    runRoti,
}

func init() {
	rootCmd.AddCommand(rotiCmd)
}

func runRoti(cmd *cobra.Command, args []string) {
	fmt.Printf("✅ %d\n", bestNumberEver)
}
