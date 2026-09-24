package api

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fixture struct {
	*apiService
	eventService *mock_apicore.MockEventService
}

func newFixture(t *testing.T) *fixture {
	accountService := mock_apicore.NewMockAccountService(t)
	accountService.EXPECT().GetInfo(mock.Anything).Return(&model.AccountInfo{TechSpaceId: "techSpace1"}, nil).Maybe()
	eventService := mock_apicore.NewMockEventService(t)

	svc := &apiService{
		mw:             mock_apicore.NewMockClientCommands(t),
		accountService: accountService,
		eventService:   eventService,
	}
	return &fixture{apiService: svc, eventService: eventService}
}

// dialSucceeds reports whether addr currently accepts a TCP connection.
func dialSucceeds(t *testing.T, addr string) bool {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestStartServer(t *testing.T) {
	t.Run("disabled: empty listen address does nothing and publishes nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		status := fx.startServer("")

		// then
		assert.Nil(t, status)
	})

	t.Run("listen failure reports a failed status and publishes the same event", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		occupiedAddr := ln.Addr().String()

		var published *pb.EventAccountJsonApiStatus
		fx.eventService.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			published = event.Messages[0].Value.(*pb.EventMessageValueOfAccountJsonApiStatus).AccountJsonApiStatus
		}).Once()

		// when
		status := fx.startServer(occupiedAddr)

		// then
		require.NotNil(t, status)
		assert.False(t, status.Success)
		assert.Equal(t, occupiedAddr, status.ListenAddr)
		assert.NotEmpty(t, status.Error)
		assert.Equal(t, status, published)
	})

	t.Run("successful bind reports the OS-confirmed effective address", func(t *testing.T) {
		// given
		fx := newFixture(t)

		var published *pb.EventAccountJsonApiStatus
		fx.eventService.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			published = event.Messages[0].Value.(*pb.EventMessageValueOfAccountJsonApiStatus).AccountJsonApiStatus
		}).Once()

		// when
		status := fx.startServer("127.0.0.1:0")
		if fx.httpSrv != nil {
			defer fx.httpSrv.Close()
		}

		// then
		require.NotNil(t, status)
		assert.True(t, status.Success)
		assert.NotEqual(t, "127.0.0.1:0", status.ListenAddr, "the effective address reports the OS-assigned port, not the requested wildcard")
		assert.Empty(t, status.Error)
		assert.Equal(t, status, published)
	})
}

func TestRun(t *testing.T) {
	t.Run("a bind failure never fails the component lifecycle", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		fx.listenAddr = ln.Addr().String()
		fx.eventService.EXPECT().Broadcast(mock.Anything).Return().Once()

		// when
		err = fx.Run(context.Background())

		// then
		assert.NoError(t, err)
	})
}

func TestReassignAddress(t *testing.T) {
	t.Run("closes the previous listener before the new one is reachable", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.eventService.EXPECT().Broadcast(mock.Anything).Return().Twice()

		first := fx.startServer("127.0.0.1:0")
		require.NotNil(t, first)
		require.True(t, first.Success)
		require.True(t, dialSucceeds(t, first.ListenAddr), "sanity: the first bind is actually reachable")

		// when
		status, err := fx.ReassignAddress(context.Background(), "127.0.0.1:0")
		if fx.httpSrv != nil {
			defer fx.httpSrv.Close()
		}

		// then
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.True(t, status.Success)
		assert.False(t, dialSucceeds(t, first.ListenAddr), "the old address must stop accepting connections once reassigned — a regression here means the old listener is still being served (possibly through the new server's handler)")
		assert.True(t, dialSucceeds(t, status.ListenAddr), "the new address must be reachable")
	})

	t.Run("disabling (empty address) shuts down the server and publishes nothing further", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.eventService.EXPECT().Broadcast(mock.Anything).Return().Once() // only the initial bind

		first := fx.startServer("127.0.0.1:0")
		require.NotNil(t, first)
		require.True(t, first.Success)

		// when
		status, err := fx.ReassignAddress(context.Background(), "")

		// then
		require.NoError(t, err)
		assert.Nil(t, status)
		assert.False(t, dialSucceeds(t, first.ListenAddr), "disabling must close the previously bound listener")
	})

	t.Run("a bind failure on the new address is reported via status, not via error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		occupiedAddr := ln.Addr().String()
		fx.eventService.EXPECT().Broadcast(mock.Anything).Return().Once()

		// when
		status, err := fx.ReassignAddress(context.Background(), occupiedAddr)

		// then
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.False(t, status.Success)
		assert.Equal(t, occupiedAddr, status.ListenAddr)
	})
}
