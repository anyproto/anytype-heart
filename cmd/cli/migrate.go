package main

import (
	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	core2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
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
		var mw = core.New()
		mw.EventSender = event.NewCallbackSender(func(event *pb.Event) {
			// nothing to do
		})

		resp := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: migrateAccount, RootPath: migrateRepoPath})
		if resp.Error.Code != 0 {
			c.PrintErrf("failed to open account repo: %s\n", resp.Error.Description)
			return
		}

		migrated, err := core2.ReindexAll(mw.GetAnytype().(*core2.Anytype))
		if err != nil {
			c.PrintErrf("failed to run reindex migration: %s\n", resp.Error.Description)
		}
		c.Printf("reindexed %d objects\n", migrated)
		c.Println("Shutting down account...")
		mw.Shutdown(&pb.RpcShutdownRequest{})
	},
}

func init() {
	// subcommands
	homeDir, _ := os.UserHomeDir()
	migrateCmd.AddCommand(reindex)
	migrateCmd.PersistentFlags().StringVarP(&migrateRepoPath, "repo", "r", homeDir+"/Library/Application Support/anytype2/dev/data", "path to dir with accounts folder")
	migrateCmd.PersistentFlags().StringVarP(&migrateAccount, "account", "a", "", "id of account in the repo folder")
}
