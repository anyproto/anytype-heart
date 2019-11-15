package core

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"C"

	nativeconfig "github.com/ipfs/go-ipfs-config"
	"github.com/textileio/go-textile/repo"
	tconfig "github.com/textileio/go-textile/repo/config"

	"github.com/textileio/go-textile/keypair"
	tmobile "github.com/textileio/go-textile/mobile"
	"github.com/textileio/go-textile/wallet"
)

type messenger struct {
}

var ErrRepoExists = repo.ErrRepoExists
var ErrRepoDoesNotExist = repo.ErrRepoDoesNotExist
var ErrMigrationRequired = repo.ErrMigrationRequired
var ErrRepoCorrupted = repo.ErrRepoCorrupted

func (msg *messenger) Notify(event *tmobile.Event) {
	// todo: implement real notifier
	fmt.Printf("notify: %s\n", event.Name)
}

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
	return tmobile.NewWallet(wordCount)
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
	nativeconfig.DefaultBootstrapAddresses = []string{}
	tconfig.DefaultBootstrapAddresses = BootstrapNodes
	return tmobile.InitRepo(&tmobile.InitConfig{Seed: seed, RepoPath: filepath.Join(rootPath, kp.Address()), Debug: true})
}
