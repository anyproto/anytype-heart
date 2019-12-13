package core

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	ipfsCore "github.com/ipfs/go-ipfs/core"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	tcore "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/gateway"
	tmobile "github.com/textileio/go-textile/mobile"
)

var log = logging.Logger("anytype-core")

const privateKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

var BootstrapNodes = []string{
	"/ip4/68.183.2.167/tcp/4001/ipfs/12D3KooWB2Ya2GkLLRSR322Z13ZDZ9LP4fDJxauscYwUMKLFCqaD",
	"/ip4/157.230.124.182/tcp/4001/ipfs/12D3KooWKLLf9Qc6SHaLWNPvx7Tk4AMc9i71CLdnbZuRiFMFMnEf",
}

type PredefinedBlockIds struct {
	Home    string
	Archive string
}

type Anytype struct {
	Textile            *tmobile.Mobile
	predefinedBlockIds PredefinedBlockIds
}

func (a *Anytype) ipfs() *ipfsCore.IpfsNode {
	return a.Textile.Node().Ipfs()
}

func (a *Anytype) textile() *tcore.Textile {
	return a.Textile.Node()
}

// PredefinedBlockIds returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Run()
func (a *Anytype) PredefinedBlockIds() PredefinedBlockIds {
	return a.predefinedBlockIds
}

func New(repoPath string, account string) (*Anytype, error) {
	// todo: remove this temp workaround after release of go-ipfs v0.4.23
	crypto.MinRsaKeyBits = 1024

	msg := messenger{}
	tm, err := tmobile.NewTextile(&tmobile.RunConfig{filepath.Join(repoPath, account), false, nil}, &msg)
	if err != nil {
		return nil, err
	}

	a := &Anytype{Textile: tm}
	a.SetDebug(true)
	return a, nil
}

func (a *Anytype) SetDebug(debug bool) {
	if debug {
		logging.SetLogLevel("anytype-core", "DEBUG")
	} else {
		logging.SetLogLevel("anytype-core", "WARNING")
	}
}

func (a *Anytype) Run() error {
	swarmKeyFilePath := filepath.Join(a.textile().RepoPath(), "swarm.key")
	err := ioutil.WriteFile(swarmKeyFilePath, []byte(privateKey), 0644)
	if err != nil {
		return err
	}

	err = a.Textile.Start()
	if err != nil {
		return err
	}

	err = a.ipfs().Repo.SetConfigKey("Addresses.Bootstrap", BootstrapNodes)
	if err != nil {
		return err
	}

	go func() {
		for {
			if !a.textile().Started() {
				break
			}

			if !a.ipfs().IsOnline {
				time.Sleep(time.Second)
				continue
			}

			_, err = a.textile().RegisterCafe("12D3KooWB2Ya2GkLLRSR322Z13ZDZ9LP4fDJxauscYwUMKLFCqaD", "2MsR9h7mfq53oNt8vh7RfdPr57qPsn28X3dwbviZWs3E8kEu6kpdcDHyMx7Qo")
			if err != nil {
				log.Errorf("failed to register cafe: %s", err.Error())
				time.Sleep(time.Second * 5)
				continue
			}
			break
		}
	}()

	// start IPFS gateway
	gateway.Host = &gateway.Gateway{
		Node: a.Textile.Node(),
	}
	gateway.Host.Start(a.Textile.Node().Config().Addresses.Gateway)
	fmt.Println("Gateway: " + a.Textile.Node().Config().Addresses.Gateway)

	err = a.createPredefinedBlocks()
	if err != nil {
		return err
	}

	return nil
}

func (a *Anytype) Stop() error {
	return a.textile().Stop()
}
