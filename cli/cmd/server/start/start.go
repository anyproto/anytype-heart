package start

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

func NewStartCmd() *cobra.Command {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Anytype local server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := internal.StartServer(); err != nil {
				return fmt.Errorf("X Failed to start server: %w", err)
			}
			fmt.Println("✓ Server started successfully.")
			time.Sleep(2 * time.Second) // wait for server to start

			mnemonic, err := internal.GetStoredMnemonic()
			if err == nil && mnemonic != "" {
				fmt.Println("🔐 Keychain mnemonic found. Attempting to login...")
				if err := internal.LoginAccount(mnemonic, ""); err != nil {
					fmt.Println("X Failed to login using keychain mnemonic:", err)
				} else {
					fmt.Println("✓ Successfully logged in using keychain mnemonic.")
				}
			} else {
				fmt.Println("ℹ️ No keychain mnemonic found. Please login using 'anyctl login'.")
			}
			return nil
		},
	}

	return startCmd
}
