package api

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

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
	// broadcasts receives one value per Broadcast call once expectBroadcasts
	// arms the mock. publishStatus dispatches Broadcast on its own goroutine
	// (see service.go), so tests must synchronize on this channel instead of
	// reading a plain captured variable, which would race the goroutine.
	broadcasts chan *pb.EventAccountJsonApiStatus
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
	return &fixture{apiService: svc, eventService: eventService, broadcasts: make(chan *pb.EventAccountJsonApiStatus, 8)}
}

// expectBroadcasts arms the mock for exactly n Broadcast calls, routing each
// payload onto fx.broadcasts.
func (fx *fixture) expectBroadcasts(n int) {
	fx.eventService.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
		fx.broadcasts <- event.Messages[0].Value.(*pb.EventMessageValueOfAccountJsonApiStatus).AccountJsonApiStatus
	}).Times(n)
}

// awaitBroadcast blocks for one async Broadcast call and returns its
// payload, failing the test if none arrives — bounding an otherwise
// unbounded wait on a goroutine the test doesn't control.
func (fx *fixture) awaitBroadcast(t *testing.T) *pb.EventAccountJsonApiStatus {
	t.Helper()
	select {
	case status := <-fx.broadcasts:
		return status
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the async status broadcast")
		return nil
	}
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
		fx.expectBroadcasts(1)

		// when
		status := fx.startServer(occupiedAddr)

		// then
		require.NotNil(t, status)
		assert.False(t, status.Success)
		assert.Equal(t, occupiedAddr, status.ListenAddr)
		assert.NotEmpty(t, status.Error)
		assert.Equal(t, status, fx.awaitBroadcast(t))
	})

	t.Run("successful bind reports the OS-confirmed effective address", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.expectBroadcasts(1)

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
		assert.Equal(t, status, fx.awaitBroadcast(t))
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
		fx.expectBroadcasts(1)

		// when
		err = fx.Run(context.Background())

		// then
		assert.NoError(t, err)
		fx.awaitBroadcast(t)
	})
}

func TestReassignAddress(t *testing.T) {
	t.Run("closes the previous listener before the new one is reachable", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.expectBroadcasts(2)

		first := fx.startServer("127.0.0.1:0")
		require.NotNil(t, first)
		require.True(t, first.Success)
		fx.awaitBroadcast(t)

		// Reserve a second, guaranteed-different port before reassigning:
		// two live listeners can never share a port, which rules out the OS
		// coincidentally reusing first's port for the new bind — a reused
		// port would still (correctly) serve via the new server, which would
		// make the "old address unreachable" assertion below fail for the
		// wrong reason (a false regression signal, not a real one).
		placeholder, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		newAddr := placeholder.Addr().String()
		require.NoError(t, placeholder.Close())

		// when
		status, err := fx.ReassignAddress(context.Background(), newAddr)
		if fx.httpSrv != nil {
			defer fx.httpSrv.Close()
		}

		// then
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.True(t, status.Success)
		assert.Equal(t, newAddr, status.ListenAddr)
		fx.awaitBroadcast(t)
		assert.False(t, dialSucceeds(t, first.ListenAddr), "the old address must stop accepting connections once reassigned — a regression here means the old listener is still being served (possibly through the new server's handler)")
		assert.True(t, dialSucceeds(t, status.ListenAddr), "the new address must be reachable")
	})

	t.Run("disabling (empty address) shuts down the server and publishes nothing further", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.expectBroadcasts(1) // only the initial bind

		first := fx.startServer("127.0.0.1:0")
		require.NotNil(t, first)
		require.True(t, first.Success)
		fx.awaitBroadcast(t)

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
		fx.expectBroadcasts(1)

		// when
		status, err := fx.ReassignAddress(context.Background(), occupiedAddr)

		// then
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.False(t, status.Success)
		assert.Equal(t, occupiedAddr, status.ListenAddr)
		fx.awaitBroadcast(t)
	})

	t.Run("publishStatus does not deadlock a callback that re-enters the service", func(t *testing.T) {
		// given: a "callback" that synchronously acquires s.lock, simulating
		// a mobile client reacting to the event by calling straight back
		// into this service. Broadcast runs before ReassignAddress's own
		// deferred unlock fires, so if publishStatus ever stopped
		// dispatching it on its own goroutine, this callback would run on
		// the SAME goroutine that already holds s.lock and self-deadlock on
		// a non-reentrant mutex — ReassignAddress would then never return.
		fx := newFixture(t)
		reentered := make(chan struct{}, 1)
		fx.eventService.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			fx.lock.Lock()
			fx.lock.Unlock()
			reentered <- struct{}{}
		}).Once()

		// when
		done := make(chan error, 1)
		go func() {
			_, err := fx.ReassignAddress(context.Background(), "127.0.0.1:0")
			done <- err
		}()

		// then
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("ReassignAddress deadlocked — publishStatus must dispatch Broadcast off the goroutine that holds s.lock")
		}
		select {
		case <-reentered:
		case <-time.After(2 * time.Second):
			t.Fatal("the re-entrant callback never ran")
		}
		if fx.httpSrv != nil {
			defer fx.httpSrv.Close()
		}
	})

	t.Run("overlapping reassignments never orphan a listener", func(t *testing.T) {
		// given: N goroutines reassign concurrently. AccountChangeJsonApiAddr's
		// caller only takes a read lock at the application layer, so nothing
		// prevents this in production — if shutdown+bind were ever split back
		// into two separately-locked steps, one goroutine's bind could
		// silently overwrite another's still-live s.httpSrv/s.listener,
		// leaking a listener nothing can track or close.
		const n = 16
		fx := newFixture(t)
		fx.expectBroadcasts(n)

		addrs := make([]string, n)
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func(i int) {
				defer wg.Done()
				status, err := fx.ReassignAddress(context.Background(), "127.0.0.1:0")
				assert.NoError(t, err)
				if assert.NotNil(t, status) {
					assert.True(t, status.Success)
					addrs[i] = status.ListenAddr
				}
			}(i)
		}
		wg.Wait()
		for i := 0; i < n; i++ {
			fx.awaitBroadcast(t)
		}
		if fx.httpSrv != nil {
			defer fx.httpSrv.Close()
		}

		// then: whichever bind ended up current, every other address must be
		// closed — nothing should still be listening except the final one.
		fx.lock.Lock()
		finalAddr := ""
		if fx.listener != nil {
			finalAddr = fx.listener.Addr().String()
		}
		fx.lock.Unlock()
		require.NotEmpty(t, finalAddr)

		for _, addr := range addrs {
			if addr == "" || addr == finalAddr {
				continue
			}
			assert.False(t, dialSucceeds(t, addr), "address %s is still reachable after reassignment — a listener leaked", addr)
		}
	})
}
