//go:build !android

package localdiscovery

import (
	"context"
	"fmt"
	gonet "net"
	"net/netip"
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
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		ld := f.LocalDiscovery.(*zeroconfDiscovery)
		ld.port = 6789
		err := ld.refreshInterfaces(context.Background())

		// then
		assert.Nil(t, err)
	})
	t.Run("refreshInterfaces - server run successfully and send update to peer to peer status hook", func(t *testing.T) {
		// given
		f := newFixture(t)

		// when
		ld := f.LocalDiscovery.(*zeroconfDiscovery)
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

func TestLocalDiscovery_networkStateHookDoesNotBlockOnSlowRefresh(t *testing.T) {
	// given: a started discovery whose refresh pipeline is busy
	// (refreshMu held simulates an in-flight teardown/rebuild)
	f := newFixture(t)
	f.clientServer.EXPECT().ServerStarted().Return(true)
	f.clientServer.EXPECT().Port().Return(6789)
	var hook func(model.DeviceNetworkType)
	f.deviceService.EXPECT().RegisterHook(mock.Anything).Run(func(h func(model.DeviceNetworkType)) {
		hook = h
	}).Return()
	err := f.Start()
	assert.Nil(t, err)

	ld := f.LocalDiscovery.(*zeroconfDiscovery)
	ld.refreshMu.Lock()

	// when: the device reports a network change (this call happens on the
	// DeviceNetworkStateSet RPC path and must stay fast)
	done := make(chan struct{})
	go func() {
		hook(model.DeviceNetworkType_CELLULAR)
		close(done)
	}()

	// then
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("network-state hook blocked behind an in-flight refresh")
	}
	ld.refreshMu.Unlock()
	assert.Nil(t, f.Close(context.Background()))
}

func TestLocalDiscovery_refreshRetriesFailedServerStart(t *testing.T) {
	// given: the first refresh fails to start the mdns server (port 0 is
	// rejected by RegisterProxy), simulating a transient start failure
	f := newFixture(t)
	f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
	ld := f.LocalDiscovery.(*zeroconfDiscovery)
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
	ld := f.LocalDiscovery.(*zeroconfDiscovery)
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
	ld := f.LocalDiscovery.(*zeroconfDiscovery)
	ld.port = 6789
	err := ld.refreshInterfaces(context.Background())
	assert.Nil(t, err)

	// simulate a query goroutine of the current generation that never exits
	// (e.g. an entries channel that is never closed), and clear the cached
	// addresses to force a full teardown + rebuild like a network change does
	ld.m.Lock()
	ld.closeWait.Add(1)
	ld.interfacesAddrs = addrs.InterfacesAddrs{}
	ld.m.Unlock()

	// when: a forced rebuild tears the current generation down
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := ld.refreshInterfaces(context.Background()); err != nil {
			t.Error(err)
		}
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
		ld := f.LocalDiscovery.(*zeroconfDiscovery)
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
	t.Run("a cable entry is reported with its link", func(t *testing.T) {
		// given: en16 (index 31) is a link-local-only interface
		f := newFixture(t)
		ld := f.LocalDiscovery.(*zeroconfDiscovery)
		ld.linkLocal = newLinkLocalIfaces(
			[]ownAddr{{iface: "en16", net: netip.MustParsePrefix("fe80::1/64"), linkLocal: true}},
			[]gonet.Interface{{Index: 31, Name: "en16"}},
		)
		notifier := NewMockNotifier(t)
		want := DiscoveredPeer{PeerId: "phone", Link: "31", Addrs: []string{"[fe80::62%31]:4006"}}
		called := make(chan struct{})
		notifier.EXPECT().PeerDiscovered(mock.Anything, want, mock.Anything).Run(func(context.Context, DiscoveredPeer, OwnAddresses) {
			close(called)
		}).Once()
		ld.SetNotifier(notifier)
		entries := make(chan *zeroconf.ServiceEntry, 1)
		entries <- &zeroconf.ServiceEntry{
			ServiceRecord: zeroconf.ServiceRecord{Instance: "phone"},
			Port:          4006,
			LinkIndex:     31,
			AddrIPv4:      []gonet.IP{gonet.ParseIP("169.254.10.20")},
			AddrIPv6Zoned: []netip.Addr{netip.MustParseAddr("fe80::62%31")},
		}
		close(entries)
		closeWait := &sync.WaitGroup{}
		closeWait.Add(1)

		// when
		ld.readAnswers(closeWait, entries)

		// then
		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Fatal("the cable entry was not reported")
		}
	})
	t.Run("readAnswers - send peer update to notifier", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.clientServer.EXPECT().ServerStarted().Return(true).Maybe()
		f.clientServer.EXPECT().Port().Return(6789).Maybe()

		// when
		ld := f.LocalDiscovery.(*zeroconfDiscovery)
		peerUpdate := make(chan *zeroconf.ServiceEntry)

		notifier := NewMockNotifier(t)
		accountKeys, err := accountdata.NewRandom()
		assert.Nil(t, err)

		expectedPeer := DiscoveredPeer{
			Addrs:  []string{"192.168.1.62:4006"},
			PeerId: accountKeys.PeerId,
		}
		var called = make(chan struct{})
		notifier.EXPECT().PeerDiscovered(mock.Anything, expectedPeer, mock.Anything).Run(func(ctx context.Context, peer DiscoveredPeer, own OwnAddresses) {
			close(called)
		}).Once()
		ld.SetNotifier(notifier)

		closeWait := &sync.WaitGroup{}
		closeWait.Add(1)
		go func() {
			// an entry with nothing to dial is skipped
			peerUpdate <- &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: accountKeys.PeerId},
				Port:          4006,
				AddrIPv6Zoned: []netip.Addr{netip.MustParseAddr("fe80::62%en0")},
			}
			peerUpdate <- &zeroconf.ServiceEntry{
				ServiceRecord: zeroconf.ServiceRecord{Instance: accountKeys.PeerId},
				Port:          4006,
				AddrIPv4:      []gonet.IP{gonet.ParseIP("192.168.1.62")},
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

func TestPeerAddrs(t *testing.T) {
	zones := newLinkLocalIfaces(
		[]ownAddr{{iface: "en16", net: netip.MustParsePrefix("fe80::a886:c6ff:febd:f923/64"), linkLocal: true}},
		[]gonet.Interface{{Index: 4, Name: "en0"}, {Index: 31, Name: "en16"}},
	).zones

	for _, tc := range []struct {
		name  string
		entry *zeroconf.ServiceEntry
		want  []string
	}{
		{
			name: "a cable entry has only its link-local addresses",
			entry: &zeroconf.ServiceEntry{
				Port:      62654,
				LinkIndex: 31,
				AddrIPv4:  []gonet.IP{gonet.ParseIP("192.168.1.62")},
				AddrIPv6Zoned: []netip.Addr{
					netip.MustParseAddr("2a0d:5600::62"),
					netip.MustParseAddr("fe80::a886:c6ff:febd:f9dc%31"),
				},
			},
			want: []string{"[fe80::a886:c6ff:febd:f9dc%31]:62654"},
		},
		{
			name: "a cable entry from a link that is not ours has nothing",
			entry: &zeroconf.ServiceEntry{
				Port:          4006,
				LinkIndex:     4,
				AddrIPv6Zoned: []netip.Addr{netip.MustParseAddr("fe80::62%4")},
			},
			want: nil,
		},
		{
			name: "an entry for no link has its ipv4 addresses in order, as on develop",
			entry: &zeroconf.ServiceEntry{
				Port:          4006,
				AddrIPv4:      []gonet.IP{gonet.ParseIP("10.20.9.9"), gonet.ParseIP("192.168.1.62")},
				AddrIPv6Zoned: []netip.Addr{netip.MustParseAddr("fe80::a886:c6ff:febd:f9dc%31")},
			},
			want: []string{"10.20.9.9:4006", "192.168.1.62:4006"},
		},
		{
			name: "an unzoned link-local is left out",
			entry: &zeroconf.ServiceEntry{
				Port:          4006,
				LinkIndex:     31,
				AddrIPv6Zoned: []netip.Addr{netip.MustParseAddr("fe80::62")},
			},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, peerAddrs(tc.entry, zones))
		})
	}
}

func TestLinkLocalIfacesIPTraffic(t *testing.T) {
	t.Run("ipv4 only without a link-local interface", func(t *testing.T) {
		ll := newLinkLocalIfaces([]ownAddr{{iface: "en0", net: netip.MustParsePrefix("192.168.1.10/24")}}, []gonet.Interface{{Index: 4, Name: "en0"}})
		assert.Equal(t, zeroconf.IPv4, ll.ipTraffic())
		assert.Empty(t, ll.indexes)
	})
	t.Run("both families with one", func(t *testing.T) {
		ll := newLinkLocalIfaces(
			[]ownAddr{{iface: "en16", net: netip.MustParsePrefix("fe80::1/64"), linkLocal: true}},
			[]gonet.Interface{{Index: 4, Name: "en0"}, {Index: 31, Name: "en16"}},
		)
		assert.Equal(t, zeroconf.IPType(zeroconf.IPv4AndIPv6), ll.ipTraffic())
		assert.Equal(t, []int{31}, ll.indexes)
	})
}

func TestInterfaceIds(t *testing.T) {
	ipNet := func(cidr string) gonet.Addr {
		ip, n, err := gonet.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		return n
	}
	wifiV4, wifiLL, cableLL := ipNet("192.168.1.10/24"), ipNet("fe80::1/64"), ipNet("fe80::a886:c6ff:febd:f923/64")
	set := func(cableIndex int, wifi, cable, tunnel []gonet.Addr) []addrs.NetInterfaceWithAddrCache {
		return []addrs.NetInterfaceWithAddrCache{
			addrs.WrapInterfaceWithAddrs(gonet.Interface{Index: 4, Name: "en0"}, wifi),
			addrs.WrapInterfaceWithAddrs(gonet.Interface{Index: cableIndex, Name: "en16"}, cable),
			addrs.WrapInterfaceWithAddrs(gonet.Interface{Index: 20, Name: "utun0"}, tunnel),
		}
	}
	base := interfaceIds("darwin", set(31, []gonet.Addr{wifiV4, wifiLL}, []gonet.Addr{cableLL}, nil))

	// then
	assert.Equal(t, base, interfaceIds("darwin", set(31, []gonet.Addr{wifiV4, wifiLL}, []gonet.Addr{cableLL}, nil)))
	assert.NotEqual(t, base, interfaceIds("darwin", set(42, []gonet.Addr{wifiV4, wifiLL}, []gonet.Addr{cableLL}, nil)), "the cable came back under a new index")
	assert.NotEqual(t, base, interfaceIds("darwin", set(31, []gonet.Addr{wifiLL}, []gonet.Addr{wifiV4, cableLL}, nil)), "the IPv4 address moved to the cable")
	assert.Equal(t, base, interfaceIds("darwin", set(31, []gonet.Addr{wifiV4, wifiLL}, []gonet.Addr{cableLL}, []gonet.Addr{ipNet("fe80::9/64")})), "a tunnel's addresses are not read")

	cableUp := set(31, []gonet.Addr{wifiV4, wifiLL}, []gonet.Addr{cableLL}, nil)
	cableUp[1].Flags = gonet.FlagUp | gonet.FlagMulticast
	assert.NotEqual(t, base, interfaceIds("darwin", cableUp), "the cable came up with the same name, index and address")
}
