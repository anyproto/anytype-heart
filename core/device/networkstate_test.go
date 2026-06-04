package device

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/device/mock_device"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

// spaceSyncerStub is a minimal spaceHeadSyncer that counts SyncAllSpaceHeads calls.
type spaceSyncerStub struct {
	mu     sync.Mutex
	called int
}

func (s *spaceSyncerStub) SyncAllSpaceHeads(context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
}
func (s *spaceSyncerStub) Name() string        { return "spaceSyncerStub" }
func (s *spaceSyncerStub) Init(*app.App) error { return nil }
func (s *spaceSyncerStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

type networkStateFixture struct {
	*networkState
	a             *app.App
	mockRefresher *mock_device.MockopenedObjectRefresher
	mockPool      *mock_pool.MockService
	syncer        *spaceSyncerStub
}

var ctx = context.Background()

func newNetworkStateFixture(t *testing.T) *networkStateFixture {
	ctrl := gomock.NewController(t)
	mockRefresher := mock_device.NewMockopenedObjectRefresher(t)
	mockPool := mock_pool.NewMockService(ctrl)
	syncer := &spaceSyncerStub{}
	a := &app.App{}
	ns := New().(*networkState)
	a.Register(testutil.PrepareMock(ctx, a, mockRefresher)).
		Register(testutil.PrepareMock(ctx, a, mockPool)).
		Register(syncer).
		Register(ns)
	require.NoError(t, a.Start(ctx))
	return &networkStateFixture{
		networkState:  ns,
		a:             a,
		mockRefresher: mockRefresher,
		mockPool:      mockPool,
		syncer:        syncer,
	}
}

func TestNetworkState_SetDeviceState(t *testing.T) {
	// foreground resume always refreshes opened objects; pool flush and head-sync
	// are gated by how long the app was backgrounded.
	t.Run("backgrounded longer than syncHeadsAfter: flush pool and sync all heads", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time { return startTime }
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(syncHeadsAfter + time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("backgrounded between poolFlushAfter and syncHeadsAfter: flush pool, no head-sync", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time { return startTime }
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(poolFlushAfter + time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 0, fx.syncer.callCount())
	})
	t.Run("backgrounded less than poolFlushAfter: no flush, no head-sync", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time { return startTime }
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(poolFlushAfter - time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 0, fx.syncer.callCount())
	})
}

func TestNetworkState_SetNetworkState(t *testing.T) {
	t.Run("set network state", func(t *testing.T) {
		// given
		state := &networkState{}

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR)

		// then
		assert.Equal(t, model.DeviceNetworkType_CELLULAR, state.networkState)
	})
	t.Run("update network state", func(t *testing.T) {
		// given
		state := &networkState{}

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR)
		state.SetNetworkState(model.DeviceNetworkType_WIFI)

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, state.networkState)
	})
	t.Run("update network state with hook", func(t *testing.T) {
		// given
		state := &networkState{}
		var hookState model.DeviceNetworkType
		h := func(state model.DeviceNetworkType) {
			hookState = state
		}
		state.RegisterHook(h)

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR)
		state.SetNetworkState(model.DeviceNetworkType_WIFI)

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, state.networkState)
		assert.Equal(t, model.DeviceNetworkType_WIFI, hookState)
	})
}

func TestNetworkState_GetNetworkState(t *testing.T) {
	t.Run("get default network state", func(t *testing.T) {
		// given
		state := New()

		// when
		networkType := state.GetNetworkState()

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, networkType)
	})
	t.Run("get updated network state", func(t *testing.T) {
		// given
		state := New()

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR)
		networkType := state.GetNetworkState()

		// then
		assert.Equal(t, model.DeviceNetworkType_CELLULAR, networkType)
	})
}
