package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Anytype headless server",
	Run: func(cmd *cobra.Command, args []string) {
		err := internal.StartServer()
		if err != nil {
			fmt.Println("❌ Failed to start server:", err)
		} else {
			fmt.Println("✅ Server started successfully.")
			time.Sleep(2 * time.Second) // wait for server to start

			mnemonic, err := internal.GetStoredMnemonic()
			if err == nil && mnemonic != "" {
				fmt.Println("🔐 Keychain mnemonic found. Attempting auto login...")
				if _, err := internal.LoginAccount(mnemonic, ""); err != nil {
					fmt.Println("Auto login failed:", err)
				} else {
					fmt.Println("✅ Auto login successful!")
				}
			} else {
				fmt.Println("ℹ️ No keychain mnemonic found. Please login using 'anyctl login'.")
			}
		}
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Anytype headless server",
	Run: func(cmd *cobra.Command, args []string) {
		err := internal.StopServer()
		if err != nil {
			fmt.Println("❌ Failed to stop server:", err)
		} else {
			fmt.Println("✅ Server stopped successfully.")
		}
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Anytype headless server",
	Run: func(cmd *cobra.Command, args []string) {
		status, err := internal.CheckServerStatus()
		if err != nil {
			fmt.Println("⚠️ Error checking server status:", err)
		} else {
			fmt.Println("Server Status:", status)
		}
	},
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the Anytype server",
}

func init() {
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverStatusCmd)
}
