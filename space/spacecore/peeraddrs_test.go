package spacecore

import (
	"context"
	"net/netip"
	"slices"
	"testing"

	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/stretchr/testify/assert"

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
	stubInterfaces := func(t *testing.T, present ...string) {
		orig := interfaceExists
		t.Cleanup(func() { interfaceExists = orig })
		interfaceExists = func(zone string) bool { return slices.Contains(present, zone) }
	}
	inbound := func(t *testing.T) *service {
		s, _, _ := newAccountPeersService(t, nil, nil)
		s.peerService = &recordingPeerService{}
		ctx := peer.CtxWithPeerAddr(context.Background(), "yamux://[fe80::1%en2]:51234")
		s.recordLocalPeer(ctx, "mac", &clientspaceproto.LocalServer{Ips: []string{"192.168.139.3"}, Port: 4006}, nil, false)
		return s
	}

	t.Run("an IPv4-only discovery keeps the inbound link-local in front", func(t *testing.T) {
		// given: the Mac connected over the cable, then native discovery
		// resolved only an IPv4 address the cable can't reach
		stubInterfaces(t, "en2")
		s := inbound(t)

		// when
		got := s.withInboundLinkLocal("mac", []string{"192.168.139.3:4006"})

		// then
		assert.Equal(t, []string{"[fe80::1%en2]:4006", "192.168.139.3:4006"}, got)
	})

	t.Run("a discovery that found nothing still has the cable", func(t *testing.T) {
		stubInterfaces(t, "en2")
		s := inbound(t)

		assert.Equal(t, []string{"[fe80::1%en2]:4006"}, s.withInboundLinkLocal("mac", nil))
	})

	t.Run("a discovery with its own link-local wins", func(t *testing.T) {
		stubInterfaces(t, "en2")
		s := inbound(t)
		addrs := []string{"[fe80::2%en2]:4006"}

		assert.Equal(t, addrs, s.withInboundLinkLocal("mac", addrs))
	})

	t.Run("an unplugged cable is forgotten", func(t *testing.T) {
		// given
		stubInterfaces(t)
		s := inbound(t)

		// when
		got := s.withInboundLinkLocal("mac", []string{"192.168.139.3:4006"})

		// then
		assert.Equal(t, []string{"192.168.139.3:4006"}, got)
		assert.Empty(t, s.inboundLinkLocal)
	})

	t.Run("an IPv4 inbound connection remembers nothing", func(t *testing.T) {
		stubInterfaces(t, "en2")
		s, _, _ := newAccountPeersService(t, nil, nil)
		s.peerService = &recordingPeerService{}
		ctx := peer.CtxWithPeerAddr(context.Background(), "yamux://192.168.1.62:51234")
		s.recordLocalPeer(ctx, "mac", &clientspaceproto.LocalServer{Port: 4006}, nil, false)

		assert.Equal(t, []string{"192.168.1.62:4006"}, s.withInboundLinkLocal("mac", []string{"192.168.1.62:4006"}))
	})
}
