//go:build !ios
// +build !ios

package localdiscovery

import "github.com/anyproto/anytype-heart/net/addrs"

func isP2PPossible(newAddrs addrs.InterfacesAddrs) bool {
	return len(newAddrs.Interfaces) > 0 && !addrs.IsLoopBack(newAddrs.Interfaces)
}
