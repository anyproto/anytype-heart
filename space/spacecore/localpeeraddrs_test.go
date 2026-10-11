package spacecore

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	live    bool
	lookups int
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
	origAfter, origZone, origLive := afterFunc, linkLocalZone, liveLinkLocalConn
	t.Cleanup(func() {
		afterFunc, linkLocalZone, liveLinkLocalConn = origAfter, origZone, origLive
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
	liveLinkLocalConn = func(*service, string) bool { return f.live }
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
	zones := map[string]string{"31": "31", "en16": "31", "32": "32", "en17": "32"}
	zone := func(z string) (string, bool) {
		index, ok := zones[z]
		return index, ok
	}

	t.Run("without a route the base list goes as it is", func(t *testing.T) {
		// given
		base := []string{"192.168.1.63:4006", "10.0.0.5:4006", "192.168.1.63:4006"}

		// when
		got := composeAddrs(nil, base, func(string) (string, bool) {
			t.Fatal("no interface lookup without a route")
			return "", false
		})

		// then
		assert.Equal(t, base, got)
	})

	t.Run("the inbound route, then the cable routes, then the base list, each address once", func(t *testing.T) {
		// given: the inbound route names cable 32, the second mDNS address
		// on cable 31 is the one inbound came from
		routes := map[string]*linkRoute{
			routeMDNS + "32": {hostPorts: []string{"[fe80::2%32]:4006"}},
			routeInbound:     {hostPorts: []string{"[fe80::3%en17]:4006"}},
			routeMDNS + "31": {hostPorts: []string{"[fe80::1%31]:4006", "[fe80::3%32]:4006"}},
		}
		base := []string{"192.168.1.5:4006", "[fe80::2%32]:4006"}
		want := []string{"[fe80::3%32]:4006", "[fe80::1%31]:4006", "[fe80::2%32]:4006", "192.168.1.5:4006"}

		// when
		got := composeAddrs(routes, base, zone)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a route over an unusable interface is left out", func(t *testing.T) {
		// given: index 33 is gone, down, or has routable IPv4 now
		routes := map[string]*linkRoute{routeMDNS + "33": {hostPorts: []string{"[fe80::3%33]:4006"}}}

		// when
		got := composeAddrs(routes, []string{"192.168.1.5:4006"}, zone)

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

	t.Run("a live link-local connection renews the route", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		f.live = true

		// when
		f.timers[0].fire()

		// then: still installed, and armed again
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, f.installed("phone"))
		require.Len(t, f.timers, 2)
		assert.Equal(t, linkRouteTTL, f.timers[1].d)
		f.live = false
		f.timers[1].fire()
		assert.Empty(t, f.installed("phone"))
	})

	t.Run("the pool is asked without the lock", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")
		var locked bool
		liveLinkLocalConn = func(s *service, _ string) bool {
			if locked = !s.localAddrsMu.TryLock(); !locked {
				s.localAddrsMu.Unlock()
			}
			return false
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

	t.Run("a forgotten peer drops its routes and timers", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.discover("phone", "31", "[fe80::62%31]:4006")

		// when
		f.forgetPeerIPs("phone")

		// then
		assert.False(t, f.timers[0].timer.Stop(), "the timer was stopped")
		assert.Empty(t, f.localPeers)
		f.timers[0].fire()
		assert.Len(t, f.timers, 1, "nothing armed for a forgotten peer")
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
