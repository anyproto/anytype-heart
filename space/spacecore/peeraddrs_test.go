package spacecore

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"testing"
	"time"

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
	localServer := &clientspaceproto.LocalServer{Ips: []string{"192.168.1.62"}, Port: 4006}

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
			name:     "a link-local connection address keeps its zone",
			connAddr: "yamux://[fe80::1%en16]:51234",
			want:     []string{"yamux://[fe80::1%en16]:4006", "yamux://192.168.1.62:4006"},
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
			s, _, _ := newAccountPeersService(t, nil, nil)
			ps := &recordingPeerService{}
			s.peerService = ps
			ctx := context.Background()
			if tc.connAddr != "" {
				ctx = peer.CtxWithPeerAddr(ctx, tc.connAddr)
			}

			// when
			s.recordLocalPeer(ctx, "p1", localServer, []string{"s1"}, false)

			// then
			assert.Equal(t, tc.want, ps.addrs["p1"])
		})
	}
}

func TestWithInboundLinkLocal(t *testing.T) {
	stubLinks := func(t *testing.T, usable ...string) {
		orig := linkLocalUsable
		t.Cleanup(func() { linkLocalUsable = orig })
		linkLocalUsable = func(zone string) bool { return slices.Contains(usable, zone) }
	}
	clock := func(t *testing.T) *time.Time {
		at := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
		orig := timeNow
		t.Cleanup(func() { timeNow = orig })
		timeNow = func() time.Time { return at }
		return &at
	}
	record := func(s *service, connAddr string) {
		ctx := peer.CtxWithPeerAddr(context.Background(), connAddr)
		s.recordLocalPeer(ctx, "mac", &clientspaceproto.LocalServer{Ips: []string{"192.168.139.3"}, Port: 4006}, nil, false)
	}
	inbound := func(t *testing.T) *service {
		s, _, _ := newAccountPeersService(t, nil, nil)
		s.peerService = &recordingPeerService{}
		record(s, "yamux://[fe80::1%en2]:51234")
		return s
	}

	t.Run("an IPv4-only discovery keeps the inbound link-local in front", func(t *testing.T) {
		// given: the Mac connected over the cable, then native discovery
		// resolved only an IPv4 address the cable can't reach
		clock(t)
		stubLinks(t, "en2")
		s := inbound(t)

		// when
		got := s.withInboundLinkLocal("mac", []string{"192.168.139.3:4006"})

		// then
		assert.Equal(t, []string{"[fe80::1%en2]:4006", "192.168.139.3:4006"}, got)
	})

	t.Run("a discovery that found nothing still has the cable", func(t *testing.T) {
		clock(t)
		stubLinks(t, "en2")
		s := inbound(t)

		assert.Equal(t, []string{"[fe80::1%en2]:4006"}, s.withInboundLinkLocal("mac", nil))
	})

	t.Run("a discovery with its own link-local wins", func(t *testing.T) {
		clock(t)
		stubLinks(t, "en2")
		s := inbound(t)
		addrs := []string{"[fe80::2%en2]:4006"}

		assert.Equal(t, addrs, s.withInboundLinkLocal("mac", addrs))
	})

	t.Run("an unusable link is forgotten", func(t *testing.T) {
		// given: unplugged, down, or the interface gained IPv4
		clock(t)
		stubLinks(t)
		s := inbound(t)

		// when
		got := s.withInboundLinkLocal("mac", []string{"192.168.139.3:4006"})

		// then
		assert.Equal(t, []string{"192.168.139.3:4006"}, got)
		assert.Empty(t, s.inboundLinkLocal)
	})

	t.Run("it expires without a newer connection", func(t *testing.T) {
		// given
		at := clock(t)
		stubLinks(t, "en2")
		s := inbound(t)

		// when
		*at = at.Add(inboundLinkLocalTTL + time.Second)

		// then
		assert.Equal(t, []string{"192.168.139.3:4006"}, s.withInboundLinkLocal("mac", []string{"192.168.139.3:4006"}))
	})

	t.Run("a newer connection renews it", func(t *testing.T) {
		at := clock(t)
		stubLinks(t, "en2")
		s := inbound(t)
		*at = at.Add(inboundLinkLocalTTL)
		record(s, "yamux://[fe80::1%en2]:51235")
		*at = at.Add(time.Minute)

		assert.Equal(t, []string{"[fe80::1%en2]:4006"}, s.withInboundLinkLocal("mac", nil))
	})

	t.Run("an IPv4 link-local discovery does not displace it", func(t *testing.T) {
		clock(t)
		stubLinks(t, "en2")
		s := inbound(t)

		got := s.withInboundLinkLocal("mac", []string{"169.254.10.20:4006"})

		assert.Equal(t, []string{"[fe80::1%en2]:4006", "169.254.10.20:4006"}, got)
	})

	t.Run("an IPv4 connection supersedes it", func(t *testing.T) {
		// given
		clock(t)
		stubLinks(t, "en2")
		s := inbound(t)

		// when: the peer came back over Wi-Fi
		record(s, "yamux://192.168.1.62:51234")

		// then
		assert.Equal(t, []string{"192.168.1.62:4006"}, s.withInboundLinkLocal("mac", []string{"192.168.1.62:4006"}))
	})
}

func TestInboundLinkLocalExpiresFromTheInstalledList(t *testing.T) {
	// given: the timer is captured instead of armed
	var expire []func()
	origAfter := afterFunc
	t.Cleanup(func() { afterFunc = origAfter })
	afterFunc = func(d time.Duration, f func()) *time.Timer {
		expire = append(expire, f)
		return time.NewTimer(time.Hour)
	}
	origUsable := linkLocalUsable
	t.Cleanup(func() { linkLocalUsable = origUsable })
	linkLocalUsable = func(string) bool { return true }
	s, _, _ := newAccountPeersService(t, nil, nil)
	ps := &recordingPeerService{}
	s.peerService = ps
	ctx := peer.CtxWithPeerAddr(context.Background(), "yamux://[fe80::1%en2]:51234")
	s.recordLocalPeer(ctx, "mac", &clientspaceproto.LocalServer{Ips: []string{"192.168.139.3"}, Port: 4006}, nil, false)
	require.Equal(t, []string{"yamux://[fe80::1%en2]:4006", "yamux://192.168.139.3:4006"}, ps.addrs["mac"])

	t.Run("renewed: the old timer does nothing", func(t *testing.T) {
		s.recordLocalPeer(ctx, "mac", &clientspaceproto.LocalServer{Ips: []string{"192.168.139.3"}, Port: 4006}, nil, false)

		expire[0]()

		assert.Equal(t, []string{"yamux://[fe80::1%en2]:4006", "yamux://192.168.139.3:4006"}, ps.addrs["mac"])
	})

	t.Run("a live link-local connection renews it", func(t *testing.T) {
		origLive := liveLinkLocalConn
		t.Cleanup(func() { liveLinkLocalConn = origLive })
		liveLinkLocalConn = func(*service, string) bool { return true }
		n := len(expire)

		expire[n-1]()

		assert.Equal(t, []string{"yamux://[fe80::1%en2]:4006", "yamux://192.168.139.3:4006"}, ps.addrs["mac"])
		require.Len(t, expire, n+1, "re-armed")
	})

	t.Run("unrenewed: it leaves the installed list", func(t *testing.T) {
		expire[len(expire)-1]()

		assert.Equal(t, []string{"yamux://192.168.139.3:4006"}, ps.addrs["mac"])
		assert.Empty(t, s.inboundLinkLocal)
	})
}

func TestInboundLinkLocalExpiryRestoresTheDiscoveredRoute(t *testing.T) {
	for _, tc := range []struct {
		name, inboundZone string
	}{
		{"the same route zoned by name", "en16"},
		{"the same route zoned by index, as discovery has it", "31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: the Mac discovered the phone over the cable, then the
			// phone's exchange installed the route again
			var expire []func()
			origAfter := afterFunc
			t.Cleanup(func() { afterFunc = origAfter })
			afterFunc = func(d time.Duration, f func()) *time.Timer {
				expire = append(expire, f)
				return time.NewTimer(time.Hour)
			}
			s, _, _ := newAccountPeersService(t, nil, nil)
			ps := &recordingPeerService{}
			s.peerService = ps
			s.rememberDiscovered("phone", []string{"[fe80::62%31]:4006"})
			s.setLocalPeerAddrs("phone", []string{"[fe80::62%31]:4006"})
			ctx := peer.CtxWithPeerAddr(context.Background(), "yamux://[fe80::62%"+tc.inboundZone+"]:51234")
			s.recordLocalPeer(ctx, "phone", &clientspaceproto.LocalServer{Port: 4006}, nil, false)

			// when: two minutes without an exchange or a live connection
			expire[len(expire)-1]()

			// then: the discovered route is installed, not an empty list
			assert.Equal(t, []string{"yamux://[fe80::62%31]:4006"}, ps.addrs["phone"])
		})
	}
}

func TestLinkLocalUsable(t *testing.T) {
	// the loopback interface is up and has IPv4
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 && iface.Flags&net.FlagUp != 0 {
			assert.False(t, linkLocalUsable(iface.Name), "an interface with IPv4")
			assert.False(t, linkLocalUsable(strconv.Itoa(iface.Index)), "the same by index")
		}
	}
	assert.False(t, linkLocalUsable("nonexistent0"))

	// an up interface with no IPv4 at all, if this machine has one
	// (utun*, awdl0 on a Mac)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		if !slices.ContainsFunc(addrs, func(a net.Addr) bool {
			ipNet, ok := a.(*net.IPNet)
			return ok && ipNet.IP.To4() != nil
		}) {
			assert.True(t, linkLocalUsable(iface.Name), iface.Name)
			assert.True(t, linkLocalUsable(strconv.Itoa(iface.Index)), iface.Name)
			return
		}
	}
}
