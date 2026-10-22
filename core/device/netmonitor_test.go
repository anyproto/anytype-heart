package device

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/net/addrs"
)

func ipNet(t *testing.T, cidr string) *net.IPNet {
	ip, ipn, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	ipn.IP = ip
	return ipn
}

func TestConnectivitySnapshot(t *testing.T) {
	t.Run("ipv4 kept, loopback and link-local dropped", func(t *testing.T) {
		got := connectivitySnapshot([]net.Addr{
			ipNet(t, "192.168.1.10/24"),
			ipNet(t, "127.0.0.1/8"),
			ipNet(t, "169.254.12.7/16"),
			ipNet(t, "fe80::1/64"),
		})
		assert.Equal(t, []string{"192.168.1.10"}, got)
	})
	t.Run("ipv6 reduced to /64 prefix so privacy rotation is invisible", func(t *testing.T) {
		a := connectivitySnapshot([]net.Addr{ipNet(t, "2a00:1450:4001:80b::200e/64")})
		b := connectivitySnapshot([]net.Addr{ipNet(t, "2a00:1450:4001:80b:abcd:ef12:3456:789a/64")})
		assert.Equal(t, a, b, "same /64 prefix must produce the same snapshot")
		require.Len(t, a, 1)
	})
	t.Run("empty when only unusable addresses", func(t *testing.T) {
		got := connectivitySnapshot([]net.Addr{
			ipNet(t, "127.0.0.1/8"),
			ipNet(t, "fe80::1/64"),
		})
		assert.Empty(t, got)
	})
	t.Run("sorted and deduplicated", func(t *testing.T) {
		got := connectivitySnapshot([]net.Addr{
			ipNet(t, "10.0.0.2/24"),
			ipNet(t, "10.0.0.1/24"),
			ipNet(t, "10.0.0.1/24"),
		})
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, got)
	})
}

type monitorFixture struct {
	*netMonitor
	events   []string
	linkDown []bool
	addrs    addrs.InterfacesAddrs
	addrsErr error
}

func newMonitorFixture() *monitorFixture {
	return newMonitorFixtureAddrs(nil)
}

func newMonitorFixtureAddrs(initial []net.Addr) *monitorFixture {
	fx := &monitorFixture{}
	fx.addrs.Addrs = initial
	m := newNetMonitor(
		func(reason, _ string) { fx.events = append(fx.events, reason) },
		func(key string, down bool) { fx.linkDown = append(fx.linkDown, down) },
		nil,
		func() (addrs.InterfacesAddrs, error) { return fx.addrs, fx.addrsErr },
	)
	fx.netMonitor = m
	// mirror run()'s initialization
	m.checkInterfaces()
	return fx
}

// advance runs one interface-probe tick. Freeze detection lives in
// networkState (observeGap) and is tested there.
func (fx *monitorFixture) advance() {
	fx.checkInterfaces()
}

func TestNetMonitor_Heartbeat(t *testing.T) {
	// the sampler runs on its own goroutine and only calls onHeartbeat, so a
	// blocked interface probe can't delay or hide a sample
	block := make(chan struct{})
	defer close(block)
	beats := make(chan struct{}, 1)
	m := newNetMonitor(func(string, string) {}, func(string, bool) {},
		func() {
			select {
			case beats <- struct{}{}:
			default:
			}
		},
		func() (addrs.InterfacesAddrs, error) {
			<-block // interface enumeration stuck (e.g. suspended mid-call)
			return addrs.InterfacesAddrs{}, nil
		})
	m.heartbeatEvery = time.Millisecond * 10
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.run(ctx)
	select {
	case <-beats:
	case <-time.After(time.Second * 5):
		t.Fatal("heartbeat sampler did not run while the interface probe was blocked")
	}
}

func TestNetMonitor_InterfaceChanges(t *testing.T) {
	t.Run("regaining connectivity from zero fires recovery", func(t *testing.T) {
		fx := newMonitorFixture() // empty baseline = no connectivity
		fx.addrs.Addrs = []net.Addr{ipNet(t, "192.168.1.10/24")}
		fx.advance()
		require.Len(t, fx.events, 1, "link coming back must trigger recovery")
		assert.Contains(t, fx.events[0], "regained")

		fx.advance()
		assert.Len(t, fx.events, 1, "stable addresses must not fire again")
	})
	t.Run("additions on existing connectivity do not fire, losses do", func(t *testing.T) {
		fx := newMonitorFixtureAddrs([]net.Addr{ipNet(t, "192.168.1.10/24")})

		// pure addition (docker bridge, VPN tunnel, hotspot): existing
		// connections are not invalidated, so no teardown event
		fx.addrs.Addrs = []net.Addr{ipNet(t, "192.168.1.10/24"), ipNet(t, "10.99.0.1/24")}
		fx.advance()
		assert.Empty(t, fx.events, "pure addition must not fire")

		// network switch: old address replaced -> the loss fires
		fx.addrs.Addrs = []net.Addr{ipNet(t, "10.20.30.40/24"), ipNet(t, "10.99.0.1/24")}
		fx.advance()
		require.Len(t, fx.events, 1)
		assert.Equal(t, "interface addresses lost (1)", fx.events[0], "no addresses in the reason")

		// losing everything fires the loss path (not regain)
		fx.addrs.Addrs = nil
		fx.advance()
		require.Len(t, fx.events, 2)
		assert.Contains(t, fx.events[1], "lost")

		// and coming back fires regain
		fx.addrs.Addrs = []net.Addr{ipNet(t, "10.20.30.40/24")}
		fx.advance()
		require.Len(t, fx.events, 3)
		assert.Contains(t, fx.events[2], "regained")
	})
	t.Run("link down and up tracked", func(t *testing.T) {
		fx := newMonitorFixture()
		assert.Equal(t, []bool{true}, fx.linkDown, "no addresses at start -> down")

		fx.addrs.Addrs = []net.Addr{ipNet(t, "192.168.1.10/24")}
		fx.advance()
		assert.Equal(t, []bool{true, false}, fx.linkDown)

		fx.addrs.Addrs = nil
		fx.advance()
		assert.Equal(t, []bool{true, false, true}, fx.linkDown)
	})
	t.Run("enumeration error fails open", func(t *testing.T) {
		fx := newMonitorFixture()
		fx.addrsErr = assert.AnError
		fx.advance()
		assert.Empty(t, fx.events)
		// "unknown" must not wedge linkDown=true (Android without the injected
		// getter, transient failures): the error tick reports link up
		assert.Equal(t, []bool{true, false}, fx.linkDown)
	})
}

func TestMissingFrom(t *testing.T) {
	assert.Nil(t, missingFrom(nil, []string{"a"}))
	assert.Nil(t, missingFrom([]string{"a"}, []string{"a", "b"}))
	assert.Equal(t, []string{"a"}, missingFrom([]string{"a"}, nil))
	assert.Equal(t, []string{"b"}, missingFrom([]string{"a", "b"}, []string{"a", "c"}))
	assert.Equal(t, []string{"a", "c"}, missingFrom([]string{"a", "b", "c"}, []string{"b"}))
}
