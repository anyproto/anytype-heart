package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ay",
	Short: "Anytype Headless CLI",
	Long:  `Manage the Anytype local server from the command line.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(shellCmd)
}
