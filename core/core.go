package core

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	tmobile "github.com/textileio/go-textile/mobile"
)

const privateKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

var BootstrapNodes = []string{
	"/ip4/68.183.2.167/tcp/4001/ipfs/12D3KooWE22N7rUX12WT34XsSpjMaCEuFNV3eVAo331kDgrP43WZ",
	"/ip4/157.230.124.182/tcp/4001/ipfs/12D3KooWKLLf9Qc6SHaLWNPvx7Tk4AMc9i71CLdnbZuRiFMFMnEf",
}

func init() {
	// todo: remove this temp workaround after release of go-ipfs v0.4.23
	os.Setenv("LIBP2P_ALLOW_WEAK_RSA_KEYS", "1")
}

type Anytype struct {
	Textile        *tmobile.Mobile
	documentsCache map[string]*Document
}

func New(repoPath string, account string) (*Anytype, error) {
	msg := messenger{}
	tm, err := tmobile.NewTextile(&tmobile.RunConfig{filepath.Join(repoPath, account), true, nil}, &msg)
	if err != nil {
		return nil, err
	}

	return &Anytype{Textile: tm}, nil
}

func (a *Anytype) Run() error {
	swarmKeyFilePath := filepath.Join(a.Textile.Node().RepoPath(), "swarm.key")
	err := ioutil.WriteFile(swarmKeyFilePath, []byte(privateKey), 0644)
	if err != nil {
		return err
	}

	err = a.Textile.Start()
	if err != nil {
		return err
	}

	err = a.Textile.Node().Ipfs().Repo.SetConfigKey("Addresses.Bootstrap", BootstrapNodes)
	if err != nil {
		return err
	}

	go func() {
		// todo: need to call this when IPFS node is up
		time.Sleep(time.Second * 5)
		for {
			_, err = a.Textile.Node().RegisterCafe("12D3KooWE22N7rUX12WT34XsSpjMaCEuFNV3eVAo331kDgrP43WZ", "2M7TtjbhoTaLXyXWJJJpF6WBfFyxebCGj6pEZb3akC2hHcuzYPkFTYc9UEttE")
			if err != nil {
				log.Errorf("failed to register cafe: %s", err.Error())
				time.Sleep(time.Second * 10)
				continue
			}
			break
		}
	}()

	return err
}

func pbValForEnumString(vals map[string]int32, str string) int32 {
	for v, i := range vals {
		if strings.ToLower(v) == strings.ToLower(str) {
			return i
		}
	}
	return 0
}

func shortId(id string) string {
	if len(id) < 8 {
		return id
	}

	return id[len(id)-8:]
}
