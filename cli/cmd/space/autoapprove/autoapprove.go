package autoapprove

import (
	"github.com/spf13/cobra"
)

func NewAutoapproveCmd() *cobra.Command {
	autoapproveCmd := &cobra.Command{
		Use:   "autoapprove",
		Short: "Manage autoapprove settings for spaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	autoapproveCmd.Flags().String("space", "", "ID of the space to manage autoapprove settings for")
	autoapproveCmd.Flags().String("role", "", "Role to manage autoapprove settings for")

	return autoapproveCmd
}
