package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cmd/anytypecli/process"
	"github.com/anyproto/anytype-heart/pb"
)

// Token create command
var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Generate an API token for authenticating requests to the REST API",
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure the server is running
		status, err := process.CheckGRPCServer()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if !status {
			fmt.Println("The Anytype server is not running. Start the server first with `anytype server start`.")
			return
		}

		// Get mnemonic from flag, otherwise prompt user
		mnemonic, _ := cmd.Flags().GetString("mnemonic")
		if mnemonic == "" {
			fmt.Print("Enter mnemonic (12 words): ")
			reader := bufio.NewReader(os.Stdin)
			mnemonic, _ = reader.ReadString('\n')
			mnemonic = strings.TrimSpace(mnemonic)
		}

		// Ensure mnemonic is valid (should be 12 words)
		if len(strings.Split(mnemonic, " ")) != 12 {
			fmt.Println("Invalid mnemonic format. Please enter exactly 12 words.")
			return
		}

		client, err := process.GetGRPCClient()
		if err != nil {
			fmt.Println("Error connecting to gRPC server:", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{
				Mnemonic: mnemonic,
			},
		}
		resp, err := client.WalletCreateSession(ctx, req)
		if err != nil {
			fmt.Println("Failed to generate token:", err)
			return
		}

		// Display the token
		fmt.Println("Token generated successfully:")
		fmt.Println(resp.Token)
	},
}

// Parent token command
var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage authentication tokens",
}

func init() {
	tokenCreateCmd.Flags().String("mnemonic", "", "Provide mnemonic (12 words) for authentication")
	tokenCmd.AddCommand(tokenCreateCmd)
	rootCmd.AddCommand(tokenCmd)
}
