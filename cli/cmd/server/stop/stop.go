package stop

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

func NewStopCmd() *cobra.Command {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Anytype local server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := internal.StopServer(); err != nil {
				return fmt.Errorf("X Failed to stop server: %w", err)
			}
			fmt.Println("✓ Server stopped successfully.")
			return nil
		},
	}

	return stopCmd
}
