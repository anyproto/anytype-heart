//go:build !android && !ios

package localdiscovery

// New returns heart's own zeroconf (mDNS) discovery, the only one on desktop.
func New() LocalDiscovery {
	return newZeroconf()
}
