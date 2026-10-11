package spacecore

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// recordingPeerService records SetPeerAddrs; any other call panics.
type recordingPeerService struct {
	peerservice.PeerService
	addrs map[string][]string
}

func (r *recordingPeerService) SetPeerAddrs(peerId string, addrs []string) {
	if r.addrs == nil {
		r.addrs = map[string][]string{}
	}
	r.addrs[peerId] = addrs
}

func TestHostIP(t *testing.T) {
	for _, tc := range []struct {
		addr   string
		want   netip.Addr
		wantOk bool
	}{
		{"192.168.1.5:4006", netip.MustParseAddr("192.168.1.5"), true},
		{"yamux://192.168.1.5:51234", netip.MustParseAddr("192.168.1.5"), true},
		{"[::ffff:192.168.1.5]:4006", netip.MustParseAddr("192.168.1.5"), true},
		{"yamux://[fe80::1%en16]:51234", netip.MustParseAddr("fe80::1%en16"), true},
		{"[fe80::a886:c6ff:febd:f9dc%31]:62654", netip.MustParseAddr("fe80::a886:c6ff:febd:f9dc%31"), true},
		{"", netip.Addr{}, false},
		{"192.168.1.5", netip.Addr{}, false},
		{"host.local:4006", netip.Addr{}, false},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			got, ok := hostIP(tc.addr)
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLocalPeerAddrs(t *testing.T) {
	own := localdiscovery.OwnAddresses{
		Addrs: []string{"192.168.1.10", "10.20.0.5"},
		Nets:  []netip.Prefix{netip.MustParsePrefix("192.168.1.10/24"), netip.MustParsePrefix("10.20.0.5/16")},
		Port:  4006,
	}

	t.Run("the peer learns only our address on its subnet", func(t *testing.T) {
		// given
		ps := &recordingPeerService{}
		s := &service{peerService: ps}
		s.setLocalPeerAddrs("p1", []string{"192.168.1.62:4006"})

		// when
		got := localServerOf(own, s.peerIPsOf("p1", "yamux://192.168.1.62:51234"))

		// then
		assert.Equal(t, []string{"192.168.1.10"}, got.Ips)
		assert.Equal(t, int32(4006), got.Port)
		assert.Equal(t, []string{"yamux://192.168.1.62:4006"}, ps.addrs["p1"])
	})

	t.Run("the live connection counts when the peer's addresses moved", func(t *testing.T) {
		// given: recorded on Wi-Fi, now connected over the wired subnet
		s := &service{peerService: &recordingPeerService{}}
		s.setLocalPeerAddrs("p1", []string{"192.168.1.62:4006"})

		// when
		got := s.peerIPsOf("p1", "yamux://10.20.9.9:51234")

		// then
		want := []netip.Addr{netip.MustParseAddr("192.168.1.62"), netip.MustParseAddr("10.20.9.9")}
		assert.Equal(t, want, got)
		assert.Equal(t, []string{"192.168.1.10", "10.20.0.5"}, own.For(got))
	})

	t.Run("a new address list replaces the old one", func(t *testing.T) {
		// given
		s := &service{peerService: &recordingPeerService{}}
		s.setLocalPeerAddrs("p1", []string{"192.168.1.62:4006"})

		// when
		s.setLocalPeerAddrs("p1", []string{"10.20.9.9:4006"})

		// then
		assert.Equal(t, []netip.Addr{netip.MustParseAddr("10.20.9.9")}, s.peerIPsOf("p1", ""))
	})

	t.Run("a forgotten peer gets the whole list", func(t *testing.T) {
		// given
		s := &service{peerService: &recordingPeerService{}}
		s.setLocalPeerAddrs("p1", []string{"192.168.1.62:4006"})

		// when
		s.forgetPeerIPs("p1")

		// then
		assert.Empty(t, s.peerIPsOf("p1", ""))
		assert.Equal(t, own.Addrs, localServerOf(own, s.peerIPsOf("p1", "")).Ips)
	})
}

func TestRecordLocalPeer(t *testing.T) {
	for _, tc := range []struct {
		name     string
		connAddr string
		want     []string
	}{
		{
			name:     "the connection address goes first with the advertised port",
			connAddr: "yamux://10.20.9.9:51234",
			want:     []string{"yamux://10.20.9.9:4006", "yamux://192.168.1.62:4006"},
		},
		{
			name:     "a link-local connection address goes first, zoned by index",
			connAddr: "yamux://[fe80::1%en16]:51234",
			want:     []string{"yamux://[fe80::1%31]:4006", "yamux://192.168.1.62:4006"},
		},
		{
			name:     "an advertised address equal to the connection is not repeated",
			connAddr: "yamux://192.168.1.62:51234",
			want:     []string{"yamux://192.168.1.62:4006"},
		},
		{
			name: "no connection address leaves the advertised list",
			want: []string{"yamux://192.168.1.62:4006"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			f := newLocalAddrsFixture(t)
			ctx := context.Background()
			if tc.connAddr != "" {
				ctx = peer.CtxWithPeerAddr(ctx, tc.connAddr)
			}

			// when
			f.recordLocalPeer(ctx, "p1", &clientspaceproto.LocalServer{Ips: []string{"192.168.1.62"}, Port: 4006}, []string{"s1"}, false)

			// then
			assert.Equal(t, tc.want, f.installed("p1"))
		})
	}
}

// stubExchange stubs the outbound exchange: each call takes the next error
// (success once they run out), and none may run under localAddrsMu.
func (f *localAddrsFixture) stubExchange(t *testing.T, errs ...error) *int {
	calls := 0
	f.exchangeFn = func(context.Context, string, *localdiscovery.OwnAddresses) ([]string, bool, error) {
		if !f.localAddrsMu.TryLock() {
			t.Error("the exchange runs under localAddrsMu")
		} else {
			f.localAddrsMu.Unlock()
		}
		calls++
		if len(errs) > 0 {
			err := errs[0]
			errs = errs[1:]
			if err != nil {
				return nil, false, err
			}
		}
		return []string{"s1"}, false, nil
	}
	return &calls
}

func (f *localAddrsFixture) peerDiscovered(peerId, link string, hostPorts ...string) {
	f.PeerDiscovered(context.Background(), localdiscovery.DiscoveredPeer{PeerId: peerId, Link: link, Addrs: hostPorts}, localdiscovery.OwnAddresses{Port: 4006})
}

func TestPeerDiscoveredSources(t *testing.T) {
	t.Run("a Wi-Fi discovery keeps the cable route", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.stubExchange(t)
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// when
		f.peerDiscovered("phone", "", "192.168.1.62:4006")

		// then
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006", "yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("a repeated cable discovery does not exchange again while a link-local connection is up", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t)
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}

		// when
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// then
		assert.Equal(t, 1, *calls)
	})

	t.Run("a failed exchange is retried on the next cable discovery", func(t *testing.T) {
		// given: the phone did not listen yet; a connection is up later
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t, errors.New("connection refused"))
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}

		// when
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// then: retried once, then done
		assert.Equal(t, 2, *calls)
	})

	t.Run("a list that changed during the exchange is exchanged over again", func(t *testing.T) {
		// given: the phone connects to us over Wi-Fi meanwhile
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t)
		exchange := f.exchangeFn
		f.exchangeFn = func(ctx context.Context, peerId string, own *localdiscovery.OwnAddresses) ([]string, bool, error) {
			if *calls == 0 {
				f.inbound("phone", "yamux://192.168.1.62:51234")
			}
			return exchange(ctx, peerId, own)
		}
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}

		// when
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// then
		assert.Equal(t, 2, *calls)
	})

	t.Run("without a link-local connection a cable discovery exchanges again", func(t *testing.T) {
		// given: our exchange taught the phone our cable address, then the
		// connection closed and the phone's route to us expired
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t)
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.live = []netip.Addr{netip.MustParseAddr("fe80::62%31")}
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		require.Equal(t, 1, *calls)
		f.live = nil

		// when: the cable answers again, unchanged
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// then: dialed again, which teaches the phone our address again
		assert.Equal(t, 2, *calls)
	})

	t.Run("duplicate base addresses go in as develop installs them", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		f.stubExchange(t)

		// when
		f.peerDiscovered("p1", "", "192.168.1.62:4006", "192.168.1.62:4006")

		// then
		assert.Equal(t, []string{"yamux://192.168.1.62:4006", "yamux://192.168.1.62:4006"}, f.installed("p1"))
	})

	t.Run("a peer forgotten during the exchange stays forgotten", func(t *testing.T) {
		// given: the peer store drops the phone while we exchange
		f := newLocalAddrsFixture(t)
		f.stubExchange(t)
		exchange := f.exchangeFn
		f.exchangeFn = func(ctx context.Context, peerId string, own *localdiscovery.OwnAddresses) ([]string, bool, error) {
			f.forgetPeerIPs(peerId)
			return exchange(ctx, peerId, own)
		}

		// when
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")

		// then
		assert.Empty(t, f.localPeers)
	})

	t.Run("a base discovery always exchanges, as on develop", func(t *testing.T) {
		// given
		f := newLocalAddrsFixture(t)
		calls := f.stubExchange(t)
		f.peerDiscovered("phone", "", "192.168.1.62:4006")

		// when
		f.peerDiscovered("phone", "", "192.168.1.62:4006")

		// then
		assert.Equal(t, 2, *calls)
	})

	t.Run("an inbound IPv4 exchange keeps the cable route from mDNS but drops the inbound one", func(t *testing.T) {
		// given: discovered over the cable, then connected over it
		f := newLocalAddrsFixture(t)
		f.stubExchange(t)
		f.peerDiscovered("phone", "31", "[fe80::62%31]:4006")
		f.inbound("phone", "yamux://[fe80::63%en16]:51234")
		require.Equal(t, []string{"yamux://[fe80::63%31]:4006", "yamux://[fe80::62%31]:4006"}, f.installed("phone"))

		// when: it connects over Wi-Fi
		f.inbound("phone", "yamux://192.168.1.62:51234", "192.168.1.62")

		// then
		assert.Equal(t, []string{"yamux://[fe80::62%31]:4006", "yamux://192.168.1.62:4006"}, f.installed("phone"))
	})

	t.Run("without a cable the list and the exchanges are develop's", func(t *testing.T) {
		// given: no link-local-only interface, so no cable discovery either
		f := newLocalAddrsFixture(t)
		f.usable = nil
		calls := f.stubExchange(t)
		steps := []struct {
			do   func()
			want []string
		}{
			{func() { f.peerDiscovered("p1", "", "192.168.1.62:4006", "10.0.0.5:4006") }, []string{"yamux://192.168.1.62:4006", "yamux://10.0.0.5:4006"}},
			{func() { f.peerDiscovered("p1", "", "192.168.1.62:4006", "10.0.0.5:4006") }, []string{"yamux://192.168.1.62:4006", "yamux://10.0.0.5:4006"}},
			{func() { f.inbound("p1", "yamux://192.168.1.70:51234", "192.168.1.62", "192.168.1.70") }, []string{"yamux://192.168.1.70:4006", "yamux://192.168.1.62:4006"}},
			{func() { f.peerDiscovered("p1", "", "10.0.0.5:4006") }, []string{"yamux://10.0.0.5:4006"}},
			{func() { f.inbound("p1", "") }, []string{}},
		}

		for i, step := range steps {
			// when
			step.do()

			// then: the latest list exactly, last writer wins
			assert.Equal(t, step.want, f.installed("p1"), "step %d", i)
		}
		assert.Equal(t, 3, *calls, "every discovery exchanges")
		assert.Zero(t, f.lookups, "no interface looked up")
		assert.Zero(t, f.picks, "no pool asked")
		assert.Empty(t, f.timers, "no route armed")
	})
}
