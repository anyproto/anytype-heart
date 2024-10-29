package device

import (
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "networkState"

type NetworkState interface {
	app.Component
	GetNetworkState() model.DeviceNetworkType
	SetNetworkState(networkState model.DeviceNetworkType)
	RegisterHook(hook func(network model.DeviceNetworkType))
}

type networkState struct {
	networkState model.DeviceNetworkType
	networkMu    sync.Mutex

	onNetworkUpdateHooks []func(network model.DeviceNetworkType)
	hookMu               sync.Mutex
}

func New() NetworkState {
	return &networkState{}
}

func (n *networkState) Init(a *app.App) (err error) {
	return
}

func (n *networkState) Name() (name string) {
	return CName
}

func (n *networkState) GetNetworkState() model.DeviceNetworkType {
	n.networkMu.Lock()
	defer n.networkMu.Unlock()
	return n.networkState
}

func (n *networkState) SetNetworkState(networkState model.DeviceNetworkType) {
	n.networkMu.Lock()
	if n.networkState == networkState {
		// to avoid unnecessary hook calls
		return
	}
	n.networkState = networkState
	n.networkMu.Unlock()
	n.runOnNetworkUpdateHook(networkState)
}

func (n *networkState) RegisterHook(hook func(network model.DeviceNetworkType)) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	n.onNetworkUpdateHooks = append(n.onNetworkUpdateHooks, hook)
}

func (n *networkState) runOnNetworkUpdateHook(networkState model.DeviceNetworkType) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	for _, hook := range n.onNetworkUpdateHooks {
		hook(networkState)
	}
}
