package server

import (
	"github.com/spf13/cobra"

	serverStartCmd "github.com/anyproto/anytype-heart/cli/cmd/server/start"
	serverStatusCmd "github.com/anyproto/anytype-heart/cli/cmd/server/status"
	serverStopCmd "github.com/anyproto/anytype-heart/cli/cmd/server/stop"
)

func NewServerCmd() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server <command>",
		Short: "Manage the Anytype local server",
	}

	serverCmd.AddCommand(serverStartCmd.NewStartCmd())
	serverCmd.AddCommand(serverStopCmd.NewStopCmd())
	serverCmd.AddCommand(serverStatusCmd.NewStatusCmd())

	return serverCmd
}
