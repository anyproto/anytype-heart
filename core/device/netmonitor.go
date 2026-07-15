package device

/*
AI generated

Name: Network Monitor
Scope: global (part of the networkState component)

## Responsibility
- Detects connectivity changes that arrive without any client RPC:
  - interface-address changes (Wi-Fi switch, VPN toggle, cable plug/unplug),
    diffed as IPv4 addresses + IPv6 /64 prefixes so IPv6 privacy-address
    rotation does not register as a change
  - wake from sleep, via wall-vs-monotonic clock drift (the monotonic clock
    pauses while the machine sleeps)
- Tracks whether any usable (global unicast) interface address exists, feeding
  NetworkState.IsOffline so periodic dialers can back off while the interface
  is clearly down
- On Android interface enumeration requires a client-injected getter
  (net/addrs); without it the monitor silently degrades to clock-jump
  detection only
*/

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/net/addrs"
)

const (
	netMonitorTickInterval = time.Second * 5
	// sleepJumpThreshold is how far wall time must outrun the monotonic clock
	// between two ticks before we treat it as a wake from sleep. Short lid
	// closes below it are covered by the transport keepalives.
	sleepJumpThreshold = time.Second * 30
)

type netMonitor struct {
	// onEvent receives a connectivity-change signal (feeds triggerRecovery).
	onEvent func(reason string)
	// onLinkDown is called every tick with whether no usable address exists.
	onLinkDown func(down bool)

	getAddrs func() (addrs.InterfacesAddrs, error)
	nowWall  func() time.Time
	elapsed  func() time.Duration

	prevWall     time.Time
	prevElapsed  time.Duration
	prevSnapshot []string
	snapshotInit bool
	addrsErrOnce sync.Once
}

func newNetMonitor(onEvent func(reason string), onLinkDown func(down bool), getAddrs func() (addrs.InterfacesAddrs, error)) *netMonitor {
	if getAddrs == nil {
		getAddrs = addrs.GetInterfacesAddrs
	}
	start := time.Now()
	return &netMonitor{
		onEvent:    onEvent,
		onLinkDown: onLinkDown,
		getAddrs:   getAddrs,
		nowWall:    time.Now,
		// time.Since uses the monotonic reading, which pauses during sleep.
		elapsed: func() time.Duration { return time.Since(start) },
	}
}

func (m *netMonitor) run(ctx context.Context) {
	m.prevWall = m.nowWall().Round(0)
	m.prevElapsed = m.elapsed()
	m.checkInterfaces()
	ticker := time.NewTicker(netMonitorTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *netMonitor) tick() {
	wall := m.nowWall().Round(0)
	el := m.elapsed()
	if gap := wall.Sub(m.prevWall) - (el - m.prevElapsed); gap > sleepJumpThreshold {
		m.onEvent(fmt.Sprintf("wake from sleep (clock jump %s)", gap.Round(time.Second)))
	}
	m.prevWall = wall
	m.prevElapsed = el
	m.checkInterfaces()
}

func (m *netMonitor) checkInterfaces() {
	ifAddrs, err := m.getAddrs()
	if err != nil {
		// Android without an injected interface getter lands here; the RPC
		// signals and the clock-jump detector still apply. Fail open: an
		// enumeration error means "unknown", and a stale linkDown=true must
		// not wedge the device into offline behavior.
		m.addrsErrOnce.Do(func() {
			log.Info("net monitor: interface enumeration unavailable", zap.Error(err))
		})
		m.onLinkDown(false)
		return
	}
	snapshot := connectivitySnapshot(ifAddrs.Addrs)
	m.onLinkDown(len(snapshot) == 0)
	// Only a *disappearing* address signals that an existing network path may
	// have died. Pure additions (docker/VM bridges, VPN tunnels, hotspots)
	// don't invalidate established connections — flushing on them would abort
	// healthy transfers; if a new interface does reroute traffic and breaks a
	// connection, the transport keepalive catches it within seconds.
	if m.snapshotInit {
		if lost := missingFrom(m.prevSnapshot, snapshot); len(lost) > 0 {
			m.onEvent("interface addresses lost: " + strings.Join(lost, ","))
		}
	}
	m.prevSnapshot = snapshot
	m.snapshotInit = true
}

// missingFrom returns the entries of prev absent from cur; both must be
// sorted (connectivitySnapshot output).
func missingFrom(prev, cur []string) (lost []string) {
	i := 0
	for _, p := range prev {
		for i < len(cur) && cur[i] < p {
			i++
		}
		if i >= len(cur) || cur[i] != p {
			lost = append(lost, p)
		}
	}
	return
}

// connectivitySnapshot reduces interface addresses to a stable network
// identity: IPv4 addresses plus IPv6 /64 prefixes of global unicast addresses
// (sorted, deduplicated). Using the prefix instead of the full IPv6 address
// keeps the periodic rotation of temporary privacy addresses from looking
// like a network change. Loopback and link-local addresses are ignored, so an
// empty snapshot means no usable connectivity.
func connectivitySnapshot(list []net.Addr) (snapshot []string) {
	for _, a := range list {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}
		if ip == nil || !ip.IsGlobalUnicast() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			snapshot = append(snapshot, ip4.String())
		} else {
			snapshot = append(snapshot, ip.Mask(net.CIDRMask(64, 128)).String()+"/64")
		}
	}
	sort.Strings(snapshot)
	return slices.Compact(snapshot)
}
