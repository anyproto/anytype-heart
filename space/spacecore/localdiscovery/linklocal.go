package localdiscovery

import (
	"net/netip"
	"regexp"
	"strings"

	"github.com/anyproto/anytype-heart/net/addrs"
)

// GO-7581: an interface with no IPv4 at all (a phone joined to a Mac by a
// cable, Wi-Fi off) still carries an IPv6 link-local address, and that is
// enough for discovery and sync over the link. IPv6 is used only there:
// an interface with IPv4 keeps today's IPv4-only behavior.

// linkLocalPrefixes are the interface kinds that may carry LAN traffic over
// IPv6 link-local alone. An allowlist, because desktops have many IPv6-only
// multicast interfaces that are no LAN: on macOS utun* (VPN and system
// tunnels), awdl*/llw* (AWDL), anpi* and ap*. Windows reports no receiving
// interface for a multicast packet, so no zone can be attached there.
var linkLocalPrefixes = map[string][]string{
	"darwin":  {"en", "bridge"},
	"ios":     {"en", "bridge"},
	"linux":   {"en", "em", "eth", "wl", "usb", "thunderbolt", "br", "bond"},
	"android": {"wlan", "eth", "rndis", "usb"},
}

// biosdevnamePCI is the Linux biosdevname name of a PCI card port: p1p1.
var biosdevnamePCI = regexp.MustCompile(`^p[0-9]+p[0-9]+`)

// linkLocalCandidate reports whether an interface of this kind may be used
// over IPv6 link-local.
func linkLocalCandidate(goos, iface string) bool {
	for _, prefix := range linkLocalPrefixes[goos] {
		if strings.HasPrefix(iface, prefix) {
			return true
		}
	}
	return goos == "linux" && biosdevnamePCI.MatchString(iface)
}

// linkLocalOnly returns the IPv6 link-local addresses of an interface that has
// no IPv4 address and may carry LAN traffic; nil for any other interface.
func linkLocalOnly(goos string, iface *addrs.NetInterfaceWithAddrCache) []netip.Addr {
	if len(iface.GetAddr()) > 0 || !linkLocalCandidate(goos, iface.Name) {
		return nil
	}
	return iface.GetLinkLocalIPv6()
}
