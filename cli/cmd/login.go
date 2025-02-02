package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your Anytype vault",
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure the server is running
		status, err := internal.IsGRPCServerRunning()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if !status {
			err := internal.StartServer()
			if err != nil {
				fmt.Println("X Failed to start server:", err)
				return
			}
			time.Sleep(2 * time.Second) // wait for server to start
		}

		// Get mnemonic from flag, otherwise try retrieving it from the keychain.
		mnemonic, _ := cmd.Flags().GetString("mnemonic")
		usedStoredMnemonic := false
		if mnemonic == "" {
			mnemonic, err = internal.GetStoredMnemonic()
			if err == nil && mnemonic != "" {
				fmt.Println("Using stored mnemonic from keychain.")
				usedStoredMnemonic = true
			} else {
				fmt.Print("Enter mnemonic (12 words): ")
				reader := bufio.NewReader(os.Stdin)
				mnemonic, _ = reader.ReadString('\n')
				mnemonic = strings.TrimSpace(mnemonic)
			}
		}

		// Ensure mnemonic is valid (should be 12 words)
		if len(strings.Split(mnemonic, " ")) != 12 {
			fmt.Println("X Invalid mnemonic format. Please enter exactly 12 words.")
			return
		}
		// Set default root path (adjust as needed)
		rootPath, _ := cmd.Flags().GetString("path")

		// Perform the common login process.
		err = internal.LoginAccount(mnemonic, rootPath)
		if err != nil {
			fmt.Println("X Login failed:", err)
			return
		}

		// Save the mnemonic in the keychain for future logins.
		if !usedStoredMnemonic {
			if err := internal.SaveMnemonic(mnemonic); err != nil {
				fmt.Println("Warning: failed to save mnemonic in keychain:", err)
			} else {
				fmt.Println("Mnemonic saved to keychain.")
			}
		}
	},
}

func init() {
	loginCmd.Flags().String("mnemonic", "", "Provide mnemonic (12 words) for authentication")
	loginCmd.Flags().String("path", "", "Provide custom root path for wallet recovery")
}
