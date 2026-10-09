package spacecore

import (
	"net/netip"
	"testing"

	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/stretchr/testify/assert"

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
