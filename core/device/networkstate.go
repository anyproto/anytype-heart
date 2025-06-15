package device

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "networkState"

type NetworkState interface {
	app.Component
	app.ComponentStatable
	GetNetworkState() model.DeviceNetworkType
	SetNetworkState(networkState model.DeviceNetworkType)
	RegisterHook(hook func(network model.DeviceNetworkType))
}

const networkInvalid = time.Second * 10

type networkState struct {
	networkState          model.DeviceNetworkType
	networkMu             sync.Mutex
	lastDeviceState       domain.CompState
	lastDeviceStateChange time.Time

	onNetworkUpdateHooks []func(network model.DeviceNetworkType)
	hookMu               sync.Mutex
	pool                 pool.Service
}

func (n *networkState) StateChange(state int) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	// ioslogger.DebugLog(fmt.Sprintf("change network state to %d", state))
	devState := domain.CompState(state)
	timePassed := time.Since(n.lastDeviceStateChange)
	if n.lastDeviceState != devState && devState == domain.CompStateAppWentForeground && timePassed > networkInvalid {
		err := n.pool.Flush(context.Background())
		if err != nil {
			log.Debug("failed to flush pool on network state change", zap.Error(err))
		}
		// ioslogger.DebugLog("flushed pool on network state change")
	}
	n.lastDeviceStateChange = time.Now()
	n.lastDeviceState = devState
}

func New() NetworkState {
	return &networkState{}
}

func (n *networkState) Init(a *app.App) (err error) {
	n.pool = a.MustComponent(pool.CName).(pool.Service)
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
	defer n.networkMu.Unlock()

	if n.networkState == networkState {
		// to avoid unnecessary hook calls
		return
	}
	n.networkState = networkState
	n.runOnNetworkUpdateHook()
}

func (n *networkState) RegisterHook(hook func(network model.DeviceNetworkType)) {
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
