package wallet

import (
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/accountdata"
	"github.com/anytypeio/any-sync/util/keys/asymmetric/encryptionkey"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	walletUtil "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"io/ioutil"
	"path/filepath"
)

const (
	CName          = accountservice.CName
	keyFileAccount = "account.key"
	keyFileDevice  = "device.key"
)

type wallet struct {
	rootPath       string
	repoPath       string // other components will init their files/dirs inside
	accountKeyPath string
	deviceKeyPath  string

	accountKeypair walletUtil.Keypair
	deviceKeypair  walletUtil.Keypair

	accountData *accountdata.AccountData
}

func (r *wallet) GetAccountPrivkey() (walletUtil.Keypair, error) {
	if r.accountKeypair == nil {
		return nil, fmt.Errorf("not set")
	}
	return r.accountKeypair, nil
}

func (r *wallet) GetDevicePrivkey() (walletUtil.Keypair, error) {
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

		r.deviceKeypair, err = walletUtil.UnmarshalBinary(b)
		if err != nil {
			return err
		}

		if r.deviceKeypair.KeypairType() != walletUtil.KeypairTypeDevice {
			return fmt.Errorf("got %s key type instead of %s", r.deviceKeypair.KeypairType(), walletUtil.KeypairTypeDevice)
		}
	}

	if r.accountKeypair == nil && r.accountKeyPath != "" {
		b, err = ioutil.ReadFile(r.accountKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read account keyfile: %w", err)
		}

		r.accountKeypair, err = walletUtil.UnmarshalBinary(b)
		if err != nil {
			return err
		}
		if r.accountKeypair.KeypairType() != walletUtil.KeypairTypeAccount {
			return fmt.Errorf("got %s key type instead of %s", r.accountKeypair.KeypairType(), walletUtil.KeypairTypeAccount)
		}
	}

	if r.deviceKeypair != nil {
		logging.SetHost(r.deviceKeypair.Address())
		metrics.SharedClient.SetDeviceId(r.deviceKeypair.Address())
	}
	if r.accountKeypair != nil {
		logging.SetAccount(r.accountKeypair.Address())
		metrics.SharedClient.SetUserId(r.accountKeypair.Address())

		rawAccountPubKey, e := r.accountKeypair.GetPublic().Raw()
		if e != nil {
			return e
		}
		peerID, e := r.deviceKeypair.PeerId()
		if e != nil {
			return e
		}
		deviceSigKey, e := r.deviceKeypair.AnySyncSignKey()
		if e != nil {
			return e
		}
		accountSigKey, e := r.accountKeypair.AnySyncSignKey()
		if e != nil {
			return e
		}
		rawAccounrPriv, e := r.accountKeypair.Raw()
		if e != nil {
			return e
		}
		// TODO: maybe need to cache it into file
		accountEncKey, _, e := encryptionkey.DeriveRSAKePair(4096, rawAccounrPriv)
		if e != nil {
			return e
		}
		r.accountData = &accountdata.AccountData{
			Identity: rawAccountPubKey,
			PeerKey:  deviceSigKey,
			SignKey:  accountSigKey,
			EncKey:   accountEncKey,
			PeerId:   peerID.String(),
		}
	}

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

func (r *wallet) Account() *accountdata.AccountData {
	return r.accountData
}

func NewWithAccountRepo(rootpath, accountId string) Wallet {
	repoPath := filepath.Join(rootpath, accountId)
	return &wallet{
		rootPath:       rootpath,
		repoPath:       repoPath,
		accountKeyPath: filepath.Join(repoPath, keyFileAccount),
		deviceKeyPath:  filepath.Join(repoPath, keyFileDevice),
	}
}

func NewWithRepoPathAndKeys(repoPath string, accountKeypair, deviceKeypair walletUtil.Keypair) Wallet {
	return &wallet{
		repoPath:       repoPath,
		accountKeypair: accountKeypair,
		deviceKeypair:  deviceKeypair,
	}
}

type Wallet interface {
	RootPath() string
	RepoPath() string
	GetAccountPrivkey() (walletUtil.Keypair, error)
	GetDevicePrivkey() (walletUtil.Keypair, error)
	accountservice.Service
	app.Component
}
