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

type openedObjectRefresher interface {
	app.Component
	RefreshOpenedObjects(ctx context.Context)
}

const networkInvalid = time.Second * 10

type networkState struct {
	networkState          model.DeviceNetworkType
	objectsRefresher      openedObjectRefresher
	networkMu             sync.Mutex
	lastDeviceState       domain.CompState
	lastDeviceStateChange time.Time

	onNetworkUpdateHooks []func(network model.DeviceNetworkType)
	hookMu               sync.Mutex
	pool                 pool.Service
}

var getTime = time.Now // for testing purposes

func (n *networkState) StateChange(state int) {
	n.hookMu.Lock()
	var (
		curTime    = getTime()
		curState   = domain.CompState(state)
		oldState   = n.lastDeviceState
		timePassed = curTime.Sub(n.lastDeviceStateChange)
	)
	n.lastDeviceStateChange = curTime
	n.lastDeviceState = curState
	n.hookMu.Unlock()
	if oldState != curState && curState == domain.CompStateAppWentForeground {
		ctx := context.Background()
		if timePassed > networkInvalid {
			err := n.pool.Flush(ctx)
			if err != nil {
				log.Debug("failed to flush pool on network state change", zap.Error(err))
			}
		}
		n.objectsRefresher.RefreshOpenedObjects(ctx)
	}
}

func New() NetworkState {
	return &networkState{}
}

func (n *networkState) Init(a *app.App) (err error) {
	n.pool = app.MustComponent[pool.Service](a)
	n.objectsRefresher = app.MustComponent[openedObjectRefresher](a)
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
