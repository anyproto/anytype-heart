//go:build !android

package localdiscovery

// New picks the discovery for this account session: an app that installed a
// DiscoveryProxy (system DNS-SD in anytype-swift) gets the provider-backed
// discovery; older iOS builds and desktop, which never installs one, use
// heart's own zeroconf responder. The app installs its proxy once at launch,
// before any account starts, and New runs in Bootstrap at account start, so
// the proxy is already known here; one installed later applies from the next
// account start.
func New() LocalDiscovery {
	return newForInstalledProvider()
}

func newForInstalledProvider() LocalDiscovery {
	if getNotifierProvider() != nil {
		log.Info("local discovery: using the app's native discovery proxy")
		return newProvider()
	}
	log.Info("local discovery: no native discovery proxy installed, using zeroconf")
	return newZeroconf()
}
