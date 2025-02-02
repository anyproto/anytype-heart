package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and remove stored mnemonic from keychain",
	Run: func(cmd *cobra.Command, args []string) {
		err := internal.DeleteStoredMnemonic()
		if err != nil {
			fmt.Println("❌ Failed to remove stored mnemonic:", err)
			return
		}
		fmt.Println("✅ Successfully logged out. Stored mnemonic removed.")
	},
}
