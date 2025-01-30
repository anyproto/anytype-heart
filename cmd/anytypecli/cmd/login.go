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
	pb "github.com/anyproto/anytype-heart/pb"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your Anytype vault",
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure the server is running
		status, err := process.IsGRPCServerRunning()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if !status {
			fmt.Println("The Anytype server is not running. Start the server first with `anytype server start`.")
			return
		}

		client, err := process.GetGRPCClient()
		if err != nil {
			fmt.Println("Error connecting to gRPC server:", err)
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
			fmt.Println("❌ Invalid mnemonic format. Please enter exactly 12 words.")
			return
		}
		// Set default root path (adjust as needed)
		rootPath, _ := cmd.Flags().GetString("path")
		if rootPath == "" {
			rootPath = "/Users/jmetrikat/Library/Application Support/anytype/alpha/data"
		}

		// Call gRPC InitialSetParameters
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
			Platform: "Mac",
			Version:  "0.0.0-test",
			Workdir:  "/Users/jmetrikat/Library/Application Support/anytype",
		})

		fmt.Println("✅ Initial parameters set.")

		// Call WalletRecover first
		recoverReq := &pb.RpcWalletRecoverRequest{
			Mnemonic: mnemonic,
			RootPath: rootPath,
		}
		_, err = client.WalletRecover(ctx, recoverReq)
		if err != nil {
			fmt.Println("❌ Wallet recovery failed:", err)
			return
		}

		fmt.Println("✅ Wallet recovered successfully.")

		// Call gRPC WalletCreateSession
		req := &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{
				Mnemonic: mnemonic,
			},
		}

		resp, err := client.WalletCreateSession(ctx, req)
		if err != nil {
			fmt.Println("❌ Failed to create session:", err)
			return
		}

		// Store the session token
		sessionToken := resp.Token
		fmt.Println("✅ Session created successfully.")

		// Start listening for session events in a goroutine
		accountIDChan := make(chan string, 1)
		errorChan := make(chan error, 1)

		go func() {
			accountID, err := process.ListenSessionEvents(sessionToken)
			if err != nil {
				errorChan <- err
			} else {
				accountIDChan <- accountID
			}
		}()

		// 🟢 Call `AccountRecover` right after starting listener
		_, err = client.AccountRecover(ctx, &pb.RpcAccountRecoverRequest{})

		// Wait for either an account ID or an error
		select {
		case accountID := <-accountIDChan:
			fmt.Println("✅ Received Account ID:", accountID)

			// Now select the account using gRPC
			client, err := process.GetGRPCClient()
			if err != nil {
				fmt.Println("❌ Error connecting to gRPC server:", err)
				return
			}

			accountSelectReq := &pb.RpcAccountSelectRequest{
				DisableLocalNetworkSync: false,
				Id:                      accountID,
				JsonApiListenAddr:       "127.0.0.1:31009",
				RootPath:                rootPath,
			}

			_, err = client.AccountSelect(ctx, accountSelectReq)
			if err != nil {
				fmt.Println("❌ Failed to select account:", err)
				return
			}

			fmt.Println("✅ Successfully selected account!")

		case err := <-errorChan:
			fmt.Println("❌ Failed to get account ID:", err)
		case <-time.After(10 * time.Second): // Timeout in case of failure
			fmt.Println("❌ Timed out waiting for session event")
		}
	},
}

func init() {
	loginCmd.Flags().String("mnemonic", "", "Provide mnemonic (12 words) for authentication")
	loginCmd.Flags().String("path", "", "Provide custom root path for wallet recovery")
}
