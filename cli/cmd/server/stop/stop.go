package stop

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/daemon"
)

func NewStopCmd() *cobra.Command {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Anytype local server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := daemon.SendTaskStop("server", nil)
			if err != nil {
				return fmt.Errorf("failed to stop server task: %w", err)
			}
			fmt.Println("✓ Server task stopped successfully. Response:", resp.Status)
			return nil
		},
	}

	return stopCmd
}
