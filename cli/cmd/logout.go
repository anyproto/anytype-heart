package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
	"github.com/anyproto/anytype-heart/pb"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and remove stored mnemonic from keychain",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := internal.GetGRPCClient()
		if err != nil {
			fmt.Println("Failed to connect to gRPC server:", err)
		}

		token, err := internal.GetStoredToken()
		if err != nil {
			fmt.Println("Failed to get stored token:", err)
			return
		}

		// Call AccountStop RPC
		ctx := internal.ClientContextWithAuth(token)
		resp, err := client.AccountStop(ctx, &pb.RpcAccountStopRequest{
			RemoveData: false,
		})

		if err != nil {
			fmt.Println("Failed to log out:", err)
			return
		}

		if resp.Error.Code != pb.RpcAccountStopResponseError_NULL {
			fmt.Println("Failed to log out:", resp.Error.Description)
		}

		// Call WalletCloseSession RPC
		resp2, err := client.WalletCloseSession(ctx, &pb.RpcWalletCloseSessionRequest{Token: token})
		if err != nil {
			fmt.Println("Failed to close session:", err)
			return
		}

		if resp2.Error.Code != pb.RpcWalletCloseSessionResponseError_NULL {
			fmt.Println("Failed to close session:", resp2.Error.Description)
		}

		err = internal.DeleteStoredMnemonic()
		if err != nil {
			fmt.Println("Failed to remove stored mnemonic:", err)
			return
		}

		fmt.Println("✓ Successfully logged out. Stored mnemonic removed.")

		err = internal.StopServer()
		if err != nil {
			fmt.Println("X Failed to stop server:", err)
		} else {
			fmt.Println("✓ Server stopped successfully.")
		}
	},
}
