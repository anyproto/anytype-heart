package application

import (
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/mr-tron/base58"

	"github.com/anyproto/anytype-heart/pb"
)

const botAccountBaseIndex = 100 // Bot accounts start at index 100

// WalletExportBot exports a bot account key from a mnemonic
// The bot account is derived at index 100 + requested index
// This keeps bot accounts separate from the main account (index 0)
func (s *Service) WalletExportBot(req *pb.RpcWalletExportBotRequest) (string, error) {
	if req.Mnemonic == "" {
		return "", fmt.Errorf("mnemonic is required")
	}

	// Calculate the actual derivation index (100 + requested index)
	derivationIndex := botAccountBaseIndex + req.Index

	// Derive the master node for the bot account
	// This uses the new any-sync function to get just the master node
	masterNode, err := crypto.Mnemonic(req.Mnemonic).DeriveMasterNode(derivationIndex)
	if err != nil {
		return "", fmt.Errorf("failed to derive bot master node: %w", err)
	}

	// Marshal the master node to binary
	masterNodeBytes, err := masterNode.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to marshal master node: %w", err)
	}

	// Encode to base58 for the account key
	accountKey := base58.Encode(masterNodeBytes)

	return accountKey, nil
}
