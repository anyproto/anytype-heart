package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrations commands",
}

var (
	migrateRepoPath string
	migrateAccount  string
)

var reindex = &cobra.Command{
	Use:   "reindex",
	Short: "Reindex all existing objects in the local repo",
	Run: func(c *cobra.Command, args []string) {
		// todo: reimplement reindex CLI using new mechanism
		fmt.Println("not implemented")
		os.Exit(1)
	},
}

func init() {
	// subcommands
	homeDir, _ := os.UserHomeDir()
	migrateCmd.AddCommand(reindex)
	migrateCmd.PersistentFlags().StringVarP(&migrateRepoPath, "repo", "r", homeDir+"/Library/Application Support/anytype2/dev/data", "path to dir with accounts folder")
	migrateCmd.PersistentFlags().StringVarP(&migrateAccount, "account", "a", "", "id of account in the repo folder")
}
