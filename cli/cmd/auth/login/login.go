package login

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

func NewLoginCmd() *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to your Anytype vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			mnemonic, _ := cmd.Flags().GetString("mnemonic")
			rootPath, _ := cmd.Flags().GetString("path")
			if err := internal.Login(mnemonic, rootPath); err != nil {
				return fmt.Errorf("X Failed to log in: %w", err)
			}
			log.Println("✓ Successfully logged in")
			return nil

		},
	}

	loginCmd.Flags().String("mnemonic", "", "Provide mnemonic (12 words) for authentication")
	loginCmd.Flags().String("path", "", "Provide custom root path for wallet recovery")

	return loginCmd
}
