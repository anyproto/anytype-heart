package status

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/daemon"
)

func NewStatusCmd() *cobra.Command {
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Get the status of the Anytype local server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := daemon.SendTaskStatus("server")
			if err != nil {
				return fmt.Errorf("failed to get server status: %w", err)
			}
			fmt.Println("ℹ Server status:", resp.Status)
			return nil
		},
	}
	return statusCmd
}
