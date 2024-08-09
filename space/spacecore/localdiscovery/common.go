package localdiscovery

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
)

const (
	CName = "client.space.localdiscovery"

	serviceName = "_anytype._tcp"
	mdnsDomain  = "local"
)

var log = logger.NewNamed(CName)

type DiscoveredPeer struct {
	Addrs  []string
	PeerId string
}

type OwnAddresses struct {
	Addrs []string
	Port  int
}

type Notifier interface {
	PeerDiscovered(peer DiscoveredPeer, own OwnAddresses)
}

type LocalDiscovery interface {
	SetNotifier(Notifier)
	Start() error // Start the local discovery. Used when automatic start is disabled.
	app.ComponentRunnable
}
