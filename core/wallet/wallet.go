package wallet

import (
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/accountdata"
	"github.com/anytypeio/any-sync/util/crypto"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"io/ioutil"
	"path/filepath"
)

const (
	CName         = accountservice.CName
	keyFileDevice = "device.key"
)

type wallet struct {
	rootPath      string
	repoPath      string // other components will init their files/dirs inside
	deviceKeyPath string

	accountKeypair crypto.PrivKey
	deviceKeypair  crypto.PrivKey
	accountData    *accountdata.AccountKeys
}

func (r *wallet) GetAccountPrivkey() (crypto.PrivKey, error) {
	return r.accountData.SignKey, nil
}

func (r *wallet) GetDevicePrivkey() (crypto.PrivKey, error) {
	return r.accountData.PeerKey, nil
}

func (r *wallet) Init(a *app.App) (err error) {
	if r.accountKeypair == nil {
		return fmt.Errorf("no account key present")
	}
	var b []byte
	if r.deviceKeypair == nil {
		if r.deviceKeyPath == "" {
			return fmt.Errorf("no path for device key")
		}
		b, err = ioutil.ReadFile(r.deviceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read device keyfile: %w", err)
		}
		dec, err := r.accountKeypair.Decrypt(b)
		if err != nil {
			return fmt.Errorf("failed to decrypt device keyfile: %w", err)
		}
		r.deviceKeypair, err = crypto.UnmarshalEd25519PrivateKeyProto(dec)
		if err != nil {
			return fmt.Errorf("failed to unmarshall device keyfile: %w", err)
		}
	}
	peerId := r.deviceKeypair.GetPublic().PeerId()
	accountId := r.accountKeypair.GetPublic().Account()
	logging.SetHost(peerId)
	metrics.SharedClient.SetDeviceId(peerId)
	logging.SetAccount(accountId)
	metrics.SharedClient.SetUserId(accountId)

	r.accountData = accountdata.New(r.deviceKeypair, r.accountKeypair)
	return nil
}

func (r *wallet) RepoPath() string {
	return r.repoPath
}

func (r *wallet) RootPath() string {
	return r.rootPath
}

func (r *wallet) Name() (name string) {
	return CName
}

func (r *wallet) Account() *accountdata.AccountKeys {
	return r.accountData
}

func NewWithAccountRepo(rootPath string, accountKey crypto.PrivKey) Wallet {
	accountId := accountKey.GetPublic().Account()
	repoPath := filepath.Join(rootPath, accountId)
	return &wallet{
		rootPath:       rootPath,
		repoPath:       repoPath,
		accountKeypair: accountKey,
		deviceKeyPath:  filepath.Join(repoPath, keyFileDevice),
	}
}

func NewWithRepoPathAndKeys(repoPath string, accountKeypair, deviceKeypair crypto.PrivKey) Wallet {
	return &wallet{
		repoPath:       repoPath,
		accountKeypair: accountKeypair,
		deviceKeypair:  deviceKeypair,
	}
}

type Wallet interface {
	RootPath() string
	RepoPath() string
	GetAccountPrivkey() (crypto.PrivKey, error)
	GetDevicePrivkey() (crypto.PrivKey, error)
	accountservice.Service
	app.Component
}
