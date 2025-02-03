package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/cmd/auth"
	"github.com/anyproto/anytype-heart/cli/cmd/server"
	"github.com/anyproto/anytype-heart/cli/cmd/shell"
	"github.com/anyproto/anytype-heart/cli/cmd/token"
)

var rootCmd = &cobra.Command{
	Use:   "anyctl <command> <subcommand> [flags]",
	Short: "Anytype CLI",
	Long:  "Seamlessly interact with Anytype from the command line",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
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
		token.NewTokenCmd(),
		shell.NewShellCmd(rootCmd),
	)
}
