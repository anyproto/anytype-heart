package status

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

func NewStatusCmd() *cobra.Command {
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Get the status of the Anytype local server",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := internal.CheckServerStatus()
			if err != nil {
				return fmt.Errorf("X Failed to get server status: %w", err)
			}
			fmt.Println(status)
			return nil
		},
	}

	return statusCmd
}
