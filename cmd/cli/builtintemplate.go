package main

import (
	"time"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/builtintemplate"
	"github.com/spf13/cobra"
)

var builtintemplaeCmd = &cobra.Command{
	Use:   "builtintemplate",
	Short: "Generates builtin templates from the special account",
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
		time.Sleep(time.Second)

		templateService := mw.GetApp().MustComponent(builtintemplate.CName).(builtintemplate.BuiltinTemplate)
		n, err := templateService.GenerateTemplates()
		if err != nil {
			c.Println("can't generate templates: ", err)
			mw.Shutdown(&pb.RpcShutdownRequest{})
			return
		}
		c.Printf("success! generated %d templates\n", n)
		mw.Shutdown(&pb.RpcShutdownRequest{})
	},
}

func init() {
	migrateCmd.AddCommand(builtintemplaeCmd)
}
