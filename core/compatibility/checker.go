package compatibility

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/secureservice"
)

const CName = "core.compatibility.checker"

type Checker interface {
	app.Component
	AddPeerVersion(peerId string, version uint32)
	IsVersionCompatibleWithPeers() bool
}

func New() Checker {
	return &compatibilityChecker{peersToVersions: make(map[string]uint32, 0)}
}

type compatibilityChecker struct {
	peersToVersions map[string]uint32
	sync.Mutex
}

func (i *compatibilityChecker) Init(a *app.App) (err error) {
	return
}

func (i *compatibilityChecker) AddPeerVersion(peerId string, version uint32) {
	i.Lock()
	defer i.Unlock()
	i.peersToVersions[peerId] = version
}

func (i *compatibilityChecker) IsVersionCompatibleWithPeers() bool {
	i.Lock()
	defer i.Unlock()
	for _, version := range i.peersToVersions {
		if version != secureservice.ProtoVersion {
			return false
		}
	}
	return true
}

func (i *compatibilityChecker) Name() (name string) {
	return CName
}
