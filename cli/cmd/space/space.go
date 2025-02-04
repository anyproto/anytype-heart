package space

import (
	"github.com/spf13/cobra"

	spaceAutoapproveCmd "github.com/anyproto/anytype-heart/cli/cmd/space/autoapprove"
)

func NewSpaceCmd() *cobra.Command {
	spaceCmd := &cobra.Command{
		Use:   "space <command>",
		Short: "Manage spaces",
	}

	spaceCmd.AddCommand(spaceAutoapproveCmd.NewAutoapproveCmd())

	return spaceCmd
}
