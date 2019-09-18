package mobile

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"C"

	"github.com/golang/protobuf/proto"
	nativeconfig "github.com/ipfs/go-ipfs-config"
	tconfig "github.com/textileio/go-textile/repo/config"

	"github.com/requilence/go-anytype/core"
	"github.com/requilence/go-anytype/pb"
	"github.com/textileio/go-textile/keypair"
	tmobile "github.com/textileio/go-textile/mobile"
)

type mobile struct {
	*tmobile.Mobile
	*core.Anytype
}

type messenger struct{
}

func (msg *messenger) Notify(event *tmobile.Event){
	fmt.Printf("notify: %s\n", event.Name)
}

type сonfig struct{
	RepoPath string
}

var cfg = сonfig{}
var anytype *mobile

func SetRepoPath(path string) {
	cfg = сonfig{path}
}

func ListAccounts() ([]byte) {
	repos, err := ioutil.ReadDir(cfg.RepoPath)
	if err != nil {
		return nil
	}

	var accounts []string
	for _, f := range repos {
		if len(f.Name()) == 48 {
			accounts = append(accounts, f.Name())
		}
	}

	r, err := proto.Marshal(&pb.StringList{Items: accounts})
	if err != nil {
		return nil
	}

	return r
}

func GenerateMnemonic(wordCount int) (string, error) {
	return tmobile.NewWallet(wordCount)
}

func WalletAccountAt(mnemonic string, index int, passphrase string) ([]byte, error) {
	return tmobile.WalletAccountAt(mnemonic, index, passphrase)
}

/*func ListAccountsForWallet(mnemonic, passphrase string) ([]byte, error) {
	var index = 0
	for {
		//return tcore.WalletAccountAt(mnemonic, index, passphrase)
	}
}*/

func InitRepo(seed string) error {
	kp, err := keypair.Parse(seed)
	if err != nil {
		return err
	}
	nativeconfig.DefaultBootstrapAddresses = []string{}
	tconfig.DefaultBootstrapAddresses = core.BootstrapNodes
	return tmobile.InitRepo(&tmobile.InitConfig{Seed: seed, RepoPath: filepath.Join(cfg.RepoPath, kp.Address()), Debug: true})
}

func StartAccount(account string) error {

	msg := messenger{}
	tm, err := tmobile.NewTextile(&tmobile.RunConfig{filepath.Join(cfg.RepoPath, account), true, nil}, &msg)
	if err != nil {
		return err
	}

	anytype = &mobile{tm, core.New(tm.Node())}

	return anytype.Run()
}
