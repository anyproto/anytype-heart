package token

import (
	"github.com/spf13/cobra"

	tokenCreateCmd "github.com/anyproto/anytype-heart/cli/cmd/token/create"
)

func NewTokenCmd() *cobra.Command {
	tokenCmd := &cobra.Command{
		Use:   "token <command>",
		Short: "Manage API tokens for authenticating requests to the REST API",
	}

	tokenCmd.AddCommand(tokenCreateCmd.NewCreateCmd())

	return tokenCmd
}
