package localdiscovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/libp2p/zeroconf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/device/mock_device"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver/mock_clientserver"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	LocalDiscovery
	nodeConf      *mock_nodeconf.MockService
	eventSender   *mock_event.MockSender
	clientServer  *mock_clientserver.MockClientServer
	deviceService *mock_device.MockNetworkState
	account       *mock_accountservice.MockService
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	c := &config.Config{}
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	eventSender := mock_event.NewMockSender(t)
	deviceService := mock_device.NewMockNetworkState(t)
	clientServer := mock_clientserver.NewMockClientServer(t)
	accountKeys, err := accountdata.NewRandom()
	assert.Nil(t, err)

	account := mock_accountservice.NewAccountServiceWithAccount(ctrl, accountKeys)
	a := &app.App{}
	ctx := context.Background()
	a.Register(c).
		Register(testutil.PrepareMock(ctx, a, nodeConf)).
		Register(testutil.PrepareMock(ctx, a, eventSender)).
		Register(account).
		Register(testutil.PrepareMock(ctx, a, deviceService)).
		Register(testutil.PrepareMock(ctx, a, clientServer))

	discovery := New()
	err = discovery.Init(a)
	assert.Nil(t, err)

	f := &fixture{
		LocalDiscovery: discovery,
		nodeConf:       nodeConf,
		eventSender:    eventSender,
		clientServer:   clientServer,
		deviceService:  deviceService,
		account:        account,
	}
	return f
}

func TestLocalDiscovery_Init(t *testing.T) {
	t.Run("init success", func(t *testing.T) {
		// given
		f := newFixture(t)
		// when
		f.clientServer.EXPECT().ServerStarted().Return(true)
		f.clientServer.EXPECT().Port().Return(6789)
		f.deviceService.EXPECT().RegisterHook(mock.Anything).Return()

		err := f.Run(context.Background())
		assert.Nil(t, err)

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestLocalDiscovery_checkAddrs(t *testing.T) {
	t.Run("refreshInterfaces - server run successfully", func(t *testing.T) {
		// given
		f := newFixture(t)

		// when
		ld := f.LocalDiscovery.(*localDiscovery)
		ld.port = 6789
		err := ld.refreshInterfaces(context.Background())

		// then
		assert.Nil(t, err)
	})
	t.Run("refreshInterfaces - server run successfully and send update to peer to peer status hook", func(t *testing.T) {
		// given
		f := newFixture(t)

		// when
		ld := f.LocalDiscovery.(*localDiscovery)
		var hookCalled atomic.Int64
		ld.RegisterDiscoveryPossibilityHook(func(state DiscoveryPossibility) {
			hookCalled.Store(int64(state))
		})
		ld.port = 6789
		err := ld.refreshInterfaces(context.Background())

		// then
		assert.Nil(t, err)
		assert.True(t, hookCalled.Load() == int64(DiscoveryPossible))
	})
}

func TestLocalDiscovery_refreshRetriesFailedServerStart(t *testing.T) {
	// given: the first refresh fails to start the mdns server (port 0 is
	// rejected by RegisterProxy), simulating a transient start failure
	f := newFixture(t)
	f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
	ld := f.LocalDiscovery.(*localDiscovery)
	ld.port = 0
	err := ld.refreshInterfaces(context.Background())
	assert.Error(t, err)

	// when: the failure condition is gone but the addresses are unchanged
	ld.port = 6789
	err = ld.refreshInterfaces(context.Background())

	// then: the next tick must retry instead of treating the state as done
	assert.Nil(t, err)
	ld.m.Lock()
	server := ld.server
	ld.m.Unlock()
	assert.NotNil(t, server, "failed server start was never retried")
	assert.Nil(t, f.Close(context.Background()))
}

func TestLocalDiscovery_refreshKeepsServerOnEnumerationError(t *testing.T) {
	// given: a healthy running server
	f := newFixture(t)
	f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
	ld := f.LocalDiscovery.(*localDiscovery)
	ld.port = 6789
	err := ld.refreshInterfaces(context.Background())
	assert.Nil(t, err)
	ld.m.Lock()
	assert.NotNil(t, ld.server)
	ld.m.Unlock()

	// when: interface enumeration fails transiently
	origGetter := getInterfacesAddrs
	getInterfacesAddrs = func() (addrs.InterfacesAddrs, error) {
		return addrs.InterfacesAddrs{}, fmt.Errorf("transient enumeration failure")
	}
	t.Cleanup(func() { getInterfacesAddrs = origGetter })
	err = ld.refreshInterfaces(context.Background())

	// then: the error is surfaced and the running server is not torn down
	assert.Error(t, err)
	ld.m.Lock()
	server := ld.server
	ld.m.Unlock()
	assert.NotNil(t, server, "transient enumeration error must not tear down the server")
	getInterfacesAddrs = origGetter
	assert.Nil(t, f.Close(context.Background()))
}

func TestLocalDiscovery_refreshSurvivesStuckQueryGoroutine(t *testing.T) {
	// given
	origTimeout := queryStopTimeout
	queryStopTimeout = 200 * time.Millisecond
	t.Cleanup(func() { queryStopTimeout = origTimeout })

	f := newFixture(t)
	f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
	ld := f.LocalDiscovery.(*localDiscovery)
	ld.port = 6789
	err := ld.refreshInterfaces(context.Background())
	assert.Nil(t, err)

	// simulate a query goroutine of the current generation that never exits
	// (e.g. an entries channel that is never closed)
	ld.m.Lock()
	ld.closeWait.Add(1)
	ld.m.Unlock()

	// when: a forced rebuild tears the current generation down
	done := make(chan struct{})
	go func() {
		defer close(done)
		ld.onNetworkStateChanged()
	}()

	// then: the refresh must not wedge on the stuck goroutine
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("refreshInterfaces wedged on a stuck query goroutine")
	}
	assert.Nil(t, f.Close(context.Background()))
}

func TestLocalDiscovery_readAnswers(t *testing.T) {
	t.Run("readAnswers - send peer update for itself", func(t *testing.T) {
		// given
		f := newFixture(t)
		notifier := NewMockNotifier(t)
		f.LocalDiscovery.SetNotifier(notifier)

		// when
		ld := f.LocalDiscovery.(*localDiscovery)
		peerUpdate := make(chan *zeroconf.ServiceEntry)
		closeWait := &sync.WaitGroup{}
		closeWait.Add(1)
		go func() {
			peerUpdate <- &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{
					Instance: ld.peerId,
				},
			}
			close(peerUpdate)
		}()
		ld.readAnswers(closeWait, peerUpdate)

		// then
		notifier.AssertNotCalled(t, "PeerDiscovered")
	})
	t.Run("readAnswers - send peer update to notifier", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
		f.clientServer.EXPECT().Port().Return(6789).Maybe()

		// when
		ld := f.LocalDiscovery.(*localDiscovery)
		peerUpdate := make(chan *zeroconf.ServiceEntry)

		notifier := NewMockNotifier(t)
		accountKeys, err := accountdata.NewRandom()
		assert.Nil(t, err)

		expectedPeer := DiscoveredPeer{
			PeerId: accountKeys.PeerId,
		}
		var called = make(chan struct{})
		notifier.EXPECT().PeerDiscovered(mock.Anything, expectedPeer, mock.Anything).Run(func(ctx context.Context, peer DiscoveredPeer, own OwnAddresses) {
			close(called)
		})

		notifier.EXPECT().PeerDiscovered(mock.Anything, DiscoveredPeer{
			PeerId: accountKeys.PeerId,
		}, mock.Anything).Return()
		ld.SetNotifier(notifier)

		closeWait := &sync.WaitGroup{}
		closeWait.Add(1)
		go func() {
			peerUpdate <- &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{
					Instance: accountKeys.PeerId,
				},
			}
			close(peerUpdate)
		}()
		ld.readAnswers(closeWait, peerUpdate)

		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Errorf("peer discovery did not call peer update")
		}
	})
}
