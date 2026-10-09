//go:build !android

package localdiscovery

// newForInstalledProvider is iOS's choice for one account session: an app that
// installed a DiscoveryProxy (system DNS-SD in anytype-swift) gets the
// provider-backed discovery; older app builds fall back to heart's own
// zeroconf responder. Built for every non-android platform so it is tested on
// desktop CI; only new_ios.go calls it.
func newForInstalledProvider() LocalDiscovery {
	if getNotifierProvider() != nil {
		log.Info("local discovery: using the app's native discovery proxy")
		return newProvider()
	}
	log.Info("local discovery: no native discovery proxy installed, using zeroconf")
	return newZeroconf()
}
