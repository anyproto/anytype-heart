package core

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/go-slip10"
)

var ErrRepoExists = fmt.Errorf("repo not empty, reinitializing would overwrite your account")

func WalletGenerateMnemonic(wordCount int) (string, error) {
	m, err := crypto.NewMnemonicGenerator().WithWordCount(wordCount)
	if err != nil {
		return "", err
	}
	return string(m), nil
}

func WalletAccountAt(mnemonic string, index int) (crypto.DerivationResult, error) {
	return crypto.Mnemonic(mnemonic).DeriveKeys(uint32(index))
}

// WalletDeriveFromAccountMasterNode derives keys from a base64-encoded account master node
// The accountKey contains both the seed and chain code needed for derivation
// This master node is already at the account level (m/44'/2046'/0')
func WalletDeriveFromAccountMasterNode(accountKeyMasterNodeBase64 string) (crypto.DerivationResult, error) {
	accountKeyBytes, err := base64.StdEncoding.DecodeString(accountKeyMasterNodeBase64)
	if err != nil {
		return crypto.DerivationResult{}, fmt.Errorf("failed to decode base64 account key: %w", err)
	}

	// Unmarshal the master node from the account key
	masterNode, err := slip10.UnmarshalNode(accountKeyBytes)
	if err != nil {
		return crypto.DerivationResult{}, fmt.Errorf("failed to unmarshal account master node: %w", err)
	}

	return crypto.DeriveKeysFromMasterNode(masterNode)
}

func WalletInitRepo(rootPath string, pk crypto.PrivKey) error {
	devicePriv, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return err
	}
	address := pk.GetPublic().Account()
	repoPath := filepath.Join(rootPath, address)
	_, err = os.Stat(repoPath)
	if !os.IsNotExist(err) {
		return ErrRepoExists
	}

	os.MkdirAll(repoPath, 0700)
	deviceKeyPath := filepath.Join(repoPath, "device.key")
	proto, err := devicePriv.Marshall()
	if err != nil {
		return err
	}
	encProto, err := pk.GetPublic().Encrypt(proto)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(deviceKeyPath, encProto, 0400)
}
