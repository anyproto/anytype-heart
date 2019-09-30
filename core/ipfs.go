package core

import (
	"github.com/textileio/go-textile/ipfs"
)

func (a *Anytype) Peers() (*ipfs.ConnInfos, error) {
	return ipfs.SwarmPeers(a.Textile.Node().Ipfs(), true, true, true, true)
}
