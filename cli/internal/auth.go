package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/pb"
)

// LoginAccount performs the common steps for logging in with a given mnemonic and root path.
// It returns the session token if successful.
func LoginAccount(mnemonic, rootPath string) error {
	if rootPath == "" {
		rootPath = "/Users/jmetrikat/Library/Application Support/anytype/alpha/data"
	}

	client, err := GetGRPCClient()
	if err != nil {
		return fmt.Errorf("error connecting to gRPC server: %w", err)
	}

	// Create a context for the initial calls.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set initial parameters (adjust these values as needed).
	_, err = client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
		Platform: "Mac",
		Version:  "0.0.0-test",
		Workdir:  "/Users/jmetrikat/Library/Application Support/anytype",
	})
	if err != nil {
		return fmt.Errorf("failed to set initial parameters: %w", err)
	}

	// Recover the wallet.
	_, err = client.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{
		Mnemonic: mnemonic,
		RootPath: rootPath,
	})
	if err != nil {
		return fmt.Errorf("wallet recovery failed: %w", err)
	}

	// Create a session.
	resp, err := client.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{
		Auth: &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{
			Mnemonic: mnemonic,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	sessionToken := resp.Token
	err = SaveToken(sessionToken)
	if err != nil {
		return fmt.Errorf("failed to save session token: %w", err)
	}

	// Start listening for session events using the universal event listener.
	er, err := ListenForEvents(sessionToken)
	if err != nil {
		return fmt.Errorf("failed to start event listener: %w", err)
	}

	// Recover the account.
	ctx = ClientContextWithAuth(sessionToken)
	_, err = client.AccountRecover(ctx, &pb.RpcAccountRecoverRequest{})
	if err != nil {
		return fmt.Errorf("account recovery failed: %w", err)
	}

	// Wait for the account ID from the event receiver.
	accountID, err := WaitForAccountID(er)
	if err != nil {
		return fmt.Errorf("error waiting for account ID: %w", err)
	}

	// Select the account.
	_, err = client.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
		DisableLocalNetworkSync: false,
		Id:                      accountID,
		JsonApiListenAddr:       "127.0.0.1:31009",
		RootPath:                rootPath,
	})
	if err != nil {
		return fmt.Errorf("failed to select account: %w", err)
	}

	return nil
}
