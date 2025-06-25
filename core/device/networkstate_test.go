package device

import (
	"context"
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

type networkStateFixture struct {
	*networkState
	a             *app.App
	mockRefresher *mock_device.MockopenedObjectRefresher
	mockPool      *mock_pool.MockService
}

var ctx = context.Background()

func newNetworkStateFixture(t *testing.T) *networkStateFixture {
	ctrl := gomock.NewController(t)
	mockRefresher := mock_device.NewMockopenedObjectRefresher(t)
	mockPool := mock_pool.NewMockService(ctrl)
	a := &app.App{}
	ns := New().(*networkState)
	a.Register(testutil.PrepareMock(ctx, a, mockRefresher)).
		Register(testutil.PrepareMock(ctx, a, mockPool)).
		Register(ns)
	require.NoError(t, a.Start(ctx))
	return &networkStateFixture{
		networkState:  ns,
		a:             a,
		mockRefresher: mockRefresher,
		mockPool:      mockPool,
	}
}

func TestNetworkState_SetDeviceState(t *testing.T) {
	t.Run("set device state background -> foreground, more than networkInvalid", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time {
			return startTime
		}
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(networkInvalid + time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
	})
	t.Run("set device state background -> foreground, less than networkInvalid", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time {
			return startTime
		}
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
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
