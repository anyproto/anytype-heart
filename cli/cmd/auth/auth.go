package auth

import (
	"github.com/spf13/cobra"

	authLoginCmd "github.com/anyproto/anytype-heart/cli/cmd/auth/login"
	authLogoutCmd "github.com/anyproto/anytype-heart/cli/cmd/auth/logout"
)

func NewAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate with Anytype",
	}

	authCmd.AddCommand(authLoginCmd.NewLoginCmd())
	authCmd.AddCommand(authLogoutCmd.NewLogoutCmd())

	return authCmd
}
