package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/cmd/auth"
	"github.com/anyproto/anytype-heart/cli/cmd/server"
)

var rootCmd = &cobra.Command{
	Use:   "anyctl",
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
	rootCmd.AddCommand(
		auth.NewAuthCmd(),
		server.NewServerCmd(),
		tokenCmd,
		shellCmd,
	)
}
