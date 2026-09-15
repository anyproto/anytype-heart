package gateway

import (
	"fmt"
	"net"
	"strconv"
)

// listenConfig describes how the gateway picks the address it listens on.
type listenConfig struct {
	// override is an explicit address (ANYTYPE_GATEWAY_ADDR). It is honoured or reported as a
	// failure, never quietly substituted: a caller that names an address needs that one.
	override string
	// candidates are the ports to try, in order, before asking the OS for one. Only ports travel
	// between runs - the host is always loopback, so a hand-edited config cannot move the gateway
	// onto a public interface.
	candidates []int
}

// listenGateway binds the gateway listener, preferring a stable port but never insisting on one:
// an OS-assigned port is always the last resort.
//
// Letting the OS choose matters on Windows, where WinNAT/Hyper-V reserve large contiguous blocks of
// ports that move on every boot. The allocator skips reserved ports by construction, so a
// reservation cannot block it the way it blocks a scan of a fixed range.
func listenGateway(cfg listenConfig) (net.Listener, error) {
	if cfg.override != "" {
		ln, err := net.Listen("tcp", cfg.override)
		if err != nil {
			return nil, fmt.Errorf("listen on requested address %s: %w", cfg.override, err)
		}
		return ln, nil
	}

	for _, port := range cfg.candidates {
		if port <= 0 {
			continue
		}
		ln, err := net.Listen("tcp", loopbackAddr(port))
		if err != nil {
			log.Infof("gateway: port %d unavailable, trying the next candidate: %v", port, err)
			continue
		}
		return ln, nil
	}

	ln, err := net.Listen("tcp", loopbackAddr(0))
	if err != nil {
		return nil, fmt.Errorf("listen on an OS-assigned port: %w", err)
	}
	return ln, nil
}

func loopbackAddr(port int) string {
	return net.JoinHostPort(gatewayHost, strconv.Itoa(port))
}

// portFromAddr extracts the port of an address we persisted earlier, returning 0 for anything we
// cannot use as a candidate.
func portFromAddr(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 {
		return 0
	}
	return p
}
