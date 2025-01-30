package cmd

import (
	"fmt"

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
