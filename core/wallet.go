package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/textileio/go-textile/keypair"
	"github.com/textileio/go-textile/wallet"
)

var ErrRepoExists = fmt.Errorf("repo not empty, reinitializing would overwrite your account")
var ErrRepoDoesNotExist = fmt.Errorf("repo does not exist, initialization is required")
var ErrMigrationRequired = fmt.Errorf("repo needs migration")
var ErrRepoCorrupted = fmt.Errorf("repo is corrupted")

func WalletListLocalAccounts(rootPath string) ([]string, error) {
	repos, err := ioutil.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	var accounts []string
	for _, f := range repos {
		if len(f.Name()) == 48 {
			accounts = append(accounts, f.Name())
		}
	}

	return accounts, nil
}

func WalletGenerateMnemonic(wordCount int) (string, error) {
	w, err := wallet.WalletFromWordCount(wordCount)
	if err != nil {
		return "", err
	}
	return w.RecoveryPhrase, nil
}

func WalletAccountAt(mnemonic string, index int, passphrase string) (*keypair.Full, error) {
	w := wallet.WalletFromMnemonic(mnemonic)
	return w.AccountAt(index, passphrase)
}

func WalletInitRepo(rootPath string, seed string) error {
	kp, err := keypair.Parse(seed)
	if err != nil {
		return err
	}

	repoPath := filepath.Join(rootPath, kp.Address())
	_, err = os.Stat(repoPath)
	if !os.IsNotExist(err) {
		return ErrRepoExists
	}

	os.MkdirAll(repoPath, 0700)
	keyPath := filepath.Join(repoPath, "key")

	priv, err := kp.LibP2PPrivKey()
	if err != nil {
		return err
	}

	key, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	if err = ioutil.WriteFile(keyPath, key, 0400); err != nil {
		panic(err)
	}

	return nil
}
