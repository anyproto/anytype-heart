package internal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
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

func Login(mnemonic, rootPath string) error {
	// Ensure the server is running
	status, err := IsGRPCServerRunning()
	if err != nil {
		return fmt.Errorf("failed to check server status: %w", err)
	}
	if !status {
		err := StartServer()
		if err != nil {
			return fmt.Errorf("failed to start server: %w", err)
		}
		time.Sleep(2 * time.Second) // wait for server to start
	}

	usedStoredMnemonic := false
	if mnemonic == "" {
		mnemonic, err = GetStoredMnemonic()
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
		return fmt.Errorf("mnemonic must be 12 words")
	}

	// Perform the common login process.
	err = LoginAccount(mnemonic, rootPath)
	if err != nil {
		return fmt.Errorf("failed to log in: %w", err)
	}

	// Save the mnemonic in the keychain for future logins.
	if !usedStoredMnemonic {
		if err := SaveMnemonic(mnemonic); err != nil {
			fmt.Println("Warning: failed to save mnemonic in keychain:", err)
		} else {
			fmt.Println("Mnemonic saved to keychain.")
		}
	}

	return nil
}

func Logout() error {
	client, err := GetGRPCClient()
	if err != nil {
		fmt.Println("Failed to connect to gRPC server:", err)
	}

	token, err := GetStoredToken()
	if err != nil {
		return fmt.Errorf("failed to get stored token: %w", err)
	}

	// Call AccountStop RPC
	ctx := ClientContextWithAuth(token)
	resp, err := client.AccountStop(ctx, &pb.RpcAccountStopRequest{
		RemoveData: false,
	})

	if err != nil {
		return fmt.Errorf("failed to log out: %w", err)
	}

	if resp.Error.Code != pb.RpcAccountStopResponseError_NULL {
		fmt.Println("Failed to log out:", resp.Error.Description)
	}

	// Call WalletCloseSession RPC
	resp2, err := client.WalletCloseSession(ctx, &pb.RpcWalletCloseSessionRequest{Token: token})
	if err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}

	if resp2.Error.Code != pb.RpcWalletCloseSessionResponseError_NULL {
		fmt.Println("Failed to close session:", resp2.Error.Description)
	}

	err = DeleteStoredMnemonic()
	if err != nil {
		return fmt.Errorf("failed to delete stored mnemonic: %w", err)
	}

	fmt.Println("✓ Successfully logged out. Stored mnemonic removed.")

	err = StopServer()
	if err != nil {
		fmt.Println("X Failed to stop server:", err)
	} else {
		fmt.Println("✓ Server stopped successfully.")
	}

	return nil
}
