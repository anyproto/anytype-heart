package device

import (
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
	wall     time.Time
	elapsed  time.Duration
	addrs    addrs.InterfacesAddrs
	addrsErr error
}

func newMonitorFixture() *monitorFixture {
	fx := &monitorFixture{wall: time.Unix(1000000, 0)}
	m := &netMonitor{
		getAddrs: func() (addrs.InterfacesAddrs, error) { return fx.addrs, fx.addrsErr },
		nowWall:  func() time.Time { return fx.wall },
		elapsed:  func() time.Duration { return fx.elapsed },
	}
	m.onEvent = func(reason string) { fx.events = append(fx.events, reason) }
	m.onLinkDown = func(down bool) { fx.linkDown = append(fx.linkDown, down) }
	fx.netMonitor = m
	// mirror run()'s initialization
	m.prevWall = fx.wall
	m.prevElapsed = fx.elapsed
	m.checkInterfaces()
	return fx
}

// advance moves wall time by wallDelta and the monotonic clock by monoDelta,
// then runs one tick. During sleep the monotonic clock pauses, so a wake shows
// wallDelta >> monoDelta.
func (fx *monitorFixture) advance(wallDelta, monoDelta time.Duration) {
	fx.wall = fx.wall.Add(wallDelta)
	fx.elapsed += monoDelta
	fx.tick()
}

func TestNetMonitor_ClockJump(t *testing.T) {
	t.Run("sleep longer than threshold fires wake event", func(t *testing.T) {
		fx := newMonitorFixture()
		fx.advance(2*time.Minute, netMonitorTickInterval) // slept ~2min
		require.Len(t, fx.events, 1)
		assert.Contains(t, fx.events[0], "wake from sleep")
	})
	t.Run("normal ticks do not fire", func(t *testing.T) {
		fx := newMonitorFixture()
		for i := 0; i < 10; i++ {
			fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		}
		assert.Empty(t, fx.events)
	})
	t.Run("short sleep below threshold does not fire", func(t *testing.T) {
		fx := newMonitorFixture()
		fx.advance(netMonitorTickInterval+sleepJumpThreshold/2, netMonitorTickInterval)
		assert.Empty(t, fx.events)
	})
}

func TestNetMonitor_InterfaceChanges(t *testing.T) {
	t.Run("address change fires event, stable set does not", func(t *testing.T) {
		fx := newMonitorFixture()
		fx.addrs.Addrs = []net.Addr{ipNet(t, "192.168.1.10/24")}
		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		// baseline was empty (no addrs at construction) -> this is a change
		require.Len(t, fx.events, 1)

		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		assert.Len(t, fx.events, 1, "stable addresses must not fire")

		fx.addrs.Addrs = []net.Addr{ipNet(t, "10.20.30.40/24")} // new network
		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		assert.Len(t, fx.events, 2)
	})
	t.Run("link down and up tracked", func(t *testing.T) {
		fx := newMonitorFixture()
		assert.Equal(t, []bool{true}, fx.linkDown, "no addresses at start -> down")

		fx.addrs.Addrs = []net.Addr{ipNet(t, "192.168.1.10/24")}
		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		assert.Equal(t, []bool{true, false}, fx.linkDown)

		fx.addrs.Addrs = nil
		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		assert.Equal(t, []bool{true, false, true}, fx.linkDown)
	})
	t.Run("enumeration error degrades silently", func(t *testing.T) {
		fx := newMonitorFixture()
		fx.addrsErr = assert.AnError
		fx.advance(netMonitorTickInterval, netMonitorTickInterval)
		assert.Empty(t, fx.events)
		assert.Len(t, fx.linkDown, 1, "no link updates while enumeration fails")
	})
}
