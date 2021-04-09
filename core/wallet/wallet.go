package wallet

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	wallet2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"io/ioutil"
	"path/filepath"
)

const (
	CName          = "wallet"
	keyFileAccount = "account.key"
	keyFileDevice  = "device.key"
)

type wallet struct {
	repoPath       string // other components will init their files/dirs inside
	accountKeyPath string
	deviceKeyPath  string

	accountKeypair wallet2.Keypair
	deviceKeypair  wallet2.Keypair
}

func (r *wallet) GetAccountPrivkey() (wallet2.Keypair, error) {
	if r.accountKeypair == nil {
		return nil, fmt.Errorf("not set")
	}
	return r.accountKeypair, nil
}

func (r *wallet) GetDevicePrivkey() (wallet2.Keypair, error) {
	if r.deviceKeypair == nil {
		return nil, fmt.Errorf("not set")
	}
	return r.deviceKeypair, nil
}

func (r *wallet) Init(a *app.App) (err error) {
	var b []byte
	if r.deviceKeypair == nil && r.deviceKeyPath != "" {
		b, err = ioutil.ReadFile(r.deviceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read device keyfile: %w", err)
		}

		r.deviceKeypair, err = wallet2.UnmarshalBinary(b)
		if err != nil {
			return err
		}

		if r.deviceKeypair.KeypairType() != wallet2.KeypairTypeDevice {
			return fmt.Errorf("got %s key type instead of %s", r.deviceKeypair.KeypairType(), wallet2.KeypairTypeDevice)
		}
	}

	if r.accountKeypair == nil && r.accountKeyPath != "" {
		b, err = ioutil.ReadFile(r.accountKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read account keyfile: %w", err)
		}

		r.accountKeypair, err = wallet2.UnmarshalBinary(b)
		if err != nil {
			return err
		}
		if r.accountKeypair.KeypairType() != wallet2.KeypairTypeAccount {
			return fmt.Errorf("got %s key type instead of %s", r.accountKeypair.KeypairType(), wallet2.KeypairTypeAccount)
		}
	}

	if r.deviceKeypair != nil {
		logging.SetHost(r.deviceKeypair.Address())
	}
	return nil
}

func (r *wallet) RepoPath() string {
	return r.repoPath
}

func (r *wallet) Name() (name string) {
	return CName
}

func (r *wallet) Close() (err error) {
	return nil
}

func NewWithAccountRepo(rootpath, accountId string) Wallet {
	repoPath := filepath.Join(rootpath, accountId)
	return &wallet{
		repoPath:       repoPath,
		accountKeyPath: filepath.Join(repoPath, keyFileAccount),
		deviceKeyPath:  filepath.Join(repoPath, keyFileDevice),
	}
}

func NewWithRepoPathAndKeys(repoPath string, accountKeypair, deviceKeypair wallet2.Keypair) Wallet {
	return &wallet{
		repoPath:       repoPath,
		accountKeypair: accountKeypair,
		deviceKeypair:  deviceKeypair,
	}
}

type Wallet interface {
	RepoPath() string
	GetAccountPrivkey() (wallet2.Keypair, error)
	GetDevicePrivkey() (wallet2.Keypair, error)
	app.Component
}
