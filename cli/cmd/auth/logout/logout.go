package logout

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

func NewLogoutCmd() *cobra.Command {
	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out and remove stored mnemonic from keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := internal.Logout(); err != nil {
				return fmt.Errorf("X Failed to log out: %w", err)
			}
			fmt.Println("✓ Successfully logged out")
			return nil
		},
	}

	return logoutCmd
}
