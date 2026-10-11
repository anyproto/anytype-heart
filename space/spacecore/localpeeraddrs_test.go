package spacecore

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/peer/mock_peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/net/addrs"
)

// armedTimer is one afterFunc call: the callback to fire by hand and the
// timer handed back, never firing on its own.
type armedTimer struct {
	d     time.Duration
	fire  func()
	timer *time.Timer
}

type localAddrsFixture struct {
	*service
	peers   *recordingPeerService
	timers  []*armedTimer
	usable  map[string]string // zone -> decimal index of a usable interface
	live    []netip.Addr      // the link-local addresses pooled connections run over
	lookups int
	picks   int // liveLinkLocalAddrs calls
}

func newLocalAddrsFixture(t *testing.T) *localAddrsFixture {
	t.Helper()
	s, _, _ := newAccountPeersService(t, nil, nil)
	f := &localAddrsFixture{
		service: s,
		peers:   &recordingPeerService{},
		usable:  map[string]string{"31": "31", "en16": "31", "32": "32"},
	}
	s.peerService = f.peers
	origAfter, origZone, origLive := afterFunc, linkLocalZone, liveLinkLocalAddrs
	t.Cleanup(func() {
		afterFunc, linkLocalZone, liveLinkLocalAddrs = origAfter, origZone, origLive
		for _, a := range f.timers {
			a.timer.Stop()
		}
	})
	afterFunc = func(d time.Duration, fire func()) *time.Timer {
		a := &armedTimer{d: d, fire: fire, timer: time.NewTimer(time.Hour)}
		f.timers = append(f.timers, a)
		return a.timer
	}
	linkLocalZone = func(zone string) (string, bool) {
		f.lookups++
		index, ok := f.usable[zone]
		return index, ok
	}
	liveLinkLocalAddrs = func(s *service, _ string) []netip.Addr {
		if !s.localAddrsMu.TryLock() {
			t.Error("the pool is asked under localAddrsMu")
		} else {
			s.localAddrsMu.Unlock()
		}
		f.picks++
		return f.live
	}
	return f
}

// discover installs a discovery's list the way PeerDiscovered does.
func (f *localAddrsFixture) discover(peerId, link string, hostPorts ...string) {
	f.localAddrsMu.Lock()
	defer f.localAddrsMu.Unlock()
	if link == "" {
		f.setBaseAddrs(peerId, hostPorts)
	} else {
		f.setRoute(peerId, routeMDNS+link, hostPorts)
	}
	f.installLocalAddrs(peerId)
}

func (f *localAddrsFixture) inbound(peerId, connAddr string, ips ...string) {
	ctx := peer.CtxWithPeerAddr(context.Background(), connAddr)
	f.recordLocalPeer(ctx, peerId, &clientspaceproto.LocalServer{Ips: ips, Port: 4006}, nil, false)
}

func (f *localAddrsFixture) installed(peerId string) []string {
	return f.peers.addrs[peerId]
}

func TestComposeAddrs(t *testing.T) {
	usable := func(index string) bool { return index == "31" || index == "32" }

	t.Run("without a route the base list goes as it is", func(t *testing.T) {
		// given
		base := []string{"192.168.1.63:4006", "10.0.0.5:4006", "192.168.1.63:4006"}

		// when
		got := composeAddrs(nil, base, func(string) bool {
			t.Fatal("no interface lookup without a route")
			return false
		})

		// then
		assert.Equal(t, base, got)
	})

	t.Run("the inbound route, then the cable routes, then the base list, each address once", func(t *testing.T) {
		// given: the inbound route came over cable 32, which mdns:31 also
		// lists
		routes := map[string]*linkRoute{
			routeMDNS + "32": {hostPorts: []string{"[fe80::2%32]:4006"}},
			routeInbound:     {hostPorts: []string{"[fe80::3%32]:4006"}},
			routeMDNS + "31": {hostPorts: []string{"[fe80::1%31]:4006", "[fe80::3%32]:4006"}},
		}
		base := []string{"192.168.1.5:4006", "[fe80::2%32]:4006"}
		want := []string{"[fe80::3%32]:4006", "[fe80::1%31]:4006", "[fe80::2%32]:4006", "192.168.1.5:4006"}

		// when
		got := composeAddrs(routes, base, usable)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a route over an unusable interface is left out", func(t *testing.T) {
		// given: index 33 is gone, down, or has routable IPv4 now
		routes := map[string]*linkRoute{routeMDNS + "33": {hostPorts: []string{"[fe80::3%33]:4006"}}}

		// when
		got := composeAddrs(routes, []string{"192.168.1.5:4006"}, usable)

		// then
		assert.Equal(t, []string{"192.168.1.5:4006"}, got)
	})

	t.Run("a route zoned by name is left out, never looked up", func(t *testing.T) {
		// given: a name may be taken over by another cable
		routes := map[string]*linkRoute{routeInbound: {hostPorts: []string{"[fe80::1%en16]:4006"}}}

		// when
		got := composeAddrs(routes, []string{"192.168.1.5:4006"}, func(string) bool { return true })

		// then
		assert.Equal(t, []string{"192.168.1.5:4006"}, got)
	})
}

func TestLocalPeerAddrsStore(t *testing.T) {
	t.Run("a Wi-Fi discovery keeps the cable route", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")

		// when
		f.discover("phone", "", "192.168.1.62:4006")

		// then
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006", "yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("a cable discovery keeps the Wi-Fi list", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "", "192.168.1.62:4006")

		// when
		f.discover("phone", "31", "[fe80::62%31]:4006")

		// then
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006", "yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("an unplugged cable leaves the list at the next install", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		delete(f.usable, "31")

		// when
		f.discover("phone", "", "192.168.1.62:4006")

		// then
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("an expired route leaves the list", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "", "192.168.1.62:4006")
		f.discover("phone", "31", "[fe80::62%31]:4006")
		require.Len(t, f.timers, 1)
		assert.Equal(t, linkRouteTTL, f.timers[0].d)

		// when
		f.timers[0].fire()

		// then
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("a live connection over the route renews it", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}

		// when
		f.timers[0].fire()

		// then: still installed, and armed again
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, f.installed("phone"))
		require.Len(t, f.timers, 2)
		assert.Equal(t, linkRouteTTL, f.timers[1].d)
		f.live = nil
		f.timers[1].fire()
		assert.Empty(t, f.installed("phone"))
	})

	t.Run("a live connection over another cable does not renew it", func(t *testing.T) {
		// given: the phone answers on 31 from an old address, syncs over 32
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.discover("phone", "32", "[fe80::62%32]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%32")}

		// when
		f.timers[0].fire()

		// then
		assert.Equal(t, []string{"yamux://[fe80::62%32]:4006"}, f.installed("phone"))
		assert.Len(t, f.timers, 2, "not armed again")
	})

	t.Run("a live connection to another address on the same cable does not renew it", func(t *testing.T) {
		// given: the phone came in from fe80::62, now answers as fe80::63
		f := newLocalAddrsFixture(t)
		f.inbound("phone", "yamux://[fe80::62%en16]:51234")
		f.discover("phone", "31", "[fe80::63%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::63%31")}

		// when: the inbound route reaches its deadline
		f.timers[0].fire()

		// then
		assert.Equal(t, []string{"yamux://[fe80::63%31]:4006"}, f.installed("phone"))
		assert.Len(t, f.timers, 2, "not armed again")
	})

	t.Run("a renewed route over an interface no longer usable leaves the list", func(t *testing.T) {
		// given: the cable interface got routable IPv4 meanwhile
		f := newLocalAddrsFixture(t)
		f.discover("phone", "", "192.168.1.62:4006")
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}
		delete(f.usable, "31")

		// when
		f.timers[0].fire()

		// then
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("the pool is asked without the lock", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		var locked bool
		liveLinkLocalAddrs = func(s *service, _ string) []netip.Addr {
			if locked = !s.localAddrsMu.TryLock(); !locked {
				s.localAddrsMu.Unlock()
			}
			return nil
		}

		// when
		f.timers[0].fire()

		// then
		assert.False(t, locked, "localAddrsMu held across the pool")
	})

	t.Run("a renewed route ignores its old timer", func(t *testing.T) {
		// given: the cable answered again
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.discover("phone", "31", "[fe80::62%31]:4006")
		require.Len(t, f.timers, 2)

		// when
		f.timers[0].fire()

		// then
		assert.False(t, f.timers[0].timer.Stop(), "the old timer was stopped")
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, f.installed("phone"))
		assert.Len(t, f.timers, 2)
	})

	t.Run("a forgotten peer drops its routes and timers, and the routes leave the list", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "", "192.168.1.62:4006")
		f.discover("phone", "31", "[fe80::62%31]:4006")

		// when: the peer manager gave up on it
		f.forgetPeerIPs("phone")

		// then
		assert.False(t, f.timers[0].timer.Stop(), "the timer was stopped")
		assert.Empty(t, f.localPeers)
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, f.installed("phone"))
		f.timers[0].fire()
		assert.Len(t, f.timers, 1, "nothing armed for a forgotten peer")
	})

	t.Run("a forgotten peer without routes keeps its list, as on develop", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "", "192.168.1.62:4006")
		f.peers.addrs["phone"] = []string{"sentinel"}

		// when
		f.forgetPeerIPs("phone")

		// then: not installed again
		assert.Equal(t, []string{"sentinel"}, f.installed("phone"))
	})

	t.Run("closing stops every route, and nothing is installed after", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t)
		f.componentCtx, f.componentCtxCancel = context.WithCancel(context.Background())
		f.spaceCache = ocache.New(func(context.Context, string) (ocache.Object, error) { return nil, errors.New("none") })
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}
		before := f.installed("phone")

		// when
		require.NoError(t, f.Close(context.Background()))
		f.timers[0].fire()
		f.peerDiscovered("phone", "", "192.168.1.62:4006")
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.inbound("phone", "yamux://[fe80::62%en16]:51234", "192.168.1.62")

		// then
		assert.False(t, f.timers[0].timer.Stop(), "the timer was stopped")
		assert.Len(t, f.timers, 1, "nothing armed")
		assert.Equal(t, before, f.installed("phone"))
		assert.Zero(t, *calls, "no exchange")
		assert.Empty(t, f.localPeers)
	})
}

func TestRecordLocalPeerRoutes(t *testing.T) {
	t.Run("an IPv4-only discovery keeps the inbound route in front", func(t *testing.T) {
		// given: the Mac connected over the cable
		f := newLocalAddrsFixture(t)
		f.inbound("mac", "yamux://[fe80::1%en16]:51234", "192.168.139.3")

		// when: native discovery resolved only an IPv4 address
		f.discover("mac", "", "192.168.139.3:4006")

		// then: zoned by index, as every route
		assert.Equal(t, []string{"yamux://[fe80::1%31]:4006", "yamux://192.168.139.3:4006"}, f.installed("mac"))
	})

	t.Run("a discovery that found nothing still has the cable", func(t *testing.T) {
		f := newLocalAddrsFixture(t)
		f.inbound("mac", "yamux://[fe80::1%en16]:51234")

		f.discover("mac", "")

		assert.Equal(t, []string{"yamux://[fe80::1%31]:4006"}, f.installed("mac"))
	})

	t.Run("an IPv4 connection drops the inbound route", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.inbound("mac", "yamux://[fe80::1%en16]:51234", "192.168.139.3")

		// when: the peer came back over Wi-Fi
		f.inbound("mac", "yamux://192.168.1.62:51234")

		// then
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, f.installed("mac"))
		assert.False(t, f.timers[0].timer.Stop(), "its timer was stopped")
	})

	t.Run("the same cable from mDNS and inbound is one address, and each expires on its own", func(t *testing.T) {
		// given: discovered over the cable by index, then connected over it by name
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.inbound("phone", "yamux://[fe80::62%en16]:51234")
		require.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, f.installed("phone"))

		// when: the inbound route expires
		f.timers[1].fire()

		// then: the mDNS route still holds it
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, f.installed("phone"))
	})
}

func TestInboundRouteZone(t *testing.T) {
	t.Run("the route keeps its cable when the name moves to another", func(t *testing.T) {
		// given: connected over en16, index 31
		f := newLocalAddrsFixture(t)
		f.inbound("mac", "yamux://[fe80::1%en16]:51234")

		// when: 31 is gone, and a new cable came up as en16 at 42
		f.usable = map[string]string{"en16": "42", "42": "42"}
		f.discover("mac", "", "192.168.139.3:4006")

		// then
		assert.Equal(t, []string{"yamux://192.168.139.3:4006"}, f.installed("mac"))
	})

	t.Run("a zone that names no usable interface admits no route", func(t *testing.T) {
		// given: an inbound route from before
		f := newLocalAddrsFixture(t)
		f.inbound("mac", "yamux://[fe80::1%en16]:51234")
		delete(f.usable, "en16")

		// when
		f.inbound("mac", "yamux://[fe80::1%en16]:51235", "192.168.139.3")

		// then: neither the new nor the old one
		assert.Equal(t, []string{"yamux://192.168.139.3:4006"}, f.installed("mac"))
		assert.Len(t, f.timers, 1)
		assert.False(t, f.timers[0].timer.Stop(), "the old route was dropped")
	})
}

// fakePools serves the two pools liveLinkLocalAddrs reads.
type fakePools struct{ unary, stream pool.Pool }

func (p fakePools) UnaryPeerPool() pool.Pool  { return p.unary }
func (p fakePools) StreamPeerPool() pool.Pool { return p.stream }

func TestPooledLinkLocalAddrs(t *testing.T) {
	connected := func(ctrl *gomock.Controller, addr string) peer.Peer {
		pr := mock_peer.NewMockPeer(ctrl)
		pr.EXPECT().Context().Return(peer.CtxWithPeerAddr(context.Background(), addr)).AnyTimes()
		return pr
	}
	missing := errors.New("no connection")
	for _, tc := range []struct {
		name          string
		unary, stream string // the pooled connection's address, "" when none
		want          []netip.Addr
	}{
		{name: "a link-local connection, zoned by index", unary: "yamux://[fe80::62%en16]:51234", want: []netip.Addr{netip.MustParseAddr("fe80::62%31")}},
		{name: "an IPv4 connection does not count", unary: "yamux://192.168.1.62:4006", stream: "yamux://[fe80::63%31]:4006", want: []netip.Addr{netip.MustParseAddr("fe80::63%31")}},
		{name: "the same address in both pools once", unary: "yamux://[fe80::62%31]:4006", stream: "yamux://[fe80::62%en16]:4006", want: []netip.Addr{netip.MustParseAddr("fe80::62%31")}},
		{name: "no connection"},
		{name: "a connection over an unusable interface does not count", unary: "yamux://[fe80::62%en18]:4006"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: pools that only Pick, never dial
			f := newLocalAddrsFixture(t)
			ctrl := gomock.NewController(t)
			pools := fakePools{unary: mock_pool.NewMockPool(ctrl), stream: mock_pool.NewMockPool(ctrl)}
			for p, addr := range map[*mock_pool.MockPool]string{pools.unary.(*mock_pool.MockPool): tc.unary, pools.stream.(*mock_pool.MockPool): tc.stream} {
				if addr == "" {
					p.EXPECT().Pick(gomock.Any(), "phone").Return(nil, missing)
				} else {
					p.EXPECT().Pick(gomock.Any(), "phone").Return(connected(ctrl, addr), nil)
				}
			}
			f.poolManager = pools

			// when
			got := pooledLinkLocalAddrs(f.service, "phone")

			// then
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUsableLinkZone(t *testing.T) {
	wrap := func(index int, name string, flags net.Flags, cidrs ...string) addrs.NetInterfaceWithAddrCache {
		list := make([]net.Addr, 0, len(cidrs))
		for _, c := range cidrs {
			ip, n, err := net.ParseCIDR(c)
			require.NoError(t, err)
			n.IP = ip
			list = append(list, n)
		}
		return addrs.WrapInterfaceWithAddrs(net.Interface{Index: index, Name: name, Flags: flags}, list)
	}
	ifaces := []addrs.NetInterfaceWithAddrCache{
		wrap(4, "en0", net.FlagUp, "192.168.1.10/24", "fe80::4/64"),
		wrap(31, "en16", net.FlagUp, "fe80::1/64"),
		wrap(32, "en17", net.FlagUp, "169.254.10.2/16", "fe80::2/64"),
		wrap(33, "en18", 0, "fe80::3/64"),
	}

	for _, tc := range []struct {
		zone      string
		wantIndex string
		wantOk    bool
	}{
		{"en16", "31", true},
		{"31", "31", true},
		{"en17", "32", true},
		{"en0", "", false},
		{"4", "", false},
		{"en18", "", false},
		{"en99", "", false},
		{"", "", false},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			// when
			index, ok := usableLinkZone(ifaces, tc.zone)

			// then
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.wantIndex, index)
		})
	}
}
