package core

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	tcore "github.com/textileio/go-textile/core"
)

const privateKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

var BootstrapNodes = []string{
	"/ip4/68.183.2.167/tcp/4001/ipfs/12D3KooWE22N7rUX12WT34XsSpjMaCEuFNV3eVAo331kDgrP43WZ",
"/ip4/157.230.124.182/tcp/4001/ipfs/12D3KooWKLLf9Qc6SHaLWNPvx7Tk4AMc9i71CLdnbZuRiFMFMnEf",
}


type Anytype struct{
	*tcore.Textile
	documentsCache map[string]*Document
}

func (a *Anytype) Run() error {
	swarmKeyFilePath := filepath.Join(a.Textile.RepoPath(), "swarm.key")
	err := ioutil.WriteFile(swarmKeyFilePath, []byte(privateKey), 0644)
	if err != nil {
		return err
	}

	err =  a.Textile.Start()
	if err != nil {
		return err
	}

	err = a.Textile.Ipfs().Repo.SetConfigKey("Addresses.Bootstrap", BootstrapNodes)
	if err != nil {
		return err
	}

	go func(){
		time.Sleep(time.Second*5)
		for {
			_, err = a.RegisterCafe("12D3KooWE22N7rUX12WT34XsSpjMaCEuFNV3eVAo331kDgrP43WZ", "2M7TtjbhoTaLXyXWJJJpF6WBfFyxebCGj6pEZb3akC2hHcuzYPkFTYc9UEttE")
			if err != nil {
				log.Errorf("failed to register cafe: %s", err.Error())
				time.Sleep(time.Second*10)
				continue
			}
			break
		}
	}()

	return err
}

func New(textile *tcore.Textile) *Anytype{
	return &Anytype{
		Textile: textile,
		documentsCache: make(map[string]*Document),
	}
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
