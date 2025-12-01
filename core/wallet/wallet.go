package wallet

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CName         = accountservice.CName
	keyFileDevice = "device.key"
)

type EthPrivateKey = *ecdsa.PrivateKey
type EthAddress = common.Address

var EmptyEthereumAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")

type wallet struct {
	rootPath      string
	repoPath      string // other components will init their files/dirs inside
	lang          string
	deviceKeyPath string

	accountKey    crypto.PrivKey
	deviceKey     crypto.PrivKey
	masterKey     crypto.PrivKey
	oldAccountKey crypto.PrivKey

	// this key is used to sign ethereum transactions
	// and use Any Naming Service
	ethereumKey ecdsa.PrivateKey

	// this is needed for any-sync
	accountData *accountdata.AccountKeys
}

func (r *wallet) GetAccountPrivkey() crypto.PrivKey {
	return r.accountData.SignKey
}

func (r *wallet) GetDevicePrivkey() crypto.PrivKey {
	return r.accountData.PeerKey
}

func (r *wallet) GetOldAccountKey() crypto.PrivKey {
	return r.oldAccountKey
}

func (r *wallet) GetMasterKey() crypto.PrivKey {
	return r.masterKey
}

func (r *wallet) GetAccountEthPrivkey() *ecdsa.PrivateKey {
	return &r.ethereumKey
}

func (r *wallet) GetAccountEthAddress() EthAddress {
	key := r.ethereumKey
	if key == (ecdsa.PrivateKey{}) {
		return EmptyEthereumAddress
	}

	publicKey := r.ethereumKey.Public()

	// eat the error, we know it's an ecdsa.PublicKey
	publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)
	ethAddress := ethcrypto.PubkeyToAddress(*publicKeyECDSA)

	return common.HexToAddress(ethAddress.String())
}

func (r *wallet) Init(a *app.App) (err error) {
	if r.accountKey == nil {
		return fmt.Errorf("no account key present")
	}
	var b []byte
	if r.deviceKey == nil {
		if r.deviceKeyPath == "" {
			return fmt.Errorf("no path for device key")
		}
		b, err = ioutil.ReadFile(r.deviceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read device keyfile: %w", err)
		}
		dec, err := r.accountKey.Decrypt(b)
		if err != nil {
			return fmt.Errorf("failed to decrypt device keyfile: %w", err)
		}
		r.deviceKey, err = crypto.UnmarshalEd25519PrivateKeyProto(dec)
		if err != nil {
			return fmt.Errorf("failed to unmarshall device keyfile: %w", err)
		}
	}

	err = os.MkdirAll(filepath.Join(r.repoPath, appLinkKeysDirectory), 0700)
	if err != nil {
		return fmt.Errorf("failed to create app link directory: %w", err)
	}

	peerId := r.deviceKey.GetPublic().PeerId()
	accountId := r.accountKey.GetPublic().Account()
	logging.SetHost(peerId)
	metrics.Service.SetDeviceId(peerId)
	logging.SetAccount(accountId)
	metrics.Service.SetUserId(accountId)

	r.accountData = accountdata.New(r.deviceKey, r.accountKey)
	return nil
}

func (r *wallet) RepoPath() string {
	return r.repoPath
}

func (r *wallet) FtsPrimaryLang() string {
	return r.lang
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

func NewWithAccountRepo(rootPath string, derivationResult crypto.DerivationResult, lang string) Wallet {
	accountId := derivationResult.Identity.GetPublic().Account()
	repoPath := filepath.Join(rootPath, accountId)
	return &wallet{
		rootPath:      rootPath,
		repoPath:      repoPath,
		lang:          lang,
		masterKey:     derivationResult.MasterKey,
		oldAccountKey: derivationResult.OldAccountKey,
		accountKey:    derivationResult.Identity,
		deviceKeyPath: filepath.Join(repoPath, keyFileDevice),
		ethereumKey:   derivationResult.EthereumIdentity,
	}
}

func NewWithRepoDirAndRandomKeys(repoPath string) Wallet {
	pk1, _, _ := crypto.GenerateRandomEd25519KeyPair()
	pk2, _, _ := crypto.GenerateRandomEd25519KeyPair()

	return NewWithRepoPathAndKeys(repoPath, pk1, pk2)
}
func NewWithRepoPathAndKeys(repoPath string, accountKeypair, deviceKeypair crypto.PrivKey) Wallet {
	return &wallet{
		repoPath:   repoPath,
		accountKey: accountKeypair,
		deviceKey:  deviceKeypair,
	}
}

type Wallet interface {
	RootPath() string
	RepoPath() string
	FtsPrimaryLang() string
	GetAccountPrivkey() crypto.PrivKey
	GetDevicePrivkey() crypto.PrivKey
	GetOldAccountKey() crypto.PrivKey
	GetMasterKey() crypto.PrivKey

	GetAccountEthPrivkey() EthPrivateKey
	GetAccountEthAddress() EthAddress

	ReadAppLink(appKey string) (*AppLinkInfo, error)
	PersistAppLink(name string, scope model.AccountAuthLocalApiScope) (appInfo *AppLinkInfo, err error)
	ListAppLinks() ([]*AppLinkInfo, error)
	RevokeAppLink(appHash string) error

	accountservice.Service
	app.Component
}
