package main

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/spf13/cobra"
	"os"
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
		for _, arg := range args{
			fmt.Print(arg+": ")
			t, err := smartblock.SmartBlockTypeFromID(arg)
			if err!= nil {
				fmt.Println(err.Error())
			} else {
				fmt.Printf("%d\n", t)
			}
		}
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
