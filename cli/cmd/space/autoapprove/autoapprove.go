package autoapprove

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/daemon"
)

func NewAutoapproveCmd() *cobra.Command {
	var spaceID string
	var role string

	autoapproveCmd := &cobra.Command{
		Use:   "autoapprove",
		Short: "Start autoapproval of join requests for a space (runs in background)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("space id is required (use --space)")
			}
			if role == "" {
				return fmt.Errorf("role is required (use --role)")
			}

			params := map[string]string{
				"space": spaceID,
				"role":  role,
			}
			resp, err := daemon.SendTaskStart("autoapprove", params)
			if err != nil {
				return fmt.Errorf("failed to start autoapprove task: %w", err)
			}
			fmt.Printf("Autoapprove task started for space %s with role %s. Response: %s\n", spaceID, role, resp.Status)
			return nil
		},
	}

	autoapproveCmd.Flags().StringVar(&spaceID, "space", "", "ID of the space to monitor")
	autoapproveCmd.Flags().StringVar(&role, "role", "", "Role to assign to approved join requests (e.g., Editor, Viewer)")

	return autoapproveCmd
}
