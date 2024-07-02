package device

import (
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "networkState"

type OnNetworkUpdateHook func(network model.DeviceNetworkType)

type NetworkState interface {
	app.Component
	GetNetworkState() model.DeviceNetworkType
	SetNetworkState(networkState model.DeviceNetworkType)
	RegisterHook(hook OnNetworkUpdateHook)
}

type networkState struct {
	networkState model.DeviceNetworkType
	networkMu    sync.Mutex

	onNetworkUpdateHooks []OnNetworkUpdateHook
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
	n.networkState = networkState
	defer n.networkMu.Unlock()
	n.runOnNetworkUpdateHook()
}

func (n *networkState) RegisterHook(hook OnNetworkUpdateHook) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	n.onNetworkUpdateHooks = append(n.onNetworkUpdateHooks, hook)
}

func (n *networkState) runOnNetworkUpdateHook() {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	for _, hook := range n.onNetworkUpdateHooks {
		hook(n.networkState)
	}
}
