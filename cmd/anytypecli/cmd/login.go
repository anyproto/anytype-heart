package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your Anytype vault",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter mnemonic (12 words): ")
		mnemonic, _ := reader.ReadString('\n')
		mnemonic = strings.TrimSpace(mnemonic)

		if len(strings.Split(mnemonic, " ")) != 12 {
			fmt.Println("Invalid mnemonic format. Please enter exactly 12 words.")
			return
		}

		fmt.Println("Logging in...")
		// TODO: gRPC call to authenticate the user
		fmt.Println("Login successful.")
	},
}
